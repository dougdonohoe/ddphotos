package photogen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("a folder that has never synced reads as an empty record", func(t *testing.T) {
		t.Parallel()
		m, err := loadSyncMetadata(t.TempDir())
		require.NoError(t, err)
		assert.Empty(t, m.Photos)
		assert.Empty(t, m.byAssetID())
	})

	t.Run("what is written is what is read back", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		want := &SyncMetadata{
			Provider:    "mock",
			AlbumID:     "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18",
			SyncedAt:    time.Date(2026, 9, 20, 14, 4, 11, 0, time.UTC),
			Name:        "Meribel 2025",
			Description: "A &lt;walk&gt; &amp; a ski",
			Photos: []SyncPhotoMeta{{
				AssetID:          "b68fbc77-bbed-4008-a118-ba8637148081",
				File:             "IMG_1583.jpeg",
				OriginalFileName: "IMG_1583.jpeg",
				Size:             4495360,
				Checksum:         "u6WCo/UHcoQb5egQ184RHR9ZUWw=",
				UpdatedAt:        time.Date(2026, 9, 18, 22, 43, 56, 0, time.UTC),
				Caption:          "So wide open!",
			}},
		}
		require.NoError(t, writeSyncMetadata(dir, want))

		got, err := loadSyncMetadata(dir)
		require.NoError(t, err)
		assert.Equal(t, want.Provider, got.Provider)
		assert.Equal(t, want.AlbumID, got.AlbumID)
		assert.True(t, want.SyncedAt.Equal(got.SyncedAt))
		assert.Equal(t, want.Name, got.Name)
		assert.Equal(t, want.Description, got.Description)
		require.Len(t, got.Photos, 1)
		assert.Equal(t, want.Photos[0].File, got.Photos[0].File)
		assert.Equal(t, want.Photos[0].Checksum, got.Photos[0].Checksum)
		assert.Equal(t, want.Photos[0].Caption, got.Photos[0].Caption)
		assert.True(t, want.Photos[0].UpdatedAt.Equal(got.Photos[0].UpdatedAt))
	})

	t.Run("the file says it is generated", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, writeSyncMetadata(dir, &SyncMetadata{Provider: "mock"}))
		data, err := os.ReadFile(filepath.Join(dir, SyncMetadataFileName))
		require.NoError(t, err)
		assert.Contains(t, string(data), "Do not edit")
	})

	t.Run("indexes are keyed the two ways the sync run needs", func(t *testing.T) {
		t.Parallel()
		m := &SyncMetadata{Photos: []SyncPhotoMeta{
			{AssetID: "a1", File: "one.jpg", Caption: "One"},
			{AssetID: "a2", File: "two.jpg", Caption: "Two"},
		}}
		byID := m.byAssetID()
		require.Contains(t, byID, "a2")
		assert.Equal(t, "two.jpg", byID["a2"].File)
		assert.Equal(t, map[string]string{"one.jpg": "One", "two.jpg": "Two"}, m.captionByFile())
	})
}

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	t.Run("replaces the file and leaves no temp behind", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "thing.txt")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
		require.NoError(t, writeFileAtomic(path, []byte("new")))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "new", string(data))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "a temp file was left behind")
		assert.Equal(t, "thing.txt", entries[0].Name())
	})

	// Files written inside a Docker container run as root, and the host user that mounted
	// the volume has to be able to edit photogen.txt afterward.
	t.Run("the result is group and world writable, not CreateTemp's 0600", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "thing.txt")
		require.NoError(t, writeFileAtomic(path, []byte("x")))
		st, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, filePerms, st.Mode().Perm()&filePerms)
	})

	t.Run("creates missing parent directories", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "a", "b", "thing.txt")
		require.NoError(t, writeFileAtomic(path, []byte("x")))
		assert.FileExists(t, path)
	})
}
