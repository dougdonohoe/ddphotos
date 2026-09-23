package photogen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncFixture is a mock upstream: a listing written to disk and a media folder the
// provider reads bytes from. Everything a real provider would do over the network is done
// against these two, so the whole sync run — transfer, skip, prune, merge — is exercised.
type syncFixture struct {
	t        *testing.T
	dir      string // the sync album folder, i.e. AlbumConfig.Path
	mediaDir string
	listing  mockFixture
	fail     string
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	root := t.TempDir()
	f := &syncFixture{
		t:        t,
		dir:      filepath.Join(root, "album"),
		mediaDir: filepath.Join(root, "media"),
	}
	require.NoError(t, os.MkdirAll(f.dir, 0o755))
	require.NoError(t, os.MkdirAll(f.mediaDir, 0o755))
	f.listing.Album.Name = "Upstream Name"
	f.listing.Album.Description = "Upstream <description> & more"
	return f
}

// add puts one asset in the listing and its bytes in the media folder.
func (f *syncFixture) add(id, name, caption string, body string) *syncFixture {
	f.t.Helper()
	require.NoError(f.t, os.WriteFile(filepath.Join(f.mediaDir, name), []byte(body), 0o644))
	f.listing.Assets = append(f.listing.Assets, mockAsset{
		ID: id, FileName: name, Caption: caption, Checksum: "sum-" + id,
	})
	return f
}

// setCaption changes what upstream says about an asset.
func (f *syncFixture) setCaption(name, caption string) {
	f.t.Helper()
	for i := range f.listing.Assets {
		if f.listing.Assets[i].FileName == name {
			f.listing.Assets[i].Caption = caption
			return
		}
	}
	f.t.Fatalf("no asset named %q in the fixture", name)
}

// remove drops an asset from the listing, the way deleting it upstream would.
func (f *syncFixture) remove(name string) {
	f.t.Helper()
	kept := f.listing.Assets[:0]
	for _, a := range f.listing.Assets {
		if a.FileName != name {
			kept = append(kept, a)
		}
	}
	f.listing.Assets = kept
}

// sync writes the listing out and runs one sync, returning the error and the warnings.
func (f *syncFixture) sync() (error, []string) { //nolint:revive
	f.t.Helper()
	data, err := json.Marshal(f.listing)
	require.NoError(f.t, err)
	assetsPath := filepath.Join(filepath.Dir(f.dir), "assets.json")
	require.NoError(f.t, os.WriteFile(assetsPath, data, 0o644))

	wc := &WarnCollector{}
	ac := &AlbumConfig{
		Slug: "album",
		Path: f.dir,
		Sync: &AlbumSyncConfig{
			Provider: mockProviderName,
			AlbumID:  "album",
			Captions: true,
			Mock:     &MockSyncConfig{AssetsPath: assetsPath, MediaDir: f.mediaDir, Fail: f.fail},
		},
	}
	return syncOneAlbum(context.Background(), &Config{Warn: wc}, ac, 1, 1), wc.warnings
}

// names lists the folder's file names.
func (f *syncFixture) names() []string {
	f.t.Helper()
	entries, err := os.ReadDir(f.dir)
	require.NoError(f.t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func (f *syncFixture) read(name string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, name))
	require.NoError(f.t, err)
	return string(data)
}

func (f *syncFixture) mtimes() map[string]int64 {
	f.t.Helper()
	out := map[string]int64{}
	for _, n := range f.names() {
		st, err := os.Stat(filepath.Join(f.dir, n))
		require.NoError(f.t, err)
		out[n] = st.ModTime().UnixNano()
	}
	return out
}

