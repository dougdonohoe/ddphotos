package photogen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// A sidecar supplies metadata a media file cannot carry itself. The case it exists for is an
// edited photo from Immich: the edited rendition is written without any EXIF, so without a
// sidecar it would have no date and sort to the end of its album. Nothing about it is
// Immich-specific, though, so it is read for every album, synced or not, and can be written
// by hand.
//
// It sits beside the file it describes, named after the file's full name, extension
// included (IMG_1.jpg.photogen.json). The full name is what keeps IMG_1.jpg and IMG_1.png,
// which are separate photos, from sharing one. The .photogen infix is what keeps it apart
// from Google Takeout, whose older exports put a differently shaped IMG_1.jpg.json beside
// every photo.
//
// The source scan never sees a sidecar as a photo, because it only admits IsMediaFile
// names.

// sidecarSuffix is appended to a media file's full name to name its sidecar.
const sidecarSuffix = ".photogen.json"

// sidecarPath returns the sidecar for the media file at path. Sync and the build both call
// it, so the naming lives in one place.
func sidecarPath(mediaPath string) string {
	return mediaPath + sidecarSuffix
}

// Sidecar is the sidecar file's schema.
type Sidecar struct {
	// DateTaken overrides the date read from the file. It follows the EXIF convention
	// photogen uses everywhere: the camera's local clock, written as UTC
	// ("2007-10-17T10:24:19Z").
	DateTaken *time.Time `json:"dateTaken,omitempty"`
}

// readSidecar reads the sidecar for the media file at path. A missing sidecar is a nil
// result and no error.
//
// Anything else wrong with it is an error naming the file, unknown fields included: a
// sidecar is either written by sync or by hand, and a handwritten one that silently does
// nothing, because of a typo in a field name, is worse than a build that stops and says so.
func readSidecar(mediaPath string) (*Sidecar, error) {
	path := sidecarPath(mediaPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sidecar: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var sc Sidecar
	if err := dec.Decode(&sc); err != nil {
		return nil, fmt.Errorf("parse sidecar %s: %w", path, err)
	}
	return &sc, nil
}

// applySidecar overrides meta with whatever the media file's sidecar supplies.
//
// It runs after the metadata cache rather than inside it. The cache keys on the media
// file's mtime and size, so a sidecar edited on its own would otherwise go unnoticed until
// the photo itself changed.
func applySidecar(mediaPath string, meta *PhotoMetadata) error {
	sc, err := readSidecar(mediaPath)
	if err != nil || sc == nil {
		return err
	}
	if sc.DateTaken != nil {
		meta.DateTaken = sc.DateTaken.UTC()
		meta.DateFromSidecar = true
	}
	return nil
}

// writeSidecar writes the sidecar for the media file at path, leaving an identical one
// untouched so its mtime stays put. It reports whether it wrote anything.
func writeSidecar(mediaPath string, sc Sidecar) (bool, error) {
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	path := sidecarPath(mediaPath)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err := writeFileAtomic(path, data); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}
