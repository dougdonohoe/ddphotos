package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		assert.False(t, a.Recurse)

		a = af.Albums[1]
		assert.Equal(t, "nepal", a.Slug)
		assert.Equal(t, "Nepal 2018", a.Name)
		assert.Equal(t, "t7", a.Base)
		assert.Equal(t, "2018-Nepal", a.Source)
		assert.Equal(t, "", a.Cover)
		assert.False(t, a.ManualSortOrder)
		assert.True(t, a.Recurse)

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

		configs, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		require.Len(t, configs, 1)

		c := configs[0]
		assert.Equal(t, "antarctica", c.Slug)
		assert.Equal(t, "Antarctica", c.Name)
		assert.Equal(t, antarcticaDir, c.Path)
		assert.Equal(t, "IMG_001.jpg", c.Cover)
		assert.True(t, c.ManualSortOrder)
		assert.False(t, c.Recurse)
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

		configs, err := af.ToAlbumConfigs(configDir, nil)
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

		configs, err := af.ToAlbumConfigs(configDir, nil)
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
		configs, err := af.ToAlbumConfigs(configDir, nil)
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
		_, err := af.ToAlbumConfigs(configDir, nil)
		require.ErrorContains(t, err, "does not exist")
	})

	t.Run("inline description used when no descriptions file", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := t.TempDir()

		af := parseYAML(t, configDir, fmt.Sprintf(`
albums:
  - slug: myalbum
    name: My Album
    source: %s
    description: Inline description here.
`, photoDir))

		configs, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, "Inline description here.", configs[0].Description)
	})

	t.Run("inline description takes precedence over descriptions file", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := t.TempDir()

		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, "descriptions.txt"),
			[]byte("myalbum  From the file.\n"),
			0o644,
		))

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  descriptions: descriptions.txt
albums:
  - slug: myalbum
    name: My Album
    source: %s
    description: Inline wins.
`, photoDir))

		configs, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, "Inline wins.", configs[0].Description)
	})

	t.Run("descriptions file used when inline description is absent", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := t.TempDir()

		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, "descriptions.txt"),
			[]byte("myalbum  From the file.\n"),
			0o644,
		))

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  descriptions: descriptions.txt
albums:
  - slug: myalbum
    name: My Album
    source: %s
`, photoDir))

		configs, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, "From the file.", configs[0].Description)
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

		configs, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, "", configs[0].Description)
	})
}

