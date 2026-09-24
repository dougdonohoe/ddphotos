package photogen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// immichFixture reads one recorded response. The fixtures come off a real Immich 3.2.2 via
// bin/immich-record, so a field Immich renames breaks these tests rather than a live run.
func immichFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "immich", name))
	require.NoError(t, err)
	return data
}

// immichServer is a stand-in instance. It answers the three endpoints the provider calls and
// records what it was asked, so a test can assert the request shape as well as the result.
type immichServer struct {
	t     *testing.T
	album []byte            // GET /api/albums/{id}
	pages [][]byte          // POST /api/search/metadata, one entry per page
	media map[string]string // asset id -> its bytes, for /original

	// status and body, when set, make every API call fail this way instead.
	status int
	body   string
	// failFirst answers this many calls with 503 before behaving normally.
	failFirst int

	mu       sync.Mutex
	searches []map[string]any // decoded search request bodies, in order
	calls    int
	apiKeys  []string
}

//goland:noinspection GoUnhandledErrorResult
func (s *immichServer) start() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		call := s.calls
		s.apiKeys = append(s.apiKeys, r.Header.Get("x-api-key"))
		s.mu.Unlock()

		if call <= s.failFirst {
			w.Header().Set("x-immich-cid", "cid-503")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"message":"Internal server error"}`)
			return
		}
		if s.status != 0 {
			w.Header().Set("x-immich-cid", "cid-abc")
			w.WriteHeader(s.status)
			fmt.Fprint(w, s.body)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/albums/"):
			w.Write(s.album) //nolint:errcheck

		case r.Method == http.MethodPost && r.URL.Path == "/api/search/metadata":
			var got map[string]any
			require.NoError(s.t, json.NewDecoder(r.Body).Decode(&got))
			s.mu.Lock()
			s.searches = append(s.searches, got)
			page := len(s.searches)
			s.mu.Unlock()
			require.LessOrEqual(s.t, page, len(s.pages), "asked for a page that was not set up")
			w.Write(s.pages[page-1]) //nolint:errcheck

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/original"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/assets/"), "/original")
			bytes, ok := s.media[id]
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"message":"Not found or no asset.download access"}`)
				return
			}
			fmt.Fprint(w, bytes)

		default:
			s.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	s.t.Cleanup(srv.Close)
	return srv
}

// provider wires the server to a provider, skipping credential resolution so the test does not
// have to touch the environment. Returns the warnings it collects.
func (s *immichServer) provider(srv *httptest.Server) (*immichProvider, *[]string) {
	var warnings []string
	var mu sync.Mutex
	p := &immichProvider{
		creds: immichCredentials{APIKey: "test-key", BaseURL: srv.URL},
		warnf: func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
		},
	}
	return p, &warnings
}

func TestNormalizeImmichURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"a bare instance URL is left alone", "http://localhost:2283", "http://localhost:2283"},
		{"a trailing slash is dropped", "http://localhost:2283/", "http://localhost:2283"},
		{"the /api suffix people paste is dropped", "http://localhost:2283/api", "http://localhost:2283"},
		{"so is /api/ with its slash", "http://localhost:2283/api/", "http://localhost:2283"},
		{"case does not save /API", "http://example.com/API", "http://example.com"},
		{"surrounding whitespace is trimmed", "  http://localhost:2283  ", "http://localhost:2283"},
		{"https is fine", "https://photos.example.com", "https://photos.example.com"},
		// A reverse-proxied instance lives under a path, and dropping it would send every
		// request to the proxy's root.
		{"a path prefix survives", "https://example.com/immich", "https://example.com/immich"},
		{"a path prefix survives losing /api", "https://example.com/immich/api", "https://example.com/immich"},
		{"a query string is discarded", "http://localhost:2283/?x=1", "http://localhost:2283"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, dockerRewritten, err := normalizeImmichURL(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.False(t, dockerRewritten, "no rewrite outside Docker")
		})
	}

	// A pasted "localhost:2283" parses as scheme "localhost", which is the single likeliest
	// mistake, so the message has to say what to add rather than that parsing failed.
	t.Run("a URL with no scheme says to add one", func(t *testing.T) {
		t.Parallel()
		_, _, err := normalizeImmichURL("localhost:2283")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "http://")
	})

	t.Run("an empty URL is an error", func(t *testing.T) {
		t.Parallel()
		_, _, err := normalizeImmichURL("   ")
		require.Error(t, err)
	})

	t.Run("a scheme photogen cannot speak is an error", func(t *testing.T) {
		t.Parallel()
		_, _, err := normalizeImmichURL("ftp://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "http://")
	})
}

