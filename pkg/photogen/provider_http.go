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
	"strings"
	"sync"
	"time"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
)

// This is the plumbing every HTTP provider shares: where credentials come from, how a pasted
// URL becomes a base URL, and how one request is made, retried and explained when it fails.
// What a provider's endpoints are and what its error bodies say stay in its own file
// (provider_immich.go, provider_photoprism.go).

const (
	// inDockerEnv is set by docker/Dockerfile. photogen has no other way to tell, and it
	// needs to know because localhost inside the container is the container.
	inDockerEnv = "DDPHOTOS_IN_DOCKER"

	// dockerHostAlias is the name Docker resolves to the host machine. Docker Desktop
	// provides it natively; Docker Engine on Linux needs --add-host, which docker/ddphotos
	// passes.
	dockerHostAlias = "host.docker.internal"

	// syncJSONTimeout bounds a metadata call. It is a whole-request deadline, which is only
	// safe because these responses are small and read in full before returning.
	syncJSONTimeout = 30 * time.Second

	// syncFetchTimeout bounds one asset download. Generous rather than tight: the body is
	// streamed by downloadSyncAsset, and a large video on a slow link is not a failure.
	syncFetchTimeout = 15 * time.Minute

	// syncHTTPAttempts is how many times one request is tried in total. Retries cover
	// connection errors, 5xx and 429 only; see syncShouldRetry.
	syncHTTPAttempts = 3
)

// syncRetryBackoff is the pause before the second attempt; it doubles after that. A var rather
// than a const only so the retry tests do not spend three real seconds waiting.
var syncRetryBackoff = time.Second

// syncHTTPClient is shared by every album's provider. http.Client is goroutine-safe, and
// runPool calls Fetch from four goroutines at once, so pooling connections here is the point.
//
// Client.Timeout is deliberately unset: it covers reading the body, and Fetch hands its body
// to the caller unread. Per-request contexts provide the deadlines instead, and
// ResponseHeaderTimeout catches a server that accepts a connection and then says nothing.
var syncHTTPClient = &http.Client{
	Transport: func() http.RoundTripper {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.ResponseHeaderTimeout = syncJSONTimeout
		return t
	}(),
}

// errNoSyncCredentials is returned when neither the environment nor the env file supplies
// what a provider needs. Tests match on it with errors.Is, so the wording that wraps it is free
// to change.
var errNoSyncCredentials = errors.New("sync credentials not configured")

// httpUpstream describes one kind of HTTP upstream to the shared plumbing. Each provider
// declares exactly one, as a package-level pointer, because the Docker notice it carries is
// once per run per upstream.
type httpUpstream struct {
	// Display is the product name used in messages ("Immich").
	Display string
	// EnvFileName is the credentials file beside albums.yaml ("immich.env").
	EnvFileName string
	// SecretEnv and URLEnv are the two variables, read from the environment first and the
	// env file second.
	SecretEnv string
	URLEnv    string
	// SecretHelp says where the secret comes from, appended to the "not set" error.
	SecretHelp string
	// URLExample is a sample value, appended to the "not set" error.
	URLExample string
	// APISuffixes are trailing paths to drop from a pasted URL, longest first, because people
	// paste what the API docs show. Matched case-insensitively.
	APISuffixes []string

	// dockerNotice keeps the localhost-rewrite line to one per run. One instance is assumed,
	// so one notice is right however many albums sync.
	dockerNotice sync.Once
}

// syncCredentials is what an HTTP provider needs to talk to an instance.
type syncCredentials struct {
	// Secret is the API key or app password. It is never logged, never put in an error, and
	// never written to metadata.yaml.
	Secret string
	// BaseURL is normalized: scheme, host, optional port and optional path prefix, with no
	// trailing slash and no API suffix. Request paths are appended to it.
	BaseURL string
	// DockerSourceURL is the URL as configured, set only when the Docker rewrite changed it.
	// It is kept so a connection failure can name what was configured as well as what was
	// actually tried, which is the difference between a baffling error and an obvious one for
	// someone who wrote localhost and meant their laptop.
	DockerSourceURL string
}

