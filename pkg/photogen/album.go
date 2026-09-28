package photogen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

// defaultMetadataWorkers is the concurrency used when there is no Config to ask
// (unit tests construct AlbumProcessors directly).
const defaultMetadataWorkers = 4

var allowedPhotoExtensions = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".webp": {},
	".tif":  {},
	".tiff": {},
	".heic": {},
	".heif": {},
	".avif": {},
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
	IsVideo      bool   `json:"-"` // true when the source is a video container rather than a still
	// OutputStem, when set, replaces FileName's stem in output names: "IMG_1.png" makes
	// IMG_1.png.webp. disambiguateStems sets it on every photo of a same-stem group but the
	// first, which is the only way two sources can avoid writing one IMG_1.webp.
	OutputStem string `json:"-"`
	*PhotoMetadata
}

// String returns a human-readable representation of the photo for logging.
func (p *Photo) String() string {
	dateStr := "no EXIF date"
	if !p.DateTaken.IsZero() {
		dateStr = p.DateTaken.Format("2006-01-02 15:04")
	}

	nameInfo := p.FileName
	if p.SourcePath != p.FileName {
		nameInfo = fmt.Sprintf("%s [%s]", p.FileName, p.SourcePath)
	}

	kind := ""
	if p.IsVideo {
		kind = fmt.Sprintf(" video %.1fs,", p.Duration)
	}

	s := fmt.Sprintf("%s (%dx%d %s,%s %s)", nameInfo, p.Width, p.Height, p.Orientation, kind, dateStr)
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

	// An encrypted album must not have a cover.jpg. It is a readable JPEG of the album's
	// cover photo, published for OG tags, and GetAlbumSummary already withholds CoverJpeg
	// for encrypted albums for exactly that reason. Skipping WriteCoverJPEG below is not
	// enough on its own: an album that was public before its password was added keeps the
	// file an earlier run wrote, on disk and on the server. Removing it here rather than
	// relying on -clean, which the user has to opt into.
	//
	// A removal that fails stops the album, because the file it failed to remove is the one
	// thing on the encrypted path that is still readable.
	encrypted := ap.Config.IsAlbumEncrypted(ap.AlbumConfig.Slug)
	if encrypted && !ap.Config.DryRun {
		if err := removeCounterpart(ap.OutputPath(CoverJPEGName), true, ap.warnf); err != nil {
			return err
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

	// Over the assembled list rather than per directory, so a subfolder's prefixed ID
	// colliding with a root file named to match it ("sub/photo.jpg" and "sub_photo.png"
	// both yield "sub_photo") is settled the same way as two files in one folder.
	disambiguateStems(photos)
	if err := checkDuplicateIDs(ap.AlbumConfig.Path, photos); err != nil {
		return err
	}

	// Global date sort across all photos (unless manual sort order is in use).
	if !ap.AlbumConfig.ManualSortOrder {
		sortByDate(photos)
	}

	// Checked against the full list, and so before the limit truncates it. -limit keeps only
	// the first N on purpose, so a cover that sorts later is still in the album and warning
	// that it is "not found" would be a lie, in exactly the mode people iterate in.
	coverMissing := ap.AlbumConfig.Cover != "" && !containsCover(photos, ap.coverSourcePath())

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
	if coverMissing {
		ap.warnf("  WARN: cover source path %q not found in album, using first photo\n", ap.AlbumConfig.Cover)
	}

	return nil
}

// coverSourcePath returns the configured cover as it appears in a photo's SourcePath:
// source-relative, prefixed with the album directory name, slash-separated.
func (ap *AlbumProcessor) coverSourcePath() string {
	return filepath.ToSlash(filepath.Join(filepath.Base(ap.AlbumConfig.Path), ap.AlbumConfig.Cover))
}

// containsCover reports whether any photo has the given SourcePath.
func containsCover(photos []*Photo, sourcePath string) bool {
	for _, p := range photos {
		if p.SourcePath == sourcePath {
			return true
		}
	}
	return false
}

// disambiguateStems gives every photo a distinct ID and output name when several share a
// stem, such as IMG_1.jpg and IMG_1.png, or clip.mov and clip.mp4.
//
// An ID is the source file name with its extension stripped, and output names are built
// from the same stem, so without this both files would write one grid/IMG_1.webp. The
// first of each group by SourcePath keeps the bare stem; that keeps every album that has
// no such group, which is nearly all of them, byte-for-byte as it was. Each other member
// takes its full file name as the stem: ID img_1.png, output IMG_1.png.webp.
//
// The winner is not sticky. Adding IMG_1.heic next to an existing IMG_1.jpg hands it
// IMG_1.webp and moves the jpg to IMG_1.jpg.webp, which is safe because the metadata
// cache stamps each output with its source and regenerates one whose source changed.
//
// A photo and a video never reach here as a pair from one folder: collectPhotosRecursive
// has already dropped the video, per dropLivePhotoVideos.
func disambiguateStems(photos []*Photo) {
	groups := map[string][]*Photo{}
	for _, p := range photos {
		groups[p.ID] = append(groups[p.ID], p)
	}
	for id, group := range groups {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].SourcePath < group[j].SourcePath })
		for _, p := range group[1:] {
			p.ID = strings.ToLower(p.FileName)
			p.OutputStem = p.FileName
			fmt.Printf("    %s shares stem %q with %s, so its output is named %s.*\n",
				p.SourcePath, id, group[0].SourcePath, p.OutputStem)
		}
	}
}

