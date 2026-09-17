package photogen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Kind        string        `json:"kind,omitempty"`        // "video"; omitted entirely for stills
	Duration    float64       `json:"duration,omitempty"`    // seconds; video only
	Src         PhotoSrcIndex `json:"src"`
}

// KindVideo is the PhotoIndex.Kind value marking an entry as a video. Stills omit Kind
// rather than setting "photo", so existing index.json files stay byte-identical.
const KindVideo = "video"

// PhotoSrcIndex contains paths to image variants.
// For a video, Grid and Full are the poster stills and Video is the playable MP4.
type PhotoSrcIndex struct {
	Grid  string `json:"grid"`
	Full  string `json:"full"`
	Video string `json:"video,omitempty"` // e.g. "video/clip.mp4"
}

// AlbumSummary is the structure for each album in albums.json
type AlbumSummary struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Count       int    `json:"count"`                 // total media items (photos + videos)
	VideoCount  int    `json:"videoCount,omitempty"`  // how many of Count are videos
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
		if photo.IsVideo {
			pi.Kind = KindVideo
			pi.Duration = photo.Duration
			pi.Src.Video = ap.relativeVideoPath(photo.FileName)
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
	return removeCounterpart(ap.OutputPath(counterpart), password != "", ap.warnf)
}

// relativeSrcPath returns the relative path for a photo variant (relative to album dir).
func (ap *AlbumProcessor) relativeSrcPath(size ImageSize, fileName string) string {
	return filepath.Join(string(size), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, fileName))
}

// relativeVideoPath returns the relative path for a transcoded video (relative to album dir).
func (ap *AlbumProcessor) relativeVideoPath(fileName string) string {
	return filepath.Join(VideoDirName, ap.Config.PhotoOutputName(ap.AlbumConfig.Slug, fileName, ".mp4"))
}

// GetAlbumSummary returns summary info for albums.json
func (ap *AlbumProcessor) GetAlbumSummary() AlbumSummary {
	videos := 0
	for _, p := range ap.Photos {
		if p.IsVideo {
			videos++
		}
	}
	summary := AlbumSummary{
		Slug:       ap.AlbumConfig.Slug,
		Title:      ap.AlbumConfig.Name,
		Count:      len(ap.Photos),
		VideoCount: videos,
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
			summary.CoverJpeg = filepath.Join(ap.AlbumConfig.Slug, CoverJPEGName)
		}
		summary.DateSpan = ap.computeDateSpan()
	}

	summary.Description = ap.AlbumConfig.Description
	return summary
}

// computeDateSpan returns a human-readable date range for the album.
// Uses the earliest and latest dated photos; undated photos are ignored.
//
// The endpoints are found by scanning rather than read off the ends of the slice, because
// ap.Photos is not necessarily in date order: LoadPhotos skips sortByDate entirely when an
// album sets manual_sort_order, and such an album is often deliberately newest-first.
// Taking the ends there produced backwards spans like "Mar 2025 - May 2023".
func (ap *AlbumProcessor) computeDateSpan() string {
	var earliest, latest time.Time
	for _, p := range ap.Photos {
		if p.DateTaken.IsZero() {
			continue
		}
		if earliest.IsZero() || p.DateTaken.Before(earliest) {
			earliest = p.DateTaken
		}
		if latest.IsZero() || p.DateTaken.After(latest) {
			latest = p.DateTaken
		}
	}

	if earliest.IsZero() {
		return "" // no dated photos
	}

	// Same month and year
	if earliest.Year() == latest.Year() && earliest.Month() == latest.Month() {
		return earliest.Format("Jan 2006")
	}

	// Same year, different months
	if earliest.Year() == latest.Year() {
		return fmt.Sprintf("%s - %s %d", earliest.Format("Jan"), latest.Format("Jan"), earliest.Year())
	}

	// Different years
	return fmt.Sprintf("%s - %s", earliest.Format("Jan 2006"), latest.Format("Jan 2006"))
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
	if err := removeCounterpart(c.SiteOutputPath(counterpart), c.IsSiteEncrypted(), c.Warn.Warnf); err != nil {
		return err
	}
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
	Encrypted       bool              `json:"encrypted,omitempty"`    // true if any password is configured (site or per-album); false for a key-only passwords file
	HeroImage       string            `json:"heroImage,omitempty"`    // "hero.jpg" if a hero image is configured
	CustomCSS       string            `json:"customCss,omitempty"`    // "custom.css" if a CSS override is configured
	DefaultTheme    string            `json:"defaultTheme,omitempty"` // "light" or "dark"; omitted when dark (the built-in default)
	HTMLFile        string            `json:"htmlFile,omitempty"`     // "html.json" or "html.enc.json" when HTML fields are configured
	AlbumNav        []NavLink         `json:"albumNav,omitempty"`     // replaces the album page's "← Albums" link when set
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
		// Note this is not `true` just because a passwords file exists: a key-only file
		// obfuscates filenames but leaves every album readable, so nothing is protected.
		cfg.Encrypted = c.Encrypt.HasAnyPassword()
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
	// Nav links ride in config.json rather than html.json: the album page already has
	// config.json from the layout load, and it stays readable when a visitor unlocked with
	// only a per-album password (they hold no site key to decrypt html.enc.json with).
	cfg.AlbumNav = c.AlbumNav
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
	if err := removeCounterpart(c.SiteOutputPath(counterpart), c.IsSiteEncrypted(), c.Warn.Warnf); err != nil {
		return err
	}
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
	c.TrackFile(outputPath)

	// hero.jpg has a fixed output name, so "the file exists" is not enough to skip it:
	// the configured hero image and its crop can both change. The cache stamps the
	// output with the source and crop that produced it, which makes the skip safe.
	// Without a cache this falls through to the unconditional regeneration it replaces.
	if !c.Force && c.MetaCache.DerivedUpToDate(outputPath, c.Hero.ImagePath, c.Hero.Crop) {
		fmt.Printf("  exists: %s (hero jpeg)\n", outputPath)
		return nil
	}

	result, err := ResizeHeroJPEG(c.Hero.ImagePath, outputPath, c.Hero.Crop, true, c.DryRun)
	if err != nil {
		return err
	}
	if result.Written {
		c.MetaCache.RecordDerived(outputPath, c.Hero.ImagePath, c.Hero.Crop)
	}
	fmt.Println(result.Message)
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
	// Through writeBytes like every other output, for its MkdirAll: os.WriteFile on its own
	// fails when the site directory does not exist, which only stays hidden because
	// WriteAlbumsIndex runs first in the same block and creates it.
	if err := writeBytes(outputPath, data); err != nil {
		return fmt.Errorf("write css: %w", err)
	}
	fmt.Printf("  copied: %s\n", outputPath)
	c.TrackFile(outputPath)
	return nil
}

