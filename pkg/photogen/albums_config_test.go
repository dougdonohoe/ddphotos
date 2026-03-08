package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAlbumsFile(t *testing.T) {
	t.Parallel()

	t.Run("valid file", func(t *testing.T) {
		af, err := LoadAlbumsFile("testdata/albums.yaml")
		require.NoError(t, err)

		assert.Equal(t, "https://photos.example.com", af.Settings.SiteURL)
		assert.Equal(t, "web/static", af.Settings.OutputDir)
		assert.Equal(t, "descriptions.txt", af.Settings.Descriptions)

		assert.Equal(t, "/Volumes/T7/Photos", af.Bases["t7"])
		assert.Equal(t, "/Users/example/Dropbox/Photos", af.Bases["dropbox"])

		require.Len(t, af.Albums, 3)

		a := af.Albums[0]
		assert.Equal(t, "antarctica", a.Slug)
		assert.Equal(t, "Antarctica", a.Name)
		assert.Equal(t, "t7", a.Base)
		assert.Equal(t, "2004-Antarctica", a.Source)
		assert.Equal(t, "IMG_001.jpg", a.Cover)
		assert.True(t, a.ManualSortOrder)

		a = af.Albums[1]
		assert.Equal(t, "nepal", a.Slug)
		assert.Equal(t, "Nepal 2018", a.Name)
		assert.Equal(t, "t7", a.Base)
		assert.Equal(t, "2018-Nepal", a.Source)
		assert.Equal(t, "", a.Cover)
		assert.False(t, a.ManualSortOrder)

		a = af.Albums[2]
		assert.Equal(t, "localtest", a.Slug)
		assert.Equal(t, "", a.Base, "album with no base should have empty base")
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadAlbumsFile("testdata/nonexistent.yaml")
		require.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.yaml")
		require.NoError(t, os.WriteFile(path, []byte(":\nthis: [is: {not valid"), 0o644))
		_, err := LoadAlbumsFile(path)
		require.Error(t, err)
	})

	t.Run("missing slug", func(t *testing.T) {
		af := writeYAML(t, `albums:
  - name: No Slug
    source: /tmp/photos`)
		_, err := LoadAlbumsFile(af)
		require.ErrorContains(t, err, "slug is required")
	})

	t.Run("missing name", func(t *testing.T) {
		af := writeYAML(t, `albums:
  - slug: no-name
    source: /tmp/photos`)
		_, err := LoadAlbumsFile(af)
		require.ErrorContains(t, err, "name is required")
	})

	t.Run("missing source", func(t *testing.T) {
		af := writeYAML(t, `albums:
  - slug: no-source
    name: No Source`)
		_, err := LoadAlbumsFile(af)
		require.ErrorContains(t, err, "source is required")
	})

	t.Run("unknown base reference", func(t *testing.T) {
		af := writeYAML(t, `albums:
  - slug: bad-base
    name: Bad Base
    source: some/path
    base: nonexistent`)
		_, err := LoadAlbumsFile(af)
		require.ErrorContains(t, err, `base "nonexistent" not defined`)
	})
}

func TestLoadAlbumDescriptions(t *testing.T) {
	t.Parallel()

	t.Run("valid file", func(t *testing.T) {
		descs, err := LoadAlbumDescriptions("testdata/descriptions.txt")
		require.NoError(t, err)
		assert.Equal(t, "A cruise through Antarctica and the Falkland Islands.", descs["antarctica"])
		assert.Equal(t, "Trekking to Everest Base Camp.", descs["nepal"])
		assert.Equal(t, "", descs["missing"], "absent slug returns empty string")
	})

	t.Run("slug with no description", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "desc.txt")
		require.NoError(t, os.WriteFile(path, []byte("solo-slug\n"), 0o644))
		descs, err := LoadAlbumDescriptions(path)
		require.NoError(t, err)
		assert.Equal(t, "", descs["solo-slug"])
	})

	t.Run("comments and blank lines ignored", func(t *testing.T) {
		content := "# comment\n\nantarctica  Penguins!\n\n# another comment\n"
		path := filepath.Join(t.TempDir(), "desc.txt")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		descs, err := LoadAlbumDescriptions(path)
		require.NoError(t, err)
		assert.Len(t, descs, 1)
		assert.Equal(t, "Penguins!", descs["antarctica"])
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadAlbumDescriptions("testdata/nonexistent.txt")
		require.Error(t, err)
	})
}

func TestToAlbumConfigs(t *testing.T) {
	t.Parallel()

	t.Run("resolves paths and loads descriptions", func(t *testing.T) {
		configDir := t.TempDir()
		photoBase := t.TempDir()
		antarcticaDir := filepath.Join(photoBase, "antarctica")
		require.NoError(t, os.Mkdir(antarcticaDir, 0o755))

		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, "descriptions.txt"),
			[]byte("antarctica  Penguins and icebergs.\n"),
			0o644,
		))

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  descriptions: descriptions.txt
bases:
  photos: %s
