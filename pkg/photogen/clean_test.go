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
		err := CleanOutputDir("/nonexistent/path/that/does/not/exist", nil, nil, nil, false, nil)
		assert.NoError(t, err)
	})

	t.Run("removes stale file in site root", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		stale := filepath.Join(dir, "stale.json")
		require.NoError(t, os.WriteFile(stale, []byte("{}"), 0o644))

		err := CleanOutputDir(dir, nil, nil, map[string]bool{}, false, nil)
		require.NoError(t, err)
		assert.NoFileExists(t, stale)
	})

	t.Run("keeps tracked file in site root", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		kept := filepath.Join(dir, "albums.json")
		require.NoError(t, os.WriteFile(kept, []byte("[]"), 0o644))

		err := CleanOutputDir(dir, nil, nil, map[string]bool{kept: true}, false, nil)
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

		err := CleanOutputDir(dir, []string{"myalbum"}, []string{"myalbum"}, map[string]bool{}, false, nil)
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
		err := CleanOutputDir(dir, []string{"myalbum"}, []string{"myalbum", "other-album"}, map[string]bool{}, false, nil)
		require.NoError(t, err)
		assert.FileExists(t, kept)
	})

	t.Run("dryrun does not remove files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		stale := filepath.Join(dir, "stale.json")
		require.NoError(t, os.WriteFile(stale, []byte("{}"), 0o644))

		err := CleanOutputDir(dir, nil, nil, map[string]bool{}, true, nil)
		require.NoError(t, err)
		assert.FileExists(t, stale, "dryrun should not remove files")
	})

	// A removal that fails must reach the end-of-run summary, not just scroll past: the
	// file stays in the output and the next deploy ships it.
	t.Run("failed removal is collected as a warning", func(t *testing.T) {
		t.Parallel()
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions, so the removal would succeed")
		}
		dir := t.TempDir()
		gridDir := filepath.Join(dir, "myalbum", "grid")
		require.NoError(t, os.MkdirAll(gridDir, 0o755))
		stale := filepath.Join(gridDir, "old.webp")
		require.NoError(t, os.WriteFile(stale, []byte("img"), 0o644))

		// Unlinking needs write permission on the directory, not the file.
		require.NoError(t, os.Chmod(gridDir, 0o555))
		t.Cleanup(func() { _ = os.Chmod(gridDir, 0o755) })

		wc := &WarnCollector{}
		err := CleanOutputDir(dir, []string{"myalbum"}, []string{"myalbum"}, map[string]bool{}, false, wc)
		require.NoError(t, err, "a failed removal is a warning, not an error")
		assert.FileExists(t, stale)
		require.Len(t, wc.warnings, 1)
		assert.Contains(t, wc.warnings[0], stale)
	})

	// A renamed album leaves its old directory behind. It is in no album's slug, so it is
	// removed whole rather than walked, even though it was not processed this run.
	t.Run("removes unconfigured album dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		oldDir := filepath.Join(dir, "tbd-2027")
		require.NoError(t, os.MkdirAll(filepath.Join(oldDir, "grid"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(oldDir, "grid", "a.webp"), []byte("img"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(oldDir, "index.json"), []byte("{}"), 0o644))
		newDir := filepath.Join(dir, "park-city-canyons-2027")
		require.NoError(t, os.MkdirAll(newDir, 0o755))

		slugs := []string{"park-city-canyons-2027"}
		err := CleanOutputDir(dir, slugs, slugs, map[string]bool{}, false, nil)
		require.NoError(t, err)
		assert.NoDirExists(t, oldDir)
		assert.DirExists(t, newDir)
	})

	t.Run("dryrun keeps unconfigured album dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		oldDir := filepath.Join(dir, "tbd-2027")
		require.NoError(t, os.MkdirAll(oldDir, 0o755))

		err := CleanOutputDir(dir, nil, []string{"other"}, map[string]bool{}, true, nil)
		require.NoError(t, err)
		assert.DirExists(t, oldDir)
	})

	t.Run("keeps dot-directories at site root", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		dotDir := filepath.Join(dir, ".keep")
		require.NoError(t, os.MkdirAll(dotDir, 0o755))

		err := CleanOutputDir(dir, nil, nil, map[string]bool{}, false, nil)
		require.NoError(t, err)
		assert.DirExists(t, dotDir)
	})
}
