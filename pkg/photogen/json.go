package photogen

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	SourcePath  string        `json:"sourcePath"` // relative path from album source base directory to the original source file
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Orientation string        `json:"orientation"`
	DateTime    string        `json:"datetime"`              // ISO 8601 datetime (camera local time, normalized to UTC)
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
			dateStr = photo.DateTaken.UTC().Format(time.RFC3339)
		}
		pi := PhotoIndex{
			ID:          photo.ID,
			FileName:    photo.FileName,
			SourcePath:  photo.SourcePath,
			Width:       photo.Width,
			Height:      photo.Height,
			Orientation: photo.Orientation,
			DateTime:    dateStr,
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

	outputName, counterpart := jsonNames("index", password != "")
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

	albumEncrypted := ap.Config.IsAlbumEncrypted(ap.AlbumConfig.Slug)
	summary.Encrypted = albumEncrypted

	if cover := ap.coverPhoto(); cover != nil {
		// Include the WebP cover URL only when it is accessible without an album-specific
		// password: either the album is unencrypted (always visible) or the site is encrypted
		// and the album has no per-album password (cover is safe behind the site password).
		// If the album has its own per-album password, omit the cover even when the site is
		// encrypted — the per-album password provides stronger protection than the site password.
		if !albumEncrypted || (ap.Config.IsSiteEncrypted() && !ap.Config.HasPerAlbumPassword(ap.AlbumConfig.Slug)) {
			summary.Cover = filepath.Join(ap.AlbumConfig.Slug, string(SizeGrid), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, cover.FileName))
		}
		// CoverJpeg is used for OG/crawler meta tags — only set for unencrypted albums so
		// search engines cannot index content that requires a password.
		if !albumEncrypted {
			summary.CoverJpeg = filepath.Join(ap.AlbumConfig.Slug, "cover.jpg")
		}
		summary.DateSpan = ap.computeDateSpan()
	}

	summary.Description = ap.AlbumConfig.Description
	return summary
}

// computeDateSpan returns a human-readable date range for the album.
// Uses the first and last dated photos; undated photos are ignored.
func (ap *AlbumProcessor) computeDateSpan() string {
	var first, last time.Time
	for _, p := range ap.Photos {
		if p.DateTaken.IsZero() {
			continue
		}
		if first.IsZero() {
			first = p.DateTaken
		}
		last = p.DateTaken
	}

	if first.IsZero() {
		return "" // no dated photos
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
	outputName, counterpart := c.JsonNames("albums")
	outputPath := c.SiteOutputPath(outputName)

	if c.DryRun {
		action := "write"
		if c.IsSiteEncrypted() {
			action = "encrypt+write"
		}
		fmt.Printf("DRYRUN: would %s %s (%d albums)\n", action, outputPath, len(summaries))
		c.TrackFile(outputPath)
		return nil
	}

	b, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal albums: %w", err)
	}
	b = append(b, '\n')

	if c.IsSiteEncrypted() {
		b, err = EncryptJSON(b, c.Encrypt.SitePassword, c.Encrypt.PwFile)
		if err != nil {
			return fmt.Errorf("encrypt albums: %w", err)
		}
	}

	if err := writeBytes(outputPath, b); err != nil {
		return err
	}
	removeIfExists(c.SiteOutputPath(counterpart))
	c.TrackFile(outputPath)
	return nil
}

