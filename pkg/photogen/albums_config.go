package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AlbumsFile is the top-level structure parsed from an albums YAML file.
type AlbumsFile struct {
	Settings AlbumsSettings    `yaml:"settings"`
	Bases    map[string]string `yaml:"bases"`
	Albums   []AlbumEntry      `yaml:"albums"`
}

// AlbumsSettings holds site-level configuration from the YAML settings block.
type AlbumsSettings struct {
	ID               string     `yaml:"id"`                 // site identifier; output goes to {DDPHOTOS_ALBUMS_DIR}/{id}/
	SiteName         string     `yaml:"site_name"`          // displayed in page title and OG tags
	SiteURL          string     `yaml:"site_url"`           // base URL for sitemap and OG tags
	SiteDescription  string     `yaml:"site_description"`   // meta description and OG description
	CopyrightOwner   string     `yaml:"copyright_owner"`    // name shown in footer copyright
	CopyrightYear    int        `yaml:"copyright_year"`     // start year shown in footer copyright
	AllowCrawling    bool       `yaml:"allow_crawling"`     // controls robots.txt (default: false)
	Descriptions     string     `yaml:"descriptions"`       // filename relative to config dir
	Passwords        string     `yaml:"passwords"`          // filename relative to config dir; enables encryption
	CustomCSS        string     `yaml:"css"`                // filename relative to config dir; copied to output
	DefaultTheme     string     `yaml:"default_theme"`      // "light" or "dark" (default: "dark")
	SiteTitleHTML    string     `yaml:"site_title_html"`    // HTML for site title; falls back to site_name
	SiteSubtitleHTML string     `yaml:"site_subtitle_html"` // HTML shown below site title
	SiteOverviewHTML string     `yaml:"site_overview_html"` // HTML shown above album cards
	Hero             *HeroEntry `yaml:"hero"`

	// Resolved paths (populated by ToAlbumConfigs; not from YAML).
	HeroImagePath string `yaml:"-"`
	CustomCSSPath string `yaml:"-"`
}

// HeroEntry configures a full-width hero image displayed at the top of the home page.
type HeroEntry struct {
	Image string `yaml:"image"` // filename; joined to Base if set, else relative to config dir
	Base  string `yaml:"base"`  // optional key into Bases map (same as album entries)
	Crop  string `yaml:"crop"`  // vertical crop anchor: "top" | "center" | "bottom" (default: center)
}

// AlbumEntry is the YAML representation of a single album.
type AlbumEntry struct {
	Slug            string `yaml:"slug"`
	Name            string `yaml:"name"`
	Base            string `yaml:"base"`        // optional key into Bases map
	Source          string `yaml:"source"`      // path joined to base, or absolute/configDir-relative
	Cover           string `yaml:"cover"`       // optional cover photo source-relative path (e.g. "subfolder/photo.jpg")
	Description     string `yaml:"description"` // optional inline description; takes precedence over descriptions file
	ManualSortOrder bool   `yaml:"manual_sort_order"`
	Recurse         bool   `yaml:"recurse"` // if true, collect photos from subdirectories recursively
}

// LoadAlbumsFile reads and parses an albums YAML file. It validates required fields
// and base references but does not resolve or check path existence on disk.
func LoadAlbumsFile(path string) (*AlbumsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var af AlbumsFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := af.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &af, nil
}

// validate checks required fields and that all base references exist in the bases map.
func (af *AlbumsFile) validate() error {
	for i, a := range af.Albums {
		if a.Slug == "" {
			return fmt.Errorf("album[%d]: slug is required", i)
		}
		if a.Name == "" {
			return fmt.Errorf("album %q: name is required", a.Slug)
		}
		if a.Source == "" {
			return fmt.Errorf("album %q: source is required", a.Slug)
		}
		if a.Base != "" {
			if _, ok := af.Bases[a.Base]; !ok {
				return fmt.Errorf("album %q: base %q not defined in bases", a.Slug, a.Base)
			}
		}
	}
	if t := af.Settings.DefaultTheme; t != "" && t != "light" && t != "dark" {
		return fmt.Errorf("settings: default_theme must be \"light\" or \"dark\", got %q", t)
	}
	if h := af.Settings.Hero; h != nil {
		if h.Image == "" {
			return fmt.Errorf("hero: image is required")
		}
		if ext := strings.ToLower(filepath.Ext(h.Image)); ext != "" {
			if _, ok := allowedPhotoExtensions[ext]; !ok {
				return fmt.Errorf("hero: image %q has unsupported extension %q", h.Image, ext)
			}
		}
		if h.Base != "" && filepath.IsAbs(h.Image) {
			return fmt.Errorf("hero: image is an absolute path — remove base or use a relative path")
		}
		if h.Base != "" {
			if _, ok := af.Bases[h.Base]; !ok {
				return fmt.Errorf("hero: base %q not defined in bases", h.Base)
			}
		}
	}
	return nil
}

