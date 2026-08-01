package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCustomization writes content to dir/customization.yaml and returns the path.
func writeCustomization(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, DefaultCustomizationFile)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadCustomizations(t *testing.T) {
	t.Parallel()

	t.Run("empty file yields no nav", func(t *testing.T) {
		c, err := LoadCustomizations(writeCustomization(t, t.TempDir(), "# nothing configured\n"))
		require.NoError(t, err)
		assert.Empty(t, c.AlbumNav)
	})

	t.Run("parses all fields", func(t *testing.T) {
		c, err := LoadCustomizations(writeCustomization(t, t.TempDir(), `album_nav:
  - label: Back to Maps
    href: https://example.com/
    id: back-to-maps
    new_tab: true
  - label: All Albums
    href: /
`))
		require.NoError(t, err)
		require.Len(t, c.AlbumNav, 2)

		l := c.AlbumNav[0]
		assert.Equal(t, "Back to Maps", l.Label)
		assert.Equal(t, "https://example.com/", l.Href)
		assert.Equal(t, "back-to-maps", l.ID)
		assert.True(t, l.NewTab)

		l = c.AlbumNav[1]
		assert.Equal(t, "All Albums", l.Label)
		assert.Equal(t, "/", l.Href)
		assert.Empty(t, l.ID, "id is optional")
		assert.False(t, l.NewTab, "new_tab defaults to false")
	})

	t.Run("accepts a mailto href", func(t *testing.T) {
		_, err := LoadCustomizations(writeCustomization(t, t.TempDir(), `album_nav:
  - label: Email
    href: mailto:me@example.com
`))
		require.NoError(t, err)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadCustomizations(filepath.Join(t.TempDir(), "nope.yaml"))
		require.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := LoadCustomizations(writeCustomization(t, t.TempDir(), ":\nthis: [is: {not valid"))
		require.Error(t, err)
	})

	invalid := []struct {
		name    string
		nav     string
		wantErr string
	}{
		{
			name:    "missing label",
			nav:     "  - href: /\n",
			wantErr: "label is required",
		},
		{
			name:    "missing href",
			nav:     "  - label: Nowhere\n",
			wantErr: "href is required",
		},
		{
			name:    "relative href",
			nav:     "  - label: Albums\n    href: albums/foo\n",
			wantErr: `must start with "/"`,
		},
		{
			name:    "id starting with a digit",
			nav:     "  - label: Maps\n    href: /\n    id: 1st-link\n",
			wantErr: "must start with a letter",
		},
		{
			name:    "id with invalid characters",
			nav:     "  - label: Maps\n    href: /\n    id: back to maps\n",
			wantErr: "must start with a letter",
		},
		{
			name:    "duplicate ids",
			nav:     "  - label: One\n    href: /\n    id: dup\n  - label: Two\n    href: /x\n    id: dup\n",
			wantErr: `duplicate id "dup"`,
		},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadCustomizations(writeCustomization(t, t.TempDir(), "album_nav:\n"+tc.nav))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestResolveCustomizations(t *testing.T) {
	t.Parallel()

	const oneLink = "album_nav:\n  - label: Home\n    href: /\n"

	t.Run("picks up the default file in the config dir", func(t *testing.T) {
		configDir := t.TempDir()
		writeCustomization(t, configDir, oneLink)

		c, err := ResolveCustomizations(configDir, "", false)
		require.NoError(t, err)
		require.Len(t, c.AlbumNav, 1)
		assert.Equal(t, "Home", c.AlbumNav[0].Label)
	})

	t.Run("no default file is not an error", func(t *testing.T) {
		c, err := ResolveCustomizations(t.TempDir(), "", false)
		require.NoError(t, err)
		assert.Empty(t, c.AlbumNav)
	})

	t.Run("override wins over the default file", func(t *testing.T) {
		configDir := t.TempDir()
		writeCustomization(t, configDir, oneLink)

		override := filepath.Join(t.TempDir(), "other.yaml")
		require.NoError(t, os.WriteFile(override,
			[]byte("album_nav:\n  - label: Elsewhere\n    href: /x\n"), 0o644))

		c, err := ResolveCustomizations(configDir, override, false)
		require.NoError(t, err)
		require.Len(t, c.AlbumNav, 1)
		assert.Equal(t, "Elsewhere", c.AlbumNav[0].Label)
	})

	t.Run("missing override is an error", func(t *testing.T) {
		// Unlike the default file, an explicit path that is not there is a mistake.
		_, err := ResolveCustomizations(t.TempDir(), filepath.Join(t.TempDir(), "nope.yaml"), false)
		require.Error(t, err)
	})

	t.Run("disabled ignores both the default file and the override", func(t *testing.T) {
		configDir := t.TempDir()
		writeCustomization(t, configDir, oneLink)

		c, err := ResolveCustomizations(configDir, filepath.Join(t.TempDir(), "nope.yaml"), true)
		require.NoError(t, err)
		assert.Empty(t, c.AlbumNav)
	})

	t.Run("never returns nil", func(t *testing.T) {
		c, err := ResolveCustomizations(t.TempDir(), "", false)
		require.NoError(t, err)
		require.NotNil(t, c)
	})
}
