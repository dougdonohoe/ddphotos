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

// locationKeys are the exifInfo fields that place a photo. The fixtures are committed to a
// public repo, so a recording made at home would otherwise publish the home location.
var locationKeys = []string{"latitude", "longitude", "city", "state", "country"}

func TestScrub(t *testing.T) {
	var v any
	require.NoError(t, json.Unmarshal([]byte(`{
		"owner": {"email": "someone@real.example", "name": "Someone"},
		"assets": {"items": [{
			"id": "a1",
			"exifInfo": {
				"latitude": 45.262378, "longitude": -111.362503,
				"city": "Big Sky", "state": "Montana", "country": "United States of America",
				"make": "Apple", "description": "Top of the lift"
			}
		}]}
	}`), &v))

	got := scrub(v).(map[string]any)

	assert.Equal(t, "owner@example.com", got["owner"].(map[string]any)["email"])
	exif := got["assets"].(map[string]any)["items"].([]any)[0].(map[string]any)["exifInfo"].(map[string]any)
	for _, k := range locationKeys {
		assert.Contains(t, exif, k, "%s is kept as a field, the way Immich sends it for a photo with no GPS", k)
		assert.Nil(t, exif[k], "%s must be scrubbed", k)
	}
	assert.Equal(t, "Apple", exif["make"], "non-location EXIF is left alone")
	assert.Equal(t, "Top of the lift", exif["description"])
}

// Guards the fixtures themselves, so a recording made before a scrub rule existed, or with
// a location field Immich adds later under a known name, fails here rather than shipping.
func TestCommittedFixturesHaveNoLocation(t *testing.T) {
	root := filepath.Join("..", "..", "pkg", "photogen", "testdata", "immich")
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
		for _, found := range findLocations(v) {
			t.Errorf("%s: %s", path, found)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Positive(t, count, "no fixtures found under %s", root)
}

// findLocations returns every non-null location field, at any depth.
func findLocations(v any) []string {
	var found []string
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			for _, lk := range locationKeys {
				if k == lk && val != nil {
					found = append(found, k+" is not null")
				}
			}
			found = append(found, findLocations(val)...)
		}
	case []any:
		for _, val := range t {
			found = append(found, findLocations(val)...)
		}
	}
	return found
}

// credentials must resolve exactly as photogen does, so a recording talks to the server a
// sync would. The old private copy stripped every leading and trailing quote character,
// where photogen strips one matching pair.
func TestCredentialsMatchPhotogen(t *testing.T) {
	t.Setenv("IMMICH_API_KEY", "")
	t.Setenv("IMMICH_INSTANCE_URL", "")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "immich.env"), []byte(
		"IMMICH_API_KEY=abc'\nIMMICH_INSTANCE_URL=\"http://localhost:2283/api/\"\n"), 0o600))

	apiKey, baseURL, err := credentials(dir)
	require.NoError(t, err)
	assert.Equal(t, "abc'", apiKey, "only a matching pair of quotes is stripped")
	assert.Equal(t, "http://localhost:2283", baseURL)

	t.Setenv("IMMICH_API_KEY", "from-env")
	apiKey, _, err = credentials(dir)
	require.NoError(t, err)
	assert.Equal(t, "from-env", apiKey, "a real environment variable wins over the file")
}
