package photogen

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigWorkers(t *testing.T) {
	t.Parallel()

	t.Run("zero auto-detects NumCPU/2 min 2", func(t *testing.T) {
		cfg := Config{NumWorkers: 0}
		expected := max(runtime.NumCPU()/2, 2)
		assert.Equal(t, expected, cfg.Workers())
	})

	t.Run("result is always at least 2 on any machine", func(t *testing.T) {
		cfg := Config{NumWorkers: 0}
		assert.GreaterOrEqual(t, cfg.Workers(), 2)
	})

	t.Run("positive value used as-is", func(t *testing.T) {
		cfg := Config{NumWorkers: 7}
		assert.Equal(t, 7, cfg.Workers())
	})
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{
			name:    "missing output root",
			config:  Config{},
			wantErr: "output directory",
		},
		{
			name:    "missing site id",
			config:  Config{OutputRoot: "/tmp/out"},
			wantErr: "settings.id",
		},
		{
			name:    "invalid site id with spaces",
			config:  Config{OutputRoot: "/tmp/out", SiteID: "my site"},
			wantErr: "settings.id",
		},
		{
			name:    "invalid site id with uppercase",
			config:  Config{OutputRoot: "/tmp/out", SiteID: "MySite"},
			wantErr: "settings.id",
		},
		{
			name:    "missing site name",
			config:  Config{OutputRoot: "/tmp/out", SiteID: "my-site"},
			wantErr: "settings.site_name",
		},
		{
			name:    "missing site description",
			config:  Config{OutputRoot: "/tmp/out", SiteID: "my-site", SiteName: "My Site"},
			wantErr: "settings.site_description",
		},
		{
			name:    "missing copyright owner",
			config:  Config{OutputRoot: "/tmp/out", SiteID: "my-site", SiteName: "My Site", SiteDescription: "Desc"},
			wantErr: "settings.copyright_owner",
		},
		{
			name:    "missing copyright year",
			config:  Config{OutputRoot: "/tmp/out", SiteID: "my-site", SiteName: "My Site", SiteDescription: "Desc", CopyrightOwner: "Me"},
			wantErr: "settings.copyright_year",
		},
		{
			name:   "valid config",
			config: Config{OutputRoot: "/tmp/out", SiteID: "my-site", SiteName: "My Site", SiteDescription: "Desc", CopyrightOwner: "Me", CopyrightYear: 2020},
		},
		{
			name:   "valid config single char id",
			config: Config{OutputRoot: "/tmp/out", SiteID: "x", SiteName: "My Site", SiteDescription: "Desc", CopyrightOwner: "Me", CopyrightYear: 2020},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.config.Validate()
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigSiteOutputPath(t *testing.T) {
	t.Parallel()

	cfg := Config{OutputRoot: "/tmp/albums", SiteID: "prod"}
	assert.Equal(t, "/tmp/albums/prod", cfg.SiteOutputPath())
	assert.Equal(t, "/tmp/albums/prod/albums.json", cfg.SiteOutputPath("albums.json"))
	assert.Equal(t, "/tmp/albums/prod/theway/grid/photo.webp", cfg.SiteOutputPath("theway", "grid", "photo.webp"))
}

// TestConfigPhotoOutputName covers the Config-level wrapper, which decides *whether* to
// obfuscate; EncryptConfig.PhotoOutputName (see encrypt_test.go) decides *how*.
//
// Expected values are literals throughout. Computing them from the function under test
// would assert only that it agrees with itself.
func TestConfigPhotoOutputName(t *testing.T) {
	t.Parallel()

	t.Run("nil Encrypt is not encrypted", func(t *testing.T) {
		t.Parallel()
		// Also guards the nil dereference: the c.Encrypt access is reachable only behind
		// IsAlbumEncrypted, which nil-checks first.
		c := &Config{}
		assert.Equal(t, "photo.webp", c.PhotoWebPName("any", "photo.jpg"))
		assert.Equal(t, "clip.mp4", c.PhotoOutputName("any", "clip.mov", ".mp4"))
	})

	t.Run("a key with no password obfuscates nothing", func(t *testing.T) {
		t.Parallel()
		// A key-only passwords file protects nothing, so it must not rename anything
		// either. Consistent with WriteConfigJSON refusing to mark such a site encrypted.
		c := &Config{Encrypt: &EncryptConfig{HMACKey: "test-key"}}
		assert.Equal(t, "photo.webp", c.PhotoWebPName("any", "photo.jpg"))
		assert.Equal(t, "clip.mp4", c.PhotoOutputName("any", "clip.mov", ".mp4"))
	})

	t.Run("a site password obfuscates every album", func(t *testing.T) {
		t.Parallel()
		c := &Config{Encrypt: &EncryptConfig{HMACKey: "test-key", SitePassword: "site-pass"}}
		for _, slug := range []string{"one", "two"} {
			assert.Regexp(t, `^[0-9a-f-]{36}\.mp4$`, c.PhotoOutputName(slug, "clip.mov", ".mp4"))
		}
	})

	t.Run("a per-album password obfuscates only that album", func(t *testing.T) {
		t.Parallel()
		// The routing decision this wrapper exists for. A public album sitting beside a
		// protected one must keep readable filenames.
		c := &Config{Encrypt: &EncryptConfig{
			HMACKey:        "test-key",
			AlbumPasswords: map[string]string{"secret": "secret-pass"},
		}}
		assert.Regexp(t, `^[0-9a-f-]{36}\.mp4$`, c.PhotoOutputName("secret", "clip.mov", ".mp4"))
		assert.Equal(t, "clip.mp4", c.PhotoOutputName("public", "clip.mov", ".mp4"))
		assert.Equal(t, "photo.webp", c.PhotoWebPName("public", "photo.jpg"))
	})

	t.Run("delegates to EncryptConfig unchanged when encrypted", func(t *testing.T) {
		t.Parallel()
		// Pins that the wrapper adds no salt of its own, e.g. the album slug: the same
		// source file must produce the same name whichever layer is asked.
		ec := &EncryptConfig{HMACKey: "test-key", SitePassword: "site-pass"}
		c := &Config{Encrypt: ec}
		assert.Equal(t, ec.PhotoOutputName("clip.mov", ".mp4"), c.PhotoOutputName("a", "clip.mov", ".mp4"))
		assert.Equal(t, ec.PhotoWebPName("photo.jpg"), c.PhotoWebPName("a", "photo.jpg"))
	})

	t.Run("PhotoWebPName is PhotoOutputName with .webp", func(t *testing.T) {
		t.Parallel()
		for _, c := range []*Config{
			{},
			{Encrypt: &EncryptConfig{HMACKey: "k", SitePassword: "site-pass"}},
		} {
			for _, name := range []string{"photo.jpg", "clip.mov", "no-extension"} {
				assert.Equal(t, c.PhotoOutputName("a", name, ".webp"), c.PhotoWebPName("a", name), name)
			}
		}
	})

	t.Run("unencrypted names keep the stem and swap only the extension", func(t *testing.T) {
		t.Parallel()
		c := &Config{}
		assert.Equal(t, "IMG_3961.webp", c.PhotoWebPName("a", "IMG_3961.jpg"))
		assert.Equal(t, "IMG_3961.mp4", c.PhotoOutputName("a", "IMG_3961.HEIC", ".mp4"))
		assert.Equal(t, "a.b.c.webp", c.PhotoWebPName("a", "a.b.c.jpg"), "only the final extension is replaced")
		assert.Equal(t, "no-extension.webp", c.PhotoWebPName("a", "no-extension"))
	})
}
