package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
	"github.com/dougdonohoe/ddphotos/pkg/photogen"
)

// repoRoot is embedded at build time via:
//
//	go build -ldflags "-X main.repoRoot=/path/to/repo"
//
// When set, loadDefaultsEnv looks for config/defaults.env there first.
// Falls back to cwd-relative path so `go run ./cmd/photogen` still works from the repo root.
var repoRoot string

// loadDefaultsEnv reads config/defaults.env and sets any keys not already in the environment.
// This mirrors the behaviour of vite.config.ts and the shell scripts.
func loadDefaultsEnv() {
	candidates := []string{filepath.Join("config", "defaults.env")}
	if repoRoot != "" {
		candidates = append([]string{filepath.Join(repoRoot, "config", "defaults.env")}, candidates...)
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				continue
			}
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if _, exists := os.LookupEnv(key); !exists {
				os.Setenv(key, val) //nolint:errcheck
			}
		}
		return // parsed successfully
	}
	// No defaults.env found — explicit env var or -out flag must provide the albums dir.
}

var (
	configDir  = flag.String("config-dir", "config", "directory containing albums YAML and descriptions files")
	outputDir  = flag.String("out", "", "albums directory override (overrides DDPHOTOS_ALBUMS_DIR env var)")
	doit       = flag.Bool("doit", false, "do actual work; otherwise log planned work without writing any files")
	limit      = flag.Int("limit", 0, "limit number of photos per album (0 = no limit)")
	force      = flag.Bool("force", false, "regenerate output files even if they already exist")
	resize     = flag.Bool("resize", false, "generate resized image variants (grid, full)")
	index      = flag.Bool("index", false, "generate JSON index files (albums.json and per-album index.json)")
	siteURL    = flag.String("site-url", "", "base URL for sitemap generation (overrides YAML site_url)")
	numWorkers = flag.Int("workers", 0, "number of concurrent resize workers (0 = auto: NumCPU/2, min 2)")
	albumFlag  = flag.String("album", "", "comma-separated list of album slugs to process (empty = all)")
	siteID     = flag.String("site-id", "", "override settings.id from albums YAML")
	passwords  = flag.String("passwords", "", "path to passwords file; overrides settings.passwords in albums YAML")
	css        = flag.String("css", "", "path to custom CSS file; overrides settings.css in albums YAML")
	clean      = flag.Bool("clean", false, "remove stale output files not generated in this run")
	heroOnly   = flag.Bool("hero-only", false, "regenerate hero image only, skipping all album and index processing")
)

