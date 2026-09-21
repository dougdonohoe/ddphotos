package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

	// UnknownDescriptionSlugs lists entries in the descriptions file that match no album,
	// sorted (populated by ToAlbumConfigs; not from YAML). Reported by the caller rather
	// than here, the same way EncryptConfig.RestrictToAlbums hands back its stale slugs, so
	// that config loading stays free of the warning machinery.
	UnknownDescriptionSlugs []string `yaml:"-"`
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

	// Sync, when set, makes this album's photos come from an upstream photo manager
	// instead of a folder the user maintains. Source and Base are then derived and must
	// not be set. Nil means an ordinary local album.
	Sync *SyncEntry `yaml:"sync"`
}

// SyncEntry configures where an album's photos are fetched from. It is the YAML shape of
// the sync: block; the resolved, run-ready form is AlbumSyncConfig in config.go.
type SyncEntry struct {
	Provider string `yaml:"provider"` // registered provider name ("immich", "mock")
	AlbumID  string `yaml:"album_id"` // provider-specific album identifier
	// Captions controls whether photogen.txt is written and maintained from the upstream
	// descriptions. A pointer because the default is true: a plain bool cannot tell
	// "captions: false" from a key that was never written.
	Captions *bool `yaml:"captions"`
	// Mock holds the mock provider's settings. Provider sub-blocks are modeled as real
	// struct fields because readYAML runs with KnownFields(true), so there is no catch-all
	// to put them in.
	Mock *MockSyncEntry `yaml:"mock"`
}

// CaptionsEnabled reports whether photogen.txt should be maintained for this album,
// applying the documented default of true.
func (s *SyncEntry) CaptionsEnabled() bool {
	return s.Captions == nil || *s.Captions
}

// MockSyncEntry configures the mock provider, whose only job is to make the sync plumbing
// testable without a real upstream. See docs/TESTING.md.
type MockSyncEntry struct {
	Assets   string `yaml:"assets"`    // listing fixture, relative to the config dir
	MediaDir string `yaml:"media_dir"` // folder Fetch reads bytes from, relative to the config dir
	Fail     string `yaml:"fail"`      // "", "list" or "fetch": make the provider fail on demand
}

// LoadAlbumsFile reads and parses an albums YAML file. It validates required fields
// and base references but does not resolve or check path existence on disk.
func LoadAlbumsFile(path string) (*AlbumsFile, error) {
	return loadYAML[AlbumsFile](path)
}

// slugPattern is the permitted album slug format: a letter or digit, then any mix of
// letters, digits, dashes and underscores. A slug is used as a URL path segment and an
// output directory name, so constraining it once here keeps both safe.
//
// It mirrors REGEXP_SLUG in the DD Photos App's PhotosConstants, so both tools accept the
// same album slugs. The site ID is deliberately stricter (validSiteID in config.go, and
// REGEXP_SITE_ID in the app): lowercase only, no underscores, since it names a directory
// that has to behave identically on case-sensitive and case-insensitive filesystems.
var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// slugMaxLen caps an album slug and the site ID. It is the field length limit the DD
// Photos App enforces on both, so anything typable there is accepted here.
const slugMaxLen = 64

