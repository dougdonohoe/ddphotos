package main

import (
	"context"
	"errors"
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
// This mirrors the behavior of vite.config.ts and the shell scripts.
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
		for line := range strings.SplitSeq(string(data), "\n") {
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
	configDir       = flag.String("config-dir", "config", "directory containing albums YAML and descriptions files")
	outputDir       = flag.String("out", "", "albums directory override (overrides DDPHOTOS_ALBUMS_DIR env var)")
	doit            = flag.Bool("doit", false, "do actual work; otherwise log planned work without writing any files")
	limit           = flag.Int("limit", 0, "limit number of photos per album (0 = no limit)")
	force           = flag.Bool("force", false, "regenerate output files even if they already exist")
	resize          = flag.Bool("resize", false, "generate resized image variants (grid, full)")
	index           = flag.Bool("index", false, "generate JSON index files (albums.json and per-album index.json)")
	siteURL         = flag.String("site-url", "", "base URL for sitemap generation (overrides YAML site_url)")
	numWorkers      = flag.Int("workers", 0, "number of concurrent resize workers (0 = auto: NumCPU/2, min 2)")
	albumFlag       = flag.String("album", "", "comma-separated list of album slugs to process (empty = all)")
	siteID          = flag.String("site-id", "", "override settings.id from albums YAML")
	passwords       = flag.String("passwords", "", "path to passwords file; overrides settings.passwords in albums YAML")
	css             = flag.String("css", "", "path to custom CSS file; overrides settings.css in albums YAML")
	clean           = flag.Bool("clean", false, "remove stale output files not generated in this run")
	heroOnly        = flag.Bool("hero-only", false, "regenerate hero image only, skipping all album and index processing")
	customization   = flag.String("customization", "", "path to customization file; overrides the default <config-dir>/"+photogen.DefaultCustomizationFile)
	noCustomization = flag.Bool("no-customization", false, "ignore "+photogen.DefaultCustomizationFile+" even if present")
	syncDir         = flag.String("sync-dir", "", "sync directory override (overrides DDPHOTOS_SYNC_DIR env var)")
	syncOnly        = flag.Bool("sync-only", false, "sync albums from their providers, then exit without resizing or indexing")
	noSync          = flag.Bool("no-sync", false, "skip syncing; build synced albums from whatever is already on disk")
)

// saveMetaCache persists the photo metadata cache, reporting failures as warnings
// rather than errors: a cache that could not be written only costs time on the next
// run, it does not invalidate anything that was generated.
func saveMetaCache(cfg *photogen.Config) {
	if err := cfg.MetaCache.Save(); err != nil {
		cfg.Warn.Warnf("WARN: could not save metadata cache: %s\n", err)
	}
}

// siteWriteStep is one site-level output, paired with the phrase used to report it.
// what completes "Error <what>: ...", matching the messages these steps printed when each
// was written out by hand.
type siteWriteStep struct {
	what string
	run  func() error
}

// runSiteWrites performs the site-level writes and returns the first error, or nil.
//
// Returning the error rather than only printing it is the whole point. These six files are
// what the site is assembled from, and photogen's exit status is what a wrapper script of
// the "photogen && deploy" shape keys off, so a run whose albums.json could not be written
// must not report success. This is the same guarantee ErrInterrupted gives the resize pool,
// arrived at from the other direction: there the run stopped early, here it finished but
// did not produce what it said it would.
//
// Every step is still attempted after one fails, which is the behavior these calls already
// had. They write independent files, and stopping at the first problem would make the user
// rediscover the rest one run at a time.
func runSiteWrites(steps []siteWriteStep) error {
	var firstErr error
	for _, step := range steps {
		if err := step.run(); err != nil {
			fmt.Printf("Error %s: %s\n", step.what, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// validateCleanFlags rejects flag combinations that would make -clean delete output the
// run did not regenerate. CleanOutputDir removes anything under a processed album that is
// not in the expected set, so any flag that leaves real output untracked turns -clean into
// a delete of the user's work.
//
// All three rules protect the same thing from different directions: without -resize nothing
// is tracked at all, without -index none of the JSON is tracked, and with -limit the photo
// list is truncated before tracking happens, so everything past the limit looks unexpected.
// outputPath is named in the first message so the user can remove the directory by hand if
// that is what they actually wanted.
func validateCleanFlags(clean, resize, index bool, limit int, outputPath string) error {
	if !clean {
		return nil
	}
	if !resize {
		return fmt.Errorf("-clean requires -resize.\n"+
			"Without -resize, photogen does not track resized images, so -clean would\n"+
			"delete all of them. If you really want to remove all output files,\n"+
			"delete the output directory manually (e.g. rm -rf %s).", outputPath)
	}
	// Measured before this rule existed: `-resize -clean` on an already-built sample site
	// removed albums.json, config.json, html.json, sitemap.xml and every album's
	// index.json, and exited 0. The images survive, so the result is a site that serves
	// nothing but a 404.
	if !index {
		return errors.New("-clean requires -index.\n" +
			"Without -index, photogen does not track albums.json, config.json,\n" +
			"sitemap.xml, html.json or any album's index.json, so -clean would delete\n" +
			"all of them and leave a site with images but no data.")
	}
	if limit > 0 {
		return errors.New("-clean cannot be combined with -limit.\n" +
			"-limit truncates the photo list before the output files are tracked, so -clean\n" +
			"would delete every already-generated photo past the limit. Drop -limit for a\n" +
			"full run, or drop -clean while developing.")
	}
	return nil
}

func main() {
	flag.Parse()
	exit.HandleSignal()
	loadDefaultsEnv()

	// Config loading runs in two halves, and the order matters.
	//
	// LoadAlbumsFile parses and validates the YAML without touching the filesystem.
	// ToAlbumConfigs is what resolves and stats every album source, and it fails outright
	// on a directory that does not exist. A synced album's source is a folder photogen
	// creates itself, under a path that includes the resolved site ID — which -site-id can
	// override — so the site ID and the sync root have to be settled, and the folders
	// created, before ToAlbumConfigs runs. Hence, the split.
	af, err := photogen.LoadAlbumsFile(filepath.Join(*configDir, "albums.yaml"))
	if err != nil {
		exit.Fatal("Error loading config", err)
	}
	settings := &af.Settings

	customizations, err := photogen.ResolveCustomizations(*configDir, *customization, *noCustomization)
	if err != nil {
		exit.Fatal("Error loading customizations", err)
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

	// Sync directory: -sync-dir flag > DDPHOTOS_SYNC_DIR env var > defaults.env, mirroring
	// the albums directory above. Only looked at when an album actually syncs, so a site
	// with no sync: blocks never needs it set.
	var syncPaths *photogen.SyncPaths
	if af.HasSyncedAlbums() {
		resolvedSyncDir := os.Getenv("DDPHOTOS_SYNC_DIR")
		if *syncDir != "" {
			resolvedSyncDir = *syncDir
		}
		if resolvedSyncDir == "" {
			fmt.Println("Error: sync directory is not set, but albums.yaml has a sync: block.")
			fmt.Println("  Set DDPHOTOS_SYNC_DIR in the environment, use the -sync-dir flag,")
			fmt.Println("  or ensure config/defaults.env is present in the working directory.")
			exit.ExitWithStatus(fmt.Errorf("DDPHOTOS_SYNC_DIR not set"))
		}
		syncPaths = &photogen.SyncPaths{Root: filepath.Clean(resolvedSyncDir), SiteID: resolvedSiteID}
		// Created before ToAlbumConfigs, which stats every album source. Unconditional, so
		// -no-sync on an album that has never synced yields an empty album rather than an
		// error about a path photogen was going to create anyway.
		if err := af.CreateSyncDirs(syncPaths); err != nil {
			exit.Fatal("Error creating sync directories", err)
		}
	}

	albums, err := af.ToAlbumConfigs(*configDir, syncPaths)
	if err != nil {
		exit.Fatal("Error loading config", err)
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
		AlbumNav:         customizations.AlbumNav,
	}

	// Summarize syncing
	if syncPaths != nil {
		n := 0
		for _, a := range albums {
			if a.Sync != nil {
				n++
			}
		}
		cfg.Sync = &photogen.SyncSummary{Root: syncPaths.Root, Albums: n, Skip: *noSync}
	}

	// Photo metadata (dimensions, orientation, EXIF date) is cached between runs so
	// unchanged photos are not re-decoded. -force means "redo everything", so it
	// re-reads every photo and rewrites those entries.
	cfg.MetaCache = photogen.LoadMetaCache(photogen.MetaCachePath(cfg.OutputRoot), warn)
	cfg.MetaCache.SetRefresh(*force)

	// The descriptions file is keyed by slug and read with a plain map lookup, so an entry
	// naming no album just produces an album with no blurb. Reported here rather than inside
	// ToAlbumConfigs, and worded like the passwords warning below, because it is the same
	// mistake in the other config file.
	if stale := settings.UnknownDescriptionSlugs; len(stale) > 0 {
		warn.Warnf("WARN: descriptions file %s has entries for albums not in albums.yaml (ignored): %s\n",
			filepath.Join(*configDir, settings.Descriptions), strings.Join(stale, ", "))
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
		// Drop entries for albums that are not in albums.yaml: they protect nothing, so
		// they must not make the site look encrypted (which would show the logout button).
		// Pruned here, against the full album list, because -album filtering happens below.
		allSlugs := make([]string, 0, len(albums))
		for _, a := range albums {
			allSlugs = append(allSlugs, a.Slug)
		}
		if stale := ec.RestrictToAlbums(allSlugs); len(stale) > 0 {
			warn.Warnf("WARN: passwords file %s has entries for albums not in albums.yaml (ignored): %s\n",
				passwordsPath, strings.Join(stale, ", "))
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

	cfg.Clean = *clean
	if err := validateCleanFlags(*clean, *resize, *index, *limit, cfg.SiteOutputPath()); err != nil {
		fmt.Printf("ERROR: %s\n", err)
		exit.ExitWithStatus(err)
	}
	if *clean {
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
			saveMetaCache(cfg)
			exit.Fatal("Error writing hero JPEG", err)
		}
		saveMetaCache(cfg)
		exit.ExitWithStatus(nil)
	}

	// Sync stage. It sits after --hero-only, which never syncs, and before -album
	// filtering, which narrows the resize and index stages only: a filtered run still
	// syncs every album, because a folder left half-synced is a folder the next full run
	// silently builds from.
	if err := photogen.RunSync(context.Background(), cfg, albums, *noSync); err != nil {
		warn.PrintSummary()
		exit.Fatal("Error syncing albums", err)
	}
	if *syncOnly {
		// Otherwise -sync-only on a config with no sync: block exits 0 in silence,
		// which reads as "it worked" rather than "there was nothing to do".
		if cfg.Sync == nil {
			fmt.Println("\n[SYNC] nothing to do: no album in albums.yaml has a sync: block")
		}
		warn.PrintSummary()
		exit.ExitWithStatus(nil)
	}

	// Filter albums if -album flag is set
	if *albumFlag != "" {
		slugs := make(map[string]bool)
		for s := range strings.SplitSeq(*albumFlag, ",") {
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
			saveMetaCache(cfg)
			// Non-zero: the remaining albums were never processed and albums.json was
			// never written, so a caller of the "photogen && deploy" shape must not read
			// this as a finished run.
			exit.ExitWithStatus(photogen.ErrInterrupted)
		}
		album := photogen.NewAlbumProcessor(cfg, albumConfig)
		if err := album.Process(i+1, len(albums)); err != nil {
			// Keep what this run already read. A failure partway through is exactly when
			// the user fixes something and runs again, so discarding the metadata cache
			// here would make them pay to re-decode every photo that was fine.
			saveMetaCache(cfg)
			warn.PrintSummary()
			exit.Fatal(fmt.Sprintf("Error processing %s", albumConfig.Name), err)
		}
		summaries = append(summaries, album.GetAlbumSummary())
	}

	// Resize hero image alongside album images when resize is enabled.
	if cfg.Resize {
		if err := cfg.WriteHeroJPEG(); err != nil {
			fmt.Printf("Error writing hero JPEG: %s\n", err)
			exit.SetExitRequestedWithError(err)
		}
	}

	// Saved after the hero so its stamp is recorded too.
	saveMetaCache(cfg)

	// Write albums.json, config.json, sitemap.xml, custom CSS, and build metadata if index generation is enabled
	var siteWriteErr error
	if cfg.Index {
		siteWriteErr = runSiteWrites([]siteWriteStep{
			{"writing albums index", func() error { return cfg.WriteAlbumsIndex(summaries) }},
			{"writing config.json", cfg.WriteConfigJSON},
			{"writing build metadata", func() error { return cfg.WriteBuildMeta(*configDir) }},
			{"writing html file", cfg.WriteHTMLFile},
			{"writing sitemap.xml", func() error { return cfg.WriteSitemap(summaries) }},
			{"copying CSS", cfg.WriteCSSFile},
		})
		if siteWriteErr != nil {
			exit.SetExitRequestedWithError(siteWriteErr)
		}
	}

	// Clean up old files
	if cfg.Clean {
		// A site writer calls TrackFile only after it has written successfully, so a file
		// that failed to write is not in the expected set and -clean would delete the
		// previous, still-good copy on top of not having replaced it. Measured: with
		// albums.json read-only, the run left the site with no albums.json at all.
		//
		// This is validateCleanFlags' rule reached by a different route, so it gets the
		// same answer: anything that leaves real output untracked must not be cleaned
		// against.
		if siteWriteErr != nil {
			fmt.Println("\nSkipping clean: a site file failed to write, so the output does " +
				"not match this run and cleaning would delete the previous copy too.")
		} else {
			fmt.Println("\nCleaning...")
			var slugs []string
			for _, a := range albums {
				slugs = append(slugs, a.Slug)
			}
			if err := photogen.CleanOutputDir(cfg.SiteOutputPath(), slugs, cfg.ExpectedFiles(), cfg.DryRun, warn); err != nil {
				fmt.Printf("Error cleaning output dir: %s\n", err)
				exit.SetExitRequestedWithError(err)
			}
		}
	}

	// Summarize warnings
	warn.PrintSummary()

	exit.ExitWithStatus(nil)
}
