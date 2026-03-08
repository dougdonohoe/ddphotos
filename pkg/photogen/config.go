package photogen

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
)

var validSiteID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Config captures build parameters for the photogen pipeline.
type Config struct {
	// OutputRoot is the destination directory for generated assets and JSON.
	OutputRoot string
	// SiteID is the site identifier from settings.id; all output goes under albums/{SiteID}/.
	SiteID string
	// DryRun toggles side effect free execution for smoke-testing.
	DryRun bool
	// SkipVariant skips Variants phase
	SkipVariant bool
	// Limit caps the number of photos processed per album (0 = no limit).
	Limit int
	// Force regenerates output files even if they already exist.
	Force bool
	// Resize enables generating resized image variants (thumb, grid, full).
	Resize bool
	// Index enables generating JSON index files (albums.json and per-album index.json).
	Index bool
	// SiteURL is the base URL for sitemap generation (e.g., "https://photos.example.com").
	SiteURL string
	// NumWorkers is the number of concurrent resize workers (0 = auto-detect based on CPU count).
	NumWorkers int
	// Warn collects warnings for re-display at the end of the run.
	Warn *WarnCollector
}

// AlbumConfig describes an album source folder and metadata overrides.
type AlbumConfig struct {
	// Slug is used in filenames and such
	Slug string
	// Name is a human-readable label surfaced in logs.
	Name string
	// Path is the absolute or repo-relative directory containing original photos.
	Path string
	// Cover is the filename of the cover photo (optional, defaults to first photo).
	Cover string
	// ManualSortOrder, if true, uses the order from photogen.txt (if present)
	// instead of sorting photos by EXIF date.
	ManualSortOrder bool
	// Description is an optional blurb shown on the album page.
	Description string
}

// Validate ensures the config is valid before running processors.
func (c *Config) Validate() error {
	if c.OutputRoot == "" {
		return fmt.Errorf("output directory must be set")
	}
	if c.SiteID == "" {
		return fmt.Errorf("settings.id is required")
	}
	if !validSiteID.MatchString(c.SiteID) {
		return fmt.Errorf("settings.id %q must contain only lowercase letters, digits, and hyphens", c.SiteID)
	}
	return nil
}

// SiteOutputPath returns the root output directory for all photogen-generated content:
// {OutputRoot}/albums/{SiteID}[/parts...]
func (c *Config) SiteOutputPath(parts ...string) string {
	base := []string{c.OutputRoot, "albums", c.SiteID}
	return filepath.Join(append(base, parts...)...)
}

// Workers returns the number of concurrent resize workers to use. If NumWorkers
// is positive it is used as-is; otherwise it auto-detects as NumCPU/2, min 2.
func (c *Config) Workers() int {
	if c.NumWorkers > 0 {
		return c.NumWorkers
	}
	n := runtime.NumCPU() / 2
	if n < 2 {
		return 2
	}
	return n
}