// validate checks required fields, that slugs are unique, and that all base references
// exist in the bases map. It runs from LoadAlbumsFile, before ToAlbumConfigs resolves or
// stats any path, so a bad file is rejected before the run touches the filesystem.
func (af *AlbumsFile) validate() error {
	// Keyed by the lowercased slug, holding the slug as written, so one pass catches both
	// an exact repeat and a pair that differs only by case.
	seenSlugs := make(map[string]string, len(af.Albums))

	for i, a := range af.Albums {
		if a.Slug == "" {
			return fmt.Errorf("album[%d]: slug is required", i)
		}
		if !slugPattern.MatchString(a.Slug) {
			return fmt.Errorf("album %q: slug must start with a letter or digit and contain "+
				"only letters, digits, dashes and underscores", a.Slug)
		}
		if len(a.Slug) > slugMaxLen {
			return fmt.Errorf("album %q: slug must be at most %d characters", a.Slug, slugMaxLen)
		}
		// A slug is both the output directory and the URL path segment, so two albums
		// sharing one means the second overwrites the first's index.json and images while
		// albums.json advertises both. Neither -clean nor checkDuplicateIDs can see it:
		// TrackFile accumulates across albums, and the ID check runs per album.
		if prior, dup := seenSlugs[strings.ToLower(a.Slug)]; dup {
			if prior == a.Slug {
				return fmt.Errorf("album %q: duplicate slug; each album needs its own, "+
					"because the slug is the output directory and the URL", a.Slug)
			}
			// Distinct to Go, one directory to macOS and Windows. The build would put both
			// albums in whichever spelling was created first, and a case-sensitive server
			// would then 404 the other spelling's URL.
			return fmt.Errorf("albums %q and %q: slugs differ only by case, which is the "+
				"same directory on a case-insensitive filesystem; give one of them a "+
				"distinct slug", prior, a.Slug)
		}
		seenSlugs[strings.ToLower(a.Slug)] = a.Slug
		// A synced album's source folder is derived (photogen creates and fills it), and
		// its name can come from upstream, so both requirements relax. Everything else
		// about the album is unchanged.
		if a.Sync == nil && a.Name == "" {
			return fmt.Errorf("album %q: name is required", a.Slug)
		}
		if a.Sync == nil && a.Source == "" {
			return fmt.Errorf("album %q: source is required", a.Slug)
		}
		if a.Sync != nil {
			if a.Source != "" || a.Base != "" {
				return fmt.Errorf("album %q: sync and source/base are mutually exclusive — "+
					"a synced album downloads into a folder photogen owns, so remove source and base", a.Slug)
			}
			if err := a.Sync.validate(a.Slug); err != nil {
				return err
			}
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
		// Deliberately photo-only, not IsMediaFile: the hero is a hard-cropped still
		// produced by libvips, so a video source has nothing sensible to fall back to.
		if ext := strings.ToLower(filepath.Ext(h.Image)); ext != "" {
			if !IsPhotoFile(h.Image) {
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

// SyncPaths locates the folders that synced albums download into. It is an input to
// ToAlbumConfigs rather than state on AlbumsFile because the site ID is not knowable from
// the YAML alone: -site-id overrides settings.id, so the caller has to resolve both the
// sync root and the ID before any album path can be derived.
//
// A nil *SyncPaths means sync is not configured for this run, which is an error for any
// album that has a sync: block.
type SyncPaths struct {
	// Root is the resolved DDPHOTOS_SYNC_DIR.
	Root string
	// SiteID is the resolved settings.id, which namespaces the sync root the same way it
	// namespaces the albums output directory.
	SiteID string
}

// validate checks the sync: block's own fields. slug names the album in every message,
// since this runs inside a loop over albums and the block itself has no identity.
func (s *SyncEntry) validate(slug string) error {
	if s.Provider == "" {
		return fmt.Errorf("album %q: sync.provider is required (one of: %s)",
			slug, strings.Join(syncProviderNames(), ", "))
	}
	if !isSyncProvider(s.Provider) {
		return fmt.Errorf("album %q: sync.provider %q is not a known provider (one of: %s)",
			slug, s.Provider, strings.Join(syncProviderNames(), ", "))
	}
	if s.AlbumID == "" {
		return fmt.Errorf("album %q: sync.album_id is required", slug)
	}
	// Only the provider-to-block pairing is checked here, because it is the one rule that
	// needs both halves. Each provider sub-block checks its own fields, so adding a
	// provider does not grow this function.
	if s.Mock != nil {
		if s.Provider != mockProviderName {
			return fmt.Errorf("album %q: sync.mock is only valid with provider %q, not %q",
				slug, mockProviderName, s.Provider)
		}
		if err := s.Mock.validate(slug); err != nil {
			return err
		}
	}
	// Last, because it is the only check that reads a value rather than the shape of the
	// block: told the wrong provider, complaining that album_id is not a UUID would send
	// the reader after the wrong mistake. What a valid id looks like is the provider's
	// business, so this only knows to ask.
	if s.Provider == immichProviderName {
		if err := validateImmichAlbumID(slug, s.AlbumID); err != nil {
			return err
		}
	}
	return nil
}

// validate checks the mock provider's own settings. slug names the album, matching every
// other message raised while validating an album entry.
func (m *MockSyncEntry) validate(slug string) error {
	if m.Assets == "" {
		return fmt.Errorf("album %q: sync.mock.assets is required", slug)
	}
	if m.MediaDir == "" {
		return fmt.Errorf("album %q: sync.mock.media_dir is required", slug)
	}
	switch m.Fail {
	case "", "list", "fetch":
	default:
		return fmt.Errorf("album %q: sync.mock.fail must be \"list\" or \"fetch\", got %q",
			slug, m.Fail)
	}
	return nil
}

// ToAlbumConfigs resolves source paths, loads descriptions, and returns []*AlbumConfig
// ready for processing. configDir is used to resolve relative paths and locate the
// descriptions file. sync locates the download folder for albums with a sync: block and
// may be nil when the run has no sync configured. Returns an error if any source path does
// not exist on disk.
func (af *AlbumsFile) ToAlbumConfigs(configDir string, sync *SyncPaths) ([]*AlbumConfig, error) {
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
		path, err := af.resolvePath(configDir, a, sync)
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
			Sync:            a.resolveSync(configDir),
		})
		delete(descriptions, a.Slug)
	}

	// Whatever is left named no album. A typo'd or renamed slug in the descriptions file
	// used to be silently dropped, leaving an album with no blurb and no hint why, while
	// the same mistake in the passwords file has always been reported.
	if len(descriptions) > 0 {
		af.Settings.UnknownDescriptionSlugs = make([]string, 0, len(descriptions))
		for slug := range descriptions {
			af.Settings.UnknownDescriptionSlugs = append(af.Settings.UnknownDescriptionSlugs, slug)
		}
		sort.Strings(af.Settings.UnknownDescriptionSlugs)
	}

	if af.Settings.Hero != nil {
		heroPath, err := af.resolveHeroPath(configDir)
		if err != nil {
			return nil, err
		}
		af.Settings.HeroImagePath = heroPath
	}

	if af.Settings.CustomCSS != "" {
		cssPath := winToUnixPath(af.Settings.CustomCSS)
		if !filepath.IsAbs(cssPath) {
			cssPath = filepath.Join(configDir, cssPath)
		}
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
	resolved := winToUnixPath(relPath)
	if base != "" {
		basePath := winToUnixPath(af.Bases[base])
		if !filepath.IsAbs(basePath) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("%s: get working directory: %w", errContext, err)
			}
			basePath = filepath.Join(cwd, basePath)
		}
		resolved = filepath.Join(basePath, resolved)
	} else if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(configDir, resolved)
	}
	if _, err := os.Stat(resolved); err != nil {
		return "", fmt.Errorf("%s: path %q does not exist", errContext, resolved)
	}
	return resolved, nil
}

