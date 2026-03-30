package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanOutputDir(t *testing.T) {
	t.Parallel()

	t.Run("nonexistent dir returns nil", func(t *testing.T) {
		t.Parallel()
		err := CleanOutputDir("/nonexistent/path/that/does/not/exist", nil, nil, false)
		assert.NoError(t, err)
	})

	t.Run("removes stale file in site root", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		stale := filepath.Join(dir, "stale.json")
		require.NoError(t, os.WriteFile(stale, []byte("{}"), 0o644))

		err := CleanOutputDir(dir, nil, map[string]bool{}, false)
		require.NoError(t, err)
		assert.NoFileExists(t, stale)
	})

	t.Run("keeps tracked file in site root", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kept := filepath.Join(dir, "albums.json")
		require.NoError(t, os.WriteFile(kept, []byte("[]"), 0o644))

		err := CleanOutputDir(dir, nil, map[string]bool{kept: true}, false)
		require.NoError(t, err)
		assert.FileExists(t, kept)
	})

	t.Run("removes stale file in processed album dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		gridDir := filepath.Join(dir, "myalbum", "grid")
		require.NoError(t, os.MkdirAll(gridDir, 0o755))

		stale := filepath.Join(gridDir, "old.webp")
		require.NoError(t, os.WriteFile(stale, []byte("img"), 0o644))

		err := CleanOutputDir(dir, []string{"myalbum"}, map[string]bool{}, false)
		require.NoError(t, err)
		assert.NoFileExists(t, stale)
	})

	t.Run("leaves unprocessed album dir untouched", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		gridDir := filepath.Join(dir, "other-album", "grid")
		require.NoError(t, os.MkdirAll(gridDir, 0o755))

		kept := filepath.Join(gridDir, "photo.webp")
		require.NoError(t, os.WriteFile(kept, []byte("img"), 0o644))

		// "other-album" is not in processedSlugs — should be left alone
		err := CleanOutputDir(dir, []string{"myalbum"}, map[string]bool{}, false)
		require.NoError(t, err)
		assert.FileExists(t, kept)
	})

	t.Run("dryrun does not remove files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		stale := filepath.Join(dir, "stale.json")
		require.NoError(t, os.WriteFile(stale, []byte("{}"), 0o644))

		err := CleanOutputDir(dir, nil, map[string]bool{}, true)
		require.NoError(t, err)
		assert.FileExists(t, stale, "dryrun should not remove files")
	})
}
