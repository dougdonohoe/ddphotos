package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProcessor(t *testing.T, photos []*Photo) *AlbumProcessor {
	t.Helper()
	cfg := &Config{
		OutputRoot: t.TempDir(),
		SiteID:     "test",
		Warn:       &WarnCollector{},
	}
	ac := &AlbumConfig{Slug: "test-album", Name: "Test Album"}
	ap := NewAlbumProcessor(cfg, ac)
	ap.Photos = photos
	return ap
}

func TestResizePhotos_Success(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
	})

	err := ap.ResizePhotos()
	require.NoError(t, err)

	// Both size variants should have been written.
	for _, size := range AllSizes() {
		outPath := ap.OutputPath(string(size), WebPFileName("landscape-1.jpg"))
		_, statErr := os.Stat(outPath)
		assert.NoError(t, statErr, "expected output file for size %s", size)
	}
}

func TestResizePhotos_SkipExisting(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
	})

	// First run writes files.
	require.NoError(t, ap.ResizePhotos())

	// Second run should skip without error.
	err := ap.ResizePhotos()
	require.NoError(t, err)
}

// TestResizePhotos_SkipExistingWithoutReadingSource proves the skip happens before any
// work is dispatched: the source file is removed after the first run, so a second run
// that still tried to open it would fail.
func TestResizePhotos_SkipExistingWithoutReadingSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "landscape-1.jpg")
	data, err := os.ReadFile(filepath.Join("testdata", "landscape-1.jpg"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, data, 0644))

	ap := newTestProcessor(t, []*Photo{{FileName: "landscape-1.jpg", AbsolutePath: src}})
	require.NoError(t, ap.ResizePhotos())

	require.NoError(t, os.Remove(src))
	assert.NoError(t, ap.ResizePhotos(), "existing outputs must be skipped without opening the source")
}

func TestResizePhotos_DryRun(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
	})
	ap.Config.DryRun = true

	err := ap.ResizePhotos()
	require.NoError(t, err)

	// No files should have been written.
	for _, size := range AllSizes() {
		outPath := ap.OutputPath(string(size), WebPFileName("landscape-1.jpg"))
		_, statErr := os.Stat(outPath)
		assert.True(t, os.IsNotExist(statErr), "expected no output file for size %s in dry-run", size)
	}
}

func TestResizePhotos_ErrorOnBadInput(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "missing.jpg", AbsolutePath: "/nonexistent/missing.jpg"},
	})

	err := ap.ResizePhotos()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resize")
}

func TestResizePhotos_Empty(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{})
	err := ap.ResizePhotos()
	require.NoError(t, err)
}
