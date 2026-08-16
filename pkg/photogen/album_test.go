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

func TestLoadPhotoDescriptionsQuotedNames(t *testing.T) {
	t.Parallel()

	t.Run("quoted names keep their spaces", func(t *testing.T) {
		dir := t.TempDir()
		content := `
"Doug and Cindy Chicago.jpg" A cool trip to Chicago
doug-and-cindy-chicago.jpg A cool trip to Chicago
"Doug at the Bean.jpg" Under the Bean
"Doug alone.jpg"
"Ski 2007"
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"), []byte(content), 0o644))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)

		// Photos sharing a first word stay separate entries.
		assert.Equal(t, []string{
			"doug and cindy chicago",
			"doug-and-cindy-chicago",
			"doug at the bean",
			"doug alone",
			"ski 2007",
		}, pd.order)
		assert.Equal(t, "A cool trip to Chicago", pd.descriptions["doug and cindy chicago"])
		assert.Equal(t, "A cool trip to Chicago", pd.descriptions["doug-and-cindy-chicago"])
		assert.Equal(t, "Under the Bean", pd.descriptions["doug at the bean"])
		assert.Equal(t, "", pd.descriptions["doug alone"], "quoted name with no description")
		assert.Equal(t, "", pd.descriptions["ski 2007"], "quoted subfolder name")
	})

	t.Run("unterminated quote falls back to first-space split", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"),
			[]byte("\"img_0001.jpg A caption.\n"), 0o644))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)

		assert.Equal(t, []string{`"img_0001`}, pd.order)
		assert.Equal(t, "A caption.", pd.descriptions[`"img_0001`])
	})
}

func TestParsePhotogenLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line     string
		wantName string
		wantDesc string
	}{
		{"img_0001.jpg A caption.", "img_0001.jpg", "A caption."},
		{"img_0001.jpg", "img_0001.jpg", ""},
		{`"Doug and Cindy Chicago.jpg" A cool trip to Chicago`, "Doug and Cindy Chicago.jpg", "A cool trip to Chicago"},
		{`"Doug and Cindy Chicago.jpg"`, "Doug and Cindy Chicago.jpg", ""},
		{`"Ski 2007"`, "Ski 2007", ""},
		{`img_0001.jpg He said "hi" loudly`, "img_0001.jpg", `He said "hi" loudly`},
		{`"unterminated.jpg A caption.`, `"unterminated.jpg`, "A caption."},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			name, desc := parsePhotogenLine(tt.line)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantDesc, desc)
		})
	}
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
		photos, err := ap.collectPhotosRecursive(dir, "", true)
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
		photos, err := ap.collectPhotosRecursive(root, "", true)
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
		assert.Equal(t, "Craig's/inner.jpg", subPhoto.SourcePath)
		assert.Equal(t, "root.jpg", rootPhoto.SourcePath)
	})

	t.Run("nested subfolders accumulate prefix", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "Ski 2007", "Alan's")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		copyPhoto(t, nested, "portrait-1.jpg", "photo.jpg")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(root, "", true)
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
		photos, err := ap.collectPhotosRecursive(root, "", true)
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
		photos, err := ap.collectPhotosRecursive(dir, "", true)
		require.NoError(t, err)

		require.Len(t, photos, 2)
		byID := map[string]*Photo{}
		for _, p := range photos {
			byID[p.ID] = p
		}
		assert.Equal(t, "First caption.", byID["photo_a"].Description)
		assert.Equal(t, "Second caption.", byID["photo_b"].Description)
	})

	t.Run("photogen.txt captions applied to quoted names with spaces", func(t *testing.T) {
		dir := t.TempDir()
		copyPhoto(t, dir, "landscape-1.jpg", "Doug and Cindy Chicago.jpg")
		copyPhoto(t, dir, "portrait-1.jpg", "Doug at the Bean.jpg")
		content := "\"Doug and Cindy Chicago.jpg\" A cool trip to Chicago\n" +
			"\"Doug at the Bean.jpg\" Under the Bean\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photogen.txt"), []byte(content), 0o644))

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(dir, "", true)
		require.NoError(t, err)

		require.Len(t, photos, 2)
		byID := map[string]*Photo{}
		for _, p := range photos {
			byID[p.ID] = p
		}
		// Photos sharing a first word each get their own caption.
		require.Contains(t, byID, "doug and cindy chicago")
		require.Contains(t, byID, "doug at the bean")
		assert.Equal(t, "A cool trip to Chicago", byID["doug and cindy chicago"].Description)
		assert.Equal(t, "Under the Bean", byID["doug at the bean"].Description)
	})

	t.Run("manual order: quoted names order photos and subfolders", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "Ski 2007")
		require.NoError(t, os.Mkdir(sub, 0o755))
		copyPhoto(t, root, "landscape-1.jpg", "Doug and Cindy Chicago.jpg")
		copyPhoto(t, root, "portrait-1.jpg", "Doug at the Bean.jpg")
		copyPhoto(t, sub, "landscape-1.jpg", "inner.jpg")

		content := "\"Doug at the Bean.jpg\" Under the Bean\n" +
			"\"Ski 2007\"\n" +
			"\"Doug and Cindy Chicago.jpg\" A cool trip to Chicago\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, "photogen.txt"), []byte(content), 0o644))

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{ManualSortOrder: true}}
		photos, err := ap.collectPhotosRecursive(root, "", true)
		require.NoError(t, err)

		require.Len(t, photos, 3)
		assert.Equal(t, "doug at the bean", photos[0].ID)
		assert.Equal(t, "ski2007_inner", photos[1].ID, "quoted subfolder name should expand inline")
		assert.Equal(t, "doug and cindy chicago", photos[2].ID)
		assert.Equal(t, "Under the Bean", photos[0].Description)
		assert.Equal(t, "A cool trip to Chicago", photos[2].Description)
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
		photos, err := ap.collectPhotosRecursive(root, "", true)
		require.NoError(t, err)

		require.Len(t, photos, 2)
		assert.Equal(t, "craigs_inner", photos[0].ID, "subfolder photo should come first")
		assert.Equal(t, "root", photos[1].ID)
		assert.Equal(t, "Root caption.", photos[1].Description)
	})

	t.Run("undated photos sort to end in scan order", func(t *testing.T) {
		dir := t.TempDir()
		// Scan order: no-date-1, dated-early, no-date-2, dated-late
		copyPhoto(t, dir, "no-exif.jpg", "no-date-1.jpg")
		copyPhoto(t, dir, "landscape-1.jpg", "dated-early.jpg") // 2024-05-16
		copyPhoto(t, dir, "no-exif.jpg", "no-date-2.jpg")
		copyPhoto(t, dir, "portrait-1.jpg", "dated-late.jpg") // 2024-05-31

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(dir, "", false)
		require.NoError(t, err)

		require.Len(t, photos, 4)
		// Dated photos first, sorted by date
		assert.Equal(t, "dated-early", photos[0].ID)
		assert.Equal(t, "dated-late", photos[1].ID)
		// Undated photos at end, in original scan order
		assert.Equal(t, "no-date-1", photos[2].ID)
		assert.Equal(t, "no-date-2", photos[3].ID)
	})

	t.Run("recurse=false: subfolders ignored", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "subdir")
		require.NoError(t, os.Mkdir(sub, 0o755))
		copyPhoto(t, root, "landscape-1.jpg", "root.jpg")
		copyPhoto(t, sub, "portrait-1.jpg", "inner.jpg")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(root, "", false)
		require.NoError(t, err)

		require.Len(t, photos, 1)
		assert.Equal(t, "root", photos[0].ID, "only root-level photo should be returned")
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
		photos, err := ap.collectPhotosRecursive(dir, "", true)
		require.NoError(t, err)

		require.Len(t, photos, 2)
		assert.Equal(t, "photo_a", photos[0].ID)
		assert.Equal(t, "photo_b", photos[1].ID, "unlisted photo appended at end")
		assert.Len(t, wc.warnings, 2, "expect warning for ghost entry and unlisted photo_b")
	})
}

