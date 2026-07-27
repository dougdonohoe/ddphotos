package photogen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// metaCacheVersion is the on-disk schema version. Bump it whenever the shape of
// metaCacheEntry or PhotoMetadata changes; a mismatch discards the whole file.
const metaCacheVersion = 1

// MetaCacheFileName is the cache file written under {OutputRoot}/.build/.
const MetaCacheFileName = "metadata-cache.json"

// MetaCache caches PhotoMetadata keyed by absolute source path so that re-runs skip
// the libvips decode in ReadPhotoMetadata for source files that have not changed.
//
// Reading metadata is the dominant cost of a run with nothing to resize: resizing
// already short-circuits on an os.Stat of the output file, but every photo was
// previously decoded on every run just to recover width, height, orientation and date.
//
// Metadata depends only on the source file, so a single cache is shared by every
// site ID under the same albums directory.
//
// A nil *MetaCache is valid and means "no caching": Metadata falls through to
// ReadPhotoMetadata and Save does nothing.
type MetaCache struct {
	mu      sync.Mutex
	path    string
	dirty   bool
	refresh bool
	entries map[string]metaCacheEntry
	derived map[string]derivedCacheEntry
}

// metaCacheFile is the on-disk representation of a MetaCache.
type metaCacheFile struct {
	Version int                          `json:"version"`
	Entries map[string]metaCacheEntry    `json:"entries"`
	Derived map[string]derivedCacheEntry `json:"derived"`
}

// derivedCacheEntry records which source file (and which settings) produced a derived
// output whose filename is fixed, such as cover.jpg and hero.jpg. Those cannot use the
// usual "output exists, skip it" rule, because the same output name is produced from a
// source that may have been swapped for a different photo. Stamping the output with its
// source lets a re-run tell "already up to date" apart from "needs regenerating".
type derivedCacheEntry struct {
	Source      string `json:"source"`
	ModTimeNano int64  `json:"modTimeNano"`
	Size        int64  `json:"size"`
	Variant     string `json:"variant,omitempty"` // settings that affect the output, e.g. hero crop
}

// metaCacheEntry is one cached photo, stamped with the source file's modification
// time and size. Both must match for the entry to be considered valid.
type metaCacheEntry struct {
	ModTimeNano int64          `json:"modTimeNano"`
	Size        int64          `json:"size"`
	Meta        *PhotoMetadata `json:"meta"`
}

// MetaCachePath returns the cache file location for an albums output root. It lives
// beside the .build/<site-id>.json files, outside the per-site directory, so it is
// never synced to a server and never seen by -clean.
func MetaCachePath(outputRoot string) string {
	return filepath.Join(outputRoot, ".build", MetaCacheFileName)
}

// NewMetaCache returns an empty cache bound to path.
func NewMetaCache(path string) *MetaCache {
	return &MetaCache{
		path:    path,
		entries: map[string]metaCacheEntry{},
		derived: map[string]derivedCacheEntry{},
	}
}

// SetRefresh makes Metadata ignore existing entries and re-decode every photo, while
// still recording what it reads. This is what -force uses: entries for the photos in
// this run are rebuilt, and entries for albums that were not processed (say, because
// -album narrowed the run) are preserved rather than discarded.
func (mc *MetaCache) SetRefresh(refresh bool) {
	if mc == nil {
		return
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.refresh = refresh
}

// LoadMetaCache reads the cache at path. It never fails: a missing, unreadable,
// malformed, or wrong-version file simply yields an empty cache that repopulates
// itself over the course of the run.
func LoadMetaCache(path string) *MetaCache {
	mc := NewMetaCache(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return mc // no cache yet: normal on a first run
	}

	var f metaCacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		fmt.Printf("  WARN: ignoring unreadable metadata cache %s: %v\n", path, err)
		return mc
	}
	if f.Version != metaCacheVersion {
		fmt.Printf("  Metadata cache %s is version %d (want %d), starting fresh\n", path, f.Version, metaCacheVersion)
		return mc
	}
	if f.Entries != nil {
		mc.entries = f.Entries
	}
	if f.Derived != nil {
		mc.derived = f.Derived
	}
	return mc
}