// WriteSitemap generates sitemap.xml into the site output dir.
func (c *Config) WriteSitemap(summaries []AlbumSummary) error {
	outputPath := c.SiteOutputPath("sitemap.xml")

	// Encrypted albums are left out. The slug is the one thing a crawler could learn about
	// a password-protected album, and withholding it matches the decision in
	// GetAlbumSummary to omit CoverJpeg so search engines cannot index content that needs a
	// password. The site root stays listed either way: it is a real public page that serves
	// the password prompt, so a fully encrypted site gets a sitemap with that one entry.
	//
	// Every <loc> is XML-escaped. slugPattern (albums_config.go) already restricts a slug
	// to characters that need no escaping, but SiteURL is free-form config, and a raw "&"
	// there would make the whole document unparseable to a crawler.
	locs := []string{c.SiteURL + "/"}
	for _, album := range summaries {
		if album.Encrypted {
			continue
		}
		locs = append(locs, c.SiteURL+"/albums/"+album.Slug)
	}

	if c.DryRun {
		fmt.Printf("DRYRUN: would write %s (%d URLs)\n", outputPath, len(locs))
		c.TrackFile(outputPath)
		return nil
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
`)
	for _, loc := range locs {
		b.WriteString("  <url>\n    <loc>")
		if err := xml.EscapeText(&b, []byte(loc)); err != nil {
			return fmt.Errorf("escape sitemap URL %s: %w", loc, err)
		}
		b.WriteString("</loc>\n  </url>\n")
	}
	b.WriteString(`</urlset>
`)

	if err := writeBytes(outputPath, []byte(b.String())); err != nil {
		return err
	}
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

// removeIfExists deletes path, treating "already gone" as success.
//
// The error is returned rather than printed because what a failure means depends entirely
// on what is being removed, and only the caller knows. See removeCounterpart.
//
// It is returned unwrapped: os.Remove already yields a *PathError reading
// "remove <path>: <reason>", so anything added here only repeats the path.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// removeCounterpart deletes an output this run has superseded, and decides what a failure
// to do so means.
//
// confidential says whether path is the readable copy of something this run encrypted: the
// plaintext index.json beside a new index.enc.json, or an album's cover.jpg once it has a
// password. There a failed removal is an error, because the removal is the entire security
// measure. The file is rsynced like any other, so a warning nobody reads leaves a public
// URL serving every caption, filename and photo path of an album the site believes is
// private, and with an HMAC key configured it is also the map from obfuscated output names
// back to content.
//
// When it is not confidential, path is a leftover .enc.json from a run that had a password
// and this one does not. It is unreadable without that password, so it is untidy rather
// than dangerous: report it through warnf, where it reaches the end-of-run summary, and
// carry on.
func removeCounterpart(path string, confidential bool, warnf func(string, ...any)) error {
	err := removeIfExists(path)
	switch {
	case err == nil:
		return nil
	case confidential:
		return fmt.Errorf("%w\n    this is the readable copy of a file this run encrypted, so it "+
			"must not reach the server; remove it by hand and re-run before deploying", err)
	default:
		warnf("  WARN: %v\n", err)
		return nil
	}
}
