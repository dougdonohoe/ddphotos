package photogen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
)

// This is the Immich client: the only part of syncing that knows Immich exists.
//
// It decides exactly three things about an asset, because those are the three that need
// Immich's own fields: whether it is trashed, what its visibility is, and whether it was
// edited upstream. Unsupported extensions, RAW included, and photo-vs-video base-name clashes
// are decided by filterSyncAssets in sync.go, which every provider shares, and are
// deliberately not repeated here.
//
// Written against Immich v3.2.2, whose recorded responses are in testdata/immich.

const (
	// ImmichEnvFileName is where the credentials live, beside albums.yaml in the config
	// directory, so secrets stay out of the YAML.
	ImmichEnvFileName = "immich.env"

	// immichAPIKeyEnv and immichURLEnv are read from the real environment first, then from
	// ImmichEnvFileName, matching how DDPHOTOS_ALBUMS_DIR resolves. CI and Docker runs
	// therefore need no secrets file on disk.
	immichAPIKeyEnv = "IMMICH_API_KEY"
	immichURLEnv    = "IMMICH_INSTANCE_URL"

	// inDockerEnv is set by docker/Dockerfile. photogen has no other way to tell, and it
	// needs to know because localhost inside the container is the container.
	inDockerEnv = "DDPHOTOS_IN_DOCKER"

	// dockerHostAlias is the name Docker resolves to the host machine. Docker Desktop
	// provides it natively; Docker Engine on Linux needs --add-host, which docker/ddphotos
	// passes.
	dockerHostAlias = "host.docker.internal"

	// immichPageSize is "size" in the search request.
	immichPageSize = 500

	// immichJSONTimeout bounds a metadata call. It is a whole-request deadline, which is
	// only safe because these responses are small and read in full before returning.
	immichJSONTimeout = 30 * time.Second

	// immichFetchTimeout bounds one asset download. Generous rather than tight: the body is
	// streamed by downloadSyncAsset, and a large video on a slow link is not a failure.
	immichFetchTimeout = 15 * time.Minute

	// immichAttempts is how many times one request is tried in total. Retries cover
	// connection errors, 5xx and 429 only; see immichShouldRetry.
	immichAttempts = 3
)

// immichRetryBackoff is the pause before the second attempt; it doubles after that. A var
// rather than a const only so the retry tests do not spend three real seconds waiting.
var immichRetryBackoff = time.Second

// immichHTTPClient is shared by every album's provider. http.Client is goroutine-safe, and
// runPool calls Fetch from four goroutines at once, so pooling connections here is the point.
//
// Client.Timeout is deliberately unset: it covers reading the body, and Fetch hands its body
// to the caller unread. Per-request contexts provide the deadlines instead, and
// ResponseHeaderTimeout catches a server that accepts a connection and then says nothing.
var immichHTTPClient = &http.Client{
	Transport: func() http.RoundTripper {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.ResponseHeaderTimeout = immichJSONTimeout
		return t
	}(),
}

// dockerRewriteNotice keeps the localhost-rewrite line to one per run. One Immich instance is
// assumed, so one notice is right however many albums sync.
var dockerRewriteNotice sync.Once

// errImmichNoCredentials is returned when neither the environment nor the env file supplies
// what is needed. Tests match on it with errors.Is, so the wording that wraps it is free to
// change.
var errImmichNoCredentials = errors.New("immich credentials not configured")

// immichCredentials is what the provider needs to talk to an instance.
type immichCredentials struct {
	// APIKey is sent as the x-api-key header. It is never logged, never put in an error,
	// and never written to metadata.yaml.
	APIKey string
	// BaseURL is normalized: scheme, host, optional port and optional path prefix, with no
	// trailing slash and no trailing /api. Request paths are appended to it.
	BaseURL string
	// DockerSourceURL is the URL as configured, set only when the Docker rewrite changed
	// it. It is kept so a connection failure can name what was configured as well as what
	// was actually tried, which is the difference between a baffling error and an obvious
	// one for someone who wrote localhost and meant their laptop.
	DockerSourceURL string
}