func TestSyncOneAlbum(t *testing.T) {
	t.Parallel()

	t.Run("a first sync writes the media, the record and the captions", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "First", "one-bytes").add("a2", "two.jpg", "", "two-bytes")

		err, warnings := f.sync()
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.ElementsMatch(t,
			[]string{"one.jpg", "two.jpg", SyncMetadataFileName, photogenFileName}, f.names())
		assert.Equal(t, "one-bytes", f.read("one.jpg"))
		assert.Equal(t, "one.jpg First\ntwo.jpg\n", f.read(photogenFileName))

		meta, err := loadSyncMetadata(f.dir)
		require.NoError(t, err)
		assert.Equal(t, "Upstream Name", meta.Name)
		assert.Equal(t, "Upstream &lt;description&gt; &amp; more", meta.Description)
		require.Len(t, meta.Photos, 2)
		assert.Equal(t, "one.jpg", meta.Photos[0].File)
		assert.False(t, meta.SyncedAt.IsZero())
	})

	// The metadata cache keys on path + mtime + size, so a needless re-download would make
	// photogen re-decode every photo in the album on the next build.
	t.Run("running twice downloads nothing and touches no mtime", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "First", "one-bytes")
		err, _ := f.sync()
		require.NoError(t, err)
		before := f.mtimes()

		err, _ = f.sync()
		require.NoError(t, err)
		assert.Equal(t, before["one.jpg"], f.mtimes()["one.jpg"])
	})

	t.Run("an asset removed upstream is pruned, along with its caption line", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "First", "one").add("a2", "two.jpg", "Second", "two")
		err, _ := f.sync()
		require.NoError(t, err)

		f.remove("two.jpg")
		err, _ = f.sync()
		require.NoError(t, err)
		assert.ElementsMatch(t,
			[]string{"one.jpg", SyncMetadataFileName, photogenFileName}, f.names())
		assert.Equal(t, "one.jpg First\n", f.read(photogenFileName))
	})

	// Pruning against a listing that never completed would delete photos the upstream
	// still has, which is the one failure in this whole feature that loses data.
	t.Run("a failed listing prunes nothing and writes no record", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "First", "one").add("a2", "two.jpg", "Second", "two")
		err, _ := f.sync()
		require.NoError(t, err)
		metaBefore := f.read(SyncMetadataFileName)

		f.remove("two.jpg")
		f.fail = "list"
		err, _ = f.sync()
		require.Error(t, err)
		assert.ErrorIs(t, err, errMockFail)

		assert.ElementsMatch(t,
			[]string{"one.jpg", "two.jpg", SyncMetadataFileName, photogenFileName}, f.names())
		assert.Equal(t, metaBefore, f.read(SyncMetadataFileName), "the record was rewritten")
	})

	t.Run("a failed download fails the album and leaves no partial file", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "First", "one")
		f.fail = "fetch"

		err, _ := f.sync()
		require.Error(t, err)
		assert.ErrorIs(t, err, errMockFail)
		assert.Empty(t, f.names(), "a partial download was left behind")
	})

	t.Run("a stray file in the folder is pruned", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "First", "one")
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, "one.jpg.tmp123"), []byte("junk"), 0o644))

		err, _ := f.sync()
		require.NoError(t, err)
		assert.NotContains(t, f.names(), "one.jpg.tmp123")
	})

	t.Run("a caption edited locally survives a re-sync", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "Upstream", "one")
		err, _ := f.sync()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(f.dir, photogenFileName),
			[]byte("one.jpg My own words\n"), 0o644))
		err, warnings := f.sync()
		require.NoError(t, err)
		assert.Equal(t, "one.jpg My own words\n", f.read(photogenFileName))
		assert.Empty(t, warnings)
	})

	t.Run("a caption changed in both places warns and takes the upstream text", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "Upstream", "one")
		err, _ := f.sync()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(f.dir, photogenFileName),
			[]byte("one.jpg My own words\n"), 0o644))
		f.setCaption("one.jpg", "Changed upstream")
		err, warnings := f.sync()
		require.NoError(t, err)
		assert.Equal(t, "one.jpg Changed upstream\n", f.read(photogenFileName))
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "one.jpg")
	})

	t.Run("over the photo limit is an error naming the album and the cap", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		for i := range maxAlbumPhotos + 1 {
			f.listing.Assets = append(f.listing.Assets, mockAsset{
				ID: string(rune('a' + i%26)), FileName: "p.jpg", Checksum: "c",
			})
		}
		require.NoError(t, os.WriteFile(filepath.Join(f.mediaDir, "p.jpg"), []byte("x"), 0o644))

		err, _ := f.sync()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "album")
		assert.Contains(t, err.Error(), "501")
		assert.Empty(t, f.names(), "nothing was downloaded past the limit")
	})

	t.Run("captions: false leaves photogen.txt alone", func(t *testing.T) {
		t.Parallel()
		f := newSyncFixture(t)
		f.add("a1", "one.jpg", "Upstream", "one")

		data, err := json.Marshal(f.listing)
		require.NoError(t, err)
		assetsPath := filepath.Join(filepath.Dir(f.dir), "assets.json")
		require.NoError(t, os.WriteFile(assetsPath, data, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, photogenFileName),
			[]byte("one.jpg Mine forever\n"), 0o644))

		ac := &AlbumConfig{Slug: "album", Path: f.dir, Sync: &AlbumSyncConfig{
			Provider: mockProviderName, AlbumID: "album", Captions: false,
			Mock: &MockSyncConfig{AssetsPath: assetsPath, MediaDir: f.mediaDir},
		}}
		require.NoError(t, syncOneAlbum(context.Background(), &Config{Warn: &WarnCollector{}}, ac, 1, 1))
		assert.Equal(t, "one.jpg Mine forever\n", f.read(photogenFileName))
	})
}