func TestSortByDate(t *testing.T) {
	t.Parallel()

	day := func(d int) time.Time {
		return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC)
	}

	t.Run("dated photos sorted ascending", func(t *testing.T) {
		photos := []*Photo{
			{ID: "c", PhotoMetadata: &PhotoMetadata{DateTaken: day(3)}},
			{ID: "a", PhotoMetadata: &PhotoMetadata{DateTaken: day(1)}},
			{ID: "b", PhotoMetadata: &PhotoMetadata{DateTaken: day(2)}},
		}
		sortByDate(photos)
		assert.Equal(t, "a", photos[0].ID)
		assert.Equal(t, "b", photos[1].ID)
		assert.Equal(t, "c", photos[2].ID)
	})

	t.Run("undated photos sort to end", func(t *testing.T) {
		photos := []*Photo{
			{ID: "no-date-1", PhotoMetadata: &PhotoMetadata{}},
			{ID: "dated", PhotoMetadata: &PhotoMetadata{DateTaken: day(1)}},
			{ID: "no-date-2", PhotoMetadata: &PhotoMetadata{}},
		}
		sortByDate(photos)
		assert.Equal(t, "dated", photos[0].ID)
		assert.Equal(t, "no-date-1", photos[1].ID, "undated preserve scan order")
		assert.Equal(t, "no-date-2", photos[2].ID, "undated preserve scan order")
	})

	t.Run("all undated preserves scan order", func(t *testing.T) {
		photos := []*Photo{
			{ID: "first", PhotoMetadata: &PhotoMetadata{}},
			{ID: "second", PhotoMetadata: &PhotoMetadata{}},
			{ID: "third", PhotoMetadata: &PhotoMetadata{}},
		}
		sortByDate(photos)
		assert.Equal(t, "first", photos[0].ID)
		assert.Equal(t, "second", photos[1].ID)
		assert.Equal(t, "third", photos[2].ID)
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

	t.Run("unmentioned undated photos appended after dated extras in scan order", func(t *testing.T) {
		mixed := []*Photo{
			{ID: "no-date-a", FileName: "no-date-a.jpg", PhotoMetadata: &PhotoMetadata{}},
			{ID: "img_0002", FileName: "IMG_0002.jpg", PhotoMetadata: &PhotoMetadata{DateTaken: day(2)}},
			{ID: "no-date-b", FileName: "no-date-b.jpg", PhotoMetadata: &PhotoMetadata{}},
			{ID: "img_0004", FileName: "IMG_0004.jpg", PhotoMetadata: &PhotoMetadata{DateTaken: day(4)}},
		}
		order := []string{"img_0002"} // only mention one; rest are extras
		result := ap.reorderByDescriptionFile(mixed, order)
		require.Len(t, result, 4)
		assert.Equal(t, "img_0002", result[0].ID)
		assert.Equal(t, "img_0004", result[1].ID, "dated extra sorted by date")
		assert.Equal(t, "no-date-a", result[2].ID, "undated extras in scan order")
		assert.Equal(t, "no-date-b", result[3].ID, "undated extras in scan order")
	})
}

// TestDuplicatePhotoIDs covers the collision that Apple Photos makes routine: a Live Photo
// exports as IMG_1234.HEIC plus IMG_1234.MOV, and stripping the extension leaves both with
// the same ID. Before this check the pair silently produced one output file, and with a
// photogen.txt one entry replaced the other so the album published the same item twice.
func TestDuplicatePhotoIDs(t *testing.T) {
	t.Parallel()

	copyFile := func(t *testing.T, dir, src, dst string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", src))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, dst), b, 0o644))
	}

	t.Run("still and video sharing a stem is rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		copyFile(t, dir, "landscape-1.jpg", "IMG_1234.jpg")
		copyFile(t, dir, "landscape.mov", "IMG_1234.mov")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		_, err := ap.collectPhotosRecursive(dir, "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate photo ID")
		// Both offending files must be named, or the user cannot act on the message.
		assert.Contains(t, err.Error(), "IMG_1234.jpg")
		assert.Contains(t, err.Error(), "IMG_1234.mov")
		assert.Contains(t, err.Error(), "img_1234")
	})

	t.Run("two stills sharing a stem is rejected too", func(t *testing.T) {
		t.Parallel()
		// Pre-dates video: the same collision was already possible between two image
		// formats, and was equally broken. The rule is about IDs, not about media kind.
		dir := t.TempDir()
		copyFile(t, dir, "landscape-1.jpg", "IMG_1234.jpg")
		copyFile(t, dir, "landscape-1.heic", "IMG_1234.heic")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		_, err := ap.collectPhotosRecursive(dir, "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate photo ID")
	})

	t.Run("case differences collide, matching how IDs are lowercased", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		copyFile(t, dir, "landscape-1.jpg", "photo.jpg")
		copyFile(t, dir, "portrait-1.jpg", "PHOTO.JPG")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		_, err := ap.collectPhotosRecursive(dir, "", true)
		// A case-insensitive filesystem (macOS) cannot hold both names, so the second
		// write lands on the first file and there is nothing to collide. Assert the
		// outcome that matches what the filesystem actually did rather than assuming.
		if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) == 1 {
			assert.NoError(t, err, "only one file exists on a case-insensitive filesystem")
			return
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate photo ID")
	})

	t.Run("a subfolder prefix colliding with a root file is rejected", func(t *testing.T) {
		t.Parallel()
		// Not visible to the per-directory check: "sub/photo.jpg" becomes "sub_photo" only
		// after prefixing, which is why LoadPhotos re-checks the assembled list.
		dir := t.TempDir()
		copyFile(t, dir, "landscape-1.jpg", "sub_photo.jpg")
		sub := filepath.Join(dir, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		copyFile(t, sub, "portrait-1.jpg", "photo.jpg")

		ap := NewAlbumProcessor(
			&Config{OutputRoot: t.TempDir(), SiteID: "s", Warn: &WarnCollector{}},
			&AlbumConfig{Slug: "a", Name: "A", Path: dir, Recurse: true},
		)
		err := ap.LoadPhotos()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate photo ID")
		assert.Contains(t, err.Error(), "sub_photo")
	})

	t.Run("distinct stems are unaffected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		copyFile(t, dir, "landscape-1.jpg", "one.jpg")
		copyFile(t, dir, "portrait-1.jpg", "two.jpg")

		ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{}}
		photos, err := ap.collectPhotosRecursive(dir, "", true)
		require.NoError(t, err)
		assert.Len(t, photos, 2)
	})
}