// Not parallel: t.Setenv, and the rewrite is deliberately driven by an environment variable
// because there is no other way to know the process is containerized.
func TestNormalizeImmichURLInDocker(t *testing.T) {
	t.Setenv(inDockerEnv, "1")

	tests := []struct {
		name    string
		in      string
		want    string
		rewrote bool
	}{
		{"localhost becomes the host alias", "http://localhost:2283", "http://host.docker.internal:2283", true},
		{"so does 127.0.0.1", "http://127.0.0.1:2283", "http://host.docker.internal:2283", true},
		{"so does ::1", "http://[::1]:2283", "http://host.docker.internal:2283", true},
		{"so does 0.0.0.0", "http://0.0.0.0:2283", "http://host.docker.internal:2283", true},
		{"a missing port stays missing", "http://localhost", "http://host.docker.internal", true},
		{"the /api suffix is still dropped", "http://localhost:2283/api", "http://host.docker.internal:2283", true},
		// Anything that already names a machine other than this one resolves inside the
		// container just as it does outside, so touching it would only break it.
		{"a LAN address is left alone", "http://10.0.0.251:2283", "http://10.0.0.251:2283", false},
		{"a hostname is left alone", "http://immich.lan:2283", "http://immich.lan:2283", false},
		{"a public URL is left alone", "https://photos.example.com", "https://photos.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dockerRewritten, err := normalizeImmichURL(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.rewrote, dockerRewritten)
		})
	}

	// Someone who wrote localhost and meant their laptop must not be left looking at an
	// error about a host name they have never seen. Nothing listens on 2283 inside this
	// test, which is exactly the failure the message is for.
	t.Run("a connection failure explains the rewritten host", func(t *testing.T) {
		t.Setenv(immichAPIKeyEnv, "test-key")
		t.Setenv(immichURLEnv, "http://localhost:2283")
		creds, err := loadImmichCredentials("")
		require.NoError(t, err)
		require.Equal(t, "http://host.docker.internal:2283", creds.BaseURL)

		p := &immichProvider{creds: creds}
		_, err = p.Album(context.Background(), "album")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "http://localhost:2283", "names what was configured")
		assert.Contains(t, err.Error(), dockerHostAlias, "names what was tried")
		assert.Contains(t, err.Error(), "--add-host", "names the fix")
	})
}

