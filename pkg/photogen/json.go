package photogen

import (
	"bufio"
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

//
// IMPORTANT: Keep structs in sync with TypeScript types in web/src/lib/types.ts.
//

// AlbumIndex is the structure for each album's index.json
type AlbumIndex struct {
	Slug        string       `json:"slug"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	DateSpan    string       `json:"dateSpan,omitempty"`
	Cover       string       `json:"cover,omitempty"` // grid path of cover photo (e.g. "grid/foo.webp")
	Photos      []PhotoIndex `json:"photos"`
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
	Cover       string `json:"cover,omitempty"`       // path to cover image (first photo's thumb, WebP)
	CoverJpeg   string `json:"coverJpeg,omitempty"`   // path to cover JPEG for OG images (broad crawler support)
	DateSpan    string `json:"dateSpan"`              // e.g., "Apr 2024" or "Apr - May 2024"
	Description string `json:"description,omitempty"` // optional blurb shown on album page
	Encrypted   bool   `json:"encrypted,omitempty"`   // true if album index is encrypted
}

// WriteAlbumIndex writes the index.json (or index.enc.json if encrypted) for this album.
func (ap *AlbumProcessor) WriteAlbumIndex() error {
	cover := ""
	if cp := ap.coverPhoto(); cp != nil {
		cover = ap.relativeSrcPath(SizeGrid, cp.FileName)
	}
	index := AlbumIndex{
		Slug:        ap.AlbumConfig.Slug,
		Title:       ap.AlbumConfig.Name,
		Description: ap.AlbumConfig.Description,
		DateSpan:    ap.computeDateSpan(),
		Cover:       cover,
		Photos:      make([]PhotoIndex, 0, len(ap.Photos)),
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

	password := ""
	if ap.Config.Encrypt != nil {
		password = ap.Config.Encrypt.AlbumPassword(ap.AlbumConfig.Slug)
	}

	outputName := "index.json"
	counterpart := "index.enc.json"
	if password != "" {
		outputName = "index.enc.json"
		counterpart = "index.json"
	}
	outputPath := ap.OutputPath(outputName)
	ap.Config.TrackFile(outputPath)

	if ap.Config.DryRun {
		action := "write"
		if password != "" {
			action = "encrypt+write"
		}
		fmt.Printf("  DRYRUN: would %s %s (%d photos)\n", action, outputPath, len(index.Photos))
		return nil
	}

	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal album index: %w", err)
	}
	b = append(b, '\n')

	if password != "" {
		b, err = EncryptJSON(b, password, ap.Config.Encrypt.PwFile)
		if err != nil {
			return fmt.Errorf("encrypt album index: %w", err)
		}
	}

	if err := writeBytes(outputPath, b); err != nil {
		return err
	}
	removeIfExists(ap.OutputPath(counterpart))
	return nil
}

// relativeSrcPath returns the relative path for a photo variant (relative to album dir).
func (ap *AlbumProcessor) relativeSrcPath(size ImageSize, fileName string) string {
	return filepath.Join(string(size), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, fileName))
}

// GetAlbumSummary returns summary info for albums.json
func (ap *AlbumProcessor) GetAlbumSummary() AlbumSummary {
	summary := AlbumSummary{
		Slug:  ap.AlbumConfig.Slug,
		Title: ap.AlbumConfig.Name,
		Count: len(ap.Photos),
	}

	encrypted := ap.Config.Encrypt != nil && ap.Config.Encrypt.IsAlbumEncrypted(ap.AlbumConfig.Slug)
	summary.Encrypted = encrypted

	if cover := ap.coverPhoto(); cover != nil {
		// Include the WebP cover URL only when it is accessible without an album-specific
		// password: either the album is unencrypted (always visible) or the site is encrypted
		// and the album has no per-album password (cover is safe behind the site password).
		// If the album has its own per-album password, omit the cover even when the site is
		// encrypted — the per-album password provides stronger protection than the site password.
		siteEncrypted := ap.Config.Encrypt != nil && ap.Config.Encrypt.IsSiteEncrypted()
		hasPerAlbumPw := ap.Config.Encrypt != nil && ap.Config.Encrypt.HasPerAlbumPassword(ap.AlbumConfig.Slug)
		if !encrypted || (siteEncrypted && !hasPerAlbumPw) {
			summary.Cover = filepath.Join(ap.AlbumConfig.Slug, string(SizeGrid), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, cover.FileName))
		}
		// CoverJpeg is used for OG/crawler meta tags — only set for unencrypted albums so
		// search engines cannot index content that requires a password.
		if !encrypted {
			summary.CoverJpeg = filepath.Join(ap.AlbumConfig.Slug, "cover.jpg")
		}
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

// WriteAlbumsIndex writes albums.json (or albums.enc.json if encrypted) into the site output dir.
func (c *Config) WriteAlbumsIndex(summaries []AlbumSummary) error {
	siteDir := c.SiteOutputPath()
	encrypted := c.Encrypt != nil && c.Encrypt.IsSiteEncrypted()
	outputName := "albums.json"
	counterpart := "albums.enc.json"
	if encrypted {
		outputName = "albums.enc.json"
		counterpart = "albums.json"
	}
	outputPath := filepath.Join(siteDir, outputName)

	if c.DryRun {
		action := "write"
		if encrypted {
			action = "encrypt+write"
		}
		fmt.Printf("DRYRUN: would %s %s (%d albums)\n", action, outputPath, len(summaries))
		return nil
	}

	b, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal albums: %w", err)
	}
	b = append(b, '\n')

	if encrypted {
		b, err = EncryptJSON(b, c.Encrypt.SitePassword, c.Encrypt.PwFile)
		if err != nil {
			return fmt.Errorf("encrypt albums: %w", err)
		}
	}

	if err := writeBytes(outputPath, b); err != nil {
		return err
	}
	removeIfExists(filepath.Join(siteDir, counterpart))
	c.TrackFile(outputPath)
	return nil
}

// SiteConfig is the structure for config.json (always unencrypted).
type SiteConfig struct {
	SiteID     string            `json:"siteId"`
	AlbumsFile string            `json:"albumsFile"`
	SiteHint   string            `json:"siteHint,omitempty"`
	AlbumHints map[string]string `json:"albumHints,omitempty"`
}

// WriteConfigJSON writes config.json indicating which albums file to load.
func (c *Config) WriteConfigJSON() error {
	siteEncrypted := c.Encrypt != nil && c.Encrypt.IsSiteEncrypted()
	albumsFile := "albums.json"
	if siteEncrypted {
		albumsFile = "albums.enc.json"
	}
	outputPath := c.SiteOutputPath("config.json")
	if c.DryRun {
		fmt.Printf("DRYRUN: would write %s\n", outputPath)
		return nil
	}
	cfg := SiteConfig{SiteID: c.SiteID, AlbumsFile: albumsFile}
	if c.Encrypt != nil {
		cfg.SiteHint = c.Encrypt.SiteHint
		if len(c.Encrypt.AlbumHints) > 0 {
			cfg.AlbumHints = c.Encrypt.AlbumHints
		}
	}
	if err := writeJSON(outputPath, cfg); err != nil {
		return err
	}
	c.TrackFile(outputPath)
	return nil
}

// WriteSitemap generates sitemap.xml into the site output dir.
func (c *Config) WriteSitemap(summaries []AlbumSummary) error {
	outputPath := c.SiteOutputPath("sitemap.xml")

	if c.DryRun {
		fmt.Printf("DRYRUN: would write %s (%d URLs)\n", outputPath, len(summaries)+1)
		return nil
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create sitemap: %w", err)
	}
	defer file.Close()

	w := bufio.NewWriter(file)

	// Write XML header and urlset opening tag
	w.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>` + c.SiteURL + `/</loc>
  </url>
`)

	// Write each album URL
	for _, album := range summaries {
		w.WriteString(`  <url>
    <loc>` + c.SiteURL + `/albums/` + album.Slug + `</loc>
  </url>
`)
	}

	w.WriteString(`</urlset>
`)

	if err := w.Flush(); err != nil {
		return fmt.Errorf("write sitemap: %w", err)
	}

	fmt.Printf("  wrote: %s\n", outputPath)
	c.TrackFile(outputPath)
	return nil
}

// writeBytes writes data to path, creating directories as needed.
func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("  wrote: %s\n", path)
	return nil
}

// writeJSON writes data as formatted JSON to path.
func writeJSON(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return writeBytes(path, append(b, '\n'))
}

// removeIfExists deletes path if it exists, silently ignoring not-found errors.
func removeIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  WARN: failed to remove %s: %v\n", path, err)
	}
}