func TestApplySyncMetadata(t *testing.T) {
	t.Parallel()

	seed := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, writeSyncMetadata(dir, &SyncMetadata{
			Name: "Upstream Name", Description: "Upstream description",
		}))
		return dir
	}

	t.Run("an album with no name or description takes both from the record", func(t *testing.T) {
		t.Parallel()
		ac := &AlbumConfig{Slug: "album", Path: seed(t), Sync: &AlbumSyncConfig{}}
		require.NoError(t, applySyncMetadata(ac))
		assert.Equal(t, "Upstream Name", ac.Name)
		assert.Equal(t, "Upstream description", ac.Description)
	})

	// ToAlbumConfigs has already resolved an inline description: over the descriptions
	// file, so a non-empty value here means one of those two won and upstream must not.
	t.Run("configured values win over the record", func(t *testing.T) {
		t.Parallel()
		ac := &AlbumConfig{Slug: "album", Path: seed(t), Name: "Mine", Description: "Also mine",
			Sync: &AlbumSyncConfig{}}
		require.NoError(t, applySyncMetadata(ac))
		assert.Equal(t, "Mine", ac.Name)
		assert.Equal(t, "Also mine", ac.Description)
	})

	t.Run("with no record at all the name falls back to the slug", func(t *testing.T) {
		t.Parallel()
		ac := &AlbumConfig{Slug: "galapagos", Path: t.TempDir(), Sync: &AlbumSyncConfig{}}
		require.NoError(t, applySyncMetadata(ac))
		assert.Equal(t, "galapagos", ac.Name)
		assert.Empty(t, ac.Description)
	})

	t.Run("a local album is left alone", func(t *testing.T) {
		t.Parallel()
		ac := &AlbumConfig{Slug: "local", Path: t.TempDir()}
		require.NoError(t, applySyncMetadata(ac))
		assert.Empty(t, ac.Name)
	})
}

