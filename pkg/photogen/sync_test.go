package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscapeSyncText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is unchanged", "So wide open!", "So wide open!"},
		{"newlines and tabs collapse to one space", "one\ntwo\tthree", "one two three"},
		{"whitespace runs collapse", "a    b", "a b"},
		{"leading and trailing space is trimmed", "  hi  ", "hi"},
		{"html is escaped", "<b>a & b</b>", "&lt;b&gt;a &amp; b&lt;/b&gt;"},
		// Ampersand first, or the escapes escape each other and &lt; becomes &amp;lt;.
		{"ampersand is escaped once", "a & <b>", "a &amp; &lt;b&gt;"},
		// Only a leading quote on the file name is significant to parsePhotogenLine, and
		// the description is the rest of the line, so quotes are left as typed.
		{"double quotes are left alone", `she said "hi"`, `she said "hi"`},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, escapeSyncText(tt.in))
		})
	}
}

func TestSanitizeSyncFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ordinary name survives", "IMG_1583.jpeg", "IMG_1583.jpeg"},
		{"extension is lowercased", "IMG_1583.JPEG", "IMG_1583.jpeg"},
		{"stem case is preserved", "Sunset At Dusk.JPG", "Sunset At Dusk.jpg"},
		{"windows-illegal characters are dropped", `a<b>c:d"e|f?g*h.jpg`, "abcdefgh.jpg"},
		{"path separators are dropped", "sub/dir/photo.jpg", "photo.jpg"},
		{"whitespace runs collapse", "a    b.jpg", "a b.jpg"},
		{"leading dot is trimmed", ".hidden.jpg", "hidden.jpg"},
		{"reserved windows name is suffixed", "CON.jpg", "CON_.jpg"},
		{"reserved name in any case is suffixed", "com1.jpg", "com1_.jpg"},
		{"a name that sanitizes to nothing gets a fallback", `<>:.jpg`, "photo.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, sanitizeSyncFileName(tt.in))
		})
	}

	t.Run("control characters are dropped", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "ab.jpg", sanitizeSyncFileName("a\x01b\x7f.jpg"))
	})
}

func TestAssignSyncFileNames(t *testing.T) {
	t.Parallel()

	t.Run("two assets with the same upstream name get distinct local names", func(t *testing.T) {
		t.Parallel()
		items := assignSyncFileNames([]SyncAsset{
			{ID: "b68fbc77-bbed-4008-a118-ba8637148081", FileName: "IMG_1234.jpg"},
			{ID: "c79ffd88-ccff-5119-b229-cb9748259192", FileName: "IMG_1234.jpg"},
		}, &SyncMetadata{})
		assert.Equal(t, "IMG_1234.jpg", items[0].file)
		assert.Equal(t, "IMG_1234-c79ffd88.jpg", items[1].file)
	})

	// The name is the photogen.txt key, the cover: value and the accessible label, so
	// moving it later would break all three. An upstream rename is therefore ignored.
	t.Run("a name already recorded is kept even when upstream renamed the asset", func(t *testing.T) {
		t.Parallel()
		prev := &SyncMetadata{Photos: []SyncPhotoMeta{
			{AssetID: "a1", File: "old-name.jpg", OriginalFileName: "old-name.jpg"},
		}}
		items := assignSyncFileNames([]SyncAsset{{ID: "a1", FileName: "brand-new-name.jpg"}}, prev)
		assert.Equal(t, "old-name.jpg", items[0].file)
	})

	// Reserving recorded names in a pass of their own is what stops this: otherwise the
	// new asset would be handed "shared.jpg" before the existing one reclaimed it.
	t.Run("a new asset cannot take a name an existing asset still holds", func(t *testing.T) {
		t.Parallel()
		prev := &SyncMetadata{Photos: []SyncPhotoMeta{{AssetID: "a2", File: "shared.jpg"}}}
		items := assignSyncFileNames([]SyncAsset{
			{ID: "newasset-1111", FileName: "shared.jpg"},
			{ID: "a2", FileName: "shared.jpg"},
		}, prev)
		assert.Equal(t, "shared-newasset.jpg", items[0].file)
		assert.Equal(t, "shared.jpg", items[1].file)
	})

	t.Run("captions are escaped on the way in", func(t *testing.T) {
		t.Parallel()
		items := assignSyncFileNames([]SyncAsset{
			{ID: "a1", FileName: "a.jpg", Caption: "two\nlines & <b>bold</b>"},
		}, &SyncMetadata{})
		assert.Equal(t, "two lines &amp; &lt;b&gt;bold&lt;/b&gt;", items[0].caption)
	})
}

