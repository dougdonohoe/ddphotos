package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResizeImage_AllSizes(t *testing.T) {
	inputPath := filepath.Join("testdata", "landscape-1.jpg")
	tmpDir := t.TempDir()

	for _, size := range AllSizes() {
		t.Run(string(size), func(t *testing.T) {
			outputPath := filepath.Join(tmpDir, string(size)+".jpg")

			result, err := ResizeImage(inputPath, outputPath, size, false, false)
			require.NoError(t, err)

			assert.True(t, result.Written)
			assert.False(t, result.Skipped)
			assert.False(t, result.DryRun)

			// Verify file was created
			info, err := os.Stat(outputPath)
			require.NoError(t, err)
			assert.Greater(t, info.Size(), int64(0))

			// Verify dimensions are within limits
			meta, err := ReadPhotoMetadata(outputPath)
			require.NoError(t, err)

			cfg, _ := GetSizeConfig(size)
			assert.LessOrEqual(t, meta.Width, cfg.MaxDimension)
			assert.LessOrEqual(t, meta.Height, cfg.MaxDimension)

			t.Logf("%s: %dx%d (max %d)", size, meta.Width, meta.Height, cfg.MaxDimension)
		})
	}
}

func TestResizeImage_HEIC(t *testing.T) {
	inputPath := filepath.Join("testdata", "landscape-1.heic")
	tmpDir := t.TempDir()

	for _, size := range AllSizes() {
		t.Run(string(size), func(t *testing.T) {
			outputPath := filepath.Join(tmpDir, string(size)+".webp")

			result, err := ResizeImage(inputPath, outputPath, size, false, false)
			require.NoError(t, err)
			assert.True(t, result.Written)

			meta, err := ReadPhotoMetadata(outputPath)
			require.NoError(t, err)

			cfg, _ := GetSizeConfig(size)
			assert.LessOrEqual(t, meta.Width, cfg.MaxDimension)
			assert.LessOrEqual(t, meta.Height, cfg.MaxDimension)
		})
	}
}

func TestResizeImage_Portrait(t *testing.T) {
	inputPath := filepath.Join("testdata", "portrait-1.jpg")
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "portrait-thumb.jpg")

	result, err := ResizeImage(inputPath, outputPath, SizeGrid, false, false)
	require.NoError(t, err)
	assert.True(t, result.Written)

	meta, err := ReadPhotoMetadata(outputPath)
	require.NoError(t, err)

	// Portrait image: height should be the constrained dimension
	cfg, _ := GetSizeConfig(SizeGrid)
	assert.LessOrEqual(t, meta.Height, cfg.MaxDimension)
	assert.Equal(t, "portrait", meta.Orientation)

	t.Logf("portrait thumb: %dx%d", meta.Width, meta.Height)
}

func TestResizeImage_SkipExisting(t *testing.T) {
	inputPath := filepath.Join("testdata", "landscape-1.jpg")
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "existing.jpg")

	// Create file first time
	result1, err := ResizeImage(inputPath, outputPath, SizeGrid, false, false)
	require.NoError(t, err)
	assert.True(t, result1.Written)

	// Get file info
	info1, _ := os.Stat(outputPath)
	modTime1 := info1.ModTime()

	// Second call should skip (file exists, force=false)
	result2, err := ResizeImage(inputPath, outputPath, SizeGrid, false, false)
	require.NoError(t, err)
	assert.True(t, result2.Skipped)
	assert.False(t, result2.Written)

	// Verify file wasn't modified
	info2, _ := os.Stat(outputPath)
	assert.Equal(t, modTime1, info2.ModTime())
}

func TestResizeImage_ForceOverwrite(t *testing.T) {
	inputPath := filepath.Join("testdata", "landscape-1.jpg")
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "force.jpg")

	// Create file first time
	result1, err := ResizeImage(inputPath, outputPath, SizeGrid, false, false)
	require.NoError(t, err)
	assert.True(t, result1.Written)

	// With force=true, should overwrite
	result2, err := ResizeImage(inputPath, outputPath, SizeGrid, true, false)
	require.NoError(t, err)
	assert.True(t, result2.Written)
	assert.False(t, result2.Skipped)
}

