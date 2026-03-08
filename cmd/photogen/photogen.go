package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
	"github.com/dougdonohoe/ddphotos/pkg/photogen"
)

var (
	configDir  = flag.String("config-dir", "config", "directory containing albums YAML and descriptions files")
	albumsFile = flag.String("albums", "albums.yaml", "albums YAML filename within config-dir")
	outputDir  = flag.String("out", "", "output directory for generated albums (overrides YAML output_dir)")
	doit       = flag.Bool("doit", false, "do actual work; otherwise log planned work without writing any files")
	limit      = flag.Int("limit", 0, "limit number of photos per album (0 = no limit)")
	force      = flag.Bool("force", false, "regenerate output files even if they already exist")
	resize     = flag.Bool("resize", false, "generate resized image variants (grid, full)")
	index      = flag.Bool("index", false, "generate JSON index files (albums.json and per-album index.json)")
	siteURL    = flag.String("site-url", "", "base URL for sitemap generation (overrides YAML site_url)")
	numWorkers = flag.Int("workers", 0, "number of concurrent resize workers (0 = auto: NumCPU/2, min 2)")
	albumFlag  = flag.String("album", "", "comma-separated list of album slugs to process (empty = all)")
)

func main() {
	flag.Parse()
	exit.HandleSignal()

	albums, settings, err := photogen.LoadAlbumConfigs(*configDir, *albumsFile)
	if err != nil {
		fmt.Printf("Error loading config: %s\n", err)
		exit.ExitWithStatus(err)
	}

	// CLI flags override YAML settings when provided
	resolvedSiteURL := settings.SiteURL
	if *siteURL != "" {
		resolvedSiteURL = *siteURL
	}
	resolvedOutputDir := settings.OutputDir
	if *outputDir != "" {
		resolvedOutputDir = *outputDir
	}

	warn := &photogen.WarnCollector{}
	cfg := &photogen.Config{
		OutputRoot:  filepath.Clean(resolvedOutputDir),
		SiteID:      settings.ID,
		DryRun:      !(*doit),
		SkipVariant: true,
		Limit:       *limit,
		Force:       *force,
		Resize:      *resize,
		Index:       *index,
		SiteURL:     resolvedSiteURL,
		NumWorkers:  *numWorkers,
		Warn:        warn,
	}

	if err := cfg.Validate(); err != nil {
		fmt.Printf("Error: %s\n", err)
		exit.ExitWithStatus(err)
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

	// Print settings
	mode := "DRYRUN"
	if *doit {
		mode = "DOIT"
	}
	settings2 := fmt.Sprintf("[%s] %d albums", mode, len(albums))
	if *limit > 0 {
		settings2 += fmt.Sprintf(", limit %d photos/album", *limit)
	}
	fmt.Println(settings2 + fmt.Sprintf(" (id = %s)", settings.ID))

	var summaries []photogen.AlbumSummary

	for i, albumConfig := range albums {
		if exit.ExitRequested() {
			fmt.Println("Exit requested, stopping.")
			exit.ExitWithStatus(nil)
		}
		album := photogen.NewAlbumProcessor(cfg, albumConfig)
		err := album.Process(i+1, len(albums))
		if err != nil {
			fmt.Printf("Error processing %s: %s\n", albumConfig.Name, err)
			continue
		}
		summaries = append(summaries, album.GetAlbumSummary())
	}

	// Write albums.json and sitemap.xml if index generation is enabled
	if cfg.Index {
		if err := photogen.WriteAlbumsIndex(cfg.SiteOutputPath(), summaries, cfg.DryRun); err != nil {
			fmt.Printf("Error writing albums.json: %s\n", err)
		}
		if err := photogen.WriteSitemap(cfg.SiteOutputPath(), cfg.SiteURL, summaries, cfg.DryRun); err != nil {
			fmt.Printf("Error writing sitemap.xml: %s\n", err)
		}
	}

	warn.PrintSummary()
	exit.ExitWithStatus(nil)
}
