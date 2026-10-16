package photogen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This is the PhotoPrism client: the only part of syncing that knows PhotoPrism exists.
//
// It decides what needs PhotoPrism's own fields: whether a photo is private, hidden or
// archived, and which of a photo's files is the one to download. PhotoPrism keeps every file
// of a photo together (a HEIC original and the JPEG it converted it to, a video and its
// poster frame, a RAW and its JPEG), so choosing one is the provider's real work here.
// Unsupported extensions and photo-vs-video base-name clashes are decided by filterSyncAssets
// in sync.go, which every provider shares, and are deliberately not repeated here.
//
// Written against PhotoPrism 261007-65faaae5d, whose recorded responses are in
// testdata/photoprism.

const (
	// PhotoPrismEnvFileName is where the credentials live, beside albums.yaml in the config
	// directory, so secrets stay out of the YAML.
	PhotoPrismEnvFileName = "photoprism.env"

	// photoprismPasswordEnv and photoprismURLEnv are read from the real environment first,
	// then from PhotoPrismEnvFileName, the same precedence as Immich's.
	photoprismPasswordEnv = "PHOTOPRISM_APP_PASSWORD"
	photoprismURLEnv      = "PHOTOPRISM_SITE_URL"

	// photoprismPageSize is "count" in the search request. It counts file rows, not photos:
	// see Assets.
	photoprismPageSize = 500
)

// photoprismUpstream is PhotoPrism as the shared HTTP plumbing (provider_http.go) sees it.
var photoprismUpstream = &httpUpstream{
	Display:     "PhotoPrism",
	EnvFileName: PhotoPrismEnvFileName,
	SecretEnv:   photoprismPasswordEnv,
	URLEnv:      photoprismURLEnv,
	SecretHelp:  "PhotoPrism: Settings > Account > Apps and Devices > Add, with Full Access",
	URLExample:  "http://localhost:2342",
	// The site URL is what PhotoPrism's own PHOTOPRISM_SITE_URL holds; the API lives under
	// /api/v1, and the browser shows /library.
	APISuffixes: []string{"api/v1", "api", "library"},
}

// loadPhotoPrismCredentials resolves the app password and site URL; see loadSyncCredentials.
func loadPhotoPrismCredentials(envFile string) (syncCredentials, error) {
	return loadSyncCredentials(photoprismUpstream, envFile)
}

// LoadPhotoPrismCredentials resolves the app password and normalized base URL exactly as the
// photoprism provider does, for cmd/photoprism-record.
func LoadPhotoPrismCredentials(envFile string) (appPassword, baseURL string, err error) {
	creds, err := loadPhotoPrismCredentials(envFile)
	return creds.Secret, creds.BaseURL, err
}

// normalizePhotoPrismURL normalizes a PhotoPrism URL; see normalizeSyncURL.
func normalizePhotoPrismURL(raw string) (normalized string, dockerRewritten bool, err error) {
	return normalizeSyncURL(photoprismUpstream, raw)
}

// photoprismAlbumIDPattern is PhotoPrism's album UID: an "a" prefix and 15 lowercase
// alphanumerics, as rnd.GenerateUID makes them.
var photoprismAlbumIDPattern = regexp.MustCompile(`^a[0-9a-z]{15}$`)

// validatePhotoPrismAlbumID checks an album_id while the config is being read, for the same
// reason validateImmichAlbumID does: a typo should fail before the first album downloads.
func validatePhotoPrismAlbumID(slug, albumID string) error {
	if photoprismAlbumIDPattern.MatchString(albumID) {
		return nil
	}
	return fmt.Errorf("album %q: sync.album_id %q is not a PhotoPrism album UID — "+
		"it is the path segment after /albums/ in the album's URL in PhotoPrism, for example "+
		"atmyuuj99diyoq1s in http://localhost:2342/library/albums/atmyuuj99diyoq1s/the-way",
		slug, albumID)
}