func main() {
	flag.Parse()
	exit.HandleSignal()
	loadDefaultsEnv()

	albums, settings, err := photogen.LoadAlbumConfigs(*configDir, "albums.yaml")
	if err != nil {
		exit.Fatal("Error loading config", err)
	}

	// CLI flags override YAML settings when provided
	resolvedSiteID := settings.ID
	if *siteID != "" {
		resolvedSiteID = *siteID
	}
	resolvedSiteURL := settings.SiteURL
	if *siteURL != "" {
		resolvedSiteURL = *siteURL
	}

	// Albums directory: -out flag > DDPHOTOS_ALBUMS_DIR env var > defaults.env (loaded above)
	resolvedAlbumsDir := os.Getenv("DDPHOTOS_ALBUMS_DIR")
	if *outputDir != "" {
		resolvedAlbumsDir = *outputDir
	}
	if resolvedAlbumsDir == "" {
		fmt.Println("Error: albums output directory is not set.")
		fmt.Println("  Set DDPHOTOS_ALBUMS_DIR in the environment, use the -out flag,")
		fmt.Println("  or ensure config/defaults.env is present in the working directory.")
		exit.ExitWithStatus(fmt.Errorf("DDPHOTOS_ALBUMS_DIR not set"))
	}

	resolvedCSSPath := settings.CustomCSSPath
	if *css != "" {
		resolvedCSSPath = *css
	}

	warn := &photogen.WarnCollector{}
	cfg := &photogen.Config{
		OutputRoot:       filepath.Clean(resolvedAlbumsDir),
		SiteID:           resolvedSiteID,
		DryRun:           !(*doit),
		SkipVariant:      true,
		Limit:            *limit,
		Force:            *force,
		Resize:           *resize,
		Index:            *index,
		SiteName:         settings.SiteName,
		SiteURL:          resolvedSiteURL,
		SiteDescription:  settings.SiteDescription,
		CopyrightOwner:   settings.CopyrightOwner,
		CopyrightYear:    settings.CopyrightYear,
		AllowCrawling:    settings.AllowCrawling,
		NumWorkers:       *numWorkers,
		Warn:             warn,
		CustomCSS:        resolvedCSSPath,
		DefaultTheme:     settings.DefaultTheme,
		SiteTitleHTML:    settings.SiteTitleHTML,
		SiteSubtitleHTML: settings.SiteSubtitleHTML,
		SiteOverviewHTML: settings.SiteOverviewHTML,
	}

	// -passwords overrides settings.passwords; fall back to YAML setting if flag not provided
	passwordsPath := *passwords
	if passwordsPath == "" && settings.Passwords != "" {
		passwordsPath = filepath.Join(*configDir, settings.Passwords)
	}
	if passwordsPath != "" {
		ec, err := photogen.LoadEncryptConfig(passwordsPath)
		if err != nil {
			exit.Fatal("Error loading encrypt config", err)
		}
		cfg.Encrypt = ec
	}

	if settings.HeroImagePath != "" {
		crop := "center"
		if settings.Hero != nil && settings.Hero.Crop != "" {
			crop = settings.Hero.Crop
		}
		cfg.Hero = &photogen.HeroConfig{
			ImagePath: settings.HeroImagePath,
			Crop:      crop,
		}
	}

	// Don't allow -clean without -resize (you can delete all resized photos!)
	cfg.Clean = *clean
	if *clean {
		if !*resize {
			fmt.Println("ERROR: -clean requires -resize.")
			fmt.Println("Without -resize, photogen does not track resized images, so -clean would")
			fmt.Println("delete all of them. If you really want to remove all output files,")
			fmt.Printf("delete the output directory manually (e.g. rm -rf %s).\n", cfg.SiteOutputPath())
			exit.ExitWithStatus(fmt.Errorf("-clean requires -resize"))
		}
		cfg.InitClean()
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		exit.Fatal("Error", err)
	}

	// --hero-only: regenerate hero image and exit, skipping album and index processing.
	if *heroOnly {
		if cfg.Hero == nil {
			exit.Fatal("Error: --hero-only requires hero image to be configured in YAML", nil)
		}
		cfg.Force = true
		if err := cfg.WriteHeroJPEG(); err != nil {
			exit.Fatal("Error writing hero JPEG", err)
		}
		exit.ExitWithStatus(nil)
	}

	// Filter albums if -album flag is set
	if *albumFlag != "" {
		slugs := make(map[string]bool)
		for _, s := range strings.Split(*albumFlag, ",") {
			slugs[strings.TrimSpace(s)] = true
		}
		var filtered []*photogen.AlbumConfig
		for _, a := range albums {
			if slugs[a.Slug] {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			fmt.Printf("No albums matched -album=%q. Available slugs: ", *albumFlag)
			for i, a := range albums {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Print(a.Slug)
			}
			fmt.Println()
			return
		}
		albums = filtered
	}

	// Print settings info
	mode := "DRYRUN"
	if *doit {
		mode = "DOIT"
	}
	info := fmt.Sprintf("[%s] %d albums", mode, len(albums))
	if *limit > 0 {
		info += fmt.Sprintf(", limit %d photos/album", *limit)
	}
	fmt.Println(info + fmt.Sprintf(" (id = %s)", cfg.SiteID))
	fmt.Println(cfg.Summary())

	var summaries []photogen.AlbumSummary

	for i, albumConfig := range albums {
		if exit.ExitRequested() {
			fmt.Println("Exit requested, stopping.")
			exit.ExitWithStatus(nil)
		}
		album := photogen.NewAlbumProcessor(cfg, albumConfig)
		err := album.Process(i+1, len(albums))
		if err != nil {
			exit.Fatal(fmt.Sprintf("Error processing %s", albumConfig.Name), err)
		}
		summaries = append(summaries, album.GetAlbumSummary())
	}

	// Resize hero image alongside album images when resize is enabled.
	if cfg.Resize {
		if err := cfg.WriteHeroJPEG(); err != nil {
			fmt.Printf("Error writing hero JPEG: %s\n", err)
		}
	}

	// Write albums.json, config.json, sitemap.xml, and custom CSS if index generation is enabled
	if cfg.Index {
		if err := cfg.WriteAlbumsIndex(summaries); err != nil {
			fmt.Printf("Error writing albums index: %s\n", err)
		}
		if err := cfg.WriteConfigJSON(); err != nil {
			fmt.Printf("Error writing config.json: %s\n", err)
		}
		if err := cfg.WriteHTMLFile(); err != nil {
			fmt.Printf("Error writing html file: %s\n", err)
		}
		if err := cfg.WriteSitemap(summaries); err != nil {
			fmt.Printf("Error writing sitemap.xml: %s\n", err)
		}
		if err := cfg.WriteCSSFile(); err != nil {
			fmt.Printf("Error copying CSS: %s\n", err)
		}
	}

	// Clean up old files
	if cfg.Clean {
		fmt.Println("\nCleaning...")
		var slugs []string
		for _, a := range albums {
			slugs = append(slugs, a.Slug)
		}
		if err := photogen.CleanOutputDir(cfg.SiteOutputPath(), slugs, cfg.ExpectedFiles(), cfg.DryRun); err != nil {
			fmt.Printf("Error cleaning output dir: %s\n", err)
		}
	}

	// Summarize warnings
	warn.PrintSummary()

	exit.ExitWithStatus(nil)
}