// loadImmichCredentials resolves the API key and instance URL.
//
// The real environment wins over the file, which is the same precedence DDPHOTOS_ALBUMS_DIR
// has and what config/albums.example.yaml promises. A missing file is therefore not an error
// on its own: it is only an error if the environment did not supply the value either.
func loadImmichCredentials(envFile string) (immichCredentials, error) {
	var creds immichCredentials

	fileVals := map[string]string{}
	if envFile != "" {
		vals, err := ParseEnvFile(envFile)
		switch {
		case err == nil:
			fileVals = vals
		case os.IsNotExist(err):
			// Fine: the environment may have everything.
		default:
			return creds, fmt.Errorf("read %s: %w", envFile, err)
		}
	}

	lookup := func(key string) string {
		if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return fileVals[key]
	}

	where := ImmichEnvFileName
	if envFile != "" {
		where = envFile
	}

	creds.APIKey = lookup(immichAPIKeyEnv)
	if creds.APIKey == "" {
		return creds, fmt.Errorf("%w: set %s in the environment or in %s "+
			"(Immich: Account Settings > API Keys, with asset.read, asset.download and album.read)",
			errImmichNoCredentials, immichAPIKeyEnv, where)
	}

	rawURL := lookup(immichURLEnv)
	if rawURL == "" {
		return creds, fmt.Errorf("%w: set %s in the environment or in %s (for example http://localhost:2283)",
			errImmichNoCredentials, immichURLEnv, where)
	}
	baseURL, dockerRewritten, err := normalizeImmichURL(rawURL)
	if err != nil {
		return creds, fmt.Errorf("%s: %w", immichURLEnv, err)
	}
	creds.BaseURL = baseURL
	if dockerRewritten {
		creds.DockerSourceURL = rawURL
	}
	return creds, nil
}

// localHostNames are the hosts that mean "this machine", and therefore mean the container when
// photogen runs inside one.
var localHostNames = map[string]struct{}{
	"localhost": {}, "127.0.0.1": {}, "::1": {}, "0.0.0.0": {},
}

// normalizeImmichURL turns what someone pasted into a base URL requests can be appended to.
//
// One function owns every adjustment, so there is one place to look when a URL misbehaves:
//
//   - a trailing slash and a trailing /api are dropped, because people paste what the browser
//     shows and the API docs show /api;
//   - any other path is kept, so an instance behind a reverse proxy at https://host/immich
//     still works;
//   - inside Docker, a loopback host is rewritten to host.docker.internal, because otherwise
//     localhost is the photogen container rather than the machine Immich runs on.
func normalizeImmichURL(raw string) (normalized string, dockerRewritten bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, errors.New("is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("%q is not a valid URL: %w", raw, err)
	}
	// A bare "localhost:2283" parses as scheme "localhost", opaque "2283", which is the
	// likeliest paste, so say what to do rather than what went wrong.
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false, fmt.Errorf("%q needs an http:// or https:// prefix", raw)
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("%q has no host", raw)
	}

	p := strings.TrimSuffix(u.Path, "/")
	if strings.EqualFold(path.Base(p), "api") {
		p = strings.TrimSuffix(p, path.Base(p))
		p = strings.TrimSuffix(p, "/")
	}
	u.Path = p
	u.RawQuery = ""
	u.Fragment = ""

	host := u.Hostname()
	if _, local := localHostNames[host]; local && os.Getenv(inDockerEnv) != "" {
		if port := u.Port(); port != "" {
			u.Host = net.JoinHostPort(dockerHostAlias, port)
		} else {
			u.Host = dockerHostAlias
		}
		rewritten := u.String()
		dockerRewriteNotice.Do(func() {
			fmt.Printf("  note: running in Docker, so %s %q is used as %q\n",
				immichURLEnv, raw, rewritten)
		})
		return rewritten, true, nil
	}
	return u.String(), false, nil
}

