package photogen

import (
	"bufio"
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
	ID           string `yaml:"id"` // site identifier; output goes to albums-{id}/
	SiteURL      string `yaml:"site_url"`
	OutputDir    string `yaml:"output_dir"`
	Descriptions string `yaml:"descriptions"` // filename relative to config dir
}

// AlbumEntry is the YAML representation of a single album.
type AlbumEntry struct {
	Slug            string `yaml:"slug"`
	Name            string `yaml:"name"`
	Base            string `yaml:"base"`   // optional key into Bases map
	Source          string `yaml:"source"` // path joined to base, or absolute/configDir-relative
	Cover           string `yaml:"cover"`  // optional cover photo filename override
	ManualSortOrder bool   `yaml:"manual_sort_order"`
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
		configs = append(configs, &AlbumConfig{
			Slug:            a.Slug,
			Name:            a.Name,
			Path:            path,
			Cover:           a.Cover,
			ManualSortOrder: a.ManualSortOrder,
			Description:     descriptions[a.Slug],
		})
	}
	return configs, nil
}

// resolvePath returns the absolute source path for an album entry, verifying it exists.
// If base is set, source is joined to the named base path; relative base paths are
// resolved relative to the working directory (so they work when photogen is run from
// the repo root). If no base is set, a relative source is resolved relative to configDir
// (so a config-adjacent source directory works without an absolute path).
func (af *AlbumsFile) resolvePath(configDir string, a AlbumEntry) (string, error) {
	src := a.Source
	if a.Base != "" {
		base := af.Bases[a.Base]
		if !filepath.IsAbs(base) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("album %q: get working directory: %w", a.Slug, err)
			}
			base = filepath.Join(cwd, base)
		}
		src = filepath.Join(base, a.Source)
	} else if !filepath.IsAbs(src) {
		src = filepath.Join(configDir, src)
	}
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("album %q: source path %q does not exist", a.Slug, src)
	}
	return src, nil
}

// LoadAlbumDescriptions reads a descriptions file and returns a slug→description map.
// Each line has the format "slug<whitespace>description". Blank lines and lines
// starting with # are ignored. A slug with no following text gets an empty description.
func LoadAlbumDescriptions(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()

	descriptions := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			descriptions[line] = ""
			continue
		}
		slug := line[:idx]
		desc := strings.TrimSpace(line[idx:])
		descriptions[slug] = desc
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
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
