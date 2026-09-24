// immich-record records an Immich album's API responses as test fixtures.
//
// Usage:
//
//	go run cmd/immich-record/immich-record.go -out <dir> <album-uuid>
//
// It writes one file per call the sync provider makes:
//
//	album.json           GET  /api/albums/{id}
//	albums.json          GET  /api/albums (every album the key can see; used by the DD Photos App)
//	api-key-me.json      GET  /api/api-keys/me (the key's own record; used by the DD Photos App)
//	search-page1.json    POST /api/search/metadata, page 1 (then page2, page3, ...)
//	request-page1.json   the request body that produced it
//
// Credentials come from IMMICH_API_KEY and IMMICH_INSTANCE_URL, or from immich.env in the
// directory given by -config-dir, exactly as photogen resolves them.
//
// Three things are scrubbed before anything is written: owner.email, which appears in asset
// and album payloads and is a real person's address; each photo's location (exifInfo
// latitude, longitude, city, state and country), since an album shot at home would otherwise
// publish the home address; and the API key, which must never reach a file that gets
// committed. -rescrub re-applies the rules to fixtures already recorded, without calling
// Immich, for when a rule is added. The recorder exists as a command rather than a one-off script
// because it doubles as a check that the request and response structs still match a real
// server — a field Immich renames shows up here first.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dougdonohoe/ddphotos/pkg/photogen"
)

// pageSize matches immichPageSize in pkg/photogen, so recorded fixtures page the way a real
// run does. Override it to record a multipage listing from a small album.
var (
	out       = flag.String("out", filepath.Join("pkg", "photogen", "testdata", "immich"), "directory to write fixtures into")
	configDir = flag.String("config-dir", "config", "directory holding immich.env")
	pageSize  = flag.Int("size", 500, "page size for the search request")
	rescrub   = flag.Bool("rescrub", false, "re-apply the scrub rules to every .json fixture under -out, without calling Immich")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: immich-record [-out <dir>] [-config-dir <dir>] [-size N] <album-uuid>\n"+
			"       immich-record [-out <dir>] -rescrub\n\n")
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

func run(albumID string) error {
	apiKey, baseURL, err := credentials(*configDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 60 * time.Second}

	body, err := call(client, apiKey, http.MethodGet, baseURL+"/api/albums/"+albumID, nil)
	if err != nil {
		return err
	}
	if err := write(filepath.Join(*out, "album.json"), body, apiKey); err != nil {
		return err
	}

	// The DD Photos App lists albums to choose from and checks a key's permissions. photogen
	// makes neither call, but recording them here keeps one source for every Immich fixture.
	for _, rec := range []struct{ path, file string }{
		{"/api/albums", "albums.json"},
		{"/api/api-keys/me", "api-key-me.json"},
	} {
		body, err := call(client, apiKey, http.MethodGet, baseURL+rec.path, nil)
		if err != nil {
			return err
		}
		if err := write(filepath.Join(*out, rec.file), body, apiKey); err != nil {
			return err
		}
	}

	for page := 1; ; {
		req := map[string]any{
			"albumIds": []string{albumID},
			"order":    "asc",
			"withExif": true,
			"size":     *pageSize,
			"page":     page,
		}
		reqBody, err := json.Marshal(req)
		if err != nil {
			return err
		}
		suffix := "page" + strconv.Itoa(page) + ".json"
		if err := write(filepath.Join(*out, "request-"+suffix), reqBody, apiKey); err != nil {
			return err
		}

		body, err := call(client, apiKey, http.MethodPost, baseURL+"/api/search/metadata", reqBody)
		if err != nil {
			return err
		}
		if err := write(filepath.Join(*out, "search-"+suffix), body, apiKey); err != nil {
			return err
		}

		var parsed struct {
			Assets struct {
				Items    []any   `json:"items"`
				NextPage *string `json:"nextPage"`
			} `json:"assets"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return fmt.Errorf("parse search page %d: %w", page, err)
		}
		fmt.Printf("  page %d: %d asset(s)\n", page, len(parsed.Assets.Items))
		if parsed.Assets.NextPage == nil || *parsed.Assets.NextPage == "" {
			return nil
		}
		next, err := strconv.Atoi(*parsed.Assets.NextPage)
		if err != nil {
			return fmt.Errorf("unexpected nextPage %q", *parsed.Assets.NextPage)
		}
		page = next
	}
}

// credentials resolves the API key and instance URL through photogen itself, so the
// environment wins over immich.env and the URL is normalized exactly as a sync run does it.
func credentials(configDir string) (apiKey, baseURL string, err error) {
	return photogen.LoadImmichCredentials(filepath.Join(configDir, photogen.ImmichEnvFileName))
}

func call(client *http.Client, apiKey, method, url string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The key is not in the message: only what the server said about it.
		return nil, fmt.Errorf("%s %s failed (%d): %s", method, url, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// write scrubs, pretty-prints and saves one payload.
func write(path string, data []byte, apiKey string) error {
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
	// Belt and braces: the key is not in any field scrub knows about, but a fixture that
	// leaked one would be committed before anyone noticed.
	if apiKey != "" && bytes.Contains(pretty, []byte(apiKey)) {
		return fmt.Errorf("refusing to write %s: it contains the API key", path)
	}
	if err := os.WriteFile(path, pretty, 0o644); err != nil {
		return err
	}
	fmt.Printf("  wrote: %s\n", path)
	return nil
}

// scrubbedKeys are replaced wherever they appear, at any depth. email is a real person's
// address; the location fields place a photo, and are nulled rather than removed because
// null is what Immich sends for a photo with no GPS; the others are tokens.
var scrubbedKeys = map[string]any{
	"email":          "owner@example.com",
	"latitude":       nil,
	"longitude":      nil,
	"city":           nil,
	"state":          nil,
	"country":        nil,
	"apiKey":         "scrubbed",
	"accessToken":    "scrubbed",
	"password":       "scrubbed",
	"profileImageId": "",
}

// rescrubDir rewrites every .json file under dir through write, so fixtures recorded before a
// scrub rule existed get it too. The output is byte-identical to a fresh recording's, since
// it goes through the same unmarshal, scrub and indent.
func rescrubDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return write(path, data, "")
	})
}

func scrub(v any) any {
	switch t := v.(type) {
	case map[string]any:
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