func TestToAlbumConfigs_Hero(t *testing.T) {
	t.Parallel()

	t.Run("absolute base", func(t *testing.T) {
		configDir := t.TempDir()
		photoBase := t.TempDir()
		heroFile := filepath.Join(photoBase, "hero.jpg")
		require.NoError(t, os.WriteFile(heroFile, []byte("img"), 0o644))

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  hero:
    image: hero.jpg
    base: photos
bases:
  photos: %s
albums: []
`, photoBase))

		_, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, heroFile, af.Settings.HeroImagePath)
	})

	t.Run("relative base resolves to CWD", func(t *testing.T) {
		configDir := t.TempDir()
		cwd, err := os.Getwd()
		require.NoError(t, err)
		relBase := "testdata/hero-base"
		absBase := filepath.Join(cwd, relBase)
		heroFile := filepath.Join(absBase, "hero.jpg")
		require.NoError(t, os.MkdirAll(absBase, 0o755))
		require.NoError(t, os.WriteFile(heroFile, []byte("img"), 0o644))
		t.Cleanup(func() { os.RemoveAll(absBase) })

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  hero:
    image: hero.jpg
    base: photos
bases:
  photos: %s
albums: []
`, relBase))

		_, err = af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, heroFile, af.Settings.HeroImagePath)
	})

	t.Run("relative image without base resolves to configDir", func(t *testing.T) {
		configDir := t.TempDir()
		heroFile := filepath.Join(configDir, "hero.jpg")
		require.NoError(t, os.WriteFile(heroFile, []byte("img"), 0o644))

		af := parseYAML(t, configDir, `
settings:
  hero:
    image: hero.jpg
albums: []
`)

		_, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, heroFile, af.Settings.HeroImagePath)
	})

	t.Run("absolute image without base", func(t *testing.T) {
		configDir := t.TempDir()
		heroFile := filepath.Join(t.TempDir(), "hero.jpg")
		require.NoError(t, os.WriteFile(heroFile, []byte("img"), 0o644))

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  hero:
    image: %s
albums: []
`, heroFile))

		_, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, heroFile, af.Settings.HeroImagePath)
	})

	t.Run("absolute image with base is an error", func(t *testing.T) {
		af := writeYAML(t, `
bases:
  mm: /some/base
settings:
  hero:
    image: /absolute/hero.jpg
    base: mm
albums: []
`)
		_, err := LoadAlbumsFile(af)
		require.ErrorContains(t, err, "image is an absolute path")
	})

	t.Run("unsupported image extension is an error", func(t *testing.T) {
		af := writeYAML(t, `
settings:
  hero:
    image: hero.gif
albums: []
`)
		_, err := LoadAlbumsFile(af)
		require.ErrorContains(t, err, "unsupported extension")
	})

	t.Run("hero image does not exist", func(t *testing.T) {
		configDir := t.TempDir()
		af := parseYAML(t, configDir, `
settings:
  hero:
    image: /nonexistent/hero.jpg
albums: []
`)
		_, err := af.ToAlbumConfigs(configDir, nil)
		require.ErrorContains(t, err, "does not exist")
	})
}

func TestToAlbumConfigs_CSS(t *testing.T) {
	t.Parallel()

	t.Run("valid css file", func(t *testing.T) {
		configDir := t.TempDir()
		cssFile := filepath.Join(configDir, "custom.css")
		require.NoError(t, os.WriteFile(cssFile, []byte("body{}"), 0o644))
		photoDir := t.TempDir()

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  css: custom.css
albums:
  - slug: a
    name: A
    source: %s
`, photoDir))

		_, err := af.ToAlbumConfigs(configDir, nil)
		require.NoError(t, err)
		assert.Equal(t, cssFile, af.Settings.CustomCSSPath)
	})

	t.Run("css file does not exist", func(t *testing.T) {
		configDir := t.TempDir()
		photoDir := t.TempDir()

		af := parseYAML(t, configDir, fmt.Sprintf(`
settings:
  css: nonexistent.css
albums:
  - slug: a
    name: A
    source: %s
`, photoDir))

		_, err := af.ToAlbumConfigs(configDir, nil)
		require.ErrorContains(t, err, "does not exist")
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

// A slug becomes a URL path segment, an output directory name, and a <loc> value in
// sitemap.xml, so it has to be safe in all three without per-use escaping. The pattern is
// the one the DD Photos App already enforces in its album and site dialogs, so a slug
// accepted there is accepted here and vice versa.
func TestAlbumSlugValidation(t *testing.T) {
	t.Parallel()

	fileWith := func(slug string) *AlbumsFile {
		return &AlbumsFile{
			Albums: []AlbumEntry{{Slug: slug, Name: "A", Source: "/tmp"}},
		}
	}

	valid := []string{
		"uganda",
		"the-way",
		"ski-trip-2007",
		"a",
		"9",
		"Antarctica",
		"under_score",
		"MiXeD-Case_99",
	}
	for _, slug := range valid {
		t.Run("valid/"+slug, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, fileWith(slug).validate())
		})
	}

	invalid := map[string]string{
		"leading dash":       "-uganda",
		"leading underscore": "_uganda",
		"space":              "ski trip",
		"slash":              "trips/uganda",
		"dot":                "uganda.2007",
		"ampersand":          "me&you",
		"angle bracket":      "a<b",
		"quote":              "it's",
		"percent":            "a%20b",
		"non-ascii":          "café",
		"trailing space":     "uganda ",
	}

	for name, slug := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			t.Parallel()
			err := fileWith(slug).validate()
			require.Error(t, err, "slug %q must be rejected", slug)
			assert.Contains(t, err.Error(), slug, "the message names the offending slug")
		})
	}

	// An empty slug keeps its own, more specific message rather than being swept into the
	// pattern error, since "slug is required" is the more useful thing to say.
	t.Run("invalid/empty keeps its own message", func(t *testing.T) {
		t.Parallel()
		err := fileWith("").validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})
}

