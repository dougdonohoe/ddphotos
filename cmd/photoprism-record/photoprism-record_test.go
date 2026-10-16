package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unscrubbed are the values a location field holds once scrubbed. The fixtures are committed
// to a public repo, so a recording made at home would otherwise publish the home location.
var unscrubbed = map[string]any{
	"Lat": 0.0, "Lng": 0.0, "Altitude": 0.0,
	"CellID": "zz", "PlaceID": "zz", "PlaceCountry": "zz", "Country": "zz",
	"PlaceLabel": "Unknown", "PlaceCity": "Unknown", "PlaceState": "Unknown",
}

func TestScrub(t *testing.T) {
	var v any
	require.NoError(t, json.Unmarshal([]byte(`{
		"session_id": "abc", "access_token": "secret", "config": {"downloadToken": "t"},
		"user": {"Email": "someone@real.example", "Name": "admin"},
		"photos": [{
			"UID": "pt1", "TakenAt": "2024-04-28T11:51:57Z", "Title": "Santiago Kalea / Irun / 2024",
			"Lat": 43.349, "Lng": -1.786, "Altitude": 8, "CellID": "s2:0d5108fc7804",
			"PlaceID": "es:Bm7BpeezGlqS", "PlaceLabel": "Irun, Euskadi, Spain", "PlaceCity": "Irun",
			"PlaceState": "Euskadi", "PlaceCountry": "es", "Country": "es",
			"Caption": "Top of the pass", "CameraMake": "Apple",
			"Files": [{"Markers": [{"Name": "Someone"}]}]
		}],
		"album": {"Title": "The Way", "Location": "Home", "State": "Somewhere"}
	}`), &v))

	got := scrub(v).(map[string]any)

	assert.Equal(t, "scrubbed", got["session_id"])
	assert.Equal(t, "scrubbed", got["access_token"])
	assert.Empty(t, got["config"])
	assert.Equal(t, "", got["user"].(map[string]any)["Email"])

	photo := got["photos"].([]any)[0].(map[string]any)
	for k, want := range unscrubbed {
		assert.EqualValues(t, want, photo[k], "%s must be scrubbed", k)
	}
	assert.Equal(t, "Scrubbed", photo["Title"], "a photo's generated title names the place")
	assert.Equal(t, "Top of the pass", photo["Caption"], "the user's caption is left alone")
	assert.Equal(t, "Apple", photo["CameraMake"])
	assert.Empty(t, photo["Files"].([]any)[0].(map[string]any)["Markers"])

	album := got["album"].(map[string]any)
	assert.Equal(t, "The Way", album["Title"], "an album's title is the user's own")
	assert.Equal(t, "", album["Location"])
	assert.Equal(t, "", album["State"])
}

// Guards the fixtures themselves, so a recording made before a scrub rule existed fails here
// rather than shipping.
func TestCommittedFixturesHaveNoLocation(t *testing.T) {
	root := filepath.Join("..", "..", "pkg", "photogen", "testdata", "photoprism")
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		count++
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var v any
		require.NoError(t, json.Unmarshal(data, &v), path)
		for _, found := range findUnscrubbed(v) {
			t.Errorf("%s: %s", path, found)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Positive(t, count, "no fixtures found under %s", root)
}

// findUnscrubbed returns every location field holding something other than its scrubbed
// value, and every photo title that was not scrubbed, at any depth.
func findUnscrubbed(v any) []string {
	var found []string
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if want, ok := unscrubbed[k]; ok && val != want {
				found = append(found, k+" is not scrubbed")
			}
			found = append(found, findUnscrubbed(val)...)
		}
		if _, isPhoto := t["TakenAt"]; isPhoto && t["Title"] != "Scrubbed" {
			found = append(found, "a photo Title is not scrubbed")
		}
	case []any:
		for _, val := range t {
			found = append(found, findUnscrubbed(val)...)
		}
	}
	return found
}

// write must refuse a payload that carries a known secret verbatim, whatever field it is in.
func TestWriteRefusesSecrets(t *testing.T) {
	t.Cleanup(func() { secrets = nil })
	secrets = []string{"0yRpcV-nWgN7V-XnLjiv"}
	path := filepath.Join(t.TempDir(), "x.json")

	err := write(path, []byte(`{"Notes": "pasted 0yRpcV-nWgN7V-XnLjiv by mistake"}`))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "0yRpcV", "the refusal does not repeat the secret")
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "nothing is written")

	require.NoError(t, write(path, []byte(`{"Notes": "nothing to see"}`)))
}