func TestFilterSyncAssets(t *testing.T) {
	t.Parallel()

	collect := func() (func(string, ...any), *[]string) {
		var got []string
		return func(format string, args ...any) {
			got = append(got, strings.TrimSpace(fmt.Sprintf(format, args...)))
		}, &got
	}

	t.Run("unsupported extensions are skipped with a warning", func(t *testing.T) {
		t.Parallel()
		warnf, got := collect()
		kept := filterSyncAssets([]SyncAsset{
			{ID: "1", FileName: "keep.jpg"},
			{ID: "2", FileName: "raw.arw"},
		}, warnf)
		require.Len(t, kept, 1)
		assert.Equal(t, "keep.jpg", kept[0].FileName)
		require.Len(t, *got, 1)
		assert.Contains(t, (*got)[0], "raw.arw")
		assert.Contains(t, (*got)[0], ".arw")
	})

	t.Run("provider warnings are surfaced without dropping the asset", func(t *testing.T) {
		t.Parallel()
		warnf, got := collect()
		kept := filterSyncAssets([]SyncAsset{
			{ID: "1", FileName: "edited.jpg", Warnings: []string{"edited upstream"}},
		}, warnf)
		assert.Len(t, kept, 1)
		require.Len(t, *got, 1)
		assert.Contains(t, (*got)[0], "edited upstream")
	})

	// checkDuplicateIDs is a hard error: both files reduce to the ID "img_1234", so
	// without this guard the whole run fails on an ordinary Apple Live Photo pair.
	t.Run("a video sharing a base name with a photo is skipped", func(t *testing.T) {
		t.Parallel()
		warnf, got := collect()
		kept := filterSyncAssets([]SyncAsset{
			{ID: "1", FileName: "IMG_1234.heic"},
			{ID: "2", FileName: "IMG_1234.mov", IsVideo: true},
			{ID: "3", FileName: "clip.mov", IsVideo: true},
		}, warnf)
		require.Len(t, kept, 2)
		assert.Equal(t, "IMG_1234.heic", kept[0].FileName)
		assert.Equal(t, "clip.mov", kept[1].FileName, "an unpaired video is kept")
		require.Len(t, *got, 1)
		assert.Contains(t, (*got)[0], "IMG_1234.mov")
	})

	t.Run("a video with no matching photo is kept", func(t *testing.T) {
		t.Parallel()
		warnf, got := collect()
		kept := filterSyncAssets([]SyncAsset{{ID: "1", FileName: "clip.mov", IsVideo: true}}, warnf)
		assert.Len(t, kept, 1)
		assert.Empty(t, *got)
	})
}

