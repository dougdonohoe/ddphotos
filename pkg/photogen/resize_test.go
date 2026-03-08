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
