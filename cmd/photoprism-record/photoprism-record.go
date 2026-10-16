// photoprism-record records a PhotoPrism album's API responses as test fixtures.
//
// Usage:
//
//	go run cmd/photoprism-record/photoprism-record.go -out <dir> <album-uid>
//
// It writes one file per call the sync provider makes:
//
//	album.json           GET /api/v1/albums/{uid}
//	albums.json          GET /api/v1/albums?type=album (every album the password can see; for the DD Photos App)
//	session.json         GET /api/v1/session (the app password's own session, for its scope; for the DD Photos App)
//	search-page1.json    GET /api/v1/photos?s={uid}&merged=true, page 1 (then page2, page3, ...)
//	request-page1.json   the query that produced it, and the X-Count header it came back with
//
// X-Count is recorded because the provider pages on it: PhotoPrism counts file rows, not
// photos, so a page's length says nothing about whether another page follows.
//
// Credentials come from PHOTOPRISM_APP_PASSWORD and PHOTOPRISM_SITE_URL, or from
// photoprism.env in the directory given by -config-dir, exactly as photogen resolves them.
//
// Scrubbed before anything is written: each photo's location (coordinates, place, cell,
// country, and the title PhotoPrism generates from them), since an album shot at home would otherwise publish the home address; any email
// address; face markers; and the session's tokens and config. The app password and the
// download token are also checked for verbatim, and a file containing either is refused.
// -rescrub re-applies the rules to fixtures already recorded, without calling PhotoPrism.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dougdonohoe/ddphotos/pkg/photogen"
)

// pageSize matches photoprismPageSize in pkg/photogen, so recorded fixtures page the way a
// real run does. Override it to record a multipage listing from a small album.
var (
	out       = flag.String("out", filepath.Join("pkg", "photogen", "testdata", "photoprism"), "directory to write fixtures into")
	configDir = flag.String("config-dir", "config", "directory holding photoprism.env")
	pageSize  = flag.Int("size", 500, "page size (file rows) for the search request")
	rescrub   = flag.Bool("rescrub", false, "re-apply the scrub rules to every .json fixture under -out, without calling PhotoPrism")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: photoprism-record [-out <dir>] [-config-dir <dir>] [-size N] <album-uid>\n"+
			"       photoprism-record [-out <dir>] -rescrub\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *rescrub {
		if err := rescrubDir(*out); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// secrets are the strings no fixture may contain: the app password, and every download token
// and session id seen during the run.
var secrets []string

func run(albumUID string) error {
	password, baseURL, err := photogen.LoadPhotoPrismCredentials(filepath.Join(*configDir, photogen.PhotoPrismEnvFileName))
	if err != nil {
		return err
	}
	secrets = append(secrets, password)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 60 * time.Second}

	// The session is fetched first so its id is a known secret before anything is written.
	for _, rec := range []struct{ path, file string }{
		{"/api/v1/session", "session.json"},
		{"/api/v1/albums/" + url.PathEscape(albumUID), "album.json"},
		{"/api/v1/albums?type=album&order=name&count=1000", "albums.json"},
	} {
		body, _, err := call(client, password, baseURL+rec.path)
		if err != nil {
			return err
		}
		if err := write(filepath.Join(*out, rec.file), body); err != nil {
			return err
		}
	}

	for page, offset := 1, 0; ; page, offset = page+1, offset+*pageSize {
		q := url.Values{}
		q.Set("s", albumUID)
		q.Set("merged", "true")
		q.Set("order", "oldest")
		q.Set("count", strconv.Itoa(*pageSize))
		q.Set("offset", strconv.Itoa(offset))

		body, header, err := call(client, password, baseURL+"/api/v1/photos?"+q.Encode())
		if err != nil {
			return err
		}
		rows, err := strconv.Atoi(header.Get("X-Count"))
		if err != nil {
			return fmt.Errorf("search page %d: X-Count %q is not a number", page, header.Get("X-Count"))
		}

		suffix := "page" + strconv.Itoa(page) + ".json"
		reqBody, err := json.Marshal(map[string]any{"query": q, "x_count": rows})
		if err != nil {
			return err
		}
		if err := write(filepath.Join(*out, "request-"+suffix), reqBody); err != nil {
			return err
		}
		if err := write(filepath.Join(*out, "search-"+suffix), body); err != nil {
			return err
		}

		var photos []any
		if err := json.Unmarshal(body, &photos); err != nil {
			return fmt.Errorf("parse search page %d: %w", page, err)
		}
		fmt.Printf("  page %d: %d photo row(s), %d file row(s)\n", page, len(photos), rows)
		if rows < *pageSize {
			return nil
		}
	}
}

// call makes one GET, remembering the tokens and session ids it sees as secrets.
func call(client *http.Client, password, rawURL string) ([]byte, http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+password)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The password is not in the message: only what the server said about it.
		return nil, nil, fmt.Errorf("GET %s failed (%d): %s", rawURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	for _, h := range []string{"X-Download-Token", "X-Preview-Token", "X-Session-ID"} {
		if v := resp.Header.Get(h); v != "" {
			secrets = append(secrets, v)
		}
	}
	var session struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(data, &session) == nil && session.SessionID != "" {
		secrets = append(secrets, session.SessionID)
	}
	return data, resp.Header, nil
}

// write scrubs, pretty-prints and saves one payload.
func write(path string, data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("parse the response for %s: %w", path, err)
	}
	v = scrub(v)

	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	pretty = append(pretty, '\n')
	// Belt and braces: scrub knows the fields these live in, but a fixture that leaked one
	// would be committed before anyone noticed.
	for _, s := range secrets {
		if len(s) >= 8 && bytes.Contains(pretty, []byte(s)) {
			return fmt.Errorf("refusing to write %s: it contains the app password or a token", path)
		}
	}
	if err := os.WriteFile(path, pretty, 0o644); err != nil {
		return err
	}
	fmt.Printf("  wrote: %s\n", path)
	return nil
}

// scrubbedKeys are replaced wherever they appear, at any depth. The replacements are what
// PhotoPrism itself sends for a photo with no location ("zz" is its unknown country), so a
// scrubbed fixture still parses like a real one.
var scrubbedKeys = map[string]any{
	// location
	"Lat":          0,
	"Lng":          0,
	"Altitude":     0,
	"CellID":       "zz",
	"PlaceID":      "zz",
	"PlaceLabel":   "Unknown",
	"PlaceCity":    "Unknown",
	"PlaceState":   "Unknown",
	"PlaceCountry": "zz",
	"Country":      "zz",
	"Location":     "",
	"State":        "",
	// people
	"Email":   "",
	"Markers": []any{},
	// session
	"access_token": "scrubbed",
	"id":           "scrubbed",
	"session_id":   "scrubbed",
	"config":       map[string]any{},
}

// rescrubDir rewrites every .json file under dir through write, so fixtures recorded before a
// scrub rule existed get it too.
func rescrubDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return write(path, data)
	})
}

func scrub(v any) any {
	switch t := v.(type) {
	case map[string]any:
		// A photo's Title is generated from where it was taken ("Santiago Kalea / Irun /
		// 2024"), so on a photo it is location too. An album's title is the user's own.
		if _, isPhoto := t["TakenAt"]; isPhoto {
			if _, ok := t["Title"]; ok {
				t["Title"] = "Scrubbed"
			}
		}
		for k, val := range t {
			if replacement, ok := scrubbedKeys[k]; ok {
				t[k] = replacement
				continue
			}
			t[k] = scrub(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = scrub(val)
		}
		return t
	default:
		return v
	}
}