func TestResizeImage_DryRun(t *testing.T) {
	inputPath := filepath.Join("testdata", "landscape-1.jpg")
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "dryrun.jpg")

	result, err := ResizeImage(inputPath, outputPath, SizeGrid, false, true)
	require.NoError(t, err)

	assert.True(t, result.DryRun)
	assert.False(t, result.Written)
	assert.Contains(t, result.Message, "DRYRUN")

	// Verify file was NOT created
	_, err = os.Stat(outputPath)
	assert.True(t, os.IsNotExist(err))
}

func TestResizeImage_InvalidSize(t *testing.T) {
	inputPath := filepath.Join("testdata", "landscape-1.jpg")
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "invalid.jpg")

	_, err := ResizeImage(inputPath, outputPath, "invalid", false, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown image size")
}

func TestResizeImage_InputNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.jpg")

	_, err := ResizeImage("/nonexistent/image.jpg", outputPath, SizeGrid, false, false)
	assert.Error(t, err)
}

func TestAllSizes(t *testing.T) {
	sizes := AllSizes()
	assert.Len(t, sizes, 2)
	assert.Contains(t, sizes, SizeGrid)
	assert.Contains(t, sizes, SizeFull)
}

func TestGetSizeConfig(t *testing.T) {
	tests := []struct {
		size        ImageSize
		wantMax     int
		wantQuality int
		wantFound   bool
	}{
		{SizeGrid, 600, 85, true},
		{SizeFull, 1600, 90, true},
		{"invalid", 0, 0, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.size), func(t *testing.T) {
			cfg, found := GetSizeConfig(tc.size)
			assert.Equal(t, tc.wantFound, found)
			if found {
				assert.Equal(t, tc.wantMax, cfg.MaxDimension)
				assert.Equal(t, tc.wantQuality, cfg.Quality)
			}
		})
	}
}

// --- ResizeCoverJPEG tests ---

func TestResizeCoverJPEG_Landscape(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cover.jpg")

	result, err := ResizeCoverJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, false, false)
	require.NoError(t, err)
	assert.True(t, result.Written)
	assert.False(t, result.Skipped)
	assert.False(t, result.DryRun)

	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))

	meta, err := ReadPhotoMetadata(outputPath)
	require.NoError(t, err)
	assert.LessOrEqual(t, meta.Width, coverJPEGMaxDimension)
	assert.LessOrEqual(t, meta.Height, coverJPEGMaxDimension)
	t.Logf("cover landscape: %dx%d", meta.Width, meta.Height)
}

func TestResizeCoverJPEG_Portrait(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cover-portrait.jpg")

	result, err := ResizeCoverJPEG(filepath.Join("testdata", "portrait-1.jpg"), outputPath, false, false)
	require.NoError(t, err)
	assert.True(t, result.Written)

	meta, err := ReadPhotoMetadata(outputPath)
	require.NoError(t, err)
	assert.LessOrEqual(t, meta.Width, coverJPEGMaxDimension)
	assert.LessOrEqual(t, meta.Height, coverJPEGMaxDimension)
	t.Logf("cover portrait: %dx%d", meta.Width, meta.Height)
}

func TestResizeCoverJPEG_SkipExisting(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cover-skip.jpg")

	result1, err := ResizeCoverJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, false, false)
	require.NoError(t, err)
	assert.True(t, result1.Written)

	result2, err := ResizeCoverJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, false, false)
	require.NoError(t, err)
	assert.True(t, result2.Skipped)
	assert.False(t, result2.Written)
	assert.Contains(t, result2.Message, "exists:")
}