// checkDuplicateIDs reports an error when two source files still share a photo ID after
// disambiguateStems.
//
// That takes a pair with identical output names, which only a contrived layout produces:
// IMG_1.png beside IMG_1.png.jpg, or case-only twins (photo.JPG and photo.jpg) on a
// case-sensitive filesystem. Two photos sharing an ID is not survivable, because the
// frontend keys the grid on it and both would write the same output files, so say which
// ones and stop.
func checkDuplicateIDs(where string, photos []*Photo) error {
	sources := map[string][]string{}
	for _, p := range photos {
		// Deduped by source path: reporting one file as conflicting with itself is noise,
		// and a genuine collision always involves two distinct sources.
		if !slices.Contains(sources[p.ID], p.SourcePath) {
			sources[p.ID] = append(sources[p.ID], p.SourcePath)
		}
	}

	var dupes []string
	for id, srcs := range sources {
		if len(srcs) > 1 {
			dupes = append(dupes, id)
		}
	}
	if len(dupes) == 0 {
		return nil
	}
	sort.Strings(dupes)

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d duplicate photo ID(s) — these source files would produce the "+
		"same output file. Rename one of each pair:", where, len(dupes))
	for _, id := range dupes {
		srcs := sources[id]
		sort.Strings(srcs)
		fmt.Fprintf(&b, "\n    %q: %s", id, strings.Join(srcs, ", "))
	}
	return errors.New(b.String())
}

// readMetadata reads metadata for one media file, going through the Config's metadata
// cache when there is one. A nil Config (unit tests) or nil cache reads directly.
//
// Both branches must dispatch on media kind: libvips cannot open a video container, so
// sending a .mov straight to ReadPhotoMetadata fails with "unsupported image format".
func (ap *AlbumProcessor) readMetadata(path string) (*PhotoMetadata, error) {
	if ap.Config == nil {
		return ReadMediaMetadata(path)
	}
	return ap.Config.MetaCache.Metadata(path)
}

