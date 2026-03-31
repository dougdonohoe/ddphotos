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
		assert.Equal(t, "530 miles along El Camino de Santiago.", idx.Description)
		assert.Equal(t, "Apr 2024", idx.DateSpan)
		assert.Equal(t, "grid/2024-The-Way-1.webp", idx.Cover)
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

func TestWriteAlbumIndex(t *testing.T) {
	t.Parallel()

	makeAP := func(dir string, encrypt *EncryptConfig) *AlbumProcessor {
		return &AlbumProcessor{
			Config: &Config{
				OutputRoot: dir,
				SiteID:     "testsite",
				Encrypt:    encrypt,
				Warn:       &WarnCollector{},
			},
			AlbumConfig: &AlbumConfig{Slug: "myalbum", Name: "My Album", Description: "A test album."},
			Photos: []*Photo{
				{ID: "photo1", FileName: "photo1.jpg", PhotoMetadata: &PhotoMetadata{Width: 100, Height: 200}},
			},
		}
	}

	t.Run("unencrypted writes index.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, makeAP(dir, nil).WriteAlbumIndex())

		outPath := filepath.Join(dir, "albums", "testsite", "myalbum", "index.json")
		assert.FileExists(t, outPath)
		assert.NoFileExists(t, filepath.Join(dir, "albums", "testsite", "myalbum", "index.enc.json"))

		idx, err := LoadAlbumIndex(outPath)
		require.NoError(t, err)
		assert.Equal(t, "myalbum", idx.Slug)
		assert.Equal(t, "A test album.", idx.Description)
		assert.NotEmpty(t, idx.Cover, "cover should be set from first photo")
		require.Len(t, idx.Photos, 1)
	})

	t.Run("encrypted writes index.enc.json with unreadable content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		encrypt := &EncryptConfig{SitePassword: "test-pass"}
		require.NoError(t, makeAP(dir, encrypt).WriteAlbumIndex())

		encPath := filepath.Join(dir, "albums", "testsite", "myalbum", "index.enc.json")
		assert.FileExists(t, encPath)
		assert.NoFileExists(t, filepath.Join(dir, "albums", "testsite", "myalbum", "index.json"))

		data, err := os.ReadFile(encPath)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "myalbum", "encrypted file must not contain plaintext slug")
		assert.NotContains(t, string(data), "A test album.", "encrypted file must not contain plaintext description")
	})

	t.Run("switching to unencrypted removes stale index.enc.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		albumDir := filepath.Join(dir, "albums", "testsite", "myalbum")
		require.NoError(t, os.MkdirAll(albumDir, 0o755))

		staleEnc := filepath.Join(albumDir, "index.enc.json")
		require.NoError(t, os.WriteFile(staleEnc, []byte("stale"), 0o644))

		require.NoError(t, makeAP(dir, nil).WriteAlbumIndex())

		assert.FileExists(t, filepath.Join(albumDir, "index.json"))
		assert.NoFileExists(t, staleEnc)
	})
}

func makeSiteCfg(dir string, encrypt *EncryptConfig) *Config {
	return &Config{OutputRoot: dir, SiteID: "testsite", Encrypt: encrypt, Warn: &WarnCollector{}}
}

func TestWriteAlbumsIndex(t *testing.T) {
	t.Parallel()

	summaries := []AlbumSummary{
		{Slug: "album1", Title: "Album 1", Count: 5},
		{Slug: "album2", Title: "Album 2", Count: 3},
	}

	t.Run("unencrypted writes albums.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, makeSiteCfg(dir, nil).WriteAlbumsIndex(summaries))

		outPath := filepath.Join(dir, "albums", "testsite", "albums.json")
		assert.FileExists(t, outPath)
		assert.NoFileExists(t, filepath.Join(dir, "albums", "testsite", "albums.enc.json"))

		loaded, err := LoadAlbumSummaries(outPath)
		require.NoError(t, err)
		require.Len(t, loaded, 2)
		assert.Equal(t, "album1", loaded[0].Slug)
	})

	t.Run("encrypted writes albums.enc.json with unreadable content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		encrypt := &EncryptConfig{SitePassword: "site-pass"}
		require.NoError(t, makeSiteCfg(dir, encrypt).WriteAlbumsIndex(summaries))

		encPath := filepath.Join(dir, "albums", "testsite", "albums.enc.json")
		assert.FileExists(t, encPath)
		assert.NoFileExists(t, filepath.Join(dir, "albums", "testsite", "albums.json"))

		data, err := os.ReadFile(encPath)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "album1", "encrypted file must not contain plaintext slug")
	})

	t.Run("switching to unencrypted removes stale albums.enc.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		siteDir := filepath.Join(dir, "albums", "testsite")
		require.NoError(t, os.MkdirAll(siteDir, 0o755))

		staleEnc := filepath.Join(siteDir, "albums.enc.json")
		require.NoError(t, os.WriteFile(staleEnc, []byte("stale"), 0o644))

		require.NoError(t, makeSiteCfg(dir, nil).WriteAlbumsIndex(summaries))

		assert.FileExists(t, filepath.Join(siteDir, "albums.json"))
		assert.NoFileExists(t, staleEnc)
	})
}

