package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// write puts contents in a temp file and returns its path.
func writeYAMLFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "conf.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

// readYAML rejects keys that match no field. These config files are hand-edited and nearly
// every key is optional, so a typo used to be invisible: the value was dropped, the default
// applied, and nothing was said.
func TestReadYAMLKnownFields(t *testing.T) {
	t.Parallel()

	t.Run("a known key is parsed", func(t *testing.T) {
		t.Parallel()
		c, err := readYAML[Customizations](writeYAMLFile(t, "album_nav:\n  - label: Home\n    href: /\n"))
		require.NoError(t, err)
		require.Len(t, c.AlbumNav, 1)
		assert.Equal(t, "Home", c.AlbumNav[0].Label)
	})

	t.Run("an unknown top-level key is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[Customizations](writeYAMLFile(t, "album_navv:\n  - label: Home\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "album_navv", "the message names the offending key")
		assert.Contains(t, err.Error(), "line 1", "and where it is")
	})

	// Nested structs are checked too, which is where the typos that matter live: a
	// misspelled per-album or per-link key would otherwise silently take the default.
	t.Run("an unknown nested key is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[Customizations](writeYAMLFile(t,
			"album_nav:\n  - label: Home\n    href: /\n    new_tabb: true\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new_tabb")
	})

	// An empty file has no document to decode, which the decoder reports as io.EOF. The
	// old yaml.Unmarshal treated that as the zero value, and callers depend on it:
	// ResolveCustomizations loads the default customization.yaml whenever it exists, so an
	// empty one has to keep meaning "no customizations".
	t.Run("an empty file is the zero value, not an error", func(t *testing.T) {
		t.Parallel()
		c, err := readYAML[Customizations](writeYAMLFile(t, ""))
		require.NoError(t, err)
		assert.Empty(t, c.AlbumNav)
	})

	t.Run("a comments-only file is the zero value too", func(t *testing.T) {
		t.Parallel()
		c, err := readYAML[Customizations](writeYAMLFile(t, "# nothing here yet\n# maybe later\n"))
		require.NoError(t, err)
		assert.Empty(t, c.AlbumNav)
	})

	t.Run("malformed YAML is still a parse error", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[Customizations](writeYAMLFile(t, "album_nav: [unclosed\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse")
	})

	t.Run("a missing file is a read error, not a parse error", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[Customizations](filepath.Join(t.TempDir(), "nope.yaml"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read ")
		assert.NotContains(t, err.Error(), "parse ")
	})
}

// Every config type the package parses goes through readYAML, so each gets the check. These
// assert on the real structs rather than a stand-in, since the point is that the live
// config files are covered.
func TestKnownFieldsAcrossConfigTypes(t *testing.T) {
	t.Parallel()

	const validAlbums = "settings:\n  id: t\n  site_name: T\n  site_description: d\n" +
		"  copyright_owner: o\n  copyright_year: 2020\n"

	t.Run("albums.yaml settings", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[AlbumsFile](writeYAMLFile(t, validAlbums+"  site_nmae: typo\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "site_nmae")
	})

	t.Run("albums.yaml album entry", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[AlbumsFile](writeYAMLFile(t,
			validAlbums+"albums:\n  - slug: x\n    name: X\n    source: s\n    recurze: true\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recurze")
	})

	t.Run("albums.yaml hero block", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[AlbumsFile](writeYAMLFile(t,
			validAlbums+"  hero:\n    image: a.jpg\n    kropp: center\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kropp")
	})

	t.Run("passwords file entry", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[passwordsFile](writeYAMLFile(t, "key: k\nsite:\n  password: secret123\n  hnt: oops\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hnt")
	})

	// Album slugs and base names are map keys, not struct fields, so they must stay free
	// form: the check must not reject a perfectly good album name.
	t.Run("map keys are not treated as fields", func(t *testing.T) {
		t.Parallel()
		_, err := readYAML[passwordsFile](writeYAMLFile(t,
			"key: k\nalbums:\n  anything-goes-here:\n    password: secret123\n"))
		require.NoError(t, err)

		af, err := readYAML[AlbumsFile](writeYAMLFile(t, validAlbums+"bases:\n  whatever: /tmp\n"))
		require.NoError(t, err)
		assert.Equal(t, "/tmp", af.Bases["whatever"])
	})
}