// fillMetadata populates PhotoMetadata for each photo concurrently. Decoding images is
// by far the most expensive part of a run that has nothing to resize, so it is spread
// across the same number of workers used for resizing.
//
// Each goroutine writes only to its own *Photo, so the slice needs no locking; the
// metadata cache guards its own map. Errors are collected by index and the
// lowest-index one is returned, so a failure is reported deterministically rather than
// depending on which worker lost the race.
func (ap *AlbumProcessor) fillMetadata(photos []*Photo) error {
	if len(photos) == 0 {
		return nil
	}

	numWorkers := defaultMetadataWorkers
	if ap.Config != nil {
		numWorkers = ap.Config.Workers()
	}
	if numWorkers > len(photos) {
		numWorkers = len(photos)
	}

	errs := make([]error, len(photos))
	indexes := make(chan int, len(photos))
	for i := range photos {
		indexes <- i
	}
	close(indexes)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Go(func() {
			for i := range indexes {
				meta, err := ap.readMetadata(photos[i].AbsolutePath)
				if err != nil {
					errs[i] = fmt.Errorf("read metadata for %s: %w", photos[i].AbsolutePath, err)
					continue
				}
				photos[i].PhotoMetadata = meta
			}
		})
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
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
		if !IsMediaFile(name) {
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
		localPhotos = append(localPhotos, &Photo{
			ID:           photoID,
			FileName:     outputName,
			AbsolutePath: fullPath,
			SourcePath:   filepath.ToSlash(filepath.Join(relDir, name)),
			IsVideo:      IsVideoFile(name),
		})
	}

	localPhotos = ap.dropLivePhotoVideos(dir, localPhotos)

	// Metadata is filled in before any sorting below, since sort order depends on dates.
	if err := ap.fillMetadata(localPhotos); err != nil {
		return nil, err
	}

	// Subdirectories default to alphabetical order.
	sort.Strings(subdirs)

	// Load photogen.txt for captions and (optionally) sort order.
	pd, err := loadPhotoDescriptions(dir)
	if err != nil {
		ap.warnf("  WARN: %v\n", err)
	}

	match := newPhotoMatcher(localPhotos)
	match.applyCaptions(pd)

	if ap.AlbumConfig.ManualSortOrder && len(pd.order) > 0 {
		return ap.expandManualOrder(dir, relDir, localPhotos, subdirs, subdirActual, pd, match, recurse)
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
// unlisted subfolders are alphabetically appended at the end. An entry that matches only
// photos already placed is ignored, with a warning when it is a genuine repeat; a full-name
// line after the bare stem that placed it is not one, since that is how one variant gets its
// own caption. When recurse is false
// no subfolder is collected at all, listed or not, matching the documented meaning of
// recurse: false. Every one of those cases produces a warning, which the WarnCollector
// replays in the end-of-run summary.
func (ap *AlbumProcessor) expandManualOrder(
	dir, relDir string,
	localPhotos []*Photo,
	subdirs []string,
	subdirActual map[string]string,
	pd *photoDescriptions,
	match *photoMatcher,
	recurse bool,
) ([]*Photo, error) {
	// Keyed by photo rather than by entry, because two different entries (IMG_1 and
	// IMG_1.png) can name the same file. placedByName marks the photos a full-name entry
	// has named, which is what makes a second full-name entry a repeat.
	seenPhotos := map[*Photo]bool{}
	placedByName := map[*Photo]bool{}
	seenSubdirs := map[string]bool{}
	result := make([]*Photo, 0, len(localPhotos))

	for _, entry := range pd.order {
		if photos := match.photos(entry); len(photos) > 0 {
			// A repeat is ignored rather than appended again. Appending would list one
			// photo twice in index.json, giving two entries the same id and src.grid and
			// shifting every /albums/slug/N permalink after it. checkDuplicateIDs cannot
			// catch that: it dedupes by SourcePath, and both entries are the same file.
			byName := match.isName(entry)
			repeat := true
			for _, p := range photos {
				if !seenPhotos[p] {
					result = append(result, p)
					seenPhotos[p] = true
					repeat = false
				} else if byName && !placedByName[p] {
					repeat = false // placed by its stem; this line is its own caption
				}
				if byName {
					placedByName[p] = true
				}
			}
			if repeat {
				ap.warnf("  WARN: photogen.txt in %s lists %q more than once (ignoring the repeat)\n", dir, entry)
			}
			continue
		}
		actualName, ok := subdirActual[entry]
		if !ok {
			ap.warnf("  WARN: photogen.txt in %s references unknown entry: %s\n", dir, entry)
			continue
		}
		// Checked before recursing, so a repeated subfolder costs neither the duplicate
		// photos nor the second scan of the whole folder.
		if seenSubdirs[strings.ToLower(actualName)] {
			ap.warnf("  WARN: photogen.txt in %s lists subfolder %q more than once (ignoring the repeat)\n", dir, actualName)
			continue
		}
		// Marked seen even when skipped below, so the unlisted-subfolder pass does not
		// report the same folder a second time.
		seenSubdirs[strings.ToLower(actualName)] = true
		// recurse: false wins over the entry. photogen.txt orders what gets collected; it
		// does not decide what gets collected, and the album was deliberately configured
		// as non-recursive. Silently overriding that from a file in the album directory
		// would make the album's contents depend on which folders happen to be named.
		if !recurse {
			ap.warnf("  WARN: photogen.txt in %s lists subfolder %q but the album is not recursive (ignoring it)\n", dir, actualName)
			continue
		}
		subPhotos, err := ap.collectPhotosRecursive(filepath.Join(dir, actualName), filepath.Join(relDir, actualName), recurse)
		if err != nil {
			return nil, err
		}
		result = append(result, subPhotos...)
	}

	// Append unlisted local photos (date-sorted) with a warning.
	var extraPhotos []*Photo
	for _, p := range localPhotos {
		if !seenPhotos[p] {
			extraPhotos = append(extraPhotos, p)
		}
	}
	if len(extraPhotos) > 0 {
		ap.warnf("  WARN: %d photo(s) in %s not in photogen.txt (sorted by date, appended at end)\n", len(extraPhotos), dir)
		sortByDate(extraPhotos)
		result = append(result, extraPhotos...)
	}

	// Append unlisted subfolders (alphabetically) with a warning.
	for _, sd := range subdirs {
		if seenSubdirs[strings.ToLower(sd)] {
			continue
		}
		// Same rule as a listed subfolder above, and more clear-cut: nothing here even
		// hints that the user wanted this folder included.
		if !recurse {
			ap.warnf("  WARN: subfolder %q in %s ignored because the album is not recursive\n", sd, dir)
			continue
		}
		ap.warnf("  WARN: subfolder %q in %s not in photogen.txt (appended at end)\n", sd, dir)
		subPhotos, err := ap.collectPhotosRecursive(filepath.Join(dir, sd), filepath.Join(relDir, sd), recurse)
		if err != nil {
			return nil, err
		}
		result = append(result, subPhotos...)
	}

	return result, nil
}

// photoDescriptions holds the parsed contents of a photogen.txt file.
//
// Entries are kept lowercased but otherwise as written, so "img_1" and "img_1.png" stay
// distinct: a bare stem names every file with that stem and a full name names one. The
// photoMatcher is what turns an entry into photos.
type photoDescriptions struct {
	descriptions map[string]string // entry name (lowercased) -> description
	order        []string          // entry names in file order
}

// parsePhotogenLine splits one photogen.txt line into its name and description.
//
// The name is everything up to the first space, so a name that itself contains
// spaces must be double-quoted:
//
//	"Doug and Cindy Chicago.jpg" A cool trip to Chicago
//	doug-and-cindy-chicago.jpg A cool trip to Chicago
//
// A line that opens with a quote but never closes it falls back to the unquoted
// split, so a stray quote cannot swallow the rest of the line.
func parsePhotogenLine(line string) (name, desc string) {
	if rest, ok := strings.CutPrefix(line, `"`); ok {
		if quoted, after, found := strings.Cut(rest, `"`); found {
			return quoted, strings.TrimSpace(after)
		}
	}
	name, desc, _ = strings.Cut(line, " ")
	return name, strings.TrimSpace(desc)
}

// photogenFileName is the per-directory captions and manual-order file. It sits in an
// album's source folder, is written by hand or by the DD Photos App, and is the one file
// in a synced album's folder that sync will not overwrite wholesale.
const photogenFileName = "photogen.txt"

// photogenID reduces a photogen.txt entry name or a file name to its stem: lowercase,
// with a media extension stripped, so "IMG_001.jpg" and "img_001" both give "img_001".
// It is the second, fallback tier of matching an entry (photoMatcher and the sync caption
// merge); an entry naming a file in full matches that file before its stem is considered.
//
// An extension that is neither photo nor video is left alone, because a bare subfolder
// name may legitimately contain a dot.
func photogenID(name string) string {
	id := strings.ToLower(name)
	if IsMediaFile(id) {
		id = strings.TrimSuffix(id, strings.ToLower(filepath.Ext(id)))
	}
	return id
}

// photoMatcher resolves photogen.txt entries to the photos of one directory.
//
// An entry naming a file in full (case-insensitively) matches that file alone. Anything
// else is matched on its stem and names every file with that stem, so "IMG_1" captions
// both IMG_1.jpg and IMG_1.png. The stem tier is also what keeps older files working: an
// entry written as IMG_1.jpeg still finds IMG_1.jpg after the file was re-exported.
//
// Matching is on source file names, never IDs, because IDs are not final until
// disambiguateStems runs over the whole album, after every directory has been read.
type photoMatcher struct {
	byName map[string]*Photo   // lowercased source file name
	byStem map[string][]*Photo // photogenID of the source file name, in file-name order
}

func newPhotoMatcher(photos []*Photo) *photoMatcher {
	m := &photoMatcher{
		byName: make(map[string]*Photo, len(photos)),
		byStem: make(map[string][]*Photo, len(photos)),
	}
	sorted := append([]*Photo(nil), photos...)
	sort.Slice(sorted, func(i, j int) bool {
		return filepath.Base(sorted[i].AbsolutePath) < filepath.Base(sorted[j].AbsolutePath)
	})
	for _, p := range sorted {
		name := filepath.Base(p.AbsolutePath)
		m.byName[strings.ToLower(name)] = p
		stem := photogenID(name)
		m.byStem[stem] = append(m.byStem[stem], p)
	}
	return m
}

// isName reports whether an entry names one file in full.
func (m *photoMatcher) isName(entry string) bool {
	_, ok := m.byName[strings.ToLower(entry)]
	return ok
}

// photos returns what an entry names, or nil when it names no photo (it may be a subfolder).
func (m *photoMatcher) photos(entry string) []*Photo {
	if p, ok := m.byName[strings.ToLower(entry)]; ok {
		return []*Photo{p}
	}
	return m.byStem[photogenID(entry)]
}

// applyCaptions sets each photo's description from photogen.txt. Stem entries go first and
// full-name entries second, so a line naming one file beats a line naming all its variants
// wherever the two sit in the file.
func (m *photoMatcher) applyCaptions(pd *photoDescriptions) {
	for _, exact := range []bool{false, true} {
		for _, entry := range pd.order {
			if m.isName(entry) != exact {
				continue
			}
			for _, p := range m.photos(entry) {
				p.Description = pd.descriptions[entry]
			}
		}
	}
}

// dropLivePhotoVideos skips each video that shares its stem with a photo in the same
// directory, with a warning.
//
// That pair is almost always an Apple Live Photo exported as IMG_1234.HEIC plus
// IMG_1234.MOV, and the still is the thing the album is for. Sync applies the same rule
// in filterSyncAssets, before download, so a local folder and a synced one publish the same
// items. Renaming the clip is how to publish both.
func (ap *AlbumProcessor) dropLivePhotoVideos(dir string, photos []*Photo) []*Photo {
	stills := map[string]string{}
	for _, p := range photos {
		if !p.IsVideo {
			name := filepath.Base(p.AbsolutePath)
			if _, ok := stills[photogenID(name)]; !ok {
				stills[photogenID(name)] = name
			}
		}
	}
	kept := photos[:0]
	for _, p := range photos {
		if p.IsVideo {
			name := filepath.Base(p.AbsolutePath)
			if still, clash := stills[photogenID(name)]; clash {
				ap.warnf("  WARN: skipping video %s in %s: it shares a base name with photo %s "+
					"(a Live Photo pair); rename it to publish both\n", name, dir, still)
				continue
			}
		}
		kept = append(kept, p)
	}
	return kept
}

// loadPhotoDescriptions reads photogen.txt from albumPath.
// Format: one line per entry: "name_or_filename [Description]"
// A photo entry is either a bare stem, naming every file with that stem ("img_001"), or a
// full file name, naming one ("img_001.jpg"). See photoMatcher.
// Names containing spaces must be double-quoted (see parsePhotogenLine).
// Subfolder entries are written as the bare folder name with no extension.
// Returns an empty result (no error) if the file does not exist.
func loadPhotoDescriptions(albumPath string) (*photoDescriptions, error) {
	pd := &photoDescriptions{
		descriptions: make(map[string]string),
	}

	txtPath := filepath.Join(albumPath, photogenFileName)
	err := scanLines(txtPath, func(line string) {
		name, desc := parsePhotogenLine(line)
		entry := strings.ToLower(name)
		pd.descriptions[entry] = desc
		pd.order = append(pd.order, entry)
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

// coverPhoto returns the configured cover photo, or the first photo if no cover is configured.
// Returns nil if the album has no photos.
func (ap *AlbumProcessor) coverPhoto() *Photo {
	if len(ap.Photos) == 0 {
		return nil
	}
	if ap.AlbumConfig.Cover != "" {
		fullCover := filepath.ToSlash(filepath.Join(filepath.Base(ap.AlbumConfig.Path), ap.AlbumConfig.Cover))
		for _, p := range ap.Photos {
			if p.SourcePath == fullCover {
				return p
			}
		}
	}
	return ap.Photos[0]
}

// photoOutputName returns the file name of one of a photo's derived outputs, with the given
// extension: a .webp for each image size, and a .mp4 for a video.
//
// A disambiguated photo is named as if its file were OutputStem plus its extension
// (IMG_1.png.png), which gives IMG_1.png.webp unencrypted and a distinct HMAC encrypted.
// Going through FileName would not do for an encrypted album: sub/photo.jpg and a root
// sub_photo.jpg both have the FileName sub_photo.jpg. No output of a disambiguated photo
// predates this, since the album failed to build before, so nothing is renamed.
func (ap *AlbumProcessor) photoOutputName(photo *Photo, outExt string) string {
	name := photo.FileName
	if photo.OutputStem != "" {
		name = photo.OutputStem + filepath.Ext(photo.FileName)
	}
	return ap.Config.PhotoOutputName(ap.AlbumConfig.Slug, name, outExt)
}

// coverImageSource returns the path libvips should read to build derived cover images.
//
// For a still that is the original source file. For a video it is the already-generated
// "full" poster WebP, because libvips cannot open a video container: an album whose first
// (or configured) item is a clip would otherwise fail the whole run with "unsupported
// image format". Downscaling the 1600px poster to the 1200px cover loses nothing visible.
func (ap *AlbumProcessor) coverImageSource(cover *Photo) string {
	if !cover.IsVideo {
		return cover.AbsolutePath
	}
	return ap.OutputPath(string(SizeFull), ap.photoOutputName(cover, ".webp"))
}

// CoverJPEGName is the album-level JPEG of the cover photo, served as the Open Graph image
// for crawlers that will not render a WebP. It sits in the album directory alongside
// grid/, full/ and video/, and exists only for unencrypted albums.
const CoverJPEGName = "cover.jpg"

// WriteCoverJPEG generates a JPEG version of the album cover for use as an Open Graph image.
// Output: outputRoot/albums/{slug}/cover.jpg
func (ap *AlbumProcessor) WriteCoverJPEG() error {
	cover := ap.coverPhoto()
	if cover == nil {
		return nil
	}
	outputPath := ap.OutputPath(CoverJPEGName)
	ap.Config.TrackFile(outputPath)

	source := ap.coverImageSource(cover)

	// In a dry run the video poster has not been written yet, so there is nothing to read.
	// Report the intent and move on rather than failing on a file that a real run would
	// have created moments earlier.
	if ap.Config.DryRun && cover.IsVideo {
		if _, err := os.Stat(source); err != nil {
			fmt.Printf("  DRYRUN: would write %s (cover jpeg, from video poster)\n", outputPath)
			return nil
		}
	}

	// cover.jpg has a fixed output name, so "the file exists" is not enough to skip it:
	// the album's cover can be pointed at a different photo. The cache stamps the output
	// with the source that produced it, which makes the skip safe. Without a cache this
	// falls through to the unconditional regeneration it replaces.
	if !ap.Config.Force && ap.Config.MetaCache.DerivedUpToDate(outputPath, source, "") {
		fmt.Printf("  exists: %s (cover jpeg)\n", outputPath)
		return nil
	}

	result, err := ResizeCoverJPEG(source, outputPath, true, ap.Config.DryRun)
	if err != nil {
		return fmt.Errorf("write cover jpeg: %w", err)
	}
	if result.Written {
		ap.Config.MetaCache.RecordDerived(outputPath, source, "")
	}
	fmt.Printf("  %s\n", result.Message)
	return nil
}
