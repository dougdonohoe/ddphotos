package photogen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The album the fixtures were recorded from: "The Way", 23 photos and 26 files, on
// PhotoPrism 261007-65faaae5d via bin/photoprism-record.
const photoprismTestAlbum = "atmyuuj99diyoq1s"

// photoprismFixture reads one recorded response.
func photoprismFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "photoprism", name))
	require.NoError(t, err)
	return data
}

// photoprismPage is one recorded search page: the body, and the X-Count header it came
// with, which is what the provider pages on.
type photoprismPage struct {
	body   []byte
	xCount int
}

// photoprismFixturePages loads search-pageN.json and the x_count recorded beside it in
// request-pageN.json, for every page in dir.
func photoprismFixturePages(t *testing.T, dir string) []photoprismPage {
	t.Helper()
	var pages []photoprismPage
	for n := 1; ; n++ {
		req, err := os.ReadFile(filepath.Join("testdata", "photoprism", dir, fmt.Sprintf("request-page%d.json", n)))
		if os.IsNotExist(err) {
			break
		}
		require.NoError(t, err)
		var meta struct {
			XCount int `json:"x_count"`
		}
		require.NoError(t, json.Unmarshal(req, &meta))
		pages = append(pages, photoprismPage{
			body:   photoprismFixture(t, filepath.Join(dir, fmt.Sprintf("search-page%d.json", n))),
			xCount: meta.XCount,
		})
	}
	require.NotEmpty(t, pages)
	return pages
}

// photoprismServer is a stand-in instance. It answers the three endpoints the provider calls,
// hands out a fresh download token with every listing, and records what it was asked.
type photoprismServer struct {
	t     *testing.T
	album []byte           // GET /api/v1/albums/{uid}
	pages []photoprismPage // GET /api/v1/photos, by request order; a token refresh reuses page 1
	media map[string]string

	// status and body, when set, make every API call fail this way instead.
	status int
	body   string
	// failFirst answers this many calls with 503 before behaving normally.
	failFirst int
	// expireTokens makes every token issued before this many listings unusable, the way an
	// hour-old token is.
	expireTokens int

	mu        sync.Mutex
	calls     int
	listings  int
	queries   []url.Values // search queries, in order
	auth      []string
	downloads []string // hashes asked for, including refused ones
}

func (s *photoprismServer) tokenFor(listing int) string { return fmt.Sprintf("dl-token-%d", listing) }

