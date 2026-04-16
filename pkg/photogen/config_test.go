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
		expected := runtime.NumCPU() / 2
		if expected < 2 {
			expected = 2
		}
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
		tc := tc
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