albums:
  - slug: antarctica
    name: Antarctica
    base: photos
    source: antarctica
    cover: IMG_001.jpg
    manual_sort_order: true
`, photoBase))

		configs, err := af.ToAlbumConfigs(configDir)
		require.NoError(t, err)
		require.Len(t, configs, 1)

		c := configs[0]
		assert.Equal(t, "antarctica", c.Slug)
		assert.Equal(t, "Antarctica", c.Name)
		assert.Equal(t, antarcticaDir, c.Path)
		assert.Equal(t, "IMG_001.jpg", c.Cover)
		assert.True(t, c.ManualSortOrder)
		assert.Equal(t, "Penguins and icebergs.", c.Description)
	})

	t.Run("absolute source without base", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := t.TempDir()

		af := parseYAML(t, configDir, fmt.Sprintf(`
albums:
  - slug: myalbum
    name: My Album
    source: %s
`, photoDir))

		configs, err := af.ToAlbumConfigs(configDir)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, photoDir, configs[0].Path)
	})

	t.Run("relative base resolves to CWD", func(t *testing.T) {
		configDir := t.TempDir()

		// Create source dir relative to CWD (the package test directory).
		cwd, err := os.Getwd()
		require.NoError(t, err)
		relBase := "testdata/sample-base"
		absBase := filepath.Join(cwd, relBase)
		photoDir := filepath.Join(absBase, "myalbum")
		require.NoError(t, os.MkdirAll(photoDir, 0o755))
		t.Cleanup(func() { os.RemoveAll(absBase) })

		af := parseYAML(t, configDir, fmt.Sprintf(`
bases:
  sample: %s
albums:
  - slug: myalbum
    name: My Album
    base: sample
    source: myalbum
`, relBase))

		configs, err := af.ToAlbumConfigs(configDir)
		require.NoError(t, err)
		assert.Equal(t, photoDir, configs[0].Path)
	})

	t.Run("relative source without base resolves to configDir", func(t *testing.T) {
		configDir := t.TempDir()
		photosDir := filepath.Join(configDir, "myphotos")
		require.NoError(t, os.Mkdir(photosDir, 0o755))

		af := parseYAML(t, configDir, `
albums:
  - slug: local
    name: Local
    source: myphotos
`)
		configs, err := af.ToAlbumConfigs(configDir)
		require.NoError(t, err)
		assert.Equal(t, photosDir, configs[0].Path)
	})

	t.Run("source path does not exist", func(t *testing.T) {
		configDir := t.TempDir()
		af := parseYAML(t, configDir, `
albums:
  - slug: ghost
    name: Ghost Album
    source: /nonexistent/path/to/photos
`)
		_, err := af.ToAlbumConfigs(configDir)
		require.ErrorContains(t, err, "does not exist")
	})

	t.Run("album with no description gets empty string", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := t.TempDir()

		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, "descriptions.txt"),
			[]byte("other-album  Some description.\n"),
			0o644,
		))

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  descriptions: descriptions.txt
albums:
  - slug: undescribed
    name: Undescribed
    source: %s
`, photoDir))

		configs, err := af.ToAlbumConfigs(configDir)
		require.NoError(t, err)
		assert.Equal(t, "", configs[0].Description)
	})
}

func TestLoadAlbumConfigs(t *testing.T) {
	t.Parallel()

	t.Run("end to end", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := filepath.Join(configDir, "photos")
		require.NoError(t, os.Mkdir(photoDir, 0o755))

		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, "descriptions.txt"),
			[]byte("myalbum  A great album.\n"),
			0o644,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, "albums.yaml"),
			[]byte(`
settings:
  site_url: https://my.example.com
  output_dir: /tmp/output
  descriptions: descriptions.txt
albums:
  - slug: myalbum
    name: My Album
    source: photos
`),
			0o644,
		))

		configs, settings, err := LoadAlbumConfigs(configDir, "albums.yaml")
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, "myalbum", configs[0].Slug)
		assert.Equal(t, "A great album.", configs[0].Description)
		assert.Equal(t, photoDir, configs[0].Path)
		assert.Equal(t, "https://my.example.com", settings.SiteURL)
		assert.Equal(t, "/tmp/output", settings.OutputDir)
	})

	t.Run("missing albums file", func(t *testing.T) {
		_, _, err := LoadAlbumConfigs(t.TempDir(), "albums.yaml")
		require.Error(t, err)
	})
}

// writeYAML writes content to a temp file and returns the path.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "albums.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// parseYAML writes YAML content to configDir/albums.yaml, parses it, and returns the result.
// The caller should expect LoadAlbumsFile to succeed; use writeYAML for error-path tests.
func parseYAML(t *testing.T, configDir, content string) *AlbumsFile {
	t.Helper()
	path := filepath.Join(configDir, "albums.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	af, err := LoadAlbumsFile(path)
	require.NoError(t, err)
	return af
}