//goland:noinspection GoUnhandledErrorResult
func (s *photoprismServer) start() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		call := s.calls
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		s.mu.Unlock()

		if call <= s.failFirst {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"code":503,"error":"Service unavailable"}`)
			return
		}
		if s.status != 0 {
			w.WriteHeader(s.status)
			fmt.Fprint(w, s.body)
			return
		}

		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/albums/"):
			w.Write(s.album) //nolint:errcheck

		case r.URL.Path == "/api/v1/photos":
			s.mu.Lock()
			s.queries = append(s.queries, r.URL.Query())
			s.listings++
			listing := s.listings
			s.mu.Unlock()
			// A one-row listing is a token refresh; anything else is the next page.
			page := s.pages[0]
			if r.URL.Query().Get("count") != "1" {
				offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
				count, _ := strconv.Atoi(r.URL.Query().Get("count"))
				idx := offset / count
				require.Less(s.t, idx, len(s.pages), "asked for a page that was not set up")
				page = s.pages[idx]
			}
			w.Header().Set("X-Count", strconv.Itoa(page.xCount))
			w.Header().Set("X-Download-Token", s.tokenFor(listing))
			w.Write(page.body) //nolint:errcheck

		case strings.HasPrefix(r.URL.Path, "/api/v1/dl/"):
			hash := strings.TrimPrefix(r.URL.Path, "/api/v1/dl/")
			token := r.URL.Query().Get("t")
			s.mu.Lock()
			s.downloads = append(s.downloads, hash)
			listings := s.listings
			s.mu.Unlock()
			valid := false
			for n := s.expireTokens + 1; n <= listings; n++ {
				valid = valid || token == s.tokenFor(n)
			}
			// PhotoPrism answers every download failure with an SVG, never JSON.
			w.Header().Set("Content-Type", "image/svg+xml")
			if !valid {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `<svg>broken</svg>`)
				return
			}
			body, ok := s.media[hash]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `<svg>broken</svg>`)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			fmt.Fprint(w, body)

		default:
			s.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	s.t.Cleanup(srv.Close)
	return srv
}

// provider wires the server to a provider, skipping credential resolution. Returns the
// warnings it collects.
func (s *photoprismServer) provider(srv *httptest.Server) (*photoprismProvider, *[]string) {
	var warnings []string
	var mu sync.Mutex
	p := &photoprismProvider{
		http: newPhotoPrismHTTP(syncCredentials{Secret: "test-password", BaseURL: srv.URL}),
		warnf: func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
		},
	}
	return p, &warnings
}

func TestNormalizePhotoPrismURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"a bare site URL is left alone", "http://localhost:2342", "http://localhost:2342"},
		{"a trailing slash is dropped", "http://localhost:2342/", "http://localhost:2342"},
		{"the /api/v1 the API docs show is dropped", "http://localhost:2342/api/v1", "http://localhost:2342"},
		{"so is /api/v1/", "http://localhost:2342/api/v1/", "http://localhost:2342"},
		{"so is a bare /api", "http://localhost:2342/api", "http://localhost:2342"},
		{"the /library the browser shows is dropped", "http://localhost:2342/library", "http://localhost:2342"},
		{"a path prefix survives", "https://example.com/photoprism", "https://example.com/photoprism"},
		{"a path prefix survives losing /api/v1", "https://example.com/photoprism/api/v1", "https://example.com/photoprism"},
		// Only a whole trailing segment is a suffix.
		{"a path merely ending in api is kept", "https://example.com/myapi", "https://example.com/myapi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, dockerRewritten, err := normalizePhotoPrismURL(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.False(t, dockerRewritten)
		})
	}
}

// Not parallel: t.Setenv.
func TestNormalizePhotoPrismURLInDocker(t *testing.T) {
	t.Setenv(inDockerEnv, "1")

	got, rewrote, err := normalizePhotoPrismURL("http://localhost:2342/api/v1")
	require.NoError(t, err)
	assert.True(t, rewrote)
	assert.Equal(t, "http://host.docker.internal:2342", got)

	t.Run("a connection failure explains the rewritten host", func(t *testing.T) {
		t.Setenv(photoprismPasswordEnv, "test-password")
		t.Setenv(photoprismURLEnv, "http://localhost:2342")
		creds, err := loadPhotoPrismCredentials("")
		require.NoError(t, err)

		p := &photoprismProvider{http: newPhotoPrismHTTP(creds)}
		_, err = p.Album(context.Background(), photoprismTestAlbum)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "http://localhost:2342", "names what was configured")
		assert.Contains(t, err.Error(), dockerHostAlias, "names what was tried")
		assert.Contains(t, err.Error(), "If PhotoPrism is on the host")
	})
}

// Not parallel: every subtest sets PHOTOPRISM_* to control the precedence being tested.
func TestLoadPhotoPrismCredentials(t *testing.T) {
	writeEnv := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), PhotoPrismEnvFileName)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		return path
	}
	clearEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv(photoprismPasswordEnv, "")
		t.Setenv(photoprismURLEnv, "")
	}

	t.Run("the file supplies both values", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "PHOTOPRISM_APP_PASSWORD=file-pw\nPHOTOPRISM_SITE_URL=http://localhost:2342/\n")
		creds, err := loadPhotoPrismCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, "file-pw", creds.Secret)
		assert.Equal(t, "http://localhost:2342", creds.BaseURL)
	})

	t.Run("a real environment variable wins over the file", func(t *testing.T) {
		path := writeEnv(t, "PHOTOPRISM_APP_PASSWORD=file-pw\nPHOTOPRISM_SITE_URL=http://file:2342\n")
		t.Setenv(photoprismPasswordEnv, "env-pw")
		t.Setenv(photoprismURLEnv, "http://env:2342")
		creds, err := loadPhotoPrismCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, "env-pw", creds.Secret)
		assert.Equal(t, "http://env:2342", creds.BaseURL)
	})

	t.Run("a missing password names the variable, the file and where to get one", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "PHOTOPRISM_SITE_URL=http://localhost:2342\n")
		_, err := loadPhotoPrismCredentials(path)
		require.ErrorIs(t, err, errNoSyncCredentials)
		assert.Contains(t, err.Error(), photoprismPasswordEnv)
		assert.Contains(t, err.Error(), path)
		assert.Contains(t, err.Error(), "Apps and Devices")
	})

	t.Run("a missing URL names the variable", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "PHOTOPRISM_APP_PASSWORD=file-pw\n")
		_, err := loadPhotoPrismCredentials(path)
		require.ErrorIs(t, err, errNoSyncCredentials)
		assert.Contains(t, err.Error(), photoprismURLEnv)
	})

	t.Run("the password never appears in an error", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "PHOTOPRISM_APP_PASSWORD=super-secret-pw\nPHOTOPRISM_SITE_URL=not-a-url\n")
		_, err := loadPhotoPrismCredentials(path)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "super-secret-pw")
	})
}

func TestValidatePhotoPrismAlbumID(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validatePhotoPrismAlbumID("the-way", photoprismTestAlbum))

	for _, id := range []string{
		"atmyuuj99diyoq1",   // truncated
		"atmyuuj99diyoq1sx", // one too many
		"ptmyqj5hvqiium1d",  // a photo UID, not an album's
		"ATMYUUJ99DIYOQ1S",  // PhotoPrism UIDs are lowercase
		"http://localhost:2342/library/albums/atmyuuj99diyoq1s/the-way",
		"d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18", // an Immich id
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			err := validatePhotoPrismAlbumID("the-way", id)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the-way", "names the album")
			assert.Contains(t, err.Error(), id, "quotes back what was configured")
			assert.Contains(t, err.Error(), "/albums/", "shows where to find the right one")
		})
	}
}

func TestPhotoPrismProviderAlbum(t *testing.T) {
	t.Parallel()

	s := &photoprismServer{t: t, album: photoprismFixture(t, "album.json")}
	p, _ := s.provider(s.start())

	album, err := p.Album(context.Background(), photoprismTestAlbum)
	require.NoError(t, err)
	assert.Equal(t, "The Way", album.Name)
	assert.Equal(t, "530 mile walk across Northern Spain", album.Description)
	assert.Equal(t, []string{"Bearer test-password"}, s.auth, "an app password is a bearer token")
}

func TestPhotoPrismProviderAssets(t *testing.T) {
	t.Parallel()

	s := &photoprismServer{t: t, pages: photoprismFixturePages(t, "")}
	p, warnings := s.provider(s.start())

	assets, err := p.Assets(context.Background(), photoprismTestAlbum)
	require.NoError(t, err)
	require.Len(t, assets, 23)
	assert.Empty(t, *warnings)

	// merged=true is what puts Files on each row.
	require.Len(t, s.queries, 1)
	q := s.queries[0]
	assert.Equal(t, photoprismTestAlbum, q.Get("s"))
	assert.Equal(t, "true", q.Get("merged"))
	assert.Equal(t, "oldest", q.Get("order"))
	assert.Equal(t, strconv.Itoa(photoprismPageSize), q.Get("count"))
	assert.Equal(t, "0", q.Get("offset"))

	byName := map[string]SyncAsset{}
	for _, a := range assets {
		byName[a.FileName] = a
		assert.False(t, a.Edited, "%s: nothing in this album is RAW", a.FileName)
	}

	first := assets[0]
	assert.Equal(t, "ptmyqj5hvqiium1d", first.ID)
	assert.Equal(t, "2024-The-Way-3.jpg", first.FileName, "the upload name, not PhotoPrism's renamed file")
	assert.Equal(t, "ea56d78ceadc2a11f58851b5f078957fe7b2ce15", first.Checksum)
	assert.Equal(t, int64(5535328), first.Size)
	assert.Equal(t, "2026-10-15T19:09:05Z", first.UpdatedAt.Format(time.RFC3339))

	// The caption is the user's; the generated title is not used.
	assert.Equal(t, "Matching wine and buildings", byName["2024-The-Way-10.jpg"].Caption)
	assert.Equal(t, "", first.Caption)

	// A HEIC publishes its original, not the JPEG PhotoPrism made of it.
	heic, ok := byName["IMG_9728.heic"]
	require.True(t, ok)
	assert.Equal(t, "ed968ba4e3a84b291fa66d88e34b23674f5d3438", heic.Checksum)
	assert.Equal(t, int64(2369718), heic.Size)

	// So does an AVIF.
	assert.Equal(t, "0cf394aeb66242e958c088a3c01a4d2b891bc271", byName["2024-The-Way-38.avif"].Checksum)

	// A video publishes the video, not its poster frame.
	mov, ok := byName["ocean.mov"]
	require.True(t, ok)
	assert.Equal(t, "d2ac0fb9f3e1897d5ea628a43fde6240c1b68220", mov.Checksum)

	// The listing carried a download token, so Fetch need not ask for one.
	assert.Equal(t, s.tokenFor(1), p.token)
}

// PhotoPrism pages a merged listing by file row: count and offset count files, and a photo
// whose files straddle a page boundary arrives in halves. Recorded at count=7, where the
// AVIF photo's JPEG ends page 3 and its original starts page 4.
func TestPhotoPrismProviderPaging(t *testing.T) {
	t.Parallel()

	s := &photoprismServer{t: t, pages: photoprismFixturePages(t, "paged")}
	p, warnings := s.provider(s.start())

	// The recording's page size, so offsets line up with the recorded pages.
	assets, err := p.assets(context.Background(), photoprismTestAlbum, 7)
	require.NoError(t, err)
	assert.Len(t, assets, 23, "every photo once, the straddling one included")
	assert.Empty(t, *warnings)

	require.Len(t, s.queries, 4)
	for i, want := range []string{"0", "7", "14", "21"} {
		assert.Equal(t, want, s.queries[i].Get("offset"), "offset of request %d advances by rows", i+1)
	}

	var avif *SyncAsset
	for i := range assets {
		if assets[i].ID == "ptmyqj97pvn6f0m8" {
			avif = &assets[i]
		}
	}
	require.NotNil(t, avif, "the straddling photo is kept")
	assert.Equal(t, "2024-The-Way-38.avif", avif.FileName, "its original, which only page 4 carried")
	assert.Equal(t, "0cf394aeb66242e958c088a3c01a4d2b891bc271", avif.Checksum)
}

// Only what needs PhotoPrism's own fields belongs here: privacy, hidden and archived photos,
// and which file of a photo to publish. Unsupported extensions are filterSyncAssets's job.
func TestPhotoPrismConvertPhoto(t *testing.T) {
	t.Parallel()

	convert := func(ph photoprismPhoto) (SyncAsset, bool, []string, map[string]string) {
		var warnings []string
		p := &photoprismProvider{converted: map[string]string{}, warnf: func(format string, args ...any) {
			warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
		}}
		asset, ok := p.convertPhoto(ph)
		return asset, ok, warnings, p.converted
	}
	original := func(name, hash string) photoprismFile {
		return photoprismFile{UID: "f-" + hash, Name: "2024/05/" + hash + filepath.Ext(name), Root: "/",
			Hash: hash, Size: 100, MediaType: "image", OriginalName: name}
	}
	sidecar := func(hash string) photoprismFile {
		return photoprismFile{UID: "f-" + hash, Name: "2024/05/x.jpg", Root: "sidecar",
			Hash: hash, Size: 50, MediaType: "image", Primary: true}
	}
	photo := func(files ...photoprismFile) photoprismPhoto {
		return photoprismPhoto{UID: "pt1", Type: "image", Quality: 3, UpdatedAt: "2026-10-15T19:09:05Z",
			TakenAtLocal: "2024-05-04T08:18:21Z", TakenSrc: "meta", Files: files}
	}
	primary := func(f photoprismFile) photoprismFile { f.Primary = true; return f }

	t.Run("a plain JPEG publishes itself", func(t *testing.T) {
		t.Parallel()
		a, ok, w, _ := convert(photo(primary(original("IMG_1.jpg", "h1"))))
		require.True(t, ok)
		assert.Empty(t, w)
		assert.Equal(t, "IMG_1.jpg", a.FileName)
		assert.Equal(t, "h1", a.Checksum)
		assert.False(t, a.Edited)
	})

	tests := []struct {
		name string
		edit func(*photoprismPhoto)
		want string
	}{
		{"a private photo is skipped", func(p *photoprismPhoto) { p.Private = true }, "private"},
		{"a hidden photo is skipped", func(p *photoprismPhoto) { p.Quality = -1 }, "hidden"},
		{"an archived photo is skipped", func(p *photoprismPhoto) {
			deleted := "2026-10-15T19:09:05Z"
			p.DeletedAt = &deleted
		}, "archived"},
		{"a photo with no files is skipped", func(p *photoprismPhoto) { p.Files = nil }, "no downloadable file"},
		{"a photo whose only file is missing is skipped", func(p *photoprismPhoto) { p.Files[0].Missing = true },
			"no downloadable file"},
		{"a photo whose only file has an error is skipped", func(p *photoprismPhoto) { p.Files[0].Error = "broken" },
			"no downloadable file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ph := photo(primary(original("IMG_1.jpg", "h1")))
			tt.edit(&ph)
			_, ok, w, _ := convert(ph)
			assert.False(t, ok)
			require.Len(t, w, 1)
			assert.Contains(t, w[0], tt.want)
		})
	}

	t.Run("a Live Photo publishes the still, not the video", func(t *testing.T) {
		t.Parallel()
		mov := original("IMG_2.mov", "hmov")
		mov.Video, mov.MediaType = true, "video"
		ph := photo(sidecar("hjpg"), mov, original("IMG_2.heic", "hheic"))
		ph.Type = "live"
		a, ok, _, _ := convert(ph)
		require.True(t, ok)
		assert.Equal(t, "IMG_2.heic", a.FileName)
		assert.Equal(t, "hheic", a.Checksum)
	})

	t.Run("a video with no video file is skipped", func(t *testing.T) {
		t.Parallel()
		ph := photo(sidecar("hjpg"))
		ph.Type = "video"
		_, ok, w, _ := convert(ph)
		assert.False(t, ok)
		assert.Len(t, w, 1)
	})

	// The decision for RAW: publish PhotoPrism's JPEG rather than nothing. To the shared
	// layer that is an edited rendition, so Checksum and Size stay the RAW's and the
	// capture date travels in a sidecar.
	t.Run("a RAW publishes PhotoPrism's JPEG of it", func(t *testing.T) {
		t.Parallel()
		raw := original("IMG_3.CR2", "hraw")
		raw.MediaType = "raw"
		a, ok, w, converted := convert(photo(sidecar("hjpg"), raw))
		require.True(t, ok)
		assert.Empty(t, w)
		assert.Equal(t, "IMG_3.jpg", a.FileName)
		assert.True(t, a.Edited)
		assert.Equal(t, "hraw", a.Checksum)
		assert.Equal(t, int64(100), a.Size, "the RAW's size, so servedOriginal cannot misfire")
		assert.Equal(t, map[string]string{"pt1": "hjpg"}, converted, "what Fetch downloads instead")
		assert.Equal(t, "2024-05-04T08:18:21Z", a.DateTaken.Format(time.RFC3339))
	})

	t.Run("a RAW dated from something other than its metadata gets no sidecar date", func(t *testing.T) {
		t.Parallel()
		raw := original("IMG_3.CR2", "hraw")
		ph := photo(sidecar("hjpg"), raw)
		ph.TakenSrc = "estimate"
		a, ok, _, _ := convert(ph)
		require.True(t, ok)
		assert.True(t, a.DateTaken.IsZero())
	})

	// With no JPEG to fall back on, the RAW goes through as itself, for filterSyncAssets to
	// warn about once along with every other unsupported file.
	t.Run("a RAW with no JPEG is left to the shared filter", func(t *testing.T) {
		t.Parallel()
		a, ok, w, _ := convert(photo(primary(original("IMG_4.CR2", "hraw"))))
		require.True(t, ok)
		assert.Empty(t, w)
		assert.Equal(t, "IMG_4.CR2", a.FileName)
		assert.False(t, a.Edited)
		assert.Empty(t, filterSyncAssets([]SyncAsset{a}, func(string, ...any) {}))
	})

	// A file indexed in place keeps its own name, and has no upload name.
	t.Run("a file with no upload name uses its own", func(t *testing.T) {
		t.Parallel()
		f := primary(original("", "h5"))
		f.Name = "trips/spain/IMG_5.jpg"
		a, ok, _, _ := convert(photo(f))
		require.True(t, ok)
		assert.Equal(t, "IMG_5.jpg", a.FileName)
	})
}

func TestPhotoPrismProviderFetch(t *testing.T) {
	t.Parallel()

	fetch := func(t *testing.T, p *photoprismProvider, a SyncAsset) (string, error) {
		t.Helper()
		rc, err := p.Fetch(context.Background(), a)
		if err != nil {
			return "", err
		}
		//goland:noinspection GoUnhandledErrorResult
		defer rc.Close() //nolint:errcheck
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		return string(body), nil
	}

	t.Run("it streams the file by hash with the listing's token", func(t *testing.T) {
		t.Parallel()
		s := &photoprismServer{t: t, pages: photoprismFixturePages(t, ""),
			media: map[string]string{"ea56d78ceadc2a11f58851b5f078957fe7b2ce15": "the original bytes"}}
		p, _ := s.provider(s.start())
		assets, err := p.Assets(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)

		body, err := fetch(t, p, assets[0])
		require.NoError(t, err)
		assert.Equal(t, "the original bytes", body)
		assert.Equal(t, 1, s.listings, "the listing's token was used; none was fetched")
	})

	t.Run("a RAW downloads PhotoPrism's JPEG", func(t *testing.T) {
		t.Parallel()
		s := &photoprismServer{t: t, pages: photoprismFixturePages(t, ""),
			media: map[string]string{"hjpg": "the jpeg bytes", "hraw": "the raw bytes"}}
		p, _ := s.provider(s.start())
		_, err := p.Assets(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)
		p.converted["pt-raw"] = "hjpg"

		body, err := fetch(t, p, SyncAsset{ID: "pt-raw", FileName: "IMG_3.jpg", Checksum: "hraw", Edited: true})
		require.NoError(t, err)
		assert.Equal(t, "the jpeg bytes", body)
	})

	// Tokens expire after about an hour, and a big album's downloads can outlast that.
	t.Run("an expired token is refreshed once and the download retried", func(t *testing.T) {
		t.Parallel()
		s := &photoprismServer{t: t, pages: photoprismFixturePages(t, ""), expireTokens: 1,
			media: map[string]string{"h1": "bytes"}}
		p, _ := s.provider(s.start())
		_, err := p.Assets(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)

		body, err := fetch(t, p, SyncAsset{ID: "pt1", Checksum: "h1"})
		require.NoError(t, err)
		assert.Equal(t, "bytes", body)
		assert.Equal(t, 2, s.listings, "one refresh")
		assert.Equal(t, "1", s.queries[1].Get("count"), "the refresh is a one-row listing")
		assert.Equal(t, []string{"h1", "h1"}, s.downloads)
	})

	// Four downloads refused at once must not mean four refreshes.
	t.Run("concurrent refusals share one refresh", func(t *testing.T) {
		t.Parallel()
		s := &photoprismServer{t: t, pages: photoprismFixturePages(t, ""), expireTokens: 1,
			media: map[string]string{"h1": "bytes"}}
		p, _ := s.provider(s.start())
		_, err := p.Assets(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := fetch(t, p, SyncAsset{ID: "pt1", Checksum: "h1"})
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
		assert.Equal(t, 2, s.listings)
	})

	t.Run("a token refused twice is an error that never shows the token", func(t *testing.T) {
		t.Parallel()
		s := &photoprismServer{t: t, pages: photoprismFixturePages(t, ""), expireTokens: 100}
		p, _ := s.provider(s.start())
		_, err := p.Assets(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)

		_, err = fetch(t, p, SyncAsset{ID: "pt1", Checksum: "h1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "403")
		assert.Contains(t, err.Error(), "download token")
		assert.NotContains(t, err.Error(), "dl-token", "a token is a credential")
		assert.NotContains(t, err.Error(), "<svg", "the broken-image body is not a message")
		assert.Equal(t, 2, s.listings, "one refresh, not a loop")
	})

	t.Run("a file photoprism does not have is an error", func(t *testing.T) {
		t.Parallel()
		s := &photoprismServer{t: t, pages: photoprismFixturePages(t, "")}
		p, _ := s.provider(s.start())
		_, err := p.Assets(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)

		_, err = fetch(t, p, SyncAsset{ID: "pt1", Checksum: "missing"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
		assert.Contains(t, err.Error(), "no file for this download")
	})
}

// Not parallel: the retry tests shrink syncRetryBackoff.
func TestPhotoPrismProviderErrors(t *testing.T) {
	restore := syncRetryBackoff
	syncRetryBackoff = time.Millisecond
	t.Cleanup(func() { syncRetryBackoff = restore })

	t.Run("a 401 names where the password came from but never the password", func(t *testing.T) {
		s := &photoprismServer{t: t, status: http.StatusUnauthorized, body: `{"code":401,"error":"Unauthorized"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), photoprismTestAlbum)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Unauthorized")
		assert.Contains(t, err.Error(), photoprismPasswordEnv)
		assert.Contains(t, err.Error(), PhotoPrismEnvFileName)
		assert.NotContains(t, err.Error(), "test-password")
		assert.Equal(t, 1, s.calls)
	})

	// The recorded answer for a UID of the right shape that names no album.
	t.Run("a 404 album points at sync.album_id", func(t *testing.T) {
		s := &photoprismServer{t: t, status: http.StatusNotFound,
			body: `{"code":404,"error":"Album not found","messageId":"Album not found"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), photoprismTestAlbum)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Album not found")
		assert.Contains(t, err.Error(), "sync.album_id")
		assert.Equal(t, 1, s.calls)
	})

	t.Run("a 403 on the API says what access the password needs", func(t *testing.T) {
		s := &photoprismServer{t: t, status: http.StatusForbidden, body: `{"error":"Permission denied"}`}
		p, _ := s.provider(s.start())
		_, err := p.Album(context.Background(), photoprismTestAlbum)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Permission denied")
		assert.Contains(t, err.Error(), "Full Access")
	})

	t.Run("a 503 is retried and the retry is used", func(t *testing.T) {
		s := &photoprismServer{t: t, failFirst: 2, album: photoprismFixture(t, "album.json")}
		p, _ := s.provider(s.start())
		album, err := p.Album(context.Background(), photoprismTestAlbum)
		require.NoError(t, err)
		assert.Equal(t, "The Way", album.Name)
		assert.Equal(t, syncHTTPAttempts, s.calls)
	})
}

// Not parallel: t.Setenv, which is also the point — this is the only test that goes through
// credential resolution rather than building the provider by hand.
func TestNewPhotoPrismProvider(t *testing.T) {
	t.Setenv(photoprismPasswordEnv, "env-pw")
	t.Setenv(photoprismURLEnv, "http://localhost:2342/api/v1")

	p, err := newPhotoPrismProvider(&PhotoPrismSyncConfig{EnvFile: filepath.Join(t.TempDir(), "absent.env")}, nil)
	require.NoError(t, err)
	assert.IsType(t, &photoprismProvider{}, p)

	_, err = newPhotoPrismProvider(nil, nil)
	require.NoError(t, err)

	t.Run("a missing password fails before anything is downloaded", func(t *testing.T) {
		t.Setenv(photoprismPasswordEnv, "")
		_, err := newPhotoPrismProvider(&PhotoPrismSyncConfig{EnvFile: filepath.Join(t.TempDir(), "absent.env")}, nil)
		require.ErrorIs(t, err, errNoSyncCredentials)
	})
}

// The whole chain with the real provider and the recorded album: credentials off disk,
// PhotoPrism's own file choice, then the shared naming, download, caption merge and prune.
func TestPhotoPrismFullSync(t *testing.T) {
	t.Parallel()

	pages := photoprismFixturePages(t, "")
	var photos []map[string]any
	require.NoError(t, json.Unmarshal(pages[0].body, &photos))
	media := map[string]string{}
	for _, ph := range photos {
		for _, f := range ph["Files"].([]any) {
			hash := f.(map[string]any)["Hash"].(string)
			media[hash] = "bytes of " + hash
		}
	}
	s := &photoprismServer{t: t, album: photoprismFixture(t, "album.json"), pages: pages, media: media}
	srv := s.start()

	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, PhotoPrismEnvFileName),
		[]byte("PHOTOPRISM_APP_PASSWORD=test-password\nPHOTOPRISM_SITE_URL="+srv.URL+"/library\n"), 0o644))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stale.jpg"), []byte("old"), 0o644))

	ac := &AlbumConfig{Slug: "the-way", Path: dir, Sync: &AlbumSyncConfig{
		Provider: photoprismProviderName, AlbumID: photoprismTestAlbum, Captions: true,
		PhotoPrism: &PhotoPrismSyncConfig{EnvFile: filepath.Join(configDir, PhotoPrismEnvFileName)},
	}}
	wc := &WarnCollector{}
	require.NoError(t, RunSync(context.Background(), &Config{Warn: wc}, []*AlbumConfig{ac}, false))
	assert.Empty(t, wc.warnings)

	assert.Equal(t, "The Way", ac.Name)
	assert.Equal(t, "530 mile walk across Northern Spain", ac.Description)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	assert.Len(t, names, 23+2, "23 photos, metadata.yaml and photogen.txt")
	assert.False(t, names["stale.jpg"], "pruned")
	for _, want := range []string{"IMG_9728.heic", "ocean.mov", "2024-The-Way-38.avif", "2024-The-Way-3.jpg"} {
		assert.True(t, names[want], want)
	}

	got, err := os.ReadFile(filepath.Join(dir, "IMG_9728.heic"))
	require.NoError(t, err)
	assert.Equal(t, "bytes of ed968ba4e3a84b291fa66d88e34b23674f5d3438", string(got))

	captions, err := os.ReadFile(filepath.Join(dir, photogenFileName))
	require.NoError(t, err)
	assert.Contains(t, string(captions), "\n2024-The-Way-10.jpg Matching wine and buildings\n")
	assert.Contains(t, string(captions), "\nIMG_9728.heic\n", "every synced file is listed, in upstream order")

	meta, err := loadSyncMetadata(dir)
	require.NoError(t, err)
	assert.Equal(t, photoprismProviderName, meta.Provider)
	assert.Len(t, meta.Photos, 23)

	// A second run downloads nothing: every checksum matches.
	downloads := len(s.downloads)
	require.NoError(t, RunSync(context.Background(), &Config{Warn: &WarnCollector{}}, []*AlbumConfig{ac}, false))
	assert.Equal(t, downloads, len(s.downloads), "a re-run is a no-op")
}
