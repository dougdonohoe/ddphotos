package photogen

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var allowedPhotoExtentions = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
}

type AlbumProcessor struct {
	Config      *Config
	AlbumConfig *AlbumConfig
	Photos      []*Photo
}

type Photo struct {
	ID           string `json:"id"`
	FileName     string `json:"fileName"`
	AbsolutePath string `json:"-"`
	Description  string `json:"description,omitempty"`
	*PhotoMetadata
}

// String returns a human-readable representation of the photo for logging.
func (p *Photo) String() string {
	dateStr := "no date"
	if !p.DateTaken.IsZero() {
		dateStr = p.DateTaken.Format("2006-01-02 15:04")
	}
	s := fmt.Sprintf("%s (%dx%d %s, %s)", p.FileName, p.Width, p.Height, p.Orientation, dateStr)
	if p.Description != "" {
		s += " - " + p.Description
	}
	return s
}

// warnf prints a warning immediately via the Config's WarnCollector (which also
// stores it for the end-of-run summary). The album name is inserted after
// "WARN: " so every warning is identifiable in the end-of-run summary.
// Falls back to fmt.Printf when Config is nil (e.g., in unit tests).
func (ap *AlbumProcessor) warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if ap.AlbumConfig != nil {
		msg = strings.Replace(msg, "WARN: ", "WARN: ["+ap.AlbumConfig.Name+"] ", 1)
	}
	if ap.Config != nil {
		ap.Config.Warn.Warn(msg)
	} else {
		fmt.Print(msg)
	}
}

func NewAlbumProcessor(cfg *Config, albumConfig *AlbumConfig) *AlbumProcessor {
	return &AlbumProcessor{
		Config:      cfg,
		AlbumConfig: albumConfig,
	}
}

// OutputPath returns the full path for an output file within this album's directory.
// Example: ap.OutputPath("grid", "photo.jpg") -> outputRoot/albums-{id}/album-slug/grid/photo.jpg
func (ap *AlbumProcessor) OutputPath(parts ...string) string {
	base := []string{ap.Config.SiteOutputPath(), ap.AlbumConfig.Slug}
	return filepath.Join(append(base, parts...)...)
}

func (ap *AlbumProcessor) Process(index, total int) error {
	fmt.Printf("Processing %d/%d - %s (%s)...\n", index, total, ap.AlbumConfig.Name, ap.AlbumConfig.Description)

	// load photos
	err := ap.LoadPhotos()
	if err != nil {
		fmt.Printf("Error loading photos: %v\n", err)
		return err
	}

	// resize photos if enabled
	if ap.Config.Resize {
		if err := ap.ResizePhotos(); err != nil {
			fmt.Printf("Error resizing photos: %v\n", err)
			return err
		}
		if err := ap.WriteCoverJPEG(); err != nil {
			fmt.Printf("Error writing cover JPEG: %v\n", err)
			return err
		}
	}

	// write album index.json if enabled
	if ap.Config.Index {
		if err := ap.WriteAlbumIndex(); err != nil {
			fmt.Printf("Error writing album index: %v\n", err)
			return err
		}
	}

	return nil
}

func (ap *AlbumProcessor) LoadPhotos() error {
	files, err := os.ReadDir(ap.AlbumConfig.Path)
	if err != nil {
		ap.warnf("WARN: Error reading %s: %s\n", ap.AlbumConfig.Path, err)
		return err
	}

	for _, file := range files {
		// Check limit before processing more photos
		if ap.Config.Limit > 0 && len(ap.Photos) >= ap.Config.Limit {
			break
		}

		name := file.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := allowedPhotoExtentions[ext]; !ok {
			continue
		}

		fullPath := path.Join(ap.AlbumConfig.Path, name)
		photo := &Photo{
			ID:           strings.ToLower(strings.TrimSuffix(name, ext)),
			FileName:     name,
			AbsolutePath: fullPath,
		}

		meta, err := ReadPhotoMetadata(fullPath)
		if err != nil {
			return fmt.Errorf("read metadata for %s: %w", name, err)
		}
		photo.PhotoMetadata = meta

		ap.Photos = append(ap.Photos, photo)
	}

	// Sort photos by date taken ascending (default order)
	sort.Slice(ap.Photos, func(i, j int) bool {
		return ap.Photos[i].DateTaken.Before(ap.Photos[j].DateTaken)
	})

	// Load descriptions from photogen.txt (if present)
	pd, err := loadPhotoDescriptions(ap.AlbumConfig.Path)
	if err != nil {
		ap.warnf("  WARN: %v\n", err)
	}

	// Apply descriptions to photos
	for _, photo := range ap.Photos {
		if desc, ok := pd.descriptions[photo.ID]; ok {
			photo.Description = desc
		} else if len(pd.descriptions) > 0 {
			ap.warnf("  WARN: description not found for %s\n", photo.ID)
		}
	}

	// Apply manual sort order if configured and photogen.txt was found
	if ap.AlbumConfig.ManualSortOrder && len(pd.order) > 0 {
		ap.Photos = ap.reorderByDescriptionFile(ap.Photos, pd.order)
		fmt.Printf("  Manual sort order applied from photogen.txt\n")
	}

	// Log photos after sorting, count photos with/without dates
	noDates := 0
	for _, photo := range ap.Photos {
		fmt.Printf("  %s\n", photo.String())
		if photo.DateTaken.IsZero() {
			noDates++
		}
	}
	if noDates > 0 {
		ap.warnf("  WARN: %d/%d photos have no EXIF date\n", noDates, len(ap.Photos))
	}

	return nil
}