// photoprismProvider is one album's client. warnf carries the album slug, which is why it is
// not shared between albums; the HTTP client underneath it is.
type photoprismProvider struct {
	http  *syncHTTP
	warnf func(string, ...any)

	// albumID is remembered by Assets so a download token can be refreshed later.
	albumID string
	// converted maps a photo UID to the hash of the PhotoPrism-made JPEG Fetch serves for it,
	// for a RAW photo published that way. Written by Assets before any Fetch, then only read.
	converted map[string]string

	// mu guards token. Downloads need a "?t=" token rather than the app password, which
	// PhotoPrism hands out on search responses and which expires after an hour or so, while
	// Fetch runs on four goroutines. refreshMu is held across a refresh, so downloads refused
	// together wait for one new token rather than each fetching their own.
	mu        sync.Mutex
	token     string
	refreshMu sync.Mutex
}

// newPhotoPrismProvider loads the credentials and settles the URL up front, so a
// misconfigured instance fails before anything is downloaded or pruned.
func newPhotoPrismProvider(cfg *PhotoPrismSyncConfig, warnf func(string, ...any)) (SyncProvider, error) {
	envFile := ""
	if cfg != nil {
		envFile = cfg.EnvFile
	}
	creds, err := loadPhotoPrismCredentials(envFile)
	if err != nil {
		return nil, err
	}
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	return &photoprismProvider{http: newPhotoPrismHTTP(creds), warnf: warnf}, nil
}

// newPhotoPrismHTTP wires credentials to the shared plumbing. An app password is sent as a
// bearer token, exactly like a session token.
func newPhotoPrismHTTP(creds syncCredentials) *syncHTTP {
	return &syncHTTP{
		up:    photoprismUpstream,
		creds: creds,
		authorize: func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+creds.Secret)
		},
		statusError: photoprismStatusError,
	}
}

// photoprismAlbumResponse is GET /api/v1/albums/{uid}, cut down to what sync reads.
type photoprismAlbumResponse struct {
	Title       string `json:"Title"`
	Description string `json:"Description"`
}

// Album returns the upstream album's title and description. The shared layer escapes both.
func (p *photoprismProvider) Album(ctx context.Context, albumID string) (SyncAlbum, error) {
	var resp photoprismAlbumResponse
	if _, err := p.http.getJSON(ctx, "/api/v1/albums/"+url.PathEscape(albumID), &resp); err != nil {
		return SyncAlbum{}, err
	}
	return SyncAlbum{Name: resp.Title, Description: resp.Description}, nil
}

// photoprismPhoto is one row of GET /api/v1/photos?merged=true, cut down to what sync reads.
type photoprismPhoto struct {
	UID  string `json:"UID"`
	Type string `json:"Type"` // image | raw | live | video | animated | vector | document
	// Caption is the user's text. Title is not used: PhotoPrism generates one from labels
	// and places ("Cow / Etxebarria / 2024") for every photo that has none.
	Caption      string `json:"Caption"`
	TakenAtLocal string `json:"TakenAtLocal"`
	TakenSrc     string `json:"TakenSrc"` // meta | name | manual | estimate | auto | ...
	// Quality -1 is how PhotoPrism marks a photo hidden (broken or unreadable files).
	Quality   int     `json:"Quality"`
	Private   bool    `json:"Private"`
	DeletedAt *string `json:"DeletedAt"`
	UpdatedAt string  `json:"UpdatedAt"`

	Files []photoprismFile `json:"Files"`
}

// photoprismFile is one file of a photo.
type photoprismFile struct {
	UID string `json:"UID"`
	// Name is the path under Root. Root "/" is the originals folder; "sidecar" is where
	// PhotoPrism keeps what it generated, such as the JPEG of a HEIC or RAW.
	Name string `json:"Name"`
	Root string `json:"Root"`
	// Hash is a hex SHA1 of the file, and is also how it is downloaded.
	Hash         string `json:"Hash"`
	Size         int64  `json:"Size"`
	Primary      bool   `json:"Primary"`
	Video        bool   `json:"Video"`
	MediaType    string `json:"MediaType"` // image | raw | video | sidecar | ...
	OriginalName string `json:"OriginalName"`
	Missing      bool   `json:"Missing"`
	Error        string `json:"Error"`
}

const (
	photoprismRootOriginals = "/"
	photoprismTypeVideo     = "video"
	photoprismTakenFromMeta = "meta"
)