// Not parallel: every subtest sets IMMICH_* to control the precedence being tested.
func TestLoadImmichCredentials(t *testing.T) {
	writeEnv := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), ImmichEnvFileName)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		return path
	}
	// An empty value counts as unset, so a subtest that wants the file consulted clears
	// both rather than depending on what the developer happens to have exported.
	clearEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv(immichAPIKeyEnv, "")
		t.Setenv(immichURLEnv, "")
	}

	t.Run("the file supplies both values", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "IMMICH_API_KEY=file-key\nIMMICH_INSTANCE_URL=http://localhost:2283\n")
		creds, err := loadImmichCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, "file-key", creds.APIKey)
		assert.Equal(t, "http://localhost:2283", creds.BaseURL)
	})

	// The same precedence DDPHOTOS_ALBUMS_DIR has, and what albums.example.yaml promises, so
	// CI needs no secrets file on disk.
	t.Run("a real environment variable wins over the file", func(t *testing.T) {
		path := writeEnv(t, "IMMICH_API_KEY=file-key\nIMMICH_INSTANCE_URL=http://file:2283\n")
		t.Setenv(immichAPIKeyEnv, "env-key")
		t.Setenv(immichURLEnv, "http://env:2283")
		creds, err := loadImmichCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, "env-key", creds.APIKey)
		assert.Equal(t, "http://env:2283", creds.BaseURL)
	})

	t.Run("a missing file is fine when the environment has everything", func(t *testing.T) {
		t.Setenv(immichAPIKeyEnv, "env-key")
		t.Setenv(immichURLEnv, "http://localhost:2283/api")
		creds, err := loadImmichCredentials(filepath.Join(t.TempDir(), "absent.env"))
		require.NoError(t, err)
		assert.Equal(t, "env-key", creds.APIKey)
		assert.Equal(t, "http://localhost:2283", creds.BaseURL)
	})

	t.Run("a quoted value in the file loses its quotes", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "IMMICH_API_KEY=\"quoted-key\"\nIMMICH_INSTANCE_URL='http://localhost:2283'\n")
		creds, err := loadImmichCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, "quoted-key", creds.APIKey)
		assert.Equal(t, "http://localhost:2283", creds.BaseURL)
	})

	t.Run("comments and junk lines are ignored", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "# a comment\n\nnot-an-assignment\nIMMICH_API_KEY = spaced-key \nIMMICH_INSTANCE_URL=http://localhost:2283\n")
		creds, err := loadImmichCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, "spaced-key", creds.APIKey)
	})

	t.Run("a missing key names the variable and the file", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "IMMICH_INSTANCE_URL=http://localhost:2283\n")
		_, err := loadImmichCredentials(path)
		require.ErrorIs(t, err, errImmichNoCredentials)
		assert.Contains(t, err.Error(), immichAPIKeyEnv)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("a missing URL names the variable", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "IMMICH_API_KEY=file-key\n")
		_, err := loadImmichCredentials(path)
		require.ErrorIs(t, err, errImmichNoCredentials)
		assert.Contains(t, err.Error(), immichURLEnv)
	})

	// The whole reason credentials live in a file: they must not turn up in output that
	// gets pasted into a bug report.
	t.Run("the API key never appears in an error", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "IMMICH_API_KEY=super-secret-key\nIMMICH_INSTANCE_URL=not-a-url\n")
		_, err := loadImmichCredentials(path)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "super-secret-key")
	})

	t.Run("a bad URL in the file names the variable that held it", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "IMMICH_API_KEY=file-key\nIMMICH_INSTANCE_URL=localhost:2283\n")
		_, err := loadImmichCredentials(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), immichURLEnv)
	})
}

// Immich answers a malformed id with 400 "Validation failed", one album at a time, so a run
// syncing several albums gets part-way through before reporting a typo that was visible in the
// YAML all along. Catching the shape at config time is what makes that a non-event.
func TestValidateImmichAlbumID(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed UUID passes", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateImmichAlbumID("album", "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"))
		assert.NoError(t, validateImmichAlbumID("album", "EF8ACFB8-43FB-4C63-90C0-307B88B8F97A"),
			"hex is hex whatever its case")
	})

	// Deliberately looser than Immich's own v4-only pattern: this exists to catch a typo,
	// not to second-guess what Immich might issue later.
	t.Run("a UUID that is not version 4 still passes", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateImmichAlbumID("album", "d8052d5c-9ff1-1228-0f02-5cdd3d2e2d18"))
	})

	tests := []struct {
		name string
		id   string
	}{
		{"a truncated UUID", "ef8acfb8"},
		{"a UUID missing a group", "d8052d5c-9ff1-4228-5cdd3d2e2d18"},
		{"a UUID with a non-hex character", "z8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"},
		{"a UUID with no dashes", "d8052d5c9ff142289f025cdd3d2e2d18"},
		{"a UUID with trailing text", "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18/"},
		{"the whole album URL pasted in", "http://localhost:2283/albums/d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"},
		{"a mock-style album name", "antarctica"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" is rejected", func(t *testing.T) {
			t.Parallel()
			err := validateImmichAlbumID("antarctica", tt.id)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "antarctica", "names the album")
			assert.Contains(t, err.Error(), tt.id, "quotes back what was configured")
			assert.Contains(t, err.Error(), "/albums/", "shows where to find the right one")
		})
	}
}

