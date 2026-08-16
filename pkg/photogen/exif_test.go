package photogen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotateImageLoadErr(t *testing.T) {
	t.Run("nil passes through", func(t *testing.T) {
		assert.NoError(t, annotateImageLoadErr(nil))
	})

	t.Run("unrelated error is unchanged", func(t *testing.T) {
		orig := errors.New("no such file or directory")
		got := annotateImageLoadErr(orig)
		// require before every deref below: the subtest above proves this function can
		// return nil, so an assert here would carry on and panic on got.Error().
		require.Error(t, got)
		assert.Equal(t, orig, got)
		assert.NotContains(t, got.Error(), "cloud-storage")
	})

	t.Run("vips string error gets a hint", func(t *testing.T) {
		// govips returns a plain string error (no typed errno).
		orig := fmt.Errorf("read /photos/x.jpg: resource deadlock avoided")
		got := annotateImageLoadErr(orig)
		require.Error(t, got)
		assert.Contains(t, got.Error(), "resource deadlock avoided")
		assert.Contains(t, got.Error(), "cloud-storage")
		assert.Contains(t, got.Error(), "available offline")
	})

	t.Run("typed EDEADLK errno gets a hint", func(t *testing.T) {
		orig := fmt.Errorf("read x.jpg: %w", syscall.EDEADLK)
		got := annotateImageLoadErr(orig)
		require.Error(t, got)
		assert.Contains(t, got.Error(), "cloud-storage")
	})
}

// createTestImage creates a solid-color JPEG at the specified dimensions.
// Returns the path to the created image.
func createTestImage(t *testing.T, dir string, name string, width, height int) string {
	t.Helper()

	// Create a solid color image (black)
	img, err := vips.Black(width, height)
	require.NoError(t, err, "failed to create test image")
	defer img.Close()

	path := filepath.Join(dir, name)
	ep := vips.NewJpegExportParams()
	ep.Quality = 80

	buf, _, err := img.ExportJpeg(ep)
	require.NoError(t, err, "failed to export test image")

	err = writeFile(path, buf)
	require.NoError(t, err, "failed to write test image")

	return path
}

// writeFile is a simple helper to write bytes to a file.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func TestReadPhotoMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		filename   string
		width      int
		height     int
		wantOrient string
	}{
		{
			name:       "landscape",
			filename:   "landscape.jpg",
			width:      80,
			height:     60,
			wantOrient: "landscape",
		},
		{
			name:       "portrait",
			filename:   "portrait.jpg",
			width:      60,
			height:     80,
			wantOrient: "portrait",
		},
		{
			name:       "square",
			filename:   "square.jpg",
			width:      64,
			height:     64,
			wantOrient: "square",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := createTestImage(t, tmpDir, tc.filename, tc.width, tc.height)

			meta, err := ReadPhotoMetadata(path)
			require.NoError(t, err)

			assert.Equal(t, tc.width, meta.Width)
			assert.Equal(t, tc.height, meta.Height)
			assert.Equal(t, tc.wantOrient, meta.Orientation)
		})
	}
}

func TestReadPhotoMetadata_FileNotFound(t *testing.T) {
	_, err := ReadPhotoMetadata("/nonexistent/path/photo.jpg")
	require.Error(t, err)
}