// loadSyncCredentials resolves the secret and instance URL.
//
// The real environment wins over the file, which is the same precedence DDPHOTOS_ALBUMS_DIR
// has and what config/albums.example.yaml promises. A missing file is therefore not an error
// on its own: it is only an error if the environment did not supply the value either.
func loadSyncCredentials(up *httpUpstream, envFile string) (syncCredentials, error) {
	var creds syncCredentials

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

	where := up.EnvFileName
	if envFile != "" {
		where = envFile
	}

	creds.Secret = lookup(up.SecretEnv)
	if creds.Secret == "" {
		return creds, fmt.Errorf("%w: set %s in the environment or in %s (%s)",
			errNoSyncCredentials, up.SecretEnv, where, up.SecretHelp)
	}

	rawURL := lookup(up.URLEnv)
	if rawURL == "" {
		return creds, fmt.Errorf("%w: set %s in the environment or in %s (for example %s)",
			errNoSyncCredentials, up.URLEnv, where, up.URLExample)
	}
	baseURL, dockerRewritten, err := normalizeSyncURL(up, rawURL)
	if err != nil {
		return creds, fmt.Errorf("%s: %w", up.URLEnv, err)
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

// normalizeSyncURL turns what someone pasted into a base URL requests can be appended to.
//
// One function owns every adjustment, so there is one place to look when a URL misbehaves:
//
//   - a trailing slash and a trailing API suffix are dropped, because people paste what the
//     browser shows and the API docs show the suffix;
//   - any other path is kept, so an instance behind a reverse proxy at https://host/photos
//     still works;
//   - inside Docker, a loopback host is rewritten to host.docker.internal, because otherwise
//     localhost is the photogen container rather than the machine the upstream runs on.
func normalizeSyncURL(up *httpUpstream, raw string) (normalized string, dockerRewritten bool, err error) {
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
	for _, suffix := range up.APISuffixes {
		suffix = "/" + suffix
		if len(p) >= len(suffix) && strings.EqualFold(p[len(p)-len(suffix):], suffix) {
			p = p[:len(p)-len(suffix)]
			break
		}
	}
	u.Path = p
	u.RawPath = ""
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
		up.dockerNotice.Do(func() {
			fmt.Printf("  note: running in Docker, so %s %q is used as %q\n",
				up.URLEnv, raw, rewritten)
		})
		return rewritten, true, nil
	}
	return u.String(), false, nil
}

// syncHTTP makes one provider's requests. The provider supplies how to authenticate and how
// to read its error bodies; everything else is common.
type syncHTTP struct {
	up    *httpUpstream
	creds syncCredentials
	// authorize adds the credentials to a request.
	authorize func(*http.Request)
	// statusError turns a non-2xx response into an error worth reading, and closes the body.
	statusError func(resp *http.Response, method, path string) error
}

// getJSON and postJSON are the two metadata shapes. Both read the whole body, so they can own
// a whole-request deadline; a download cannot. getJSON returns the response headers, which is
// where PhotoPrism puts its download token.

func (c *syncHTTP) getJSON(ctx context.Context, path string, out any) (http.Header, error) {
	ctx, cancel := context.WithTimeout(ctx, syncJSONTimeout)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return resp.Header, decodeJSONBody(resp, out)
}

func (c *syncHTTP) postJSON(ctx context.Context, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, syncJSONTimeout)
	defer cancel()
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request for %s: %w", path, err)
	}
	resp, err := c.do(ctx, http.MethodPost, path, data)
	if err != nil {
		return err
	}
	return decodeJSONBody(resp, out)
}

// fetch opens a download. The body outlives this function, so the context must too: cancel
// travels with the reader and fires when the caller closes it.
func (c *syncHTTP) fetch(ctx context.Context, path string) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, syncFetchTimeout)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		cancel()
		return nil, err
	}
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
func (c *syncHTTP) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= syncHTTPAttempts; attempt++ {
		if attempt > 1 {
			// A retry is not worth waiting for if the user has already pressed Ctrl-C.
			// The same signal runPool polls, since nothing cancels the context today.
			if exit.ExitRequested() || ctx.Err() != nil {
				return nil, lastErr
			}
			time.Sleep(syncRetryBackoff << (attempt - 2))
		}

		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.creds.BaseURL+path, reader)
		if err != nil {
			return nil, fmt.Errorf("build a request for %s: %w", path, err)
		}
		c.authorize(req)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := syncHTTPClient.Do(req)
		if err != nil {
			connErr := c.connectionError(method, path, err)
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

		apiErr := c.statusError(resp, method, path)
		if !syncShouldRetry(resp.StatusCode) {
			return nil, apiErr
		}
		lastErr = apiErr
	}
	return nil, fmt.Errorf("gave up after %d attempts: %w", syncHTTPAttempts, lastErr)
}

// connectionError explains a request that never reached the upstream.
//
// The Docker rewrite is the case worth spelling out: someone who wrote localhost and meant
// their laptop gets an error about host.docker.internal, a name they never typed. So the
// message names what was configured, what was tried, and the two ways out.
func (c *syncHTTP) connectionError(method, path string, err error) error {
	// A *url.Error repeats the full URL, query and all.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		urlErr.URL = stripQuery(urlErr.URL)
	}
	base := fmt.Errorf("%s %s: %w", method, stripQuery(path), err)
	if c.creds.DockerSourceURL == "" {
		return base
	}
	return fmt.Errorf("%w\n  %s is %q, which photogen is using as %q because it is running in Docker.\n"+
		"  If %s is on the host, the container needs --add-host=%s:host-gateway (the ddphotos\n"+
		"  script passes it); otherwise set %s to a hostname or LAN IP instead of localhost.",
		base, c.up.URLEnv, c.creds.DockerSourceURL, c.creds.BaseURL, c.up.Display, dockerHostAlias, c.up.URLEnv)
}

// stripQuery drops a query string from a request path before it goes into a message, since
// a query can carry a token (PhotoPrism's ?t=).
func stripQuery(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

// isPermanentDNSError reports whether a name lookup failed because the name does not exist,
// as opposed to a resolver that was briefly unreachable.
func isPermanentDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// syncShouldRetry reports whether a status is worth a second attempt. Server-side and
// rate-limit failures are; every other 4xx is a problem retrying cannot fix, and in particular
// 401 and 403 are a wrong or under-scoped credential.
func syncShouldRetry(status int) bool {
	return status >= 500 || status == http.StatusTooManyRequests
}