func TestImmichProviderAlbum(t *testing.T) {
	t.Parallel()

	s := &immichServer{t: t, album: immichFixture(t, "album.json")}
	p, _ := s.provider(s.start())

	album, err := p.Album(context.Background(), "e3daf26c-677a-4b1d-8733-55279d1abfab")
	require.NoError(t, err)
	assert.Equal(t, "Big Sky 2026", album.Name)
	assert.Equal(t, "Ski trip to Big Sky, Montana", album.Description)
	assert.Equal(t, []string{"test-key"}, s.apiKeys)
}

func TestImmichProviderAssets(t *testing.T) {
	t.Parallel()

	s := &immichServer{t: t,
		album: immichFixture(t, "album.json"),
		pages: [][]byte{immichFixture(t, "search-page1.json")},
	}
	p, warnings := s.provider(s.start())

	assets, err := p.Assets(context.Background(), "e3daf26c-677a-4b1d-8733-55279d1abfab")
	require.NoError(t, err)
	require.Len(t, assets, 7)
	assert.Empty(t, *warnings)

	// The deprecated flat shape, on purpose: it works on current and older servers where the
	// v3.2 filter/orderBy/cursor shape only works on very recent ones. withExif is not an
	// optimization — exifInfo is where the description and the file size live.
	require.Len(t, s.searches, 1)
	assert.Equal(t, map[string]any{
		"albumIds": []any{"e3daf26c-677a-4b1d-8733-55279d1abfab"},
		"order":    "asc",
		"withExif": true,
		"size":     float64(immichPageSize),
		"page":     float64(1),
	}, s.searches[0])

	first := assets[0]
	assert.Equal(t, "8f51cceb-6ac1-4db2-8e42-decaf99ff4f7", first.ID)
	assert.Equal(t, "IMG_3875.jpeg", first.FileName)
	assert.Equal(t, "c07AWIQBN+1rk1KZH3htOrXs2kk=", first.Checksum)
	assert.Equal(t, int64(4656780), first.Size)
	assert.Equal(t, "", first.Caption)
	assert.Equal(t, "2026-09-18T22:37:57.735Z", first.UpdatedAt.Format(time.RFC3339Nano))

	// Upstream order, which is all the interface promises; photogen sorts by EXIF later.
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		names = append(names, a.FileName)
	}
	assert.Equal(t, "IMG_3885.jpeg", names[len(names)-1])
}

// nextPage is a nullable *string* ("2", "3", null), not a number, and total is the count in
// the page rather than the album's, so paging arithmetic is not an option.
func TestImmichProviderPaging(t *testing.T) {
	t.Parallel()

	s := &immichServer{t: t, pages: [][]byte{
		immichFixture(t, filepath.Join("paged", "search-page1.json")),
		immichFixture(t, filepath.Join("paged", "search-page2.json")),
		immichFixture(t, filepath.Join("paged", "search-page3.json")),
	}}
	p, _ := s.provider(s.start())

	assets, err := p.Assets(context.Background(), "e3daf26c-677a-4b1d-8733-55279d1abfab")
	require.NoError(t, err)
	assert.Len(t, assets, 7)

	require.Len(t, s.searches, 3)
	for i, want := range []float64{1, 2, 3} {
		assert.Equal(t, want, s.searches[i]["page"], "page number of request %d", i+1)
	}
}

