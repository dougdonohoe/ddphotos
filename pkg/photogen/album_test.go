package photogen

import (
	"io"
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

func TestLoadPhotoDescriptionsExtensions(t *testing.T) {
	t.Parallel()

	t.Run("entries with image extensions are stripped", func(t *testing.T) {
		dir := t.TempDir()
		content := "img_0001.jpg First photo.\nimg_0002.JPG Second photo.\nimg_0003\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"), []byte(content), 0o644))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)

		assert.Equal(t, []string{"img_0001", "img_0002", "img_0003"}, pd.order)
		assert.Equal(t, "First photo.", pd.descriptions["img_0001"])
		assert.Equal(t, "Second photo.", pd.descriptions["img_0002"])
	})

	t.Run("subfolder entries have no extension and are not modified", func(t *testing.T) {
		dir := t.TempDir()
		content := "img_0001.jpg A caption.\nCraig's\nhalstead\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"), []byte(content), 0o644))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)

		// photo entry: extension stripped, lowercased
		assert.Contains(t, pd.order, "img_0001")
		// subfolder entries: just lowercased
		assert.Contains(t, pd.order, "craig's")
		assert.Contains(t, pd.order, "halstead")
	})
}

func TestSanitizePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		relDir string
		want   string
	}{
		{"", ""},
		{".", ""},
		{"Craig's", "craigs"},
		{"Ski 2007", "ski2007"},
		{"Ski 2007/Alan's", "ski2007_alans"},
		{"Jim Snell's/Mark_Weiler 2011", "jimsnells_markweiler2011"},
		{"2009 - Whistler", "2009whistler"},
	}
	for _, tt := range tests {
		t.Run(tt.relDir, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizePrefix(tt.relDir))
		})
	}
}

func TestCollectPhotosRecursive(t *testing.T) {
	t.Parallel()

	// copyPhoto copies a testdata image into dir with the given output filename.
	copyPhoto := func(t *testing.T, dir, src, dst string) {
		t.Helper()
		in, err := os.Open(filepath.Join("testdata", src))
		require.NoError(t, err)
		defer in.Close()
		out, err := os.Create(filepath.Join(dir, dst))
		require.NoError(t, err)
		defer out.Close()
		_, err = io.Copy(out, in)
		require.NoError(t, err)
	}

	t.Run("no subfolders - no prefix", func(t *testing.T) {
		dir := t.TempDir()
		copyPhoto(t, dir, "landscape-1.jpg", "photo_a.jpg")
		copyPhoto(t, dir, "portrait-1.jpg", "photo_b.jpg")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(dir, "")
		require.NoError(t, err)

		require.Len(t, photos, 2)
		ids := map[string]bool{}
		for _, p := range photos {
			ids[p.ID] = true
			assert.Equal(t, p.ID+".jpg", p.FileName, "FileName should match ID with extension")
		}
		assert.True(t, ids["photo_a"], "photo_a should be present")
		assert.True(t, ids["photo_b"], "photo_b should be present")
	})

	t.Run("subfolder photos get prefixed ID and FileName", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "Craig's")
		require.NoError(t, os.Mkdir(sub, 0o755))
		copyPhoto(t, root, "landscape-1.jpg", "root.jpg")
		copyPhoto(t, sub, "portrait-1.jpg", "inner.jpg")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(root, "")
		require.NoError(t, err)

		require.Len(t, photos, 2)
		// root photo: no prefix
		rootPhoto := photos[0]
		assert.Equal(t, "root", rootPhoto.ID)
		assert.Equal(t, "root.jpg", rootPhoto.FileName)
		// subfolder photo: prefix from sanitized dir name
		subPhoto := photos[1]
		assert.Equal(t, "craigs_inner", subPhoto.ID)
		assert.Equal(t, "craigs_inner.jpg", subPhoto.FileName)
		assert.Equal(t, filepath.Join(sub, "inner.jpg"), subPhoto.AbsolutePath)
	})

	t.Run("nested subfolders accumulate prefix", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "Ski 2007", "Alan's")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		copyPhoto(t, nested, "portrait-1.jpg", "photo.jpg")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(root, "")
		require.NoError(t, err)

		require.Len(t, photos, 1)
		assert.Equal(t, "ski2007_alans_photo", photos[0].ID)
		assert.Equal(t, "ski2007_alans_photo.jpg", photos[0].FileName)
	})

	t.Run("subfolders sorted alphabetically by default", func(t *testing.T) {
		root := t.TempDir()
		for _, sd := range []string{"zebra", "alpha", "mango"} {
			require.NoError(t, os.Mkdir(filepath.Join(root, sd), 0o755))
			copyPhoto(t, filepath.Join(root, sd), "landscape-1.jpg", "photo.jpg")
		}

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(root, "")
		require.NoError(t, err)

		require.Len(t, photos, 3)
		assert.Equal(t, "alpha_photo", photos[0].ID)
		assert.Equal(t, "mango_photo", photos[1].ID)
		assert.Equal(t, "zebra_photo", photos[2].ID)
	})

	t.Run("photogen.txt captions applied with or without extension", func(t *testing.T) {
		dir := t.TempDir()
		copyPhoto(t, dir, "landscape-1.jpg", "photo_a.jpg")
		copyPhoto(t, dir, "portrait-1.jpg", "photo_b.jpg")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"),
			[]byte("photo_a.jpg First caption.\nphoto_b Second caption.\n"), 0o644))

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(dir, "")
		require.NoError(t, err)

		require.Len(t, photos, 2)
		byID := map[string]*Photo{}
		for _, p := range photos {
			byID[p.ID] = p
		}
		assert.Equal(t, "First caption.", byID["photo_a"].Description)
		assert.Equal(t, "Second caption.", byID["photo_b"].Description)
	})

	t.Run("manual order: subfolder expanded inline", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "Craig's")
		require.NoError(t, os.Mkdir(sub, 0o755))
		copyPhoto(t, root, "landscape-1.jpg", "root.jpg")
		copyPhoto(t, sub, "portrait-1.jpg", "inner.jpg")

		// Root photogen.txt puts Craig's subfolder before root photo
		require.NoError(t, os.WriteFile(filepath.Join(root, "photogen.txt"),
			[]byte("Craig's\nroot.jpg Root caption.\n"), 0o644))

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{ManualSortOrder: true}}
		photos, err := ap.collectPhotosRecursive(root, "")
		require.NoError(t, err)

		require.Len(t, photos, 2)
		assert.Equal(t, "craigs_inner", photos[0].ID, "subfolder photo should come first")
		assert.Equal(t, "root", photos[1].ID)
		assert.Equal(t, "Root caption.", photos[1].Description)
	})

	t.Run("manual order: unknown entry warns, unlisted photos appended", func(t *testing.T) {
		dir := t.TempDir()
		copyPhoto(t, dir, "landscape-1.jpg", "photo_a.jpg")
		copyPhoto(t, dir, "portrait-1.jpg", "photo_b.jpg")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"),
			[]byte("photo_a\nghost\n"), 0o644))

		wc := &WarnCollector{}
		ap := &AlbumProcessor{
			AlbumConfig: &AlbumConfig{ManualSortOrder: true},
			Config:      &Config{Warn: wc},
		}
		photos, err := ap.collectPhotosRecursive(dir, "")
		require.NoError(t, err)

		require.Len(t, photos, 2)
		assert.Equal(t, "photo_a", photos[0].ID)
		assert.Equal(t, "photo_b", photos[1].ID, "unlisted photo appended at end")
		assert.Len(t, wc.warnings, 2, "expect warning for ghost entry and unlisted photo_b")
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
