package photogen

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// metaCacheVersion is the on-disk schema version. Bump it whenever the shape of
// metaCacheEntry or PhotoMetadata changes, or what a read puts in it (3: DateTime is no
// longer a date fallback); a mismatch discards the whole file.
//
// One addition did not bump it: PhotoMetadata.ColorProfile. A bump also discards every
// derived stamp and video entry, which would redo every cover and hero and re-run ffprobe
// on every video. Instead, a photo entry without a ColorProfile is treated as a miss (see
// Metadata), so only stills get one header read each. Prefer a bump for anything else.
const metaCacheVersion = 3

// MetaCacheFileName is the cache file written under {OutputRoot}/.build/.
const MetaCacheFileName = "metadata-cache.json"

// derivedKeysRootRelative marks a cache file whose derived keys are relative to the albums
// root (see MetaCache.derivedKey). Older files keyed them by the output path as given,
// which differs between a host run and a Docker run; LoadMetaCache migrates those.
const derivedKeysRootRelative = "root-relative"

// MetaCache caches PhotoMetadata keyed by absolute source path so that re-runs skip
// the decode in ReadMediaMetadata for source files that have not changed. For video the
// saving is far larger than for photos, since the alternative is an ffprobe subprocess.
//
// Reading metadata is the dominant cost of a run with nothing to resize: resizing
// short-circuits on the output's stamp (see OutputUpToDate), but every photo was
// previously decoded on every run just to recover width, height, orientation and date.
//
// Metadata depends only on the source file, so a single cache is shared by every
// site ID under the same albums directory.
//
// The same cache is also shared by a host run and a Docker run, which see the albums
// directory and the sources under different paths. Derived stamps are keyed relative to
// the albums root so both find them, and Save keeps entries recorded by the other
// environment rather than pruning them (see foreignPath). Without both, every switch
// re-rendered each color-converted photo and re-read every header.
//
// A nil *MetaCache is valid and means "no caching": Metadata falls through to
// ReadMediaMetadata and Save does nothing.
type MetaCache struct {
	mu      sync.Mutex
	path    string
	root    string // absolute albums root, the directory holding .build/
	dirty   bool
	refresh bool
	entries map[string]metaCacheEntry
	derived map[string]derivedCacheEntry
}

// metaCacheFile is the on-disk representation of a MetaCache.
type metaCacheFile struct {
	Version     int                          `json:"version"`
	DerivedKeys string                       `json:"derivedKeys,omitempty"`
	Entries     map[string]metaCacheEntry    `json:"entries"`
	Derived     map[string]derivedCacheEntry `json:"derived"`
}

// derivedCacheEntry records which source file (and which settings) produced a derived
// output. Neither kind of output can use a bare "output exists, skip it" rule: a
// fixed-name output (cover.jpg, hero.jpg) can be re-pointed at a different photo, and an
// output named after its source (grid/full webps, video MP4s) keeps its name when the
// source is replaced in place. Stamping the output with its source lets a re-run tell
// "already up to date" apart from "needs regenerating".
type derivedCacheEntry struct {
	Source      string `json:"source"`
	ModTimeNano int64  `json:"modTimeNano"`
	Size        int64  `json:"size"`
	Variant     string `json:"variant,omitempty"` // settings that affect the output, e.g. hero crop
	// ColorVersion is the colorPipelineVersion the output was written with; 0 for outputs
	// written or adopted before it existed. Kept apart from Variant, which is a user
	// setting compared for equality: this is compared only when the source needs
	// conversion, so a pipeline change redoes just the outputs it affects.
	ColorVersion int `json:"colorVersion,omitempty"`
}

// sameSource reports whether the stamp was written from sourcePath. A path recorded by
// the other environment (a host path read in Docker, or the reverse) cannot match
// exactly, so a differing path still matches when its file name does; the caller's mtime
// and size checks are what actually tie the stamp to the file. Re-pointing a cover or
// hero at a different photo with the same name, mtime and size is not a real case.
func (e derivedCacheEntry) sameSource(sourcePath string) bool {
	return e.Source == sourcePath ||
		filepath.Base(filepath.FromSlash(e.Source)) == filepath.Base(sourcePath)
}