// Assets lists the album.
//
// merged=true is what makes each row carry its Files, but PhotoPrism pages it by file row,
// not by photo: count and offset count files, X-Count is the number of file rows in this
// page, and a photo whose files straddle a page boundary comes back on both pages with half
// its files each. So the offset advances by the page size, paging stops when a page holds
// fewer rows than asked for, and rows are merged by photo UID.
func (p *photoprismProvider) Assets(ctx context.Context, albumID string) ([]SyncAsset, error) {
	return p.assets(ctx, albumID, photoprismPageSize)
}

// assets is Assets with the page size as a parameter, so a small recorded album can stand in
// for a multipage one.
func (p *photoprismProvider) assets(ctx context.Context, albumID string, pageSize int) ([]SyncAsset, error) {
	p.albumID = albumID
	p.converted = map[string]string{}

	var order []string
	photos := map[string]*photoprismPhoto{}
	for offset := 0; ; offset += pageSize {
		page, rows, err := p.searchPage(ctx, albumID, pageSize, offset)
		if err != nil {
			return nil, err
		}
		for _, ph := range page {
			if seen, ok := photos[ph.UID]; ok {
				seen.Files = mergePhotoPrismFiles(seen.Files, ph.Files)
				continue
			}
			ph := ph
			photos[ph.UID] = &ph
			order = append(order, ph.UID)
		}
		if rows < pageSize {
			break
		}
	}

	var out []SyncAsset
	for _, uid := range order {
		if asset, ok := p.convertPhoto(*photos[uid]); ok {
			out = append(out, asset)
		}
	}
	return out, nil
}

// searchPage fetches one page of the album listing and keeps the download token it carries.
// rows is the number of file rows PhotoPrism counted for the page.
func (p *photoprismProvider) searchPage(ctx context.Context, albumID string, count, offset int) ([]photoprismPhoto, int, error) {
	q := url.Values{}
	q.Set("s", albumID)
	q.Set("merged", "true")
	q.Set("order", "oldest")
	q.Set("count", strconv.Itoa(count))
	q.Set("offset", strconv.Itoa(offset))

	var page []photoprismPhoto
	header, err := p.http.getJSON(ctx, "/api/v1/photos?"+q.Encode(), &page)
	if err != nil {
		return nil, 0, err
	}
	if t := header.Get("X-Download-Token"); t != "" {
		p.mu.Lock()
		p.token = t
		p.mu.Unlock()
	}

	// X-Count is the page's file rows. Should it ever go missing, counting the files
	// returned is the same number.
	rows := 0
	if n, err := strconv.Atoi(header.Get("X-Count")); err == nil {
		rows = n
	} else {
		for _, ph := range page {
			rows += len(ph.Files)
		}
	}
	return page, rows, nil
}

// mergePhotoPrismFiles adds the files of a photo's later row that its earlier row lacked.
func mergePhotoPrismFiles(have, more []photoprismFile) []photoprismFile {
	seen := make(map[string]bool, len(have))
	for _, f := range have {
		seen[f.UID] = true
	}
	for _, f := range more {
		if !seen[f.UID] {
			have = append(have, f)
			seen[f.UID] = true
		}
	}
	return have
}

