package photogen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadAlbumSummaries reads an albums.json file and returns the list of album summaries.
func LoadAlbumSummaries(path string) ([]AlbumSummary, error) {
	return loadJSON[[]AlbumSummary](path)
}

// LoadAlbumIndex reads an index.json file and returns the album index.
func LoadAlbumIndex(path string) (*AlbumIndex, error) {
	idx, err := loadJSON[AlbumIndex](path)
	if err != nil {
		return nil, err
	}
	return &idx, nil
}

// Save writes the AlbumIndex to the given path as formatted JSON.
func (idx *AlbumIndex) Save(path string) error {
	return writeJSON(path, idx)
}

// SaveAlbumSummaries writes a slice of AlbumSummary to the given path as formatted JSON.
func SaveAlbumSummaries(path string, summaries []AlbumSummary) error {
	return writeJSON(path, summaries)
}

// AlbumIndex is the structure for each album's index.json
type AlbumIndex struct {
	Slug   string       `json:"slug"`
	Title  string       `json:"title"`
	Photos []PhotoIndex `json:"photos"`
}

// PhotoIndex represents a photo in the JSON output.
type PhotoIndex struct {
	ID          string        `json:"id"`
	FileName    string        `json:"fileName"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Orientation string        `json:"orientation"`
	Date        string        `json:"date"`                  // ISO 8601 date
	Description string        `json:"description,omitempty"` // from photogen.txt
	Src         PhotoSrcIndex `json:"src"`
}

// PhotoSrcIndex contains paths to image variants.
type PhotoSrcIndex struct {
	Grid string `json:"grid"`
	Full string `json:"full"`
}

// AlbumSummary is the structure for each album in albums.json
type AlbumSummary struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Count       int    `json:"count"`
	Cover       string `json:"cover"`                 // path to cover image (first photo's thumb, WebP)
	CoverJpeg   string `json:"coverJpeg"`             // path to cover JPEG for OG images (broad crawler support)
	DateSpan    string `json:"dateSpan"`              // e.g., "Apr 2024" or "Apr - May 2024"
	Description string `json:"description,omitempty"` // optional blurb shown on album page
}

// WriteAlbumIndex writes the index.json for this album.
func (ap *AlbumProcessor) WriteAlbumIndex() error {
	index := AlbumIndex{
		Slug:   ap.AlbumConfig.Slug,
		Title:  ap.AlbumConfig.Name,
		Photos: make([]PhotoIndex, 0, len(ap.Photos)),
	}

	for _, photo := range ap.Photos {
		dateStr := ""
		if !photo.DateTaken.IsZero() {
			dateStr = photo.DateTaken.Format("2006-01-02")
		}

		pi := PhotoIndex{
			ID:          photo.ID,
			FileName:    photo.FileName,
			Width:       photo.Width,
			Height:      photo.Height,
			Orientation: photo.Orientation,
			Date:        dateStr,
			Description: photo.Description,
			Src: PhotoSrcIndex{
				Grid: ap.relativeSrcPath(SizeGrid, photo.FileName),
				Full: ap.relativeSrcPath(SizeFull, photo.FileName),
			},
		}
		index.Photos = append(index.Photos, pi)
	}

	outputPath := ap.OutputPath("index.json")

	if ap.Config.DryRun {
		fmt.Printf("  DRYRUN: would write %s (%d photos)\n", outputPath, len(index.Photos))
		return nil
	}

	return index.Save(outputPath)
}

// relativeSrcPath returns the relative path for a photo variant (relative to album dir).
func (ap *AlbumProcessor) relativeSrcPath(size ImageSize, fileName string) string {
	return filepath.Join(string(size), WebPFileName(fileName))
}

// GetAlbumSummary returns summary info for albums.json
func (ap *AlbumProcessor) GetAlbumSummary() AlbumSummary {
	summary := AlbumSummary{
		Slug:  ap.AlbumConfig.Slug,
		Title: ap.AlbumConfig.Name,
		Count: len(ap.Photos),
	}

	if cover := ap.coverPhoto(); cover != nil {
		summary.Cover = filepath.Join(ap.AlbumConfig.Slug, string(SizeGrid), WebPFileName(cover.FileName))
		summary.CoverJpeg = filepath.Join(ap.AlbumConfig.Slug, "cover.jpg")
		summary.DateSpan = ap.computeDateSpan()
	}

	summary.Description = ap.AlbumConfig.Description

	return summary
}

// computeDateSpan returns a human-readable date range for the album.
func (ap *AlbumProcessor) computeDateSpan() string {
	if len(ap.Photos) == 0 {
		return ""
	}

	first := ap.Photos[0].DateTaken
	last := ap.Photos[len(ap.Photos)-1].DateTaken

	if first.IsZero() && last.IsZero() {
		return ""
	}

	if first.IsZero() {
		return last.Format("Jan 2006")
	}
	if last.IsZero() {
		return first.Format("Jan 2006")
	}

	// Same month and year
	if first.Year() == last.Year() && first.Month() == last.Month() {
		return first.Format("Jan 2006")
	}

	// Same year, different months
	if first.Year() == last.Year() {
		return fmt.Sprintf("%s - %s %d", first.Format("Jan"), last.Format("Jan"), first.Year())
	}

	// Different years
	return fmt.Sprintf("%s - %s", first.Format("Jan 2006"), last.Format("Jan 2006"))
}

// WriteAlbumsIndex writes albums.json into siteDir (cfg.SiteOutputPath()).
func WriteAlbumsIndex(siteDir string, summaries []AlbumSummary, dryRun bool) error {
	outputPath := filepath.Join(siteDir, "albums.json")

	if dryRun {
		fmt.Printf("DRYRUN: would write %s (%d albums)\n", outputPath, len(summaries))
		return nil
	}

	return SaveAlbumSummaries(outputPath, summaries)
}

// WriteSitemap generates sitemap.xml into siteDir (cfg.SiteOutputPath()).
func WriteSitemap(siteDir, siteURL string, summaries []AlbumSummary, dryRun bool) error {
	outputPath := filepath.Join(siteDir, "sitemap.xml")

	if dryRun {
		fmt.Printf("DRYRUN: would write %s (%d URLs)\n", outputPath, len(summaries)+1)
		return nil
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create sitemap: %w", err)
	}
	defer file.Close()

	// Write XML header and urlset opening tag
	file.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>` + siteURL + `/</loc>
  </url>
`)

	// Write each album URL
	for _, album := range summaries {
		file.WriteString(`  <url>
    <loc>` + siteURL + `/albums/` + album.Slug + `</loc>
  </url>
`)
	}

	file.WriteString(`</urlset>
`)

	fmt.Printf("  wrote: %s\n", outputPath)
	return nil
}

// writeJSON writes data as formatted JSON to the given path.
func writeJSON(path string, data any) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	fmt.Printf("  wrote: %s\n", path)
	return nil
}