func TestWriteConfigJSON(t *testing.T) {
	t.Parallel()

	t.Run("unencrypted references albums.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, makeSiteCfg(dir, nil).WriteConfigJSON())

		data, err := os.ReadFile(filepath.Join(dir, "albums", "testsite", "config.json"))
		require.NoError(t, err)
		assert.Contains(t, string(data), `"siteId": "testsite"`)
		assert.Contains(t, string(data), `"albumsFile": "albums.json"`)
	})

	t.Run("encrypted references albums.enc.json", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		encrypt := &EncryptConfig{SitePassword: "passw0rd"}
		require.NoError(t, makeSiteCfg(dir, encrypt).WriteConfigJSON())

		data, err := os.ReadFile(filepath.Join(dir, "albums", "testsite", "config.json"))
		require.NoError(t, err)
		assert.Contains(t, string(data), `"siteId": "testsite"`)
		assert.Contains(t, string(data), `"albumsFile": "albums.enc.json"`)
	})
}

func TestGetAlbumSummary(t *testing.T) {
	t.Parallel()

	photo := &Photo{
		ID:            "photo1",
		FileName:      "photo1.jpg",
		PhotoMetadata: &PhotoMetadata{Width: 100, Height: 200},
	}

	makeAP := func(slug string, encrypt *EncryptConfig) *AlbumProcessor {
		return &AlbumProcessor{
			Config: &Config{
				OutputRoot: "/tmp",
				SiteID:     "sample",
				Encrypt:    encrypt,
				Warn:       &WarnCollector{},
			},
			AlbumConfig: &AlbumConfig{Slug: slug, Name: "Test Album"},
			Photos:      []*Photo{photo},
		}
	}

	t.Run("unencrypted album has cover and coverJpeg", func(t *testing.T) {
		t.Parallel()
		s := makeAP("myalbum", nil).GetAlbumSummary()
		assert.False(t, s.Encrypted)
		assert.NotEmpty(t, s.Cover)
		assert.NotEmpty(t, s.CoverJpeg)
	})

	t.Run("site-encrypted album has cover but no coverJpeg", func(t *testing.T) {
		t.Parallel()
		// albums.enc.json is protected, so the Cover URL is safe to include — it is only
		// revealed after the site password is entered. CoverJpeg is omitted because it is
		// used for OG/crawler meta tags and should not index password-protected content.
		encrypt := &EncryptConfig{SitePassword: "pass"}
		s := makeAP("myalbum", encrypt).GetAlbumSummary()
		assert.True(t, s.Encrypted)
		assert.NotEmpty(t, s.Cover)
		assert.Empty(t, s.CoverJpeg)
	})

	t.Run("per-album encryption: encrypted album omits cover, public sibling keeps it", func(t *testing.T) {
		t.Parallel()
		// albums.json is plain (no site password), so the Cover URL for the encrypted album
		// must be omitted — it would expose the cover image without requiring a password.
		encrypt := &EncryptConfig{
			HMACKey:        "test-key",
			AlbumPasswords: map[string]string{"secret": "pass"},
		}

		sEncrypted := makeAP("secret", encrypt).GetAlbumSummary()
		assert.True(t, sEncrypted.Encrypted)
		assert.Empty(t, sEncrypted.Cover)

		sPublic := makeAP("public", encrypt).GetAlbumSummary()
		assert.False(t, sPublic.Encrypted)
		assert.NotEmpty(t, sPublic.Cover)
	})

	t.Run("mixed: site+per-album encryption omits cover for per-album album, includes for others", func(t *testing.T) {
		t.Parallel()
		// Per-album password takes precedence: the "secret" album has its own password so its
		// cover is omitted even though albums.enc.json is site-protected. "other" has no
		// per-album password so it is covered only by the site password and gets a cover.
		encrypt := &EncryptConfig{
			HMACKey:        "test-key",
			SitePassword:   "site-pass",
			AlbumPasswords: map[string]string{"secret": "secret-pass"},
		}

		sSecret := makeAP("secret", encrypt).GetAlbumSummary()
		assert.True(t, sSecret.Encrypted)
		assert.Empty(t, sSecret.Cover, "per-album password album must not expose cover")
		assert.Empty(t, sSecret.CoverJpeg)

		sOther := makeAP("other", encrypt).GetAlbumSummary()
		assert.True(t, sOther.Encrypted)
		assert.NotEmpty(t, sOther.Cover, "site-only encrypted album should include cover")
		assert.Empty(t, sOther.CoverJpeg)
	})
}