// immichAlbumIDPattern is the 8-4-4-4-12 hex shape of a UUID.
//
// Deliberately looser than Immich's own check, which insists on a v4: the point here is to
// catch an id that was truncated or mistyped, which is what actually happens, without a guess
// about what Immich might issue later. An id that is well-formed but wrong still has to come
// back from the server, and does, as a plain "not found".
var immichAlbumIDPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validateImmichAlbumID checks an album_id while the config is being read.
//
// Immich answers a malformed id with 400 "Validation failed", which names neither the field
// nor the reason, and it does so one album at a time — so a run syncing three albums downloads
// two of them before reporting a typo that was sitting in the YAML the whole time. Checking
// the shape here costs nothing and fails before the first byte moves.
//
// It lives in this file rather than in albums_config.go because what an album_id looks like is
// Immich's business; SyncEntry.validate only knows to ask.
func validateImmichAlbumID(slug, albumID string) error {
	if immichAlbumIDPattern.MatchString(albumID) {
		return nil
	}
	return fmt.Errorf("album %q: sync.album_id %q is not an Immich album UUID — "+
		"it is the last path segment of the album's URL in Immich, for example "+
		"http://localhost:2283/albums/d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18",
		slug, albumID)
}

// immichProvider is one album's client. warnf carries the album slug, which is why it is not
// shared between albums; the HTTP client underneath it is.
type immichProvider struct {
	creds immichCredentials
	warnf func(string, ...any)
}

// newImmichProvider loads the credentials and settles the URL up front, so a misconfigured
// instance fails before anything is downloaded or pruned.
func newImmichProvider(cfg *ImmichSyncConfig, warnf func(string, ...any)) (SyncProvider, error) {
	envFile := ""
	if cfg != nil {
		envFile = cfg.EnvFile
	}
	creds, err := loadImmichCredentials(envFile)
	if err != nil {
		return nil, err
	}
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	return &immichProvider{creds: creds, warnf: warnf}, nil
}

func (p *immichProvider) Name() string { return immichProviderName }

// immichAlbumResponse is GET /api/albums/{id}. It does not list the album's assets, which is
// why Assets is a separate search call.
type immichAlbumResponse struct {
	AlbumName string `json:"albumName"`
	// Description is "" rather than null when unset — Immich's OpenAPI spec says so
	// explicitly, and promises null in v4, at which point this becomes a *string.
	Description string `json:"description"`
}

// Album returns the upstream album's name and description. The shared layer escapes both.
func (p *immichProvider) Album(ctx context.Context, albumID string) (SyncAlbum, error) {
	var resp immichAlbumResponse
	if err := p.getJSON(ctx, "/api/albums/"+url.PathEscape(albumID), &resp); err != nil {
		return SyncAlbum{}, err
	}
	return SyncAlbum{Name: resp.AlbumName, Description: resp.Description}, nil
}

// immichSearchRequest is the body of POST /api/search/metadata.
//
// These are the *deprecated* flat fields, deliberately. They date back to v1 and still work on
// 3.2.2, so one shape covers current and older servers, where the v3.2 replacements (filter,
// orderBy, cursor) would only work on very recent ones. The server rejects a request that
// mixes the two shapes, so this must stay flat-only. The cost is known: the flat fields are
// slated for removal in v4, which will need the new shape and probably a version probe.
type immichSearchRequest struct {
	AlbumIDs []string `json:"albumIds"`
	Order    string   `json:"order"`
	// WithExif is required, not an optimization: exifInfo is where the asset description
	// and fileSizeInByte live.
	WithExif bool `json:"withExif"`
	Size     int  `json:"size"`
	Page     int  `json:"page"`
}

// immichSearchResponse is the search reply. total is ignored: it is the count in this page
// rather than the album's total, and is deprecated besides.
type immichSearchResponse struct {
	Assets struct {
		Items []immichAsset `json:"items"`
		// NextPage is a nullable *string* ("2", "3", ... then null), not a number.
		NextPage *string `json:"nextPage"`
	} `json:"assets"`
}