// immichPage builds one search response page, with nextPage "" meaning the last page.
func immichPage(t *testing.T, names []string, idOffset int, nextPage string) []byte {
	t.Helper()
	items := make([]map[string]any, len(names))
	for i, name := range names {
		items[i] = map[string]any{
			"id":               fmt.Sprintf("asset-%d", idOffset+i),
			"originalFileName": name,
			"checksum":         "sum",
			"type":             "IMAGE",
			"visibility":       immichVisibilityTimeline,
		}
	}
	var next any
	if nextPage != "" {
		next = nextPage
	}
	page, err := json.Marshal(map[string]any{
		"assets": map[string]any{"items": items, "nextPage": next},
	})
	require.NoError(t, err)
	return page
}

// The asset cap applies to what photogen can publish, which is only known after
// filterSyncAssets has dropped RAW, unsupported files and base-name clashes. So the provider
// lists the whole album and leaves the cap to syncOneAlbum, rather than counting assets that
// are about to be filtered out.
func TestImmichProviderRawPairsUnderTheCap(t *testing.T) {
	t.Parallel()

	// 300 RAW+JPEG pairs: 600 assets, of which 300 are publishable.
	names := make([]string, 0, 600)
	for i := range 300 {
		names = append(names, fmt.Sprintf("IMG_%04d.CR2", i), fmt.Sprintf("IMG_%04d.jpg", i))
	}
	s := &immichServer{t: t, pages: [][]byte{
		immichPage(t, names[:immichPageSize], 0, "2"),
		immichPage(t, names[immichPageSize:], immichPageSize, ""),
	}}
	p, _ := s.provider(s.start())

	assets, err := p.Assets(context.Background(), "raw-album")
	require.NoError(t, err)
	assert.Len(t, assets, 600)
	assert.Len(t, filterSyncAssets(assets, func(string, ...any) {}), 300)
}

// A nextPage that does not move forward would page forever.
func TestImmichProviderPageDoesNotAdvance(t *testing.T) {
	t.Parallel()

	s := &immichServer{t: t, pages: [][]byte{
		immichPage(t, []string{"a.jpg"}, 0, "2"),
		immichPage(t, []string{"b.jpg"}, 1, "2"),
	}}
	p, _ := s.provider(s.start())

	_, err := p.Assets(context.Background(), "album")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `nextPage "2"`)
	assert.Len(t, s.searches, 2)
}