func TestAlbumSlugLengthCap(t *testing.T) {
	t.Parallel()

	fileWith := func(slug string) *AlbumsFile {
		return &AlbumsFile{Albums: []AlbumEntry{{Slug: slug, Name: "A", Source: "/tmp"}}}
	}

	t.Run("exactly the maximum is allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, fileWith(strings.Repeat("a", slugMaxLen)).validate())
	})

	t.Run("one over the maximum is rejected", func(t *testing.T) {
		t.Parallel()
		err := fileWith(strings.Repeat("a", slugMaxLen+1)).validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "64")
	})
}

// A slug is both the output directory and the URL path segment, so two albums sharing one
// is not survivable: the second overwrites the first's index.json and images, albums.json
// advertises both, and sitemap.xml emits the same <loc> twice. Nothing downstream catches
// it, which is why it has to be rejected here, before ToAlbumConfigs touches the disk.
func TestDuplicateAlbumSlugs(t *testing.T) {
	t.Parallel()

	fileWith := func(slugs ...string) *AlbumsFile {
		af := &AlbumsFile{}
		for _, s := range slugs {
			af.Albums = append(af.Albums, AlbumEntry{Slug: s, Name: "A " + s, Source: "/tmp"})
		}
		return af
	}

	t.Run("distinct slugs are allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, fileWith("uganda", "antarctica", "the-way").validate())
	})

	t.Run("an exact repeat is rejected", func(t *testing.T) {
		t.Parallel()
		err := fileWith("uganda", "antarctica", "uganda").validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate slug")
		assert.Contains(t, err.Error(), "uganda", "the message names the offending slug")
	})

	// Distinct to Go, one directory to macOS and Windows: the build puts both albums in
	// whichever spelling was created first, and a case-sensitive server then 404s the
	// other spelling's URL. So the same build behaves differently per platform.
	t.Run("a case-only difference is rejected", func(t *testing.T) {
		t.Parallel()
		err := fileWith("Uganda", "UGANDA").validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "differ only by case")
		assert.Contains(t, err.Error(), "Uganda")
		assert.Contains(t, err.Error(), "UGANDA")
	})

	// The two cases get different messages because the fixes differ: an exact repeat is
	// usually a copy-paste to delete, a case clash needs one slug renamed.
	t.Run("the two cases report differently", func(t *testing.T) {
		t.Parallel()
		exact := fileWith("trip", "trip").validate()
		cased := fileWith("trip", "Trip").validate()
		require.Error(t, exact)
		require.Error(t, cased)
		assert.NotContains(t, exact.Error(), "differ only by case")
		assert.NotContains(t, cased.Error(), "duplicate slug")
	})

	// Mixed case is legal for a slug (only the site ID is lowercase-only, and the app's
	// REGEXP_SLUG agrees), so the check must not reject a single mixed-case slug.
	t.Run("a lone mixed-case slug is still allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, fileWith("Uganda", "antarctica").validate())
	})

	// Per-album field checks still run first, so a file with both problems reports the
	// missing field rather than the duplicate.
	t.Run("a missing field on an earlier album wins", func(t *testing.T) {
		t.Parallel()
		af := &AlbumsFile{Albums: []AlbumEntry{
			{Slug: "trip", Name: "", Source: "/tmp"},
			{Slug: "trip", Name: "B", Source: "/tmp"},
		}}
		err := af.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

// A descriptions entry naming no album is read with a plain map lookup, so it used to be
// silently dropped, leaving an album with no blurb and no hint why. The same mistake in the
// passwords file has always been reported, so the two config files now behave alike.
func TestUnknownDescriptionSlugs(t *testing.T) {
	t.Parallel()

	// newFile returns a two-album file plus a config dir holding the given descriptions.
	newFile := func(t *testing.T, descriptions string) (*AlbumsFile, string) {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "photos"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "desc.txt"), []byte(descriptions), 0o644))
		af := &AlbumsFile{
			Settings: AlbumsSettings{Descriptions: "desc.txt"},
			Albums: []AlbumEntry{
				{Slug: "uganda", Name: "Uganda", Source: "photos"},
				{Slug: "the-way", Name: "The Way", Source: "photos"},
			},
		}
		return af, dir
	}

	t.Run("entries that match albums are not reported", func(t *testing.T) {
		t.Parallel()
		af, dir := newFile(t, "uganda Gorillas\nthe-way Camino\n")
		configs, err := af.ToAlbumConfigs(dir, nil)
		require.NoError(t, err)
		assert.Equal(t, "Gorillas", configs[0].Description)
		assert.Empty(t, af.Settings.UnknownDescriptionSlugs)
	})

	t.Run("an entry naming no album is reported", func(t *testing.T) {
		t.Parallel()
		af, dir := newFile(t, "uganda Gorillas\nuganda-2007 Typo\nghost Nope\n")
		_, err := af.ToAlbumConfigs(dir, nil)
		require.NoError(t, err, "an unknown entry is a warning, not a failure")
		assert.Equal(t, []string{"ghost", "uganda-2007"}, af.Settings.UnknownDescriptionSlugs,
			"sorted, so the message is stable across runs")
	})

	// An inline description takes precedence, so the file entry goes unused. It still names
	// a real album, though, and must not be reported as unknown.
	t.Run("an inline description does not make the file entry unknown", func(t *testing.T) {
		t.Parallel()
		af, dir := newFile(t, "uganda From the file\nthe-way Camino\n")
		af.Albums[0].Description = "Inline wins"
		configs, err := af.ToAlbumConfigs(dir, nil)
		require.NoError(t, err)
		assert.Equal(t, "Inline wins", configs[0].Description)
		assert.Empty(t, af.Settings.UnknownDescriptionSlugs)
	})

	t.Run("no descriptions file means nothing to report", func(t *testing.T) {
		t.Parallel()
		af, dir := newFile(t, "")
		af.Settings.Descriptions = ""
		_, err := af.ToAlbumConfigs(dir, nil)
		require.NoError(t, err)
		assert.Empty(t, af.Settings.UnknownDescriptionSlugs)
	})
}

