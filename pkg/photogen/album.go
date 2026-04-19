package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var allowedPhotoExtentions = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
}

// sortByDate sorts photos ascending by date. Undated photos (zero DateTaken) sort to
// the end; among undated photos, original scan order is preserved (stable sort).
func sortByDate(photos []*Photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		iZero := photos[i].DateTaken.IsZero()
		jZero := photos[j].DateTaken.IsZero()
		if iZero != jZero {
			return !iZero // dated before undated
		}
		if iZero {
			return false // both undated: preserve scan order
		}
		return photos[i].DateTaken.Before(photos[j].DateTaken)
	})
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
	SourcePath   string `json:"sourcePath"` // relative path from album source base directory to the original source file
	Description  string `json:"description,omitempty"`
	*PhotoMetadata
}

// String returns a human-readable representation of the photo for logging.
func (p *Photo) String() string {
	dateStr := "no date"
	if !p.DateTaken.IsZero() {
		dateStr = p.DateTaken.Format("2006-01-02 15:04")
	}

	nameInfo := p.FileName
	if p.SourcePath != p.FileName {
		nameInfo = fmt.Sprintf("%s [%s]", p.FileName, p.SourcePath)
	}

	s := fmt.Sprintf("%s (%dx%d %s, %s)", nameInfo, p.Width, p.Height, p.Orientation, dateStr)
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
	fmt.Printf("Processing %d/%d - %s [recurse=%v] (%s) ...\n", index, total, ap.AlbumConfig.Name,
		ap.AlbumConfig.Recurse, ap.AlbumConfig.Description)

	// load photos
	err := ap.LoadPhotos()
	if err != nil {
		fmt.Printf("Error loading photos: %v\n", err)
		return err
	}

	// warn once if configured cover is not found
	if ap.AlbumConfig.Cover != "" {
		found := false
		for _, p := range ap.Photos {
			if p.FileName == ap.AlbumConfig.Cover {
				found = true
				break
			}
		}
		if !found {
			ap.warnf("  WARN: cover %q not found in album, using first photo\n", ap.AlbumConfig.Cover)
		}
	}

	// resize photos if enabled
	if ap.Config.Resize {
		if err := ap.ResizePhotos(); err != nil {
			fmt.Printf("Error resizing photos: %v\n", err)
			return err
		}
		// Cover JPEG is only used for OG images; skip for encrypted albums since
		// CoverJpeg is omitted from the summary and the file would be guessable.
		encrypted := ap.Config.IsAlbumEncrypted(ap.AlbumConfig.Slug)
		if !encrypted {
			if err := ap.WriteCoverJPEG(); err != nil {
				fmt.Printf("Error writing cover JPEG: %v\n", err)
				return err
			}
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
	fmt.Printf("  Loading photos from %s (recurse=%v) ...\n", ap.AlbumConfig.Path,
		ap.AlbumConfig.Recurse)

	photos, err := ap.collectPhotosRecursive(ap.AlbumConfig.Path, "", ap.AlbumConfig.Recurse)
	if err != nil {
		return err
	}

	// Prefix SourcePath with the album source directory name so it is relative
	// from the base directory rather than from the album root itself.
	albumDirName := filepath.Base(ap.AlbumConfig.Path)
	for _, p := range photos {
		p.SourcePath = filepath.ToSlash(filepath.Join(albumDirName, p.SourcePath))
	}

	// Global date sort across all photos (unless manual sort order is in use).
	if !ap.AlbumConfig.ManualSortOrder {
		sortByDate(photos)
	}

	// Apply limit (truncate after full collection)
	if ap.Config != nil && ap.Config.Limit > 0 && len(photos) > ap.Config.Limit {
		photos = photos[:ap.Config.Limit]
	}

	ap.Photos = photos

	// Log photos and count those without dates
	if len(ap.Photos) == 0 {
		fmt.Printf("    Found 0 photos.\n")
	} else {
		fmt.Printf("    Found %d photos:\n", len(photos))
	}
	noDates := 0
	for _, photo := range ap.Photos {
		fmt.Printf("      %s\n", photo.String())
		if photo.DateTaken.IsZero() {
			noDates++
		}
	}
	if noDates > 0 {
		ap.warnf("  WARN: %d/%d photos have no EXIF date\n", noDates, len(ap.Photos))
	}

	return nil
}

// collectPhotosRecursive collects photos from dir, optionally recursing into subdirectories.
// relDir is the path of dir relative to the album root (empty string for the root itself).
// Photos in subdirectories get a prefixed ID and FileName derived from the relative path
// to avoid name collisions.
//
// Sort order:
//   - If ManualSortOrder and a photogen.txt is present: use photogen.txt order, with
//     subfolder names in photogen.txt expanded inline by recursing into that subfolder.
//   - Otherwise: photos are collected with subdirectories in alphabetical order; the
//     caller (LoadPhotos) then applies a global date sort across everything.
func (ap *AlbumProcessor) collectPhotosRecursive(dir, relDir string, recurse bool) ([]*Photo, error) {
	fmt.Printf("    Scanning %s ...\n", dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	prefix := sanitizePrefix(relDir)

	// Separate directory entries into local photos and subdirectories.
	var localPhotos []*Photo
	var subdirs []string
	subdirActual := map[string]string{} // lowercase name → actual directory name

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			subdirs = append(subdirs, name)
			subdirActual[strings.ToLower(name)] = name
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := allowedPhotoExtentions[ext]; !ok {
			continue
		}

		baseID := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		photoID := baseID
		outputName := name
		if prefix != "" {
			photoID = prefix + "_" + baseID
			outputName = prefix + "_" + name
		}

		fullPath := filepath.Join(dir, name)
		photo := &Photo{
			ID:           photoID,
			FileName:     outputName,
			AbsolutePath: fullPath,
			SourcePath:   filepath.ToSlash(filepath.Join(relDir, name)),
		}
		meta, err := ReadPhotoMetadata(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read metadata for %s: %w", fullPath, err)
		}
		photo.PhotoMetadata = meta
		localPhotos = append(localPhotos, photo)
	}

	// Subdirectories default to alphabetical order.
	sort.Strings(subdirs)

	// Load photogen.txt for captions and (optionally) sort order.
	pd, err := loadPhotoDescriptions(dir)
	if err != nil {
		ap.warnf("  WARN: %v\n", err)
	}

	// Apply captions: look up by original base ID (before prefix), derived from AbsolutePath.
	photosByBaseID := make(map[string]*Photo, len(localPhotos))
	for _, p := range localPhotos {
		origExt := strings.ToLower(filepath.Ext(p.AbsolutePath))
		origBase := strings.ToLower(strings.TrimSuffix(filepath.Base(p.AbsolutePath), origExt))
		photosByBaseID[origBase] = p
	}
	for id, desc := range pd.descriptions {
		if p, ok := photosByBaseID[id]; ok {
			p.Description = desc
		}
	}

	if ap.AlbumConfig.ManualSortOrder && len(pd.order) > 0 {
		return ap.expandManualOrder(dir, relDir, localPhotos, subdirs, subdirActual, pd, photosByBaseID, recurse)
	}

	// Default: date-sort local photos, then recurse subdirectories alphabetically.
	sortByDate(localPhotos)

	result := append([]*Photo(nil), localPhotos...)
	if recurse {
		for _, sd := range subdirs {
			subPhotos, err := ap.collectPhotosRecursive(filepath.Join(dir, sd), filepath.Join(relDir, sd), recurse)
			if err != nil {
				return nil, err
			}
			result = append(result, subPhotos...)
		}
	}
	return result, nil
}

// expandManualOrder processes photogen.txt entries in order, expanding subfolder references
// by recursing into them. Unlisted photos are date-sorted and appended at the end;
// unlisted subdirectories are alphabetically appended at the end. Both produce warnings.
func (ap *AlbumProcessor) expandManualOrder(
	dir, relDir string,
	localPhotos []*Photo,
	subdirs []string,
	subdirActual map[string]string,
	pd *photoDescriptions,
	photosByBaseID map[string]*Photo,
	recurse bool,
) ([]*Photo, error) {
	seenPhotos := map[string]bool{}
	seenSubdirs := map[string]bool{}
	result := make([]*Photo, 0, len(localPhotos))

	for _, entry := range pd.order {
		if p, ok := photosByBaseID[entry]; ok {
			result = append(result, p)
			seenPhotos[entry] = true
			continue
		}
		actualName, ok := subdirActual[entry]
		if !ok {
			ap.warnf("  WARN: photogen.txt in %s references unknown entry: %s\n", dir, entry)
			continue
		}
		seenSubdirs[strings.ToLower(actualName)] = true
		subPhotos, err := ap.collectPhotosRecursive(filepath.Join(dir, actualName), filepath.Join(relDir, actualName), recurse)
		if err != nil {
			return nil, err
		}
		result = append(result, subPhotos...)
	}

	// Append unlisted local photos (date-sorted) with a warning.
	var extraPhotos []*Photo
	for _, p := range localPhotos {
		origExt := strings.ToLower(filepath.Ext(p.AbsolutePath))
		origBase := strings.ToLower(strings.TrimSuffix(filepath.Base(p.AbsolutePath), origExt))
		if !seenPhotos[origBase] {
			extraPhotos = append(extraPhotos, p)
		}
	}
	if len(extraPhotos) > 0 {
		ap.warnf("  WARN: %d photo(s) in %s not in photogen.txt (sorted by date, appended at end)\n", len(extraPhotos), dir)
		sortByDate(extraPhotos)
		result = append(result, extraPhotos...)
	}

	// Append unlisted subdirectories (alphabetically) with a warning.
	for _, sd := range subdirs {
		if seenSubdirs[strings.ToLower(sd)] {
			continue
		}
		ap.warnf("  WARN: subdirectory %q in %s not in photogen.txt (appended at end)\n", sd, dir)
		subPhotos, err := ap.collectPhotosRecursive(filepath.Join(dir, sd), filepath.Join(relDir, sd), recurse)
		if err != nil {
			return nil, err
		}
		result = append(result, subPhotos...)
	}

	return result, nil
}

// photoDescriptions holds the parsed contents of a photogen.txt file.
type photoDescriptions struct {
	descriptions map[string]string // photo ID (filename without ext) -> description
	order        []string          // photo IDs in file order
}

// loadPhotoDescriptions reads photogen.txt from albumPath.
// Format: one line per entry: "name_or_filename [Description]"
// Photo entries may include or omit the image extension (e.g. "img_001.jpg" or "img_001").
// Subfolder entries are written as the bare folder name with no extension.
// Returns an empty result (no error) if the file does not exist.
func loadPhotoDescriptions(albumPath string) (*photoDescriptions, error) {
	pd := &photoDescriptions{
		descriptions: make(map[string]string),
	}

	txtPath := filepath.Join(albumPath, "photogen.txt")
	err := scanLines(txtPath, func(line string) {
		parts := strings.SplitN(line, " ", 2)
		id := strings.ToLower(parts[0])
		// Strip image extension if present so "img_001.jpg" and "img_001" both work.
		if ext := strings.ToLower(filepath.Ext(id)); ext != "" {
			if _, ok := allowedPhotoExtentions[ext]; ok {
				id = strings.TrimSuffix(id, ext)
			}
		}
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

// sanitizePathSegment converts a directory name segment to a safe ID prefix component:
// lowercase letters and digits only. E.g. "Craig's" → "craigs", "Ski 2007" → "ski2007".
func sanitizePathSegment(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizePrefix converts a relative directory path to a photo ID prefix.
// Each path segment is sanitized and the results are joined with "_".
// Returns "" for the root (empty or ".").
// E.g. "Craig's" → "craigs", "Ski 2007/Alan's" → "ski2007_alans".
func sanitizePrefix(relDir string) string {
	if relDir == "" || relDir == "." {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relDir), "/")
	var segs []string
	for _, p := range parts {
		if s := sanitizePathSegment(p); s != "" {
			segs = append(segs, s)
		}
	}
	return strings.Join(segs, "_")
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
		sortByDate(extras)
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
	ap.Config.TrackFile(outputPath)
	// Always force-regenerate cover.jpg: the output filename is fixed, so a source
	// change won't trigger normal skip logic. Covers are few and cheap to redo.
	result, err := ResizeCoverJPEG(cover.AbsolutePath, outputPath, true, ap.Config.DryRun)
	if err != nil {
		return fmt.Errorf("write cover jpeg: %w", err)
	}
	fmt.Printf("  %s\n", result.Message)
	return nil
}