// Only the three rules that need Immich's own fields belong here. Unsupported extensions and
// photo-vs-video base-name clashes are filterSyncAssets's job, and are not repeated.
func TestImmichConvertAsset(t *testing.T) {
	t.Parallel()

	convert := func(a immichAsset) (SyncAsset, bool, []string) {
		var warnings []string
		p := &immichProvider{warnf: func(format string, args ...any) {
			warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
		}}
		asset, ok := p.convertAsset(a)
		return asset, ok, warnings
	}
	base := func() immichAsset {
		return immichAsset{
			ID: "id-1", OriginalFileName: "IMG_1.jpg", Checksum: "sum",
			Visibility: immichVisibilityTimeline,
		}
	}

	t.Run("an ordinary timeline photo is published", func(t *testing.T) {
		t.Parallel()
		asset, ok, warnings := convert(base())
		require.True(t, ok)
		assert.Equal(t, "IMG_1.jpg", asset.FileName)
		assert.Empty(t, warnings)
	})

	// An archived asset the user deliberately put in an album is in the album on purpose.
	t.Run("an archived asset is published", func(t *testing.T) {
		t.Parallel()
		a := base()
		a.Visibility = immichVisibilityArchive
		_, ok, warnings := convert(a)
		assert.True(t, ok)
		assert.Empty(t, warnings)
	})

	// Testing for the two visibilities that publish, rather than the ones that do not, means
	// a value Immich adds later is withheld rather than published by accident.
	for _, vis := range []string{"hidden", "locked", "something-new"} {
		t.Run("a "+vis+" asset is skipped with a warning", func(t *testing.T) {
			t.Parallel()
			a := base()
			a.Visibility = vis
			_, ok, warnings := convert(a)
			assert.False(t, ok)
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0], "IMG_1.jpg")
			assert.Contains(t, warnings[0], vis)
		})
	}

	// withDeleted defaults to false so this should never arrive; publishing something the
	// user deleted is the worst outcome available here, so it is checked anyway.
	t.Run("a trashed asset is skipped with a warning", func(t *testing.T) {
		t.Parallel()
		a := base()
		a.IsTrashed = true
		_, ok, warnings := convert(a)
		assert.False(t, ok)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "trash")
	})

	// The edited pixels only exist in the metadata-free full-size variant, so the original
	// is published and the warning says what the visitor will see.
	t.Run("an edited asset is published with a warning on the asset", func(t *testing.T) {
		t.Parallel()
		a := base()
		a.IsEdited = true
		asset, ok, warnings := convert(a)
		require.True(t, ok)
		assert.Empty(t, warnings, "an edited asset warns through the asset, not the channel")
		require.Len(t, asset.Warnings, 1)
		assert.Contains(t, asset.Warnings[0], "unedited")
	})

	// Immich omits exifInfo entirely rather than sending null, and every field inside it is
	// optional, so an asset it knows nothing about must not panic or invent a size.
	t.Run("a missing exifInfo leaves the caption and size empty", func(t *testing.T) {
		t.Parallel()
		asset, ok, _ := convert(base())
		require.True(t, ok)
		assert.Equal(t, "", asset.Caption)
		assert.Zero(t, asset.Size)
	})

	t.Run("the caption comes through unescaped for the shared layer to escape", func(t *testing.T) {
		t.Parallel()
		a := base()
		desc := "A <b>bold</b> caption & an ampersand"
		size := int64(1234)
		a.ExifInfo = &immichExifInfo{Description: &desc, FileSizeInByte: &size}
		asset, ok, _ := convert(a)
		require.True(t, ok)
		assert.Equal(t, desc, asset.Caption)
		assert.Equal(t, int64(1234), asset.Size)
	})

	// The file name reaches the grid's accessible label and photogen.txt's keys, so an asset
	// with none still needs something rather than an empty string.
	t.Run("an asset with no file name falls back to its ID", func(t *testing.T) {
		t.Parallel()
		a := base()
		a.OriginalFileName = ""
		asset, ok, _ := convert(a)
		require.True(t, ok)
		assert.Equal(t, "id-1", asset.FileName)
	})
}