// A sync: block changes which of an album's other keys are required, so the rules are
// worth pinning down: source becomes derived rather than configured, the name can come
// from upstream, and naming both a provider and a folder is a contradiction rather than a
// preference photogen could act on.
func TestSyncEntryValidation(t *testing.T) {
	t.Parallel()

	fileWith := func(a AlbumEntry) *AlbumsFile {
		a.Slug = "album"
		return &AlbumsFile{Albums: []AlbumEntry{a}}
	}
	minimal := func() *SyncEntry {
		return &SyncEntry{Provider: "mock", AlbumID: "abc"}
	}
	// fileWith fixes the slug, so the rules that compare albums to each other need this.
	fileWithAll := func(albums ...AlbumEntry) *AlbumsFile {
		return &AlbumsFile{Albums: albums}
	}
	immichAlbum := func(slug, albumID string) AlbumEntry {
		return AlbumEntry{Slug: slug, Sync: &SyncEntry{Provider: "immich", AlbumID: albumID}}
	}

	t.Run("a synced album needs neither name nor source", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, fileWith(AlbumEntry{Sync: minimal()}).validate())
	})

	t.Run("a local album still needs both", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, fileWith(AlbumEntry{Source: "/tmp"}).validate(), "name is required")
		require.ErrorContains(t, fileWith(AlbumEntry{Name: "A"}).validate(), "source is required")
	})

	t.Run("sync and source are mutually exclusive", func(t *testing.T) {
		t.Parallel()
		err := fileWith(AlbumEntry{Source: "/tmp", Sync: minimal()}).validate()
		require.ErrorContains(t, err, "mutually exclusive")
	})

	t.Run("sync and base are mutually exclusive", func(t *testing.T) {
		t.Parallel()
		af := fileWith(AlbumEntry{Base: "drive", Sync: minimal()})
		af.Bases = map[string]string{"drive": "/tmp"}
		require.ErrorContains(t, af.validate(), "mutually exclusive")
	})

	t.Run("provider is required and must be one photogen knows", func(t *testing.T) {
		t.Parallel()
		err := fileWith(AlbumEntry{Sync: &SyncEntry{AlbumID: "abc"}}).validate()
		require.ErrorContains(t, err, "sync.provider is required")

		err = fileWith(AlbumEntry{Sync: &SyncEntry{Provider: "flickr", AlbumID: "abc"}}).validate()
		require.ErrorContains(t, err, "not a known provider")
		// The message lists what would have worked.
		assert.Contains(t, err.Error(), "immich")
		assert.Contains(t, err.Error(), "mock")
	})

	t.Run("album_id is required", func(t *testing.T) {
		t.Parallel()
		err := fileWith(AlbumEntry{Sync: &SyncEntry{Provider: "mock"}}).validate()
		require.ErrorContains(t, err, "sync.album_id is required")
	})

	t.Run("every message names the album", func(t *testing.T) {
		t.Parallel()
		err := fileWith(AlbumEntry{Sync: &SyncEntry{Provider: "mock"}}).validate()
		require.ErrorContains(t, err, `album "album"`)
	})

	t.Run("captions default to on", func(t *testing.T) {
		t.Parallel()
		off := false
		on := true
		assert.True(t, (&SyncEntry{}).CaptionsEnabled(), "an omitted key means on")
		assert.True(t, (&SyncEntry{Captions: &on}).CaptionsEnabled())
		assert.False(t, (&SyncEntry{Captions: &off}).CaptionsEnabled())
	})

	// Two albums pointing at one upstream album is a copy/paste slip: each gets its own sync
	// folder, so the same photos download twice and publish as two identical albums.
	t.Run("two albums cannot sync the same upstream album", func(t *testing.T) {
		t.Parallel()
		id := "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"
		err := fileWithAll(immichAlbum("galapagos", id), immichAlbum("antarctica", id)).validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "galapagos", "names the album that claimed it first")
		assert.Contains(t, err.Error(), "antarctica", "and the one that repeated it")
		assert.Contains(t, err.Error(), id)
	})

	// An Immich album_id is a UUID, where case carries no meaning, so two spellings of one
	// id are one album.
	t.Run("a repeat that differs only by case is still a repeat", func(t *testing.T) {
		t.Parallel()
		af := fileWithAll(
			immichAlbum("a", "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"),
			immichAlbum("b", "D8052D5C-9FF1-4228-9F02-5CDD3D2E2D18"),
		)
		require.ErrorContains(t, af.validate(), "each synced album needs its own upstream album")
	})

	// The id is only meaningful to the provider that issued it, so the same string under two
	// providers is two different albums.
	t.Run("the same id under different providers is allowed", func(t *testing.T) {
		t.Parallel()
		af := fileWithAll(
			AlbumEntry{Slug: "a", Sync: &SyncEntry{Provider: "mock", AlbumID: "antarctica",
				Mock: &MockSyncEntry{Assets: "a.json", MediaDir: "m"}}},
			immichAlbum("b", "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"),
		)
		require.NoError(t, af.validate())
	})

	t.Run("distinct upstream albums are fine", func(t *testing.T) {
		t.Parallel()
		af := fileWithAll(
			immichAlbum("a", "d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18"),
			immichAlbum("b", "ef8acfb8-43fb-4c63-90c0-307b88b8f97a"),
		)
		require.NoError(t, af.validate())
	})

	t.Run("the mock block belongs to the mock provider only", func(t *testing.T) {
		t.Parallel()
		s := &SyncEntry{Provider: "immich", AlbumID: "abc",
			Mock: &MockSyncEntry{Assets: "a.json", MediaDir: "m"}}
		require.ErrorContains(t, fileWith(AlbumEntry{Sync: s}).validate(), "only valid with provider")
	})

	t.Run("the mock block needs both its paths", func(t *testing.T) {
		t.Parallel()
		s := minimal()
		s.Mock = &MockSyncEntry{MediaDir: "m"}
		require.ErrorContains(t, fileWith(AlbumEntry{Sync: s}).validate(), "sync.mock.assets is required")

		s.Mock = &MockSyncEntry{Assets: "a.json"}
		require.ErrorContains(t, fileWith(AlbumEntry{Sync: s}).validate(), "sync.mock.media_dir is required")
	})

	t.Run("the mock fail switch is a closed set", func(t *testing.T) {
		t.Parallel()
		s := minimal()
		for _, ok := range []string{"", "list", "fetch"} {
			s.Mock = &MockSyncEntry{Assets: "a.json", MediaDir: "m", Fail: ok}
			assert.NoError(t, fileWith(AlbumEntry{Sync: s}).validate())
		}
		s.Mock = &MockSyncEntry{Assets: "a.json", MediaDir: "m", Fail: "explode"}
		require.ErrorContains(t, fileWith(AlbumEntry{Sync: s}).validate(), "sync.mock.fail")
	})
}