// ToAlbumConfigs resolves source paths, loads descriptions, and returns []*AlbumConfig
// ready for processing. configDir is used to resolve relative paths and locate the
// descriptions file. Returns an error if any source path does not exist on disk.
func (af *AlbumsFile) ToAlbumConfigs(configDir string) ([]*AlbumConfig, error) {
	descriptions := map[string]string{}
	if af.Settings.Descriptions != "" {
		descPath := filepath.Join(configDir, af.Settings.Descriptions)
		var err error
		descriptions, err = LoadAlbumDescriptions(descPath)
		if err != nil {
			return nil, err
		}
	}

	configs := make([]*AlbumConfig, 0, len(af.Albums))
	for _, a := range af.Albums {
		path, err := af.resolvePath(configDir, a)
		if err != nil {
			return nil, err
		}
		desc := a.Description
		if desc == "" {
			desc = descriptions[a.Slug]
		}
		configs = append(configs, &AlbumConfig{
			Slug:            a.Slug,
			Name:            a.Name,
			Path:            path,
			Cover:           a.Cover,
			ManualSortOrder: a.ManualSortOrder,
			Recurse:         a.Recurse,
			Description:     desc,
		})
	}

	if af.Settings.Hero != nil {
		heroPath, err := af.resolveHeroPath(configDir)
		if err != nil {
			return nil, err
		}
		af.Settings.HeroImagePath = heroPath
	}

	if af.Settings.CustomCSS != "" {
		cssPath := filepath.Join(configDir, af.Settings.CustomCSS)
		if _, err := os.Stat(cssPath); err != nil {
			return nil, fmt.Errorf("css: file %q does not exist", cssPath)
		}
		af.Settings.CustomCSSPath = cssPath
	}

	return configs, nil
}

// resolveFSPath resolves a base+relPath combination to an absolute path and verifies
// it exists on disk. If base is non-empty it is looked up in af.Bases; relative base
// paths are anchored to the working directory. If base is empty and relPath is relative,
// it is anchored to configDir. errContext is prepended to any returned error.
func (af *AlbumsFile) resolveFSPath(configDir, base, relPath, errContext string) (string, error) {
	resolved := relPath
	if base != "" {
		basePath := af.Bases[base]
		if !filepath.IsAbs(basePath) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("%s: get working directory: %w", errContext, err)
			}
			basePath = filepath.Join(cwd, basePath)
		}
		resolved = filepath.Join(basePath, relPath)
	} else if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(configDir, resolved)
	}
	if _, err := os.Stat(resolved); err != nil {
		return "", fmt.Errorf("%s: path %q does not exist", errContext, resolved)
	}
	return resolved, nil
}

// resolvePath returns the absolute source path for an album entry, verifying it exists.
func (af *AlbumsFile) resolvePath(configDir string, a AlbumEntry) (string, error) {
	return af.resolveFSPath(configDir, a.Base, a.Source, fmt.Sprintf("album %q", a.Slug))
}

// resolveHeroPath returns the absolute path for the hero image, verifying it exists.
func (af *AlbumsFile) resolveHeroPath(configDir string) (string, error) {
	h := af.Settings.Hero
	return af.resolveFSPath(configDir, h.Base, h.Image, "hero")
}

// LoadAlbumDescriptions reads a descriptions file and returns a slug→description map.
// Each line has the format "slug<whitespace>description". Blank lines and lines
// starting with # are ignored. A slug with no following text gets an empty description.
func LoadAlbumDescriptions(path string) (map[string]string, error) {
	descriptions := map[string]string{}
	err := scanLines(path, func(line string) {
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			descriptions[line] = ""
			return
		}
		slug := line[:idx]
		desc := strings.TrimSpace(line[idx:])
		descriptions[slug] = desc
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return descriptions, nil
}

// LoadAlbumConfigs is the top-level helper: reads configDir/albumsFilename, resolves
// all paths, loads descriptions, and returns album configs ready for processing.
// The YAML settings are also returned so callers can use site_url, output_dir, etc.
func LoadAlbumConfigs(configDir, albumsFilename string) ([]*AlbumConfig, *AlbumsSettings, error) {
	path := filepath.Join(configDir, albumsFilename)
	af, err := LoadAlbumsFile(path)
	if err != nil {
		return nil, nil, err
	}
	configs, err := af.ToAlbumConfigs(configDir)
	if err != nil {
		return nil, nil, err
	}
	return configs, &af.Settings, nil
}

// LoadEncryptConfig loads the EncryptConfig from settings.passwords (resolved relative
// to configDir). Returns nil, nil if Passwords is not set.
func (s *AlbumsSettings) LoadEncryptConfig(configDir string) (*EncryptConfig, error) {
	if s.Passwords == "" {
		return nil, nil
	}
	path := filepath.Join(configDir, s.Passwords)
	return LoadEncryptConfig(path)
}