// immichAsset is one asset in a search result, cut down to what sync reads.
type immichAsset struct {
	ID               string `json:"id"`
	OriginalFileName string `json:"originalFileName"`
	// Checksum is a base64-encoded SHA1 and is always present, which is what makes the
	// incremental skip in haveSyncAsset exact rather than a size guess.
	Checksum   string `json:"checksum"`
	UpdatedAt  string `json:"updatedAt"`
	Type       string `json:"type"`       // IMAGE | VIDEO | AUDIO | OTHER
	Visibility string `json:"visibility"` // timeline | archive | hidden | locked
	IsTrashed  bool   `json:"isTrashed"`
	IsEdited   bool   `json:"isEdited"`
	// ExifInfo is omitted entirely when Immich has no EXIF for the asset, so this is a
	// pointer and every field inside it is optional too.
	ExifInfo *immichExifInfo `json:"exifInfo"`
}

// immichExifInfo is the part of Immich's exifInfo sync reads. Every field in it is nullable
// on the wire, hence the pointers: a description Immich does not have is absent, not "".
//
// fileSizeInByte living here rather than on the asset is why withExif: true is required on
// the search request and not merely an optimization.
type immichExifInfo struct {
	Description    *string `json:"description"`
	FileSizeInByte *int64  `json:"fileSizeInByte"`
}

// Immich asset types and visibilities that sync cares about by name.
const (
	immichTypeVideo          = "VIDEO"
	immichVisibilityTimeline = "timeline"
	immichVisibilityArchive  = "archive"
)

// Assets lists the album, paging until Immich says there is no more.
//
// Only the three rules that need Immich's own fields are applied here. Everything else about
// which assets are publishable belongs to filterSyncAssets, and so does the asset cap, which
// is checked in syncOneAlbum after filtering. Counting here would reject an album of RAW+JPEG
// pairs at twice its publishable size.
func (p *immichProvider) Assets(ctx context.Context, albumID string) ([]SyncAsset, error) {
	var out []SyncAsset
	page := 1
	for {
		req := immichSearchRequest{
			AlbumIDs: []string{albumID},
			Order:    "asc",
			WithExif: true,
			Size:     immichPageSize,
			Page:     page,
		}
		var resp immichSearchResponse
		if err := p.postJSON(ctx, "/api/search/metadata", req, &resp); err != nil {
			return nil, err
		}
		for _, a := range resp.Assets.Items {
			if asset, ok := p.convertAsset(a); ok {
				out = append(out, asset)
			}
		}

		if resp.Assets.NextPage == nil || *resp.Assets.NextPage == "" {
			return out, nil
		}
		// A nextPage that does not move forward would loop forever, since nothing else
		// bounds the listing.
		next, err := strconv.Atoi(*resp.Assets.NextPage)
		if err != nil || next <= page {
			return nil, fmt.Errorf("unexpected nextPage %q in the search response for page %d",
				*resp.Assets.NextPage, page)
		}
		page = next
	}
}

// convertAsset maps one Immich asset to a SyncAsset, or reports that it is being skipped.
//
// A skipped asset is warned about here rather than returned with a note, because
// filterSyncAssets only prints the warnings of assets it is handed.
func (p *immichProvider) convertAsset(a immichAsset) (SyncAsset, bool) {
	name := a.OriginalFileName
	if name == "" {
		name = a.ID
	}

	// Trashed assets should never arrive: withDeleted defaults to false. Checked anyway
	// because publishing something the user deleted is the worst outcome available here.
	if a.IsTrashed {
		p.warnf("WARN: skipping %s: it is in the Immich trash\n", name)
		return SyncAsset{}, false
	}

	// The search default is visibility "not-locked", so archived and hidden assets do come
	// back. An archived asset the user put in an album is published on purpose; anything
	// else is not. Testing for the two we publish rather than the ones we do not means a
	// visibility Immich adds later is withheld rather than published by accident.
	if a.Visibility != immichVisibilityTimeline && a.Visibility != immichVisibilityArchive {
		p.warnf("WARN: skipping %s: its Immich visibility is %q\n", name, a.Visibility)
		return SyncAsset{}, false
	}

	asset := SyncAsset{
		ID:       a.ID,
		FileName: name,
		Checksum: a.Checksum,
		IsVideo:  a.Type == immichTypeVideo,
	}
	if a.ExifInfo != nil {
		if a.ExifInfo.Description != nil {
			asset.Caption = *a.ExifInfo.Description
		}
		if a.ExifInfo.FileSizeInByte != nil {
			asset.Size = *a.ExifInfo.FileSizeInByte
		}
	}
	if a.UpdatedAt != "" {
		// A date Immich sent that this cannot parse is not worth failing a run over:
		// UpdatedAt is only the third-choice signal in haveSyncAsset, behind the checksum
		// Immich always supplies.
		if t, err := time.Parse(time.RFC3339, a.UpdatedAt); err == nil {
			asset.UpdatedAt = t
		}
	}

	// Edited assets publish, with a warning. /assets/{id}/original serves the pre-edit file,
	// and the edited pixels exist only in the full-size variant, which Immich generates
	// without any metadata at all — no EXIF date means the photo sorts to the end of the
	// album, so the unedited original is the lesser of the two problems.
	if a.IsEdited {
		asset.Warnings = append(asset.Warnings,
			"edited in Immich; the site will show the unedited original")
	}
	return asset, true
}

