package photogen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// SyncMetadataFileName is the record sync keeps in every album's download folder.
const SyncMetadataFileName = "metadata.yaml"

// syncMetadataHeader is written above the YAML. The folder's contract is that photogen.txt
// is the only file a user may edit, and this is where that is said out loud.
const syncMetadataHeader = "# Written by photogen sync. Do not edit.\n"

// SyncMetadata is what one sync run recorded about an album. It carries three jobs: the
// name and description fallback, the incremental-download record, and the baseline the
// caption merge compares against.
type SyncMetadata struct {
	Provider    string          `yaml:"provider"`
	AlbumID     string          `yaml:"album_id"`
	SyncedAt    time.Time       `yaml:"synced_at"`
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"` // escaped, see escapeSyncText
	Photos      []SyncPhotoMeta `yaml:"photos"`
}

// SyncPhotoMeta is one asset's record.
type SyncPhotoMeta struct {
	AssetID          string    `yaml:"asset_id"`
	File             string    `yaml:"file"`               // the local name, assigned once and then fixed
	OriginalFileName string    `yaml:"original_file_name"` // what it was called upstream at last sync
	Size             int64     `yaml:"size"`
	Checksum         string    `yaml:"checksum"`
	UpdatedAt        time.Time `yaml:"updated_at"`
	// Caption is the *upstream* description at last sync, escaped — not whatever ended up
	// in photogen.txt. When a local edit wins the merge this still records what upstream
	// said, because that is the baseline the next run needs to tell "the user edited this"
	// from "the upstream changed". Writing the local value here instead would make the
	// next sync read the edit as untouched and silently revert it.
	Caption string `yaml:"caption"`
}

// byAssetID indexes the recorded photos. Safe on a nil receiver, which is what a folder
// that has never synced produces.
func (m *SyncMetadata) byAssetID() map[string]*SyncPhotoMeta {
	if m == nil {
		return map[string]*SyncPhotoMeta{}
	}
	out := make(map[string]*SyncPhotoMeta, len(m.Photos))
	for i := range m.Photos {
		out[m.Photos[i].AssetID] = &m.Photos[i]
	}
	return out
}

// captionByFile indexes the recorded upstream captions by local file name.
func (m *SyncMetadata) captionByFile() map[string]string {
	out := make(map[string]string, len(m.Photos))
	for _, p := range m.Photos {
		out[p.File] = p.Caption
	}
	return out
}

// loadSyncMetadata reads an album's sync record. A folder that has never synced has no
// file, which is an empty record rather than an error.
func loadSyncMetadata(dir string) (*SyncMetadata, error) {
	path := filepath.Join(dir, SyncMetadataFileName)
	m, err := readYAML[SyncMetadata](path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &SyncMetadata{}, nil
		}
		return nil, err
	}
	return m, nil
}

// writeSyncMetadata replaces an album's sync record atomically, so an interrupted run never
// leaves a half-written one.
func writeSyncMetadata(dir string, m *SyncMetadata) error {
	path := filepath.Join(dir, SyncMetadataFileName)
	if err := writeYAMLAtomic(path, syncMetadataHeader, m); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("  wrote: %s\n", path)
	return nil
}

// writeFileAtomic writes data to path through a temp file in the same directory and a
// rename, so a reader never sees a partial file and a failed run never leaves one.
//
// It is the shape MetaCache.Save arrived at, including the explicit Chmod: os.CreateTemp
// makes 0600, and this project writes 0666 so files created inside a Docker container as
// root stay usable by the host user that mounted the volume.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, filePerms); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