// convertPhoto maps one PhotoPrism photo to a SyncAsset, or reports that it is being skipped.
//
// A skipped photo is warned about here rather than returned with a note, because
// filterSyncAssets only prints the warnings of assets it is handed.
func (p *photoprismProvider) convertPhoto(ph photoprismPhoto) (SyncAsset, bool) {
	file, converted := pickPhotoPrismFile(ph)
	name := ph.UID
	if file != nil {
		name = photoprismFileName(*file)
	}

	// Archived photos should never arrive: search excludes them unless asked. Checked anyway
	// because publishing something the user archived is the worst outcome available here.
	if ph.DeletedAt != nil && *ph.DeletedAt != "" {
		p.warnf("WARN: skipping %s: it is archived in PhotoPrism\n", name)
		return SyncAsset{}, false
	}
	// Private is PhotoPrism's "do not share this", and a DD Photos site is shared by
	// definition.
	if ph.Private {
		p.warnf("WARN: skipping %s: it is marked private in PhotoPrism\n", name)
		return SyncAsset{}, false
	}
	if ph.Quality < 0 {
		p.warnf("WARN: skipping %s: PhotoPrism has it hidden (its files could not be read)\n", name)
		return SyncAsset{}, false
	}
	if file == nil {
		p.warnf("WARN: skipping %s: PhotoPrism has no downloadable file for it\n", name)
		return SyncAsset{}, false
	}

	asset := SyncAsset{
		ID:       ph.UID,
		FileName: name,
		Caption:  ph.Caption,
		Checksum: file.Hash,
		Size:     file.Size,
	}
	if ph.UpdatedAt != "" {
		// Only the third-choice signal in haveSyncAsset, behind the checksum PhotoPrism
		// always supplies, so an unparseable one is not worth failing over.
		if t, err := time.Parse(time.RFC3339, ph.UpdatedAt); err == nil {
			asset.UpdatedAt = t
		}
	}

	if converted != nil {
		// A RAW photo publishes PhotoPrism's JPEG of it, which is what the shared layer
		// calls an edited rendition: Fetch serves something other than the original, so
		// Checksum and Size stay the original's, and the date travels in a sidecar.
		asset.Edited = true
		asset.FileName = strings.TrimSuffix(name, path.Ext(name)) + path.Ext(converted.Name)
		if p.converted != nil {
			p.converted[ph.UID] = converted.Hash
		}
		if ph.TakenSrc == photoprismTakenFromMeta && ph.TakenAtLocal != "" {
			if t, err := time.Parse(time.RFC3339, ph.TakenAtLocal); err == nil {
				asset.DateTaken = t.UTC().Truncate(time.Second)
			}
		}
	}
	return asset, true
}

// pickPhotoPrismFile chooses what to download for a photo, preferring an original.
//
//   - A video publishes its video original, never the poster frame PhotoPrism made of it.
//   - A still publishes its primary file when that is an original, which covers the plain
//     JPEG case. Otherwise, it publishes the first original photogen can read: the HEIC or
//     AVIF whose primary is PhotoPrism's JPEG of it. A Live Photo's video is passed over, since
//     the still is what the album is for.
//   - A RAW with no readable original returns that RAW as file and PhotoPrism's JPEG of it as
//     converted, which is what gets downloaded.
//
// Missing and errored files never qualify. Nil file means there is nothing to download.
func pickPhotoPrismFile(ph photoprismPhoto) (file, converted *photoprismFile) {
	var originals []*photoprismFile
	var primary *photoprismFile
	for i := range ph.Files {
		f := &ph.Files[i]
		if f.Missing || f.Error != "" {
			continue
		}
		if f.Primary {
			primary = f
		}
		if f.Root == photoprismRootOriginals {
			originals = append(originals, f)
		}
	}

	if ph.Type == photoprismTypeVideo {
		for _, f := range originals {
			if f.Video || f.MediaType == photoprismTypeVideo {
				return f, nil
			}
		}
		return nil, nil
	}

	if primary != nil && primary.Root == photoprismRootOriginals {
		return primary, nil
	}
	for _, f := range originals {
		if IsPhotoFile(photoprismFileName(*f)) {
			return f, nil
		}
	}
	// No original photogen can read. If PhotoPrism has a JPEG of a still original, that
	// JPEG is the best rendition there is.
	for _, f := range originals {
		if f.Video || f.MediaType == photoprismTypeVideo {
			continue
		}
		if primary != nil && IsPhotoFile(primary.Name) {
			return f, primary
		}
		return f, nil
	}
	return nil, nil
}

// photoprismFileName is a file's name as the user knows it: the name it was uploaded under,
// or, for a file indexed in place, its own name. PhotoPrism renames uploads to
// YYYY/MM/YYYYMMDD_HHMMSS_<crc>.ext, so the stored name is the fallback, not the first choice.
func photoprismFileName(f photoprismFile) string {
	if f.OriginalName != "" {
		return path.Base(f.OriginalName)
	}
	return path.Base(f.Name)
}

