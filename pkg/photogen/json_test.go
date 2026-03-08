package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAlbumSummaries(t *testing.T) {
	t.Parallel()

	t.Run("valid file", func(t *testing.T) {
		albums, err := LoadAlbumSummaries("testdata/albums.json")
		require.NoError(t, err)
		require.Len(t, albums, 2)

		assert.Equal(t, "way", albums[0].Slug)
		assert.Equal(t, "The Way", albums[0].Title)
		assert.Equal(t, 2, albums[0].Count)
		assert.Equal(t, "way/grid/2024-The-Way-1.webp", albums[0].Cover)
		assert.Equal(t, "Apr 2024", albums[0].DateSpan)
		assert.Equal(t, "530 miles along El Camino de Santiago.", albums[0].Description)

		assert.Equal(t, "como", albums[1].Slug)
		assert.Equal(t, "", albums[1].Description, "album without description should be empty")
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadAlbumSummaries("testdata/nonexistent.json")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))
		_, err := LoadAlbumSummaries(path)
		require.Error(t, err)
	})
}

func TestLoadAlbumIndex(t *testing.T) {
	t.Parallel()

	t.Run("valid file", func(t *testing.T) {
		idx, err := LoadAlbumIndex("testdata/index.json")
		require.NoError(t, err)

		assert.Equal(t, "way", idx.Slug)
		assert.Equal(t, "The Way", idx.Title)
		require.Len(t, idx.Photos, 2)

		p := idx.Photos[0]
		assert.Equal(t, "2024-the-way-1", p.ID)
		assert.Equal(t, "2024-The-Way-1.jpg", p.FileName)
		assert.Equal(t, 3072, p.Width)
		assert.Equal(t, 4096, p.Height)
		assert.Equal(t, "portrait", p.Orientation)
		assert.Equal(t, "2024-04-25", p.Date)
		assert.Equal(t, "Starting the journey in Saint-Jean-Pied-de-Port.", p.Description)
		assert.Equal(t, "grid/2024-The-Way-1.webp", p.Src.Grid)
		assert.Equal(t, "full/2024-The-Way-1.webp", p.Src.Full)

		assert.Equal(t, "", idx.Photos[1].Description, "photo without description should be empty")
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadAlbumIndex("testdata/nonexistent.json")
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))
		_, err := LoadAlbumIndex(path)
		require.Error(t, err)
	})
}

func TestAlbumIndexSave(t *testing.T) {
	t.Parallel()

	original, err := LoadAlbumIndex("testdata/index.json")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "index.json")
	require.NoError(t, original.Save(path))

	roundtrip, err := LoadAlbumIndex(path)
	require.NoError(t, err)

	assert.Equal(t, original.Slug, roundtrip.Slug)
	assert.Equal(t, original.Title, roundtrip.Title)
	require.Len(t, roundtrip.Photos, len(original.Photos))
	for i, p := range original.Photos {
		assert.Equal(t, p, roundtrip.Photos[i])
	}
}

func TestSaveAlbumSummaries(t *testing.T) {
	t.Parallel()

	original, err := LoadAlbumSummaries("testdata/albums.json")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "albums.json")
	require.NoError(t, SaveAlbumSummaries(path, original))

	roundtrip, err := LoadAlbumSummaries(path)
	require.NoError(t, err)

	require.Len(t, roundtrip, len(original))
	for i, a := range original {
		assert.Equal(t, a, roundtrip[i])
	}
}