// SiteConfig is the structure for config.json (always unencrypted).
type SiteConfig struct {
	SiteID          string            `json:"siteId"`
	AlbumsFile      string            `json:"albumsFile"`
	SiteName        string            `json:"siteName"`
	SiteURL         string            `json:"siteUrl"`
	SiteDescription string            `json:"siteDescription"`
	CopyrightOwner  string            `json:"copyrightOwner"`
	CopyrightYear   int               `json:"copyrightYear"`
	AllowCrawling   bool              `json:"allowCrawling,omitempty"`
	KeyID           string            `json:"keyId,omitempty"` // short fingerprint of the HMAC key; changes when the key changes
	SiteHint        string            `json:"siteHint,omitempty"`
	AlbumHints      map[string]string `json:"albumHints,omitempty"`
	Encrypted       bool              `json:"encrypted,omitempty"`    // true if any encryption is configured
	HeroImage       string            `json:"heroImage,omitempty"`    // "hero.jpg" if a hero image is configured
	CustomCSS       string            `json:"customCss,omitempty"`    // "custom.css" if a CSS override is configured
	DefaultTheme    string            `json:"defaultTheme,omitempty"` // "light" or "dark"; omitted when dark (the built-in default)
	HTMLFile        string            `json:"htmlFile,omitempty"`     // "html.json" or "html.enc.json" when HTML fields are configured
}

// SiteHTMLContent is the structure for html.json / html.enc.json.
type SiteHTMLContent struct {
	SiteTitleHTML    string `json:"siteTitleHtml,omitempty"`    // HTML for site title; falls back to siteName
	SiteSubtitleHTML string `json:"siteSubtitleHtml,omitempty"` // HTML shown below site title
	SiteOverviewHTML string `json:"siteOverviewHtml,omitempty"` // HTML shown above album cards
}

// hmacKeyID returns an 8-hex-char fingerprint of the HMAC key.
// Used in config.json so the frontend can detect when the key (and therefore all image
// filenames) has changed, and clear stale cover URLs from localStorage.
func hmacKeyID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:8]
}

// WriteConfigJSON writes config.json indicating which albums file to load.
func (c *Config) WriteConfigJSON() error {
	albumsFile, _ := c.JsonNames("albums")
	outputPath := c.SiteOutputPath("config.json")
	if c.DryRun {
		fmt.Printf("DRYRUN: would write %s\n", outputPath)
		c.TrackFile(outputPath)
		return nil
	}
	cfg := SiteConfig{
		SiteID:          c.SiteID,
		AlbumsFile:      albumsFile,
		SiteName:        c.SiteName,
		SiteURL:         c.SiteURL,
		SiteDescription: c.SiteDescription,
		CopyrightOwner:  c.CopyrightOwner,
		CopyrightYear:   c.CopyrightYear,
		AllowCrawling:   c.AllowCrawling,
	}
	if c.Encrypt != nil {
		cfg.Encrypted = true
		cfg.SiteHint = c.Encrypt.SiteHint
		if len(c.Encrypt.AlbumHints) > 0 {
			cfg.AlbumHints = c.Encrypt.AlbumHints
		}
		if c.Encrypt.HMACKey != "" {
			cfg.KeyID = hmacKeyID(c.Encrypt.HMACKey)
		}
	}
	if c.Hero != nil {
		cfg.HeroImage = "hero.jpg"
	}
	if c.CustomCSS != "" {
		cfg.CustomCSS = "custom.css"
	}
	cfg.DefaultTheme = c.DefaultTheme
	if c.SiteTitleHTML != "" || c.SiteSubtitleHTML != "" || c.SiteOverviewHTML != "" {
		cfg.HTMLFile, _ = c.JsonNames("html")
	}
	if err := writeJSON(outputPath, cfg); err != nil {
		return err
	}
	c.TrackFile(outputPath)
	return nil
}

// SiteBuildMeta is the structure for albums/.build/<site-id>.json.
// Written by photogen; read by the Vite build plugin. Never synced to the server.
type SiteBuildMeta struct {
	ConfigDir string `json:"configDir"`
}

// WriteBuildMeta writes albums/.build/<site-id>.json with the absolute config directory path.
// The Vite build plugin reads this to locate static root files (configDir/static/).
func (c *Config) WriteBuildMeta(configDir string) error {
	absConfigDir, err := filepath.Abs(configDir)
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	dir := filepath.Join(c.OutputRoot, ".build")
	outputPath := filepath.Join(dir, c.SiteID+".json")
	if c.DryRun {
		fmt.Printf("DRYRUN: would write %s\n", outputPath)
		return nil
	}
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return writeJSON(outputPath, SiteBuildMeta{ConfigDir: absConfigDir})
}