func TestHaveSyncAsset(t *testing.T) {
	t.Parallel()

	withFile := func(t *testing.T, name string, size int) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o644))
		return dir
	}

	t.Run("no previous record means fetch it", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 10)
		assert.False(t, haveSyncAsset(dir, syncItem{file: "a.jpg", asset: SyncAsset{Checksum: "x"}}))
	})

	t.Run("a recorded asset with no local file is fetched", func(t *testing.T) {
		t.Parallel()
		it := syncItem{file: "a.jpg", asset: SyncAsset{Checksum: "x"}, prev: &SyncPhotoMeta{Checksum: "x"}}
		assert.False(t, haveSyncAsset(t.TempDir(), it))
	})

	t.Run("matching checksum is enough", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 10)
		it := syncItem{file: "a.jpg", asset: SyncAsset{Checksum: "x"}, prev: &SyncPhotoMeta{Checksum: "x"}}
		assert.True(t, haveSyncAsset(dir, it))
	})

	t.Run("a changed checksum is re-fetched", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 10)
		it := syncItem{file: "a.jpg", asset: SyncAsset{Checksum: "y"}, prev: &SyncPhotoMeta{Checksum: "x"}}
		assert.False(t, haveSyncAsset(dir, it))
	})

	t.Run("size is the fallback when there is no checksum", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 10)
		it := syncItem{file: "a.jpg", asset: SyncAsset{Size: 10}, prev: &SyncPhotoMeta{Size: 10}}
		assert.True(t, haveSyncAsset(dir, it))
	})

	// The record still says 10, but someone replaced the file. Trusting the record alone
	// would publish whatever is now on disk forever.
	t.Run("a locally clobbered file is re-fetched even when the record matches", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 3)
		it := syncItem{file: "a.jpg", asset: SyncAsset{Size: 10}, prev: &SyncPhotoMeta{Size: 10}}
		assert.False(t, haveSyncAsset(dir, it))
	})

	t.Run("updated_at is the last fallback", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 10)
		when := time.Date(2026, 9, 18, 22, 43, 56, 0, time.UTC)
		it := syncItem{file: "a.jpg", asset: SyncAsset{UpdatedAt: when}, prev: &SyncPhotoMeta{UpdatedAt: when}}
		assert.True(t, haveSyncAsset(dir, it))

		it.asset.UpdatedAt = when.Add(time.Hour)
		assert.False(t, haveSyncAsset(dir, it))
	})

	// A provider that knows nothing about its own assets leaves presence as the only
	// signal. Re-fetching every run instead would rewrite every mtime and make photogen
	// re-decode the whole album each time.
	t.Run("with nothing to compare, having the file is enough", func(t *testing.T) {
		t.Parallel()
		dir := withFile(t, "a.jpg", 10)
		it := syncItem{file: "a.jpg", asset: SyncAsset{}, prev: &SyncPhotoMeta{}}
		assert.True(t, haveSyncAsset(dir, it))
	})
}

func TestSyncAlbumPath(t *testing.T) {
	t.Parallel()
	// Site-id namespaced, like albums/{site-id}/: two configs sharing a root and a slug
	// would otherwise share a folder and prune each other's photos.
	assert.Equal(t, filepath.Join("sync", "my-site", "immich", "galapagos"),
		SyncAlbumPath("sync", "my-site", "immich", "galapagos"))
}

func TestCreateSyncDirs(t *testing.T) {
	t.Parallel()

	af := &AlbumsFile{Albums: []AlbumEntry{
		{Slug: "local", Name: "Local", Source: "photos"},
		{Slug: "synced", Sync: &SyncEntry{Provider: mockProviderName, AlbumID: "x"}},
	}}

	t.Run("creates a folder for synced albums only", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		require.NoError(t, af.CreateSyncDirs(&SyncPaths{Root: root, SiteID: "s"}))
		assert.DirExists(t, filepath.Join(root, "s", mockProviderName, "synced"))
		assert.NoDirExists(t, filepath.Join(root, "s", mockProviderName, "local"))
	})

	// The folders are created before Config.Validate runs, so the site ID has to be checked
	// here: an empty one drops the site level, and one with .. escapes the sync root.
	t.Run("an invalid site ID is an error and creates nothing", func(t *testing.T) {
		t.Parallel()
		for _, siteID := range []string{"", "../other", "Bad_ID"} {
			parent := t.TempDir()
			root := filepath.Join(parent, "sync")
			require.NoError(t, os.Mkdir(root, 0o755))

			err := af.CreateSyncDirs(&SyncPaths{Root: root, SiteID: siteID})
			require.Error(t, err, "site ID %q", siteID)
			assert.Contains(t, err.Error(), "settings.id")
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			assert.Empty(t, entries, "nothing may be created under the root for site ID %q", siteID)
			assert.NoDirExists(t, filepath.Join(parent, "other"), "nothing may escape the root")
		}
	})

	t.Run("a synced album with no sync directory is an error naming the flag", func(t *testing.T) {
		t.Parallel()
		err := af.CreateSyncDirs(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-sync-dir")
	})

	t.Run("a file with no synced albums needs no sync directory", func(t *testing.T) {
		t.Parallel()
		local := &AlbumsFile{Albums: []AlbumEntry{{Slug: "local", Name: "Local", Source: "photos"}}}
		assert.NoError(t, local.CreateSyncDirs(nil))
		assert.False(t, local.HasSyncedAlbums())
	})
}