// resolvePath returns the absolute source path for an album entry, verifying it exists.
//
// A synced album's source is derived rather than configured: it is the folder the sync
// stage downloads into. The caller creates those folders before calling ToAlbumConfigs
// (see CreateSyncDirs), so the existence check below holds for them too.
func (af *AlbumsFile) resolvePath(configDir string, a AlbumEntry, sync *SyncPaths) (string, error) {
	errContext := fmt.Sprintf("album %q", a.Slug)
	if a.Sync != nil {
		if sync == nil {
			return "", fmt.Errorf("%s: has a sync: block but no sync directory is configured — "+
				"set DDPHOTOS_SYNC_DIR or pass -sync-dir", errContext)
		}
		path := SyncAlbumPath(sync.Root, sync.SiteID, a.Sync.Provider, a.Slug)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("%s: sync folder %q does not exist", errContext, path)
		}
		return path, nil
	}
	return af.resolveFSPath(configDir, a.Base, a.Source, errContext)
}

// resolveSync returns the run-ready form of the album's sync: block, with the mock
// provider's paths anchored to the config dir the same way every other relative path in
// this file is. Nil for an ordinary local album.
func (a AlbumEntry) resolveSync(configDir string) *AlbumSyncConfig {
	if a.Sync == nil {
		return nil
	}
	cfg := &AlbumSyncConfig{
		Provider: a.Sync.Provider,
		AlbumID:  a.Sync.AlbumID,
		Captions: a.Sync.CaptionsEnabled(),
	}
	if m := a.Sync.Mock; m != nil {
		cfg.Mock = &MockSyncConfig{
			AssetsPath: resolveConfigRelative(configDir, m.Assets),
			MediaDir:   resolveConfigRelative(configDir, m.MediaDir),
			Fail:       m.Fail,
		}
	}
	// Immich has no YAML block to key off, so this hangs off the provider name. Not stat'ed,
	// for the same reason the mock paths are not: the provider reports its own missing file,
	// and here the file is optional anyway because the environment may supply the credentials.
	if a.Sync.Provider == immichProviderName {
		cfg.Immich = &ImmichSyncConfig{EnvFile: filepath.Join(configDir, ImmichEnvFileName)}
	}
	return cfg
}

// resolveConfigRelative anchors a relative path to configDir, leaving an absolute path
// alone. Unlike resolveFSPath it does not stat: the mock provider reports its own missing
// files, with a message that says which key named them.
func resolveConfigRelative(configDir, path string) string {
	p := winToUnixPath(path)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(configDir, p)
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
//
// It passes no SyncPaths, so a file containing a sync: block is rejected. cmd/photogen
// does the two halves itself, because it has to resolve the site ID and create the sync
// folders in between (see the ordering comment in main).
func LoadAlbumConfigs(configDir, albumsFilename string) ([]*AlbumConfig, *AlbumsSettings, error) {
	path := filepath.Join(configDir, albumsFilename)
	af, err := LoadAlbumsFile(path)
	if err != nil {
		return nil, nil, err
	}
	configs, err := af.ToAlbumConfigs(configDir, nil)
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