// photoDescriptions holds the parsed contents of a photogen.txt file.
type photoDescriptions struct {
	descriptions map[string]string // photo ID (filename without ext) -> description
	order        []string          // photo IDs in file order
}

// loadPhotoDescriptions reads photogen.txt from albumPath.
// Format: one line per photo: "filename_without_extension Description"
// Returns an empty result (no error) if the file does not exist.
func loadPhotoDescriptions(albumPath string) (*photoDescriptions, error) {
	pd := &photoDescriptions{
		descriptions: make(map[string]string),
	}

	txtPath := filepath.Join(albumPath, "photogen.txt")
	err := scanLines(txtPath, func(line string) {
		parts := strings.SplitN(line, " ", 2)
		id := strings.ToLower(parts[0])
		desc := ""
		if len(parts) > 1 {
			desc = parts[1]
		}
		pd.descriptions[id] = desc
		pd.order = append(pd.order, id)
	})
	if err != nil {
		if os.IsNotExist(err) {
			return pd, nil
		}
		return pd, fmt.Errorf("read photogen.txt: %w", err)
	}

	fmt.Printf("  Loaded photogen.txt: %d entries\n", len(pd.order))
	return pd, nil
}

// reorderByDescriptionFile rebuilds the photo list using the order from photogen.txt.
// Photos not mentioned are warned about, sorted by date, and appended at the end.
func (ap *AlbumProcessor) reorderByDescriptionFile(photos []*Photo, order []string) []*Photo {
	byID := make(map[string]*Photo, len(photos))
	for _, p := range photos {
		byID[p.ID] = p
	}

	seen := make(map[string]bool, len(order))
	result := make([]*Photo, 0, len(photos))

	for _, id := range order {
		p, ok := byID[id]
		if !ok {
			ap.warnf("  WARN: photogen.txt references unknown photo: %s\n", id)
			continue
		}
		result = append(result, p)
		seen[p.ID] = true
	}

	// Collect photos not mentioned in photogen.txt, sort by date, append at end
	var extras []*Photo
	for _, p := range photos {
		if !seen[p.ID] {
			extras = append(extras, p)
		}
	}
	if len(extras) > 0 {
		ap.warnf("  WARN: %d photo(s) not in photogen.txt (sorted by date, appended at end)\n", len(extras))
		sort.Slice(extras, func(i, j int) bool {
			return extras[i].DateTaken.Before(extras[j].DateTaken)
		})
		result = append(result, extras...)
	}

	return result
}

// coverPhoto returns the configured cover photo, or the first photo if no cover is configured.
// Returns nil if the album has no photos.
func (ap *AlbumProcessor) coverPhoto() *Photo {
	if len(ap.Photos) == 0 {
		return nil
	}
	if ap.AlbumConfig.Cover != "" {
		for _, p := range ap.Photos {
			if p.FileName == ap.AlbumConfig.Cover {
				return p
			}
		}
	}
	return ap.Photos[0]
}

// WriteCoverJPEG generates a JPEG version of the album cover for use as an Open Graph image.
// Output: outputRoot/albums/{slug}/cover.jpg
func (ap *AlbumProcessor) WriteCoverJPEG() error {
	cover := ap.coverPhoto()
	if cover == nil {
		return nil
	}
	outputPath := ap.OutputPath("cover.jpg")
	result, err := ResizeCoverJPEG(cover.AbsolutePath, outputPath, ap.Config.Force, ap.Config.DryRun)
	if err != nil {
		return fmt.Errorf("write cover jpeg: %w", err)
	}
	fmt.Printf("  %s\n", result.Message)
	return nil
}