func TestResizeCoverJPEG_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cover-force.jpg")

	_, err := ResizeCoverJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, false, false)
	require.NoError(t, err)

	result, err := ResizeCoverJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, true, false)
	require.NoError(t, err)
	assert.True(t, result.Written)
	assert.False(t, result.Skipped)
}

func TestResizeCoverJPEG_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cover-dryrun.jpg")

	result, err := ResizeCoverJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, false, true)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.False(t, result.Written)
	assert.Contains(t, result.Message, "DRYRUN")

	_, err = os.Stat(outputPath)
	assert.True(t, os.IsNotExist(err))
}

func TestResizeCoverJPEG_InputNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ResizeCoverJPEG("/nonexistent/image.jpg", filepath.Join(tmpDir, "out.jpg"), false, false)
	assert.Error(t, err)
}

// --- ResizeHeroJPEG tests ---

func TestResizeHeroJPEG_Write(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "hero.jpg")

	result, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, "center", false, false)
	require.NoError(t, err)
	assert.True(t, result.Written)
	assert.False(t, result.Skipped)
	assert.False(t, result.DryRun)

	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))

	meta, err := ReadPhotoMetadata(outputPath)
	require.NoError(t, err)
	assert.Equal(t, heroWidth, meta.Width)
	assert.Equal(t, heroHeight, meta.Height)
	t.Logf("hero: %dx%d", meta.Width, meta.Height)
}

func TestResizeHeroJPEG_HEIC(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "hero.jpg")

	result, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.heic"), outputPath, "center", false, false)
	require.NoError(t, err)
	assert.True(t, result.Written)

	meta, err := ReadPhotoMetadata(outputPath)
	require.NoError(t, err)
	assert.Equal(t, heroWidth, meta.Width)
	assert.Equal(t, heroHeight, meta.Height)
}

func TestResizeHeroJPEG_CropVariants(t *testing.T) {
	for _, crop := range []string{"top", "bottom", "center", ""} {
		t.Run("crop="+crop, func(t *testing.T) {
			tmpDir := t.TempDir()
			outputPath := filepath.Join(tmpDir, "hero.jpg")

			result, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, crop, false, false)
			require.NoError(t, err)
			assert.True(t, result.Written)

			meta, err := ReadPhotoMetadata(outputPath)
			require.NoError(t, err)
			assert.Equal(t, heroWidth, meta.Width)
			assert.Equal(t, heroHeight, meta.Height)
		})
	}
}

func TestResizeHeroJPEG_SkipExisting(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "hero-skip.jpg")

	_, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, "center", false, false)
	require.NoError(t, err)

	result, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, "center", false, false)
	require.NoError(t, err)
	assert.True(t, result.Skipped)
	assert.False(t, result.Written)
	assert.Contains(t, result.Message, "exists:")
}

func TestResizeHeroJPEG_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "hero-force.jpg")

	_, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, "center", false, false)
	require.NoError(t, err)

	result, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, "center", true, false)
	require.NoError(t, err)
	assert.True(t, result.Written)
	assert.False(t, result.Skipped)
}

func TestResizeHeroJPEG_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "hero-dryrun.jpg")

	result, err := ResizeHeroJPEG(filepath.Join("testdata", "landscape-1.jpg"), outputPath, "center", false, true)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.False(t, result.Written)
	assert.Contains(t, result.Message, "DRYRUN")

	_, err = os.Stat(outputPath)
	assert.True(t, os.IsNotExist(err))
}

func TestResizeHeroJPEG_TooSmall(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "hero.jpg")
	inputPath := filepath.Join("testdata", "no-exif.jpg")

	_, err := ResizeHeroJPEG(inputPath, outputPath, "center", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), inputPath)
	assert.Contains(t, err.Error(), "too small for hero")

	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestResizeHeroJPEG_InputNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ResizeHeroJPEG("/nonexistent/image.jpg", filepath.Join(tmpDir, "hero.jpg"), "center", false, false)
	assert.Error(t, err)
}
