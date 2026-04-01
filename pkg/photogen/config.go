package photogen

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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
	// Encrypt holds encryption configuration. nil means no encryption.
	Encrypt *EncryptConfig
	// Clean removes stale output files after processing.
	Clean bool
	// expectedFiles tracks files generated in this run (for --clean).
	expectedFiles map[string]bool
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
	// Recurse, if true, collects photos from subdirectories recursively.
	// Subdirectory photos get a prefixed ID to avoid name collisions.
	Recurse bool
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
	if c.Encrypt != nil {
		if err := c.Encrypt.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SiteOutputPath returns the root output directory for all photogen-generated content:
// {OutputRoot}/albums/{SiteID}[/parts...]
func (c *Config) SiteOutputPath(parts ...string) string {
	base := []string{c.OutputRoot, "albums", c.SiteID}
	return filepath.Join(append(base, parts...)...)
}

// PhotoWebPName returns the WebP output filename for a photo in the given album.
// UUID-format names are only used when the album has an effective password;
// public albums always use the original WebP filename.
func (c *Config) PhotoWebPName(slug, filename string) string {
	if c.Encrypt != nil && c.Encrypt.IsAlbumEncrypted(slug) {
		return c.Encrypt.PhotoWebPName(filename)
	}
	return WebPFileName(filename)
}

// TrackFile registers path as a file generated in this run (for --clean).
// No-op if InitClean has not been called.
func (c *Config) TrackFile(path string) {
	if c.expectedFiles != nil {
		c.expectedFiles[path] = true
	}
}

// InitClean enables expected-file tracking for --clean.
// Must be called before processing begins.
func (c *Config) InitClean() {
	c.expectedFiles = map[string]bool{}
}

// ExpectedFiles returns the set of files tracked via TrackFile.
func (c *Config) ExpectedFiles() map[string]bool {
	return c.expectedFiles
}

// Summary returns a multi-line human-readable summary of the Config fields that
// are not already reported in the caller's info line (mode, limit, site ID).
func (c *Config) Summary() string {
	on := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}

	encryptDesc := "none"
	if c.Encrypt != nil {
		var parts []string
		if c.Encrypt.IsSiteEncrypted() {
			parts = append(parts, "site")
		}
		if n := len(c.Encrypt.AlbumPasswords); n > 0 {
			parts = append(parts, fmt.Sprintf("%d album(s)", n))
		}
		if len(parts) == 0 {
			parts = append(parts, "key only")
		}
		encryptDesc = strings.Join(parts, " + ")
		if c.Encrypt.PwFile != "" {
			encryptDesc += fmt.Sprintf(" (%s)", c.Encrypt.PwFile)
		}
	}

	lines := []string{
		fmt.Sprintf("  output:   %s", c.SiteOutputPath()),
		fmt.Sprintf("  resize:   %s", on(c.Resize)),
		fmt.Sprintf("  index:    %s", on(c.Index)),
		fmt.Sprintf("  force:    %s", on(c.Force)),
		fmt.Sprintf("  clean:    %s", on(c.Clean)),
		fmt.Sprintf("  workers:  %d", c.Workers()),
		fmt.Sprintf("  site_url: %s", c.SiteURL),
		fmt.Sprintf("  encrypt:  %s", encryptDesc),
	}
	return strings.Join(lines, "\n")
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