// Fetch opens one asset's original bytes. The caller closes the reader.
//
// Always the original, for photos and videos alike: Immich strips metadata from everything it
// derives. Its full-size images are produced without keepMetadata(), and its encoded videos
// are transcoded with -map_metadata -1, so neither has the EXIF date or creation_time photogen
// sorts an album by. ?edited=true is not used for the same reason — it resolves to that same
// metadata-free full-size file.
func (p *immichProvider) Fetch(ctx context.Context, a SyncAsset) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, immichFetchTimeout)

	resp, err := p.do(ctx, http.MethodGet, "/api/assets/"+url.PathEscape(a.ID)+"/original", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	// The body outlives this function, so the context must too: cancel travels with the
	// reader and fires when the caller closes it.
	return &cancelOnCloseReader{ReadCloser: resp.Body, cancel: cancel}, nil
}

// cancelOnCloseReader releases a request's context when its body is closed.
type cancelOnCloseReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReader) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}

// getJSON and postJSON are the two metadata shapes. Both read the whole body, so they can own
// a whole-request deadline; Fetch cannot.

func (p *immichProvider) getJSON(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, immichJSONTimeout)
	defer cancel()
	resp, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return decodeJSONBody(resp, out)
}

func (p *immichProvider) postJSON(ctx context.Context, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, immichJSONTimeout)
	defer cancel()
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request for %s: %w", path, err)
	}
	resp, err := p.do(ctx, http.MethodPost, path, data)
	if err != nil {
		return err
	}
	return decodeJSONBody(resp, out)
}

func decodeJSONBody(resp *http.Response, out any) error {
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close() //nolint:errcheck
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("parse the response from %s: %w", resp.Request.URL.Path, err)
	}
	return nil
}

// do performs one request, retrying the cases worth retrying, and returns a response whose
// status is 2xx. Its body is left open and unread; every caller closes it.
//
// body is re-sent on each attempt, which is why it is a []byte rather than an io.Reader.
func (p *immichProvider) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= immichAttempts; attempt++ {
		if attempt > 1 {
			// A retry is not worth waiting for if the user has already pressed Ctrl-C.
			// The same signal runPool polls, since nothing cancels the context today.
			if exit.ExitRequested() || ctx.Err() != nil {
				return nil, lastErr
			}
			time.Sleep(immichRetryBackoff << (attempt - 2))
		}

		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, p.creds.BaseURL+path, reader)
		if err != nil {
			return nil, fmt.Errorf("build a request for %s: %w", path, err)
		}
		req.Header.Set("x-api-key", p.creds.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := immichHTTPClient.Do(req)
		if err != nil {
			connErr := p.connectionError(method, path, err)
			// Neither an expired context nor a name that does not exist gets better by
			// being asked again.
			if ctx.Err() != nil || isPermanentDNSError(err) {
				return nil, connErr
			}
			lastErr = connErr
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		apiErr := p.statusError(resp, method, path)
		if !immichShouldRetry(resp.StatusCode) {
			return nil, apiErr
		}
		lastErr = apiErr
	}
	return nil, fmt.Errorf("gave up after %d attempts: %w", immichAttempts, lastErr)
}

