package photogen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPhotoDescriptions(t *testing.T) {
	t.Parallel()

	t.Run("missing file returns empty result", func(t *testing.T) {
		pd, err := loadPhotoDescriptions(t.TempDir())
		require.NoError(t, err)
		assert.Empty(t, pd.order)
		assert.Empty(t, pd.descriptions)
	})

	t.Run("valid file", func(t *testing.T) {
		dir := t.TempDir()
		content := `
# This is a comment
img_0001 First photo of the trip.
img_0002 Arrival at the hotel.

img_0003
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"), []byte(content), 0o644))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)

		assert.Equal(t, []string{"img_0001", "img_0002", "img_0003"}, pd.order)
		assert.Equal(t, "First photo of the trip.", pd.descriptions["img_0001"])
		assert.Equal(t, "Arrival at the hotel.", pd.descriptions["img_0002"])
		assert.Equal(t, "", pd.descriptions["img_0003"], "entry with no description should be empty string")
	})

	t.Run("IDs are lowercased", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"), []byte("IMG_0001 A photo.\n"), 0o644))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)

		assert.Equal(t, []string{"img_0001"}, pd.order)
		assert.Equal(t, "A photo.", pd.descriptions["img_0001"])
	})
}

func TestReorderByDescriptionFile(t *testing.T) {
	t.Parallel()

	day := func(d int) time.Time {
		return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC)
	}

	photos := []*Photo{
		{ID: "img_0001", FileName: "IMG_0001.jpg", PhotoMetadata: &PhotoMetadata{DateTaken: day(1)}},
		{ID: "img_0002", FileName: "IMG_0002.jpg", PhotoMetadata: &PhotoMetadata{DateTaken: day(2)}},
		{ID: "img_0003", FileName: "IMG_0003.jpg", PhotoMetadata: &PhotoMetadata{DateTaken: day(3)}},
		{ID: "img_0004", FileName: "IMG_0004.jpg", PhotoMetadata: &PhotoMetadata{DateTaken: day(4)}},
	}

	ap := &AlbumProcessor{}

	t.Run("full manual order", func(t *testing.T) {
		order := []string{"img_0003", "img_0001", "img_0004", "img_0002"}
		result := ap.reorderByDescriptionFile(photos, order)
		require.Len(t, result, 4)
		assert.Equal(t, "img_0003", result[0].ID)
		assert.Equal(t, "img_0001", result[1].ID)
		assert.Equal(t, "img_0004", result[2].ID)
		assert.Equal(t, "img_0002", result[3].ID)
	})

	t.Run("unmentioned photos appended sorted by date", func(t *testing.T) {
		// Only mention two photos; img_0002 and img_0004 should appear at end sorted by date
		order := []string{"img_0003", "img_0001"}
		result := ap.reorderByDescriptionFile(photos, order)
		require.Len(t, result, 4)
		assert.Equal(t, "img_0003", result[0].ID)
		assert.Equal(t, "img_0001", result[1].ID)
		assert.Equal(t, "img_0002", result[2].ID, "unmentioned photos sorted by date")
		assert.Equal(t, "img_0004", result[3].ID, "unmentioned photos sorted by date")
	})

	t.Run("unknown ID in order is skipped", func(t *testing.T) {
		order := []string{"img_0001", "img_9999", "img_0002"}
		result := ap.reorderByDescriptionFile(photos, order)
		// img_9999 is unknown, img_0003 and img_0004 are unmentioned extras
		require.Len(t, result, 4)
		assert.Equal(t, "img_0001", result[0].ID)
		assert.Equal(t, "img_0002", result[1].ID)
		assert.Equal(t, "img_0003", result[2].ID)
		assert.Equal(t, "img_0004", result[3].ID)
	})
}
