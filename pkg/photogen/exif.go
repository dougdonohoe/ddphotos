package photogen

import (
	"fmt"
	"strings"
	"time"

	"github.com/davidbyttow/govips/v2/vips"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
)

func init() {
	vips.LoggingSettings(nil, vips.LogLevelWarning)
	exit.PanicOnError(vips.Startup(nil))
}

// PhotoMetadata holds extracted image metadata.
type PhotoMetadata struct {
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Orientation string    `json:"orientation"` // "portrait", "landscape", "square"
	DateTaken   time.Time `json:"dateTaken"`
}

// ReadPhotoMetadata extracts dimensions, orientation, and date taken.
// Uses govips for dimensions and goexif for date.
func ReadPhotoMetadata(path string) (*PhotoMetadata, error) {
	img, err := vips.LoadImageFromFile(path, nil)
	if err != nil {
		return nil, fmt.Errorf("load image: %w", err)
	}
	defer img.Close()

	// Auto-rotate to get canonical dimensions (handles EXIF orientation)
	if err := img.AutoRotate(); err != nil {
		return nil, fmt.Errorf("auto-rotate: %w", err)
	}

	width := img.Width()
	height := img.Height()

	// Read date taken from EXIF (best effort - zero time if not available)
	dateTaken := readDateTaken(img)

	return &PhotoMetadata{
		Width:       width,
		Height:      height,
		Orientation: deriveOrientation(width, height),
		DateTaken:   dateTaken,
	}, nil
}

// readDateTaken extracts the photo capture date from EXIF data via libvips, which reads
// EXIF from any container format (JPEG, TIFF, HEIC, etc).
// Tries DateTimeOriginal first, then DateTimeDigitized, then DateTime (TIFF tag
// often set by image editors like Photoshop). Returns zero time if no date found.
func readDateTaken(img *vips.ImageRef) time.Time {
	for _, field := range []string{"exif-ifd2-DateTimeOriginal", "exif-ifd2-DateTimeDigitized", "exif-ifd0-DateTime"} {
		if val := img.GetString(field); val != "" {
			if dt, err := parseExifDateTime(val); err == nil {
				return dt
			}
		}
	}
	return time.Time{}
}

// parseExifDateTime parses EXIF date format "2024:01:15 10:30:45". libvips returns exif
// fields formatted as "<value> (<value>, Type, N components, N bytes)", so only the
// portion before the first " (" is used.
// EXIF timestamps carry no timezone; they reflect the camera's local clock. We treat
// them as UTC (the only sensible default) and normalize explicitly with .UTC().
func parseExifDateTime(s string) (time.Time, error) {
	if i := strings.Index(s, " ("); i != -1 {
		s = s[:i]
	}
	s = strings.Trim(s, "\"")
	t, err := time.ParseInLocation("2006:01:02 15:04:05", s, time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// deriveOrientation returns "portrait", "landscape", or "square" based on dimensions.
func deriveOrientation(width, height int) string {
	switch {
	case height > width:
		return "portrait"
	case width > height:
		return "landscape"
	default:
		return "square"
	}
}