// WriteHTMLFile writes html.json (or html.enc.json if the site is encrypted) into the site output dir.
// No-op if all three HTML fields are empty.
func (c *Config) WriteHTMLFile() error {
	if c.SiteTitleHTML == "" && c.SiteSubtitleHTML == "" && c.SiteOverviewHTML == "" {
		return nil
	}
	outputName, counterpart := c.JsonNames("html")
	outputPath := c.SiteOutputPath(outputName)

	if c.DryRun {
		action := "write"
		if c.IsSiteEncrypted() {
			action = "encrypt+write"
		}
		fmt.Printf("DRYRUN: would %s %s\n", action, outputPath)
		c.TrackFile(outputPath)
		return nil
	}

	content := SiteHTMLContent{
		SiteTitleHTML:    c.SiteTitleHTML,
		SiteSubtitleHTML: c.SiteSubtitleHTML,
		SiteOverviewHTML: c.SiteOverviewHTML,
	}
	b, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal html content: %w", err)
	}
	b = append(b, '\n')

	if c.IsSiteEncrypted() {
		b, err = EncryptJSON(b, c.Encrypt.SitePassword, c.Encrypt.PwFile)
		if err != nil {
			return fmt.Errorf("encrypt html content: %w", err)
		}
	}

	if err := writeBytes(outputPath, b); err != nil {
		return err
	}
	removeIfExists(c.SiteOutputPath(counterpart))
	c.TrackFile(outputPath)
	return nil
}

// WriteHeroJPEG generates the hero image JPEG for the site home page.
// No-op when Hero is nil. Should be called when resize is enabled.
func (c *Config) WriteHeroJPEG() error {
	if c.Hero == nil {
		return nil
	}
	outputPath := c.SiteOutputPath("hero.jpg")
	// Always force-regenerate hero.jpg: the output filename is fixed, so a source
	// change won't trigger normal skip logic.
	result, err := ResizeHeroJPEG(c.Hero.ImagePath, outputPath, c.Hero.Crop, true, c.DryRun)
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	c.TrackFile(outputPath)
	return nil
}

// WriteCSSFile copies the custom CSS file to the site output directory as custom.css.
// No-op when CustomCSS is empty. Should be called when index generation is enabled.
func (c *Config) WriteCSSFile() error {
	if c.CustomCSS == "" {
		return nil
	}
	outputPath := c.SiteOutputPath("custom.css")
	if c.DryRun {
		fmt.Printf("DRYRUN: would copy %s → %s\n", c.CustomCSS, outputPath)
		c.TrackFile(outputPath)
		return nil
	}
	data, err := os.ReadFile(c.CustomCSS)
	if err != nil {
		return fmt.Errorf("read css: %w", err)
	}
	if err := os.WriteFile(outputPath, data, filePerms); err != nil {
		return fmt.Errorf("write css: %w", err)
	}
	fmt.Printf("  copied: %s\n", outputPath)
	c.TrackFile(outputPath)
	return nil
}

// WriteSitemap generates sitemap.xml into the site output dir.
func (c *Config) WriteSitemap(summaries []AlbumSummary) error {
	outputPath := c.SiteOutputPath("sitemap.xml")

	if c.DryRun {
		fmt.Printf("DRYRUN: would write %s (%d URLs)\n", outputPath, len(summaries)+1)
		c.TrackFile(outputPath)
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
	if err := os.MkdirAll(filepath.Dir(path), dirPerms); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, filePerms); err != nil {
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

// jsonNames returns the primary output filename and its stale counterpart for a JSON artifact.
// When encrypted, the primary file gets the ".enc.json" suffix.
func jsonNames(base string, encrypted bool) (output, counterpart string) {
	enc := base + ".enc.json"
	reg := base + ".json"
	if encrypted {
		return enc, reg
	}
	return reg, enc
}

// removeIfExists deletes path if it exists, silently ignoring not-found errors.
func removeIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  WARN: failed to remove %s: %v\n", path, err)
	}
}