// colorCurrent reports whether an output stamped with e reflects the current color
// pipeline for a source with this ColorProfile.
func (e derivedCacheEntry) colorCurrent(colorProfile string) bool {
	return !needsColorConversion(colorProfile) || e.ColorVersion >= colorPipelineVersion
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

// NewMetaCache returns an empty cache bound to path, which is {root}/.build/<file> (see
// MetaCachePath); derived keys are relative to that root.
func NewMetaCache(path string) *MetaCache {
	root := filepath.Dir(filepath.Dir(path))
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return &MetaCache{
		path:    path,
		root:    root,
		entries: map[string]metaCacheEntry{},
		derived: map[string]derivedCacheEntry{},
	}
}

// derivedKey returns the key an output's stamp is stored under: its path relative to the
// albums root, with forward slashes, so a host run (albums/...) and a Docker run
// (/ddphotos/albums/...) share it. An output outside the root, which photogen never
// writes, is keyed by its absolute path.
func (mc *MetaCache) derivedKey(outputPath string) string {
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return outputPath
	}
	rel, err := filepath.Rel(mc.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}
	return filepath.ToSlash(rel)
}

// derivedPath is the inverse of derivedKey: the file a stamp's key names.
func (mc *MetaCache) derivedPath(key string) string {
	path := filepath.FromSlash(key)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(mc.root, path)
}

// migrateDerivedKeys rekeys stamps from a file written before keys were root-relative.
// Those were keyed by the output path as given, relative to the working directory or
// absolute, and resolve here only if this environment wrote them. A key from the other
// environment would never match again, so it is dropped rather than kept forever. When
// both environments stamped the same output, the stamp from the newer color pipeline wins.
func (mc *MetaCache) migrateDerivedKeys(old map[string]derivedCacheEntry) {
	for path, entry := range old {
		key := mc.derivedKey(path)
		if filepath.IsAbs(filepath.FromSlash(key)) {
			continue
		}
		if have, ok := mc.derived[key]; ok && have.ColorVersion >= entry.ColorVersion {
			continue
		}
		mc.derived[key] = entry
	}
	mc.dirty = true
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
// itself over the course of the run. An unreadable file is reported through warn, which
// may be nil (it then only prints).
func LoadMetaCache(path string, warn *WarnCollector) *MetaCache {
	mc := NewMetaCache(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return mc // no cache yet: normal on a first run
	}

	var f metaCacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		warn.Warnf("  WARN: ignoring unreadable metadata cache %s: %v\n", path, err)
		return mc
	}
	if f.Version != metaCacheVersion {
		fmt.Printf("  Metadata cache %s is version %d (want %d), starting fresh\n", path, f.Version, metaCacheVersion)
		return mc
	}
	if f.Entries != nil {
		mc.entries = f.Entries
	}
	switch {
	case f.Derived == nil:
	case f.DerivedKeys == derivedKeysRootRelative:
		mc.derived = f.Derived
	default:
		mc.migrateDerivedKeys(f.Derived)
	}
	return mc
}

// DerivedUpToDate reports whether the fixed-name output at outputPath was already
// generated from exactly this source file and these settings, and still exists on disk.
// colorProfile is the source's ColorProfile: a stamp from before the color pipeline it
// needs is out of date.
// A false result means "regenerate it", which is also what a nil cache always says, so
// caching off simply restores the unconditional regeneration this replaces.
func (mc *MetaCache) DerivedUpToDate(outputPath, sourcePath, variant, colorProfile string) bool {
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
	entry, ok := mc.derived[mc.derivedKey(outputPath)]
	return ok &&
		entry.sameSource(sourcePath) &&
		entry.ModTimeNano == stat.ModTime().UnixNano() &&
		entry.Size == stat.Size() &&
		entry.Variant == variant &&
		entry.colorCurrent(colorProfile)
}

// OutputUpToDate reports whether an output named after its source (a photo's grid and full
// webps, a video's MP4 and posters) still reflects that source. The name alone cannot say:
// a source replaced in place, which is what a sync does when an asset changes upstream or
// a new asset reuses a removed one's name, keeps the old output's name.
//
// It differs from DerivedUpToDate in three ways, each so that turning this on costs nothing:
//
//   - An existing output with no stamp is trusted and stamped, not regenerated. Outputs
//     written before the cache tracked them would otherwise all be redone on the first run,
//     video transcodes included. From that run on, a replaced source is caught.
//   - The source path is not compared, only its mtime and size. The output name already
//     ties it to the source, and a path check would redo every photo when the source
//     directory is reached through a different path (Docker mounts it elsewhere).
//   - A nil cache, or a source that cannot be stat'd, falls back to "the output exists",
//     the rule this replaces, rather than regenerating everything.
//
// colorProfile is the source's ColorProfile. An output of a source that needs conversion
// to sRGB is out of date unless its stamp is from the current color pipeline, and is never
// adopted, since an unstamped output predates the pipeline by definition.
func (mc *MetaCache) OutputUpToDate(outputPath, sourcePath, colorProfile string) bool {
	if _, err := os.Stat(outputPath); err != nil {
		return false // output missing (or unreadable): regenerate
	}
	if mc == nil {
		return true
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return true
	}

	key := mc.derivedKey(outputPath)
	mc.mu.Lock()
	defer mc.mu.Unlock()
	entry, ok := mc.derived[key]
	if !ok {
		if needsColorConversion(colorProfile) {
			return false
		}
		mc.derived[key] = derivedCacheEntry{
			Source:      sourcePath,
			ModTimeNano: stat.ModTime().UnixNano(),
			Size:        stat.Size(),
		}
		mc.dirty = true
		return true
	}
	return entry.ModTimeNano == stat.ModTime().UnixNano() && entry.Size == stat.Size() &&
		entry.colorCurrent(colorProfile)
}