func TestReadPhotoMetadata_RealImages(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		wantWidth  int
		wantHeight int
		wantOrient string
		wantDate   string // expected date in "2006-01-02" format
	}{
		{
			name:       "landscape",
			filename:   "landscape-1.jpg",
			wantWidth:  5028,
			wantHeight: 3317,
			wantOrient: "landscape",
			wantDate:   "2024-05-16", // from DateTimeDigitized (no DateTimeOriginal)
		},
		{
			name:       "portrait",
			filename:   "portrait-1.jpg",
			wantWidth:  4284,
			wantHeight: 5712,
			wantOrient: "portrait",
			wantDate:   "2024-05-31", // from DateTimeDigitized (no DateTimeOriginal)
		},
		{
			name:       "datetime-fallback",
			filename:   "no-create-date.jpg",
			wantWidth:  1440,
			wantHeight: 2160,
			wantOrient: "portrait",
			wantDate:   "2005-01-13", // from DateTime (no DateTimeOriginal or DateTimeDigitized)
		},
		{
			name:       "heic",
			filename:   "landscape-1.heic",
			wantWidth:  4032,
			wantHeight: 3024,
			wantOrient: "landscape",
			wantDate:   "2023-04-21", // from DateTimeOriginal
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.filename)

			meta, err := ReadPhotoMetadata(path)
			require.NoError(t, err)

			assert.Equal(t, tc.wantWidth, meta.Width)
			assert.Equal(t, tc.wantHeight, meta.Height)
			assert.Equal(t, tc.wantOrient, meta.Orientation)

			assert.False(t, meta.DateTaken.IsZero(), "expected date to be set")
			assert.Equal(t, tc.wantDate, meta.DateTaken.Format("2006-01-02"))
			t.Logf("%s: date taken = %s", tc.filename, meta.DateTaken.Format("2006-01-02 15:04:05"))

			// Values served from the cache, including across a save/load round trip, must
			// match the direct read exactly. Dimensions in particular are post-AutoRotate
			// canonical values, so a cache that lost the rotation would show up here.
			cachePath := filepath.Join(t.TempDir(), MetaCacheFileName)
			mc := NewMetaCache(cachePath)

			fresh, err := mc.Metadata(path)
			require.NoError(t, err)
			assert.Equal(t, meta, fresh, "cache miss must match a direct read")

			cached, err := mc.Metadata(path)
			require.NoError(t, err)
			assert.Equal(t, meta, cached, "cache hit must match a direct read")

			require.NoError(t, mc.Save())
			reloaded, err := LoadMetaCache(cachePath).Metadata(path)
			require.NoError(t, err)
			assert.Equal(t, meta, reloaded, "reloaded cache must match a direct read")
		})
	}
}

// TestReadVideoMetadata_Cached mirrors the cache round trip in
// TestReadPhotoMetadata_RealImages, for video.
//
// Duration is the field metaCacheVersion was bumped for, and it is the one that can only
// break on the *second* run: a cache that dropped it would leave every video reporting
// zero seconds, showing 0:00 on the grid badge, while a fresh checkout looked perfect.
//
// portrait-rotated.mov earns its place here for the same reason the photo test calls out
// AutoRotate: its dimensions are rotation-corrected (240x320, not the stored 320x240), so
// a cache that round-tripped the raw ffprobe numbers would surface here.
func TestReadVideoMetadata_Cached(t *testing.T) {
	requireVideoTools(t)

	for _, name := range []string{"landscape.mov", "portrait-rotated.mov"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)

			meta, err := ReadMediaMetadata(path)
			require.NoError(t, err)
			require.Positive(t, meta.Duration, "fixture needs a duration or this proves nothing")

			cachePath := filepath.Join(t.TempDir(), MetaCacheFileName)
			mc := NewMetaCache(cachePath)

			fresh, err := mc.Metadata(path)
			require.NoError(t, err)
			assert.Equal(t, meta, fresh, "cache miss must match a direct read")

			cached, err := mc.Metadata(path)
			require.NoError(t, err)
			assert.Equal(t, meta, cached, "cache hit must match a direct read")

			require.NoError(t, mc.Save())
			reloaded, err := LoadMetaCache(cachePath).Metadata(path)
			require.NoError(t, err)
			assert.Equal(t, meta, reloaded, "reloaded cache must match a direct read")
		})
	}
}

func TestDeriveOrientation(t *testing.T) {
	tests := []struct {
		width  int
		height int
		want   string
	}{
		{100, 50, "landscape"},
		{50, 100, "portrait"},
		{100, 100, "square"},
		{1, 1, "square"},
		{1920, 1080, "landscape"},
		{1080, 1920, "portrait"},
	}

	for _, tc := range tests {
		got := deriveOrientation(tc.width, tc.height)
		assert.Equal(t, tc.want, got, "deriveOrientation(%d, %d)", tc.width, tc.height)
	}
}