func TestToAlbumConfigs_Sync(t *testing.T) {
	t.Parallel()

	syncYAML := `
settings:
  id: test
  site_name: Test
  site_description: Test
  copyright_owner: Tester
  copyright_year: 2020
albums:
  - slug: galapagos
    sync:
      provider: mock
      album_id: abc
      mock:
        assets: listing.json
        media_dir: media
`

	t.Run("source is the sync folder, and it has to exist first", func(t *testing.T) {
		t.Parallel()
		configDir := t.TempDir()
		root := t.TempDir()
		af := parseYAML(t, configDir, syncYAML)
		paths := &SyncPaths{Root: root, SiteID: "test"}

		// CreateSyncDirs has not run yet, so the path check fails the same way it does
		// for a source: folder that does not exist.
		_, err := af.ToAlbumConfigs(configDir, paths)
		require.ErrorContains(t, err, "does not exist")

		require.NoError(t, af.CreateSyncDirs(paths))
		configs, err := af.ToAlbumConfigs(configDir, paths)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, SyncAlbumPath(root, "test", "mock", "galapagos"), configs[0].Path)
	})

	t.Run("the mock paths are anchored to the config dir", func(t *testing.T) {
		t.Parallel()
		configDir := t.TempDir()
		root := t.TempDir()
		af := parseYAML(t, configDir, syncYAML)
		paths := &SyncPaths{Root: root, SiteID: "test"}
		require.NoError(t, af.CreateSyncDirs(paths))

		configs, err := af.ToAlbumConfigs(configDir, paths)
		require.NoError(t, err)
		require.NotNil(t, configs[0].Sync)
		assert.Equal(t, "mock", configs[0].Sync.Provider)
		assert.Equal(t, "abc", configs[0].Sync.AlbumID)
		assert.True(t, configs[0].Sync.Captions)
		assert.Equal(t, filepath.Join(configDir, "listing.json"), configs[0].Sync.Mock.AssetsPath)
		assert.Equal(t, filepath.Join(configDir, "media"), configs[0].Sync.Mock.MediaDir)
		assert.Nil(t, configs[0].Sync.Immich, "another provider's block is not filled in")
	})

	// Immich has no YAML block, so its resolved config hangs off the provider name. Without
	// this the provider has no way to find its credentials: newSyncProvider never sees the
	// config dir.
	t.Run("the immich env file is resolved against the config dir", func(t *testing.T) {
		t.Parallel()
		configDir := t.TempDir()
		root := t.TempDir()
		af := parseYAML(t, configDir, strings.Replace(syncYAML, `      provider: mock
      album_id: abc
      mock:
        assets: listing.json
        media_dir: media
`, `      provider: immich
      album_id: d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18
`, 1))
		paths := &SyncPaths{Root: root, SiteID: "test"}
		require.NoError(t, af.CreateSyncDirs(paths))

		configs, err := af.ToAlbumConfigs(configDir, paths)
		require.NoError(t, err)
		require.NotNil(t, configs[0].Sync.Immich)
		assert.Equal(t, filepath.Join(configDir, ImmichEnvFileName), configs[0].Sync.Immich.EnvFile)
		assert.Nil(t, configs[0].Sync.Mock)
	})

	t.Run("a synced album with no sync paths names the flag that would fix it", func(t *testing.T) {
		t.Parallel()
		configDir := t.TempDir()
		af := parseYAML(t, configDir, syncYAML)
		_, err := af.ToAlbumConfigs(configDir, nil)
		require.ErrorContains(t, err, "-sync-dir")
	})

	t.Run("a local album is unaffected by sync paths being present", func(t *testing.T) {
		t.Parallel()
		configDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(configDir, "photos"), 0o755))
		af := parseYAML(t, configDir, `
settings:
  id: test
  site_name: Test
  site_description: Test
  copyright_owner: Tester
  copyright_year: 2020
albums:
  - slug: local
    name: Local
    source: photos
`)
		configs, err := af.ToAlbumConfigs(configDir, &SyncPaths{Root: t.TempDir(), SiteID: "test"})
		require.NoError(t, err)
		assert.Nil(t, configs[0].Sync)
		assert.Equal(t, filepath.Join(configDir, "photos"), configs[0].Path)
	})
}