func TestRunSync(t *testing.T) {
	t.Parallel()

	// -no-sync still has to fill in the name, or a build from disk shows the slug where
	// the upstream album name belongs.
	t.Run("-no-sync skips the download but still applies the recorded name", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, writeSyncMetadata(dir, &SyncMetadata{Name: "Recorded Name"}))
		ac := &AlbumConfig{Slug: "album", Path: dir, Sync: &AlbumSyncConfig{
			Provider: mockProviderName, AlbumID: "album",
			Mock: &MockSyncConfig{AssetsPath: "does-not-exist.json", MediaDir: dir},
		}}
		require.NoError(t, RunSync(context.Background(), &Config{Warn: &WarnCollector{}},
			[]*AlbumConfig{ac}, true))
		assert.Equal(t, "Recorded Name", ac.Name)
	})

	t.Run("a run with no synced albums does nothing", func(t *testing.T) {
		t.Parallel()
		ac := &AlbumConfig{Slug: "local", Name: "Local", Path: t.TempDir()}
		require.NoError(t, RunSync(context.Background(), &Config{Warn: &WarnCollector{}},
			[]*AlbumConfig{ac}, false))
		assert.Equal(t, "Local", ac.Name)
	})

	// The whole chain with the real provider: credentials off disk, Immich's own filtering,
	// then the shared filtering, naming, download, caption merge and prune on top of it.
	t.Run("a full run against an immich instance", func(t *testing.T) {
		t.Parallel()

		caption := "A <b>bold</b> caption & an ampersand"
		assets := []map[string]any{
			{"id": "a1", "originalFileName": "IMG_1.jpg", "checksum": "sum-1", "type": "IMAGE",
				"visibility": immichVisibilityTimeline,
				"exifInfo":   map[string]any{"description": caption, "fileSizeInByte": 5}},
			// Immich's own rule: hidden never publishes.
			{"id": "a2", "originalFileName": "IMG_2.jpg", "checksum": "sum-2", "type": "IMAGE",
				"visibility": "hidden"},
			// The shared layer's rules, which the provider must not duplicate: RAW is
			// unsupported, and a video sharing a photo's base name would collide on the
			// photo ID photogen derives from the file name.
			{"id": "a3", "originalFileName": "IMG_3.arw", "checksum": "sum-3", "type": "IMAGE",
				"visibility": immichVisibilityTimeline},
			{"id": "a4", "originalFileName": "IMG_1.mov", "checksum": "sum-4", "type": "VIDEO",
				"visibility": immichVisibilityTimeline},
		}
		page, err := json.Marshal(map[string]any{
			"assets": map[string]any{"items": assets, "nextPage": nil},
		})
		require.NoError(t, err)

		s := &immichServer{t: t,
			album: []byte(`{"albumName":"Upstream Album","description":"Ice & <snow>"}`),
			pages: [][]byte{page},
			media: map[string]string{"a1": "bytes"},
		}
		srv := s.start()

		// Through the env file rather than the environment, so this stays parallel and the
		// file half of the credential rule is what gets exercised.
		configDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(configDir, ImmichEnvFileName),
			[]byte("IMMICH_API_KEY=test-key\nIMMICH_INSTANCE_URL="+srv.URL+"\n"), 0o644))

		dir := t.TempDir()
		// A file from an earlier sync that the album no longer has, to prove the prune runs.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "stale.jpg"), []byte("old"), 0o644))

		ac := &AlbumConfig{Slug: "album", Path: dir, Sync: &AlbumSyncConfig{
			Provider: immichProviderName, AlbumID: "album-uuid", Captions: true,
			Immich: &ImmichSyncConfig{EnvFile: filepath.Join(configDir, ImmichEnvFileName)},
		}}
		wc := &WarnCollector{}
		require.NoError(t, RunSync(context.Background(), &Config{Warn: wc}, []*AlbumConfig{ac}, false))

		assert.Equal(t, "Upstream Album", ac.Name)
		assert.Equal(t, "Ice &amp; &lt;snow&gt;", ac.Description)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		assert.ElementsMatch(t, []string{"IMG_1.jpg", SyncMetadataFileName, photogenFileName}, names)

		got, err := os.ReadFile(filepath.Join(dir, photogenFileName))
		require.NoError(t, err)
		assert.Equal(t, "IMG_1.jpg A &lt;b&gt;bold&lt;/b&gt; caption &amp; an ampersand\n", string(got))

		meta, err := loadSyncMetadata(dir)
		require.NoError(t, err)
		assert.Equal(t, immichProviderName, meta.Provider)
		assert.Equal(t, "album-uuid", meta.AlbumID)
		require.Len(t, meta.Photos, 1)
		assert.Equal(t, "sum-1", meta.Photos[0].Checksum)

		// One warning per dropped asset: the hidden one from the provider, the RAW and the
		// clashing video from the shared layer.
		joined := strings.Join(wc.warnings, "")
		assert.Contains(t, joined, "IMG_2.jpg")
		assert.Contains(t, joined, "IMG_3.arw")
		assert.Contains(t, joined, "IMG_1.mov")
		assert.Len(t, wc.warnings, 3)
	})
}