// NeedsColorUpdate reports whether OutputUpToDate would say no only because the output
// predates the color pipeline its source needs. ResizePhotos uses it to say why it is
// re-rendering photos nobody touched. It does not adopt anything.
func (mc *MetaCache) NeedsColorUpdate(outputPath, sourcePath, colorProfile string) bool {
	if mc == nil || !needsColorConversion(colorProfile) {
		return false
	}
	if _, err := os.Stat(outputPath); err != nil {
		return false
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return false
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	entry, ok := mc.derived[mc.derivedKey(outputPath)]
	if !ok {
		return true
	}
	return entry.ModTimeNano == stat.ModTime().UnixNano() && entry.Size == stat.Size() &&
		!entry.colorCurrent(colorProfile)
}

// RecordDerived stamps a freshly written output with the source and settings that
// produced it, and with the current color pipeline. Safe on a nil receiver.
func (mc *MetaCache) RecordDerived(outputPath, sourcePath, variant string) {
	if mc == nil {
		return
	}
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return // nothing worth recording if we cannot stamp it
	}

	key := mc.derivedKey(outputPath)
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.derived[key] = derivedCacheEntry{
		Source:       sourcePath,
		ModTimeNano:  stat.ModTime().UnixNano(),
		Size:         stat.Size(),
		Variant:      variant,
		ColorVersion: colorPipelineVersion,
	}
	mc.dirty = true
}

// Metadata returns the metadata for the photo at path, reading it from the cache when
// the source file's modification time and size are unchanged and decoding it via
// ReadMediaMetadata otherwise. Safe for concurrent use, and safe on a nil receiver.
func (mc *MetaCache) Metadata(path string) (*PhotoMetadata, error) {
	if mc == nil {
		return ReadMediaMetadata(path)
	}

	// A failed stat is not fatal here: fall through and let ReadMediaMetadata report
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
		// A still cached before ColorProfile existed is read again (see metaCacheVersion).
		if ok && entry.Meta != nil && entry.Meta.ColorProfile == "" && !IsVideoFile(path) {
			ok = false
		}
		if ok && entry.Meta != nil && entry.ModTimeNano == modNano && entry.Size == size {
			// Copy so callers can never mutate cache state through the returned pointer.
			meta := *entry.Meta
			return &meta, nil
		}
	}

	meta, err := ReadMediaMetadata(path)
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
// so the file does not grow without bound as photos are renamed or deleted. An entry
// recorded by the other environment cannot be stat'd here either, so it is kept (see
// foreignPath); the next run there prunes it if it is really gone. It is a no-op when
// nothing changed, and safe on a nil receiver.
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
		if _, err := os.Stat(path); err != nil && !foreignPath(path) {
			delete(mc.entries, path)
		}
	}
	for key := range mc.derived {
		path := mc.derivedPath(key)
		if _, err := os.Stat(path); err != nil && !foreignPath(path) {
			delete(mc.derived, key)
		}
	}

	contents := metaCacheFile{
		Version:     metaCacheVersion,
		DerivedKeys: derivedKeysRootRelative,
		Entries:     mc.entries,
		Derived:     mc.derived,
	}
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

// foreignPath reports whether path was recorded by another environment, which it takes
// to mean its top-level directory does not exist here. Docker sees sources under /ddphotos
// and its other mounts, which a host does not have, and a host's /Users or /home is not
// mounted in the container. A path whose top-level directory exists is local, so a missing
// file there really was renamed or deleted.
func foreignPath(path string) bool {
	vol := filepath.VolumeName(path)
	rest := path[len(vol):]
	if rest == "" || !os.IsPathSeparator(rest[0]) {
		return false // relative: resolved against this environment
	}
	top, _, _ := strings.Cut(strings.TrimLeft(filepath.ToSlash(rest), "/"), "/")
	if top == "" {
		return false
	}
	_, err := os.Stat(vol + string(filepath.Separator) + top)
	return errors.Is(err, fs.ErrNotExist)
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