// connectionError explains a request that never reached Immich.
//
// The Docker rewrite is the case worth spelling out: someone who wrote localhost and meant
// their laptop gets an error about host.docker.internal, a name they never typed. So the
// message names what was configured, what was tried, and the two ways out.
func (p *immichProvider) connectionError(method, path string, err error) error {
	base := fmt.Errorf("%s %s: %w", method, path, err)
	if p.creds.DockerSourceURL == "" {
		return base
	}
	return fmt.Errorf("%w\n  %s is %q, which photogen is using as %q because it is running in Docker.\n"+
		"  If Immich is on the host, the container needs --add-host=%s:host-gateway (the ddphotos\n"+
		"  script passes it); otherwise set %s to a hostname or LAN IP instead of localhost.",
		base, immichURLEnv, p.creds.DockerSourceURL, p.creds.BaseURL, dockerHostAlias, immichURLEnv)
}

// isPermanentDNSError reports whether a name lookup failed because the name does not exist,
// as opposed to a resolver that was briefly unreachable.
func isPermanentDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// immichShouldRetry reports whether a status is worth a second attempt. Server-side and
// rate-limit failures are; every other 4xx is a problem retrying cannot fix, and in particular
// 401 and 403 are a wrong or under-scoped key.
func immichShouldRetry(status int) bool {
	return status >= 500 || status == http.StatusTooManyRequests
}

// statusError turns a non-2xx response into a message worth reading, and closes the body.
//
// Immich v3 strips "error" and "statusCode" from its error bodies, leaving {"message": "..."},
// and sends an x-immich-cid correlation id that is what its own logs are searchable by.
func (p *immichProvider) statusError(resp *http.Response, method, path string) error {
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// Immich sends {"message": "..."}, and for a rejected field also an errors array whose
	// entries carry the only useful part: a bare "Validation failed" says nothing, while its
	// error says "Invalid UUID" and which field it was.
	var payload struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string   `json:"message"`
			Path    []string `json:"path"`
		} `json:"errors"`
	}
	msg := strings.TrimSpace(string(data))
	if err := json.Unmarshal(data, &payload); err == nil && payload.Message != "" {
		msg = payload.Message
		var details []string
		for _, e := range payload.Errors {
			switch {
			case e.Message != "" && len(e.Path) > 0:
				details = append(details, strings.Join(e.Path, ".")+": "+e.Message)
			case e.Message != "":
				details = append(details, e.Message)
			}
		}
		if len(details) > 0 {
			msg += " — " + strings.Join(details, "; ")
		}
	}

	detail := ""
	if cid := resp.Header.Get("x-immich-cid"); cid != "" {
		detail = fmt.Sprintf(" (immich request id %s)", cid)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		// The key itself is never named, only where it came from.
		return fmt.Errorf("immich rejected the API key (401: %s)%s — check %s in the environment or %s",
			msg, detail, immichAPIKeyEnv, ImmichEnvFileName)
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("the immich API key lacks a required permission (403: %s)%s — "+
			"it needs asset.read, asset.download and album.read", msg, detail)
	case resp.StatusCode == http.StatusBadRequest && strings.HasPrefix(msg, "Validation failed"):
		// Reachable only when the config-time shape check did not catch it: an id of the
		// right shape that Immich still rejects, or a field it has since tightened.
		return fmt.Errorf("immich rejected the request as malformed (400: %s)%s — "+
			"verify sync.album_id is the album's UUID from its URL in Immich", msg, detail)
	case resp.StatusCode == http.StatusBadRequest && strings.Contains(msg, "Not found or no"):
		// Immich answers an unknown or invisible id with 400 from its access check, not
		// 404, so this is the ordinary "wrong album id" path rather than a bad request.
		return fmt.Errorf("immich has no album or asset for this request, or the API key cannot see it "+
			"(400: %s)%s — check sync.album_id", msg, detail)
	default:
		return fmt.Errorf("%s %s failed (%d: %s)%s", method, path, resp.StatusCode, msg, detail)
	}
}