func TestImmichProviderFetch(t *testing.T) {
	t.Parallel()

	s := &immichServer{t: t, media: map[string]string{"id-1": "the original bytes"}}
	p, _ := s.provider(s.start())

	t.Run("it streams the original bytes", func(t *testing.T) {
		t.Parallel()
		rc, err := p.Fetch(context.Background(), SyncAsset{ID: "id-1", FileName: "IMG_1.jpg"})
		require.NoError(t, err)
		//goland:noinspection GoUnhandledErrorResult
		defer rc.Close() //nolint:errcheck
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "the original bytes", string(body))
	})

	t.Run("an asset immich cannot serve is an error", func(t *testing.T) {
		t.Parallel()
		_, err := p.Fetch(context.Background(), SyncAsset{ID: "missing", FileName: "gone.jpg"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot see it")
	})
}

// Not parallel: the retry tests shrink immichRetryBackoff so they do not spend three real
// seconds waiting.
func TestImmichProviderErrors(t *testing.T) {
	restore := immichRetryBackoff
	immichRetryBackoff = time.Millisecond
	t.Cleanup(func() { immichRetryBackoff = restore })

	t.Run("a 401 names where the key came from but never the key", func(t *testing.T) {
		s := &immichServer{t: t, status: http.StatusUnauthorized, body: `{"message":"Invalid API key"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), "album")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid API key")
		assert.Contains(t, err.Error(), immichAPIKeyEnv)
		assert.Contains(t, err.Error(), ImmichEnvFileName)
		assert.Contains(t, err.Error(), "cid-abc", "the correlation id is what immich's own logs are searchable by")
		assert.NotContains(t, err.Error(), "test-key")
		// A wrong key is not a transient failure.
		assert.Equal(t, 1, s.calls)
	})

	t.Run("a 403 says which permissions the key needs", func(t *testing.T) {
		s := &immichServer{t: t, status: http.StatusForbidden,
			body: `{"message":"Missing required permission: asset.read"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), "album")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "album.read")
		assert.Equal(t, 1, s.calls)
	})

	// Immich answers an unknown or invisible id with 400 from its access check rather than
	// 404, so this is the ordinary wrong-album-id path and must read like one.
	t.Run("a 400 access failure points at sync.album_id", func(t *testing.T) {
		s := &immichServer{t: t, status: http.StatusBadRequest,
			body: `{"message":"Not found or no album.read access"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), "album")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sync.album_id")
		assert.Equal(t, 1, s.calls)
	})

	// "Validation failed" on its own names neither the field nor the reason. Immich puts
	// both in the errors array, which is the half worth reading.
	t.Run("a validation failure carries immich's own detail", func(t *testing.T) {
		s := &immichServer{t: t, status: http.StatusBadRequest, body: `{"message":"Validation failed",
			"errors":[{"code":"invalid_format","format":"uuid","path":["id"],"message":"Invalid UUID"}]}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), "ef8acfb8")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Validation failed")
		assert.Contains(t, err.Error(), "id: Invalid UUID", "the field and the reason")
		assert.Contains(t, err.Error(), "sync.album_id", "and what to go and check")
		assert.Equal(t, 1, s.calls, "a malformed request is not retried")
	})

	t.Run("a 503 is retried and the retry is used", func(t *testing.T) {
		s := &immichServer{t: t, failFirst: 2, album: immichFixture(t, "album.json")}
		p, _ := s.provider(s.start())
		album, err := p.Album(context.Background(), "album")
		require.NoError(t, err)
		assert.Equal(t, "Big Sky 2026", album.Name)
		assert.Equal(t, immichAttempts, s.calls)
	})

	t.Run("a server that is always down gives up and says so", func(t *testing.T) {
		s := &immichServer{t: t, status: http.StatusBadGateway, body: `{"message":"Bad gateway"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), "album")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gave up after 3 attempts")
		assert.Equal(t, immichAttempts, s.calls)
	})

	t.Run("a rate limit is retried", func(t *testing.T) {
		s := &immichServer{t: t, status: http.StatusTooManyRequests, body: `{"message":"Too many requests"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), "album")
		require.Error(t, err)
		assert.Equal(t, immichAttempts, s.calls)
	})
}

// Not parallel: t.Setenv, which is also the point — this is the only test that goes through
// credential resolution rather than building the provider by hand.
func TestNewImmichProvider(t *testing.T) {
	t.Setenv(immichAPIKeyEnv, "env-key")
	t.Setenv(immichURLEnv, "http://localhost:2283/api")

	p, err := newImmichProvider(&ImmichSyncConfig{EnvFile: filepath.Join(t.TempDir(), "absent.env")}, nil)
	require.NoError(t, err)
	assert.IsType(t, &immichProvider{}, p)

	// nil cfg means the AlbumSyncConfig was built by hand rather than by resolveSync. The
	// environment still has everything, so it works rather than panicking.
	_, err = newImmichProvider(nil, nil)
	require.NoError(t, err)

	t.Run("a missing key fails before anything is downloaded", func(t *testing.T) {
		t.Setenv(immichAPIKeyEnv, "")
		_, err := newImmichProvider(&ImmichSyncConfig{EnvFile: filepath.Join(t.TempDir(), "absent.env")}, nil)
		require.ErrorIs(t, err, errImmichNoCredentials)
	})
}