// Fetch opens one asset's bytes by file hash. The caller closes the reader.
//
// Downloads authenticate with a "?t=" token rather than the app password, which PhotoPrism
// does not accept on /dl. The token comes from the last listing and expires, so a 403 gets one
// fresh token and one more try.
func (p *photoprismProvider) Fetch(ctx context.Context, a SyncAsset) (io.ReadCloser, error) {
	hash := a.Checksum
	if h, ok := p.converted[a.ID]; ok && a.Edited {
		hash = h
	}

	token, err := p.downloadToken(ctx, "")
	if err != nil {
		return nil, err
	}
	rc, err := p.http.fetch(ctx, p.downloadPath(hash, token))
	var se *photoprismStatus
	if errors.As(err, &se) && se.code == http.StatusForbidden {
		if token, err = p.downloadToken(ctx, token); err != nil {
			return nil, err
		}
		rc, err = p.http.fetch(ctx, p.downloadPath(hash, token))
	}
	return rc, err
}

func (p *photoprismProvider) downloadPath(hash, token string) string {
	return "/api/v1/dl/" + url.PathEscape(hash) + "?t=" + url.QueryEscape(token)
}

// downloadToken returns the current download token, fetching a new one when there is none
// or when the current one is stale (the caller just had it refused). Several downloads can
// be refused at once; only the first refreshes, and the rest pick up its token.
func (p *photoprismProvider) downloadToken(ctx context.Context, stale string) (string, error) {
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	if t := p.currentToken(); t != "" && t != stale {
		return t, nil
	}
	if p.albumID == "" {
		return "", errors.New("photoprism: no download token, because the album was never listed")
	}

	// The cheapest call that hands out a token is a one-row listing of the album itself.
	if _, _, err := p.searchPage(ctx, p.albumID, 1, 0); err != nil {
		return "", fmt.Errorf("get a PhotoPrism download token: %w", err)
	}
	if t := p.currentToken(); t != "" && t != stale {
		return t, nil
	}
	return "", errors.New("PhotoPrism sent no download token: the app password may lack download access " +
		"(create it with Full Access)")
}

func (p *photoprismProvider) currentToken() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.token
}

// photoprismStatus is a non-2xx response, kept typed so Fetch can tell a refused download
// token from any other failure.
type photoprismStatus struct {
	code int
	msg  string
}

func (e *photoprismStatus) Error() string { return e.msg }

// photoprismStatusError turns a non-2xx response into a message worth reading, and closes the
// body.
//
// PhotoPrism's API errors are {"error": "...", "code": N}. Its download endpoint answers every
// failure with a broken-image SVG instead, deliberately the same for an unknown file as for a
// forbidden one, so a download failure has only its status to go on.
func photoprismStatusError(resp *http.Response, method, path string) error {
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	msg := ""
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &payload); err == nil && payload.Error != "" {
		msg = payload.Error
	} else if !strings.Contains(resp.Header.Get("Content-Type"), "svg") {
		msg = strings.TrimSpace(string(data))
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}

	path = stripQuery(path)
	download := strings.HasPrefix(path, "/api/v1/dl/")
	var text string
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		// The password itself is never named, only where it came from.
		text = fmt.Sprintf("photoprism rejected the app password (401: %s) — check %s in the environment or %s",
			msg, photoprismPasswordEnv, PhotoPrismEnvFileName)
	case resp.StatusCode == http.StatusForbidden && download:
		text = fmt.Sprintf("photoprism refused the download (403: %s) — the download token was not accepted, "+
			"or the app password lacks download access", msg)
	case resp.StatusCode == http.StatusForbidden:
		text = fmt.Sprintf("the photoprism app password lacks a required permission (403: %s) — "+
			"create it with Full Access", msg)
	case resp.StatusCode == http.StatusNotFound && download:
		text = fmt.Sprintf("photoprism has no file for this download, or it is not visible to the app password "+
			"(404: %s)", msg)
	case resp.StatusCode == http.StatusNotFound:
		text = fmt.Sprintf("photoprism has no album for this request (404: %s) — check sync.album_id", msg)
	default:
		text = fmt.Sprintf("%s %s failed (%d: %s)", method, path, resp.StatusCode, msg)
	}
	return &photoprismStatus{code: resp.StatusCode, msg: text}
}