// DerivedUpToDate reports whether the fixed-name output at outputPath was already
// generated from exactly this source file and these settings, and still exists on disk.
// A false result means "regenerate it", which is also what a nil cache always says, so
// caching off simply restores the unconditional regeneration this replaces.
func (mc *MetaCache) DerivedUpToDate(outputPath, sourcePath, variant string) bool {
	if mc == nil {
		return false
	}

	if _, err := os.Stat(outputPath); err != nil {
		return false // output missing (or unreadable): regenerate
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return false // let the resize report the real problem with the source
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.refresh {
		return false
	}
	entry, ok := mc.derived[outputPath]
	return ok &&
		entry.Source == sourcePath &&
		entry.ModTimeNano == stat.ModTime().UnixNano() &&
		entry.Size == stat.Size() &&
		entry.Variant == variant
}

// RecordDerived stamps a freshly written fixed-name output with the source and settings
// that produced it. Safe on a nil receiver.
func (mc *MetaCache) RecordDerived(outputPath, sourcePath, variant string) {
	if mc == nil {
		return
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return // nothing worth recording if we cannot stamp it
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.derived[outputPath] = derivedCacheEntry{
		Source:      sourcePath,
		ModTimeNano: stat.ModTime().UnixNano(),
		Size:        stat.Size(),
		Variant:     variant,
	}
	mc.dirty = true
}

// Metadata returns the metadata for the photo at path, reading it from the cache when
// the source file's modification time and size are unchanged and decoding it via
// ReadPhotoMetadata otherwise. Safe for concurrent use, and safe on a nil receiver.
func (mc *MetaCache) Metadata(path string) (*PhotoMetadata, error) {
	if mc == nil {
		return ReadPhotoMetadata(path)
	}

	// A failed stat is not fatal here: fall through and let ReadPhotoMetadata report
	// the real problem with the file.
	var modNano, size int64
	stat, statErr := os.Stat(path)
	if statErr == nil {
		modNano = stat.ModTime().UnixNano()
		size = stat.Size()

		mc.mu.Lock()
		entry, ok := mc.entries[path]
		if mc.refresh {
			ok = false
		}
		mc.mu.Unlock()
		if ok && entry.Meta != nil && entry.ModTimeNano == modNano && entry.Size == size {
			// Copy so callers can never mutate cache state through the returned pointer.
			meta := *entry.Meta
			return &meta, nil
		}
	}

	meta, err := ReadPhotoMetadata(path)
	if err != nil {
		return nil, err
	}

	if statErr == nil {
		cached := *meta
		mc.mu.Lock()
		mc.entries[path] = metaCacheEntry{ModTimeNano: modNano, Size: size, Meta: &cached}
		mc.dirty = true
		mc.mu.Unlock()
	}

	return meta, nil
}

// Save writes the cache to disk, dropping entries whose source file no longer exists
// so the file does not grow without bound as photos are renamed or deleted. It is a
// no-op when nothing changed, and safe on a nil receiver.
//
// The cache is written even in dry-run mode: it is a local performance artifact under
// .build/, never part of the generated site and never deployed, so writing it does not
// break the -doit contract and lets repeated dry-runs benefit too.
func (mc *MetaCache) Save() error {
	if mc == nil {
		return nil
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	if !mc.dirty {
		return nil
	}

	for path := range mc.entries {
		if _, err := os.Stat(path); err != nil {
			delete(mc.entries, path)
		}
	}
	for path := range mc.derived {
		if _, err := os.Stat(path); err != nil {
			delete(mc.derived, path)
		}
	}

	contents := metaCacheFile{Version: metaCacheVersion, Entries: mc.entries, Derived: mc.derived}
	b, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata cache: %w", err)
	}
	b = append(b, '\n')

	dir := filepath.Dir(mc.path)
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	// Write to a temp file and rename so an interrupted run cannot leave a truncated
	// cache behind, which would then be discarded as malformed on the next run.
	tmp, err := os.CreateTemp(dir, MetaCacheFileName+".tmp*")
	if err != nil {
		return fmt.Errorf("create temp metadata cache: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp metadata cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp metadata cache: %w", err)
	}
	if err := os.Chmod(tmpName, filePerms); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp metadata cache: %w", err)
	}
	if err := os.Rename(tmpName, mc.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", mc.path, err)
	}

	mc.dirty = false
	return nil
}

// Len returns the number of cached entries. Safe on a nil receiver.
func (mc *MetaCache) Len() int {
	if mc == nil {
		return 0
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return len(mc.entries)
}
