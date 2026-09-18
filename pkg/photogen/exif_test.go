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

// assertCacheRoundTrip pins that MetaCache never changes what a direct read reported, at
// every stage it could: a cache miss, a cache hit, and a save/load round trip through
// disk. want is the direct read to match.
//
// The round trip is the stage worth the extra lines. A field that a fresh read populates
// but the on-disk format drops looks perfect on a clean checkout and only goes wrong on
// the *second* run, which is the run nobody tests by hand.
func assertCacheRoundTrip(t *testing.T, path string, want *PhotoMetadata) {
	t.Helper()

	cachePath := filepath.Join(t.TempDir(), MetaCacheFileName)
	mc := NewMetaCache(cachePath)

	fresh, err := mc.Metadata(path)
	require.NoError(t, err)
	assert.Equal(t, want, fresh, "cache miss must match a direct read")

	cached, err := mc.Metadata(path)
	require.NoError(t, err)
	assert.Equal(t, want, cached, "cache hit must match a direct read")

	require.NoError(t, mc.Save())
	reloaded, err := LoadMetaCache(cachePath, nil).Metadata(path)
	require.NoError(t, err)
	assert.Equal(t, want, reloaded, "reloaded cache must match a direct read")
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
			// PNG carries EXIF in an eXIf chunk rather than a JPEG APP1 marker, so this
			// pins that readDateTaken reaches it through libvips all the same. Downscaled
			// from landscape-1.jpg and palette-quantized to keep the fixture small; 1600px
			// wide is deliberate, so it is also large enough for the hero tests.
			name:       "png",
			filename:   "landscape-1.png",
			wantWidth:  1600,
			wantHeight: 1056,
			wantOrient: "landscape",
			wantDate:   "2024-05-16", // from DateTimeDigitized, same source as landscape-1.jpg
		},
		{
			// WebP keeps EXIF in a RIFF chunk, a third container shape after JPEG's APP1
			// marker and PNG's eXIf chunk. Same source and downscale as landscape-1.png.
			name:       "webp",
			filename:   "landscape-1.webp",
			wantWidth:  1600,
			wantHeight: 1056,
			wantOrient: "landscape",
			wantDate:   "2024-05-16", // from DateTimeDigitized, same source as landscape-1.jpg
		},
		{
			name:       "heic",
			filename:   "landscape-1.heic",
			wantWidth:  4032,
			wantHeight: 3024,
			wantOrient: "landscape",
			wantDate:   "2023-04-21", // from DateTimeOriginal
		},
		{
			// Same source as landscape-1.heic, re-encoded to AV1 in the same HEIF
			// container, so it pins that EXIF survives the codec swap.
			name:       "avif",
			filename:   "landscape-1.avif",
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

			// Dimensions here are post-AutoRotate canonical values, so a cache that lost
			// the rotation would show up in the round trip.
			assertCacheRoundTrip(t, path, meta)
		})
	}
}

// TestReadPhotoMetadata_TIFF pins a real gap rather than a passing feature: libvips'
// tiffload does not surface EXIF, so a TIFF source is always undated.
//
// landscape-1.tiff genuinely carries the tags — they were injected with exiftool, which
// reads CreateDate and ModifyDate back out of it — and libvips still reports none. It is
// not the fixture: files written by vipsthumbnail and by ImageMagick behave the same way.
//
// The consequence is in LoadPhotos, not here. An album of TIFFs sorts entirely by scan
// order and warns "N/M photos have no EXIF date", so anyone scanning film into TIFF needs
// photogen.txt with manual_sort_order to control sequence. If a future libvips learns to
// read it, this test fails and says so, and the sort expectation changes with it.
func TestReadPhotoMetadata_TIFF(t *testing.T) {
	path := filepath.Join("testdata", "landscape-1.tiff")

	meta, err := ReadPhotoMetadata(path)
	require.NoError(t, err)

	assert.Equal(t, 1600, meta.Width)
	assert.Equal(t, 1056, meta.Height)
	assert.Equal(t, "landscape", meta.Orientation)
	assert.True(t, meta.DateTaken.IsZero(), "libvips tiffload exposes no EXIF; see doc comment")

	assertCacheRoundTrip(t, path, meta)
}

// TestReadVideoMetadata_Cached runs assertCacheRoundTrip over video, where the metadata
// comes from ffprobe rather than libvips.
//
// Duration is the field metaCacheVersion was bumped for: a cache that dropped it would
// leave every video reporting zero seconds, showing 0:00 on the grid badge.
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

			assertCacheRoundTrip(t, path, meta)
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

// corruptJPEG returns a copy of a real fixture with a run of scan data overwritten. The
// header and EXIF stay intact, so the file still declares its true dimensions and date;
// only the compressed pixels are damaged. This is the shape of defect a photo library
// actually accumulates, and the one libvips' FailOnError rejects.
func corruptJPEG(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "landscape-1.jpg"))
	require.NoError(t, err)
	start := len(data) / 2
	for i := start; i < start+8192 && i < len(data); i++ {
		data[i] = 0xAB
	}
	path := filepath.Join(dir, "corrupt.jpg")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

// loadImage is the single place photogen opens a file with libvips, and it must stay
// tolerant. The strict load it is compared against is what govips substitutes for nil
// params, so the comparison shows the setting is doing real work.
func TestLoadImageToleratesCorruptScanData(t *testing.T) {
	path := corruptJPEG(t, t.TempDir())

	// A strict load fails, but only once something forces the pixels to be decoded.
	strict, err := vips.LoadImageFromFile(path, nil)
	require.NoError(t, err, "the header is intact, so the load itself succeeds either way")
	_, _, strictErr := strict.ExportNative()
	strict.Close()
	require.Error(t, strictErr, "a strict decode must reject this file, or the test proves nothing")
	assert.Contains(t, strictErr.Error(), "Corrupt JPEG data")

	// The shared loader tolerates it through a full decode.
	img, err := loadImage(path)
	require.NoError(t, err)
	defer img.Close()
	_, _, err = img.ExportNative()
	assert.NoError(t, err, "loadImage must tolerate damaged scan data")
}

// The metadata path must accept any file the resize path accepts: a photo whose WebPs
// photogen will happily generate must not be one that kills the run when its dimensions
// are read.
func TestReadPhotoMetadataToleratesCorruptScanData(t *testing.T) {
	path := corruptJPEG(t, t.TempDir())

	meta, err := ReadPhotoMetadata(path)
	require.NoError(t, err)
	assert.Equal(t, 5028, meta.Width)
	assert.Equal(t, 3317, meta.Height)
	assert.False(t, meta.DateTaken.IsZero(), "EXIF is intact, only the scan data is damaged")

	_, err = ResizeImage(path, filepath.Join(t.TempDir(), "out.webp"), SizeGrid, true, false)
	assert.NoError(t, err, "the resize path must accept the same file")
}
