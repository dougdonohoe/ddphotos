package photogen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
)

const (
	// mockProviderName is the test provider. It ships in the binary rather than behind a
	// build tag, because a tag would keep it out of the image and defeat make docker-test.
	mockProviderName = "mock"
	// immichProviderName is registered here so a config naming it validates and the error
	// the user sees says what is actually true. The client itself lands in part 2.
	immichProviderName = "immich"

	// maxAlbumPhotos caps how many assets one synced album may have. Over the limit is an
	// error rather than a truncation: listing 1000 rows of metadata is cheap, downloading
	// and resizing 1000 photos is not, and a silently half-published album is worse than a
	// run that failed and said why.
	maxAlbumPhotos = 500

	// syncDownloadWorkers is how many assets download at once. Deliberately a fixed
	// number rather than Config.Workers(), which sizes the resize pool off the CPU count:
	// this pool is waiting on a server, not decoding JPEGs.
	syncDownloadWorkers = 4
)

// SyncProvider fetches one album's metadata, asset list and bytes. It is the only part of
// syncing that knows about a particular upstream; everything above it is shared, which is
// what makes the whole sync run testable through the mock provider.
type SyncProvider interface {
	// Name returns the provider key used in albums.yaml ("immich", "mock").
	Name() string

	// Album returns the upstream album's name and description.
	Album(ctx context.Context, albumID string) (SyncAlbum, error)

	// Assets returns every asset in the album, paginating internally and applying the
	// provider's own filtering. Order is upstream order.
	Assets(ctx context.Context, albumID string) ([]SyncAsset, error)

	// Fetch opens the bytes for one asset. The caller closes the reader.
	Fetch(ctx context.Context, a SyncAsset) (io.ReadCloser, error)
}

// SyncAlbum is an upstream album's own metadata.
type SyncAlbum struct {
	Name        string
	Description string
}

// SyncAsset is one upstream photo or video.
type SyncAsset struct {
	ID        string    // provider asset ID, stable across renames
	FileName  string    // upstream file name, used to derive the local name
	Caption   string    // upstream description, raw (unescaped)
	Size      int64     // bytes, 0 when the provider does not know
	Checksum  string    // provider checksum, "" when unavailable
	UpdatedAt time.Time // upstream last-modified, zero when unavailable
	IsVideo   bool
	Warnings  []string // provider-specific notes to surface (e.g. "edited upstream")
}

// syncProviderNames returns the registered provider names, sorted, for error messages.
func syncProviderNames() []string {
	names := []string{immichProviderName, mockProviderName}
	sort.Strings(names)
	return names
}

// isSyncProvider reports whether name is a registered provider.
func isSyncProvider(name string) bool {
	return slices.Contains(syncProviderNames(), name)
}

// newSyncProvider builds the provider an album's sync: block names. Validation has already
// checked that the name is registered, so the default branch only fires if the two lists
// drift apart.
func newSyncProvider(cfg *AlbumSyncConfig) (SyncProvider, error) {
	switch cfg.Provider {
	case mockProviderName:
		return newMockProvider(cfg.Mock)
	case immichProviderName:
		return nil, errors.New("the immich provider is not available in this build yet")
	default:
		return nil, fmt.Errorf("unknown sync provider %q", cfg.Provider)
	}
}

// SyncAlbumPath returns the folder a synced album downloads into.
//
// Namespaced by site ID exactly as albums/{site-id}/ is: without it, two config files
// sharing one root and one slug would share a folder, and each run would prune the other's
// photos.
func SyncAlbumPath(root, siteID, provider, slug string) string {
	return filepath.Join(root, siteID, provider, slug)
}

// CreateSyncDirs creates the download folder for every album with a sync: block.
//
// It runs whether the sync stage will, because ToAlbumConfigs stats every album
// source and fails on a missing one. Creating the folder unconditionally means -no-sync on
// an album that has never synced produces an empty album rather than a config error.
func (af *AlbumsFile) CreateSyncDirs(sync *SyncPaths) error {
	for _, a := range af.Albums {
		if a.Sync == nil {
			continue
		}
		if sync == nil {
			return fmt.Errorf("album %q: has a sync: block but no sync directory is configured — "+
				"set DDPHOTOS_SYNC_DIR or pass -sync-dir", a.Slug)
		}
		path := SyncAlbumPath(sync.Root, sync.SiteID, a.Sync.Provider, a.Slug)
		if err := os.MkdirAll(path, dirPerms); err != nil {
			return fmt.Errorf("album %q: create sync folder %q: %w", a.Slug, path, err)
		}
	}
	return nil
}

// HasSyncedAlbums reports whether any album in the file syncs from a provider.
func (af *AlbumsFile) HasSyncedAlbums() bool {
	for _, a := range af.Albums {
		if a.Sync != nil {
			return true
		}
	}
	return false
}

// escapeSyncText prepares an upstream description for a DD Photos caption.
//
// Upstream descriptions are plain text, possibly written by another user of that instance,
// while DD Photos captions render as HTML. So the text is escaped on the way in, and the
// escaped form is what both photogen.txt and metadata.yaml hold — the merge compares the
// two, so they have to be in the same alphabet.
//
// Double quotes are deliberately left alone: only a leading quote on the *file name* is
// significant to parsePhotogenLine, and the description is the rest of the line.
func escapeSyncText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// windowsReservedNames are device names DOS reserved and Windows still refuses to use as a
// file name, with or without an extension.
var windowsReservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// sanitizeSyncFileName makes an upstream file name safe to write on any platform, keeping
// it readable.
//
// Readable matters in three places: the grid and lightbox accessible label falls back to
// the file name when a photo has no caption, cover: has to be typed by a human, and
// photogen.txt keys are the file name without its extension.
func sanitizeSyncFileName(name string) string {
	name = filepath.Base(winToUnixPath(name))

	ext := strings.ToLower(filepath.Ext(name))
	stem := strings.TrimSuffix(name, filepath.Ext(name))

	var b strings.Builder
	for _, r := range stem {
		switch {
		case r < 0x20 || r == 0x7f:
			// control characters: drop
		case strings.ContainsRune(`<>:"/\|?*`, r):
			// Windows-illegal: drop
		default:
			b.WriteRune(r)
		}
	}
	stem = strings.Join(strings.Fields(b.String()), " ")
	stem = strings.Trim(stem, " .")

	if _, reserved := windowsReservedNames[strings.ToLower(stem)]; reserved {
		stem = stem + "_"
	}
	if stem == "" {
		stem = "photo"
	}
	return stem + ext
}

// syncItem pairs one upstream asset with the local name it was given and the record of
// what the last sync knew about it.
type syncItem struct {
	asset   SyncAsset
	file    string         // local file name, assigned once and then fixed
	caption string         // the upstream caption, escaped
	prev    *SyncPhotoMeta // last sync's record for this asset ID, nil when new
}

// assignSyncFileNames gives every asset its local file name.
//
// A name is assigned on first download and then fixed, recorded in metadata.yaml against
// the asset ID. A later rename upstream does not rename the local file, which is what
// keeps photogen.txt keys, cover: values and /albums/slug/N permalinks stable.
//
// Names already recorded are reserved first, in a pass of their own: otherwise a new asset
// could be handed a name an existing asset is about to reclaim.
func assignSyncFileNames(assets []SyncAsset, prev *SyncMetadata) []syncItem {
	byID := prev.byAssetID()

	taken := make(map[string]struct{}, len(assets))
	items := make([]syncItem, len(assets))
	for i, a := range assets {
		items[i] = syncItem{asset: a, caption: escapeSyncText(a.Caption), prev: byID[a.ID]}
		if rec := byID[a.ID]; rec != nil && rec.File != "" {
			items[i].file = rec.File
			taken[strings.ToLower(rec.File)] = struct{}{}
		}
	}

	for i := range items {
		if items[i].file != "" {
			continue
		}
		name := sanitizeSyncFileName(items[i].asset.FileName)
		if _, clash := taken[strings.ToLower(name)]; clash {
			ext := filepath.Ext(name)
			id := items[i].asset.ID
			if len(id) > 8 {
				id = id[:8]
			}
			name = strings.TrimSuffix(name, ext) + "-" + id + ext
		}
		items[i].file = name
		taken[strings.ToLower(name)] = struct{}{}
	}
	return items
}

// filterSyncAssets drops assets photogen cannot publish and surfaces provider warnings.
//
// The base-name rule is the subtle one. checkDuplicateIDs is a hard error: a photo ID is
// the file name with its extension stripped, so IMG_1234.heic and IMG_1234.mov both reduce
// to "img_1234" and the whole run fails. That check is provider-agnostic, so the guard is
// here rather than inside a provider — every provider needs it, and putting it here is
// what makes it testable without a network.
func filterSyncAssets(assets []SyncAsset, warnf func(string, ...any)) []SyncAsset {
	kept := make([]SyncAsset, 0, len(assets))
	for _, a := range assets {
		for _, w := range a.Warnings {
			warnf("WARN: %s: %s\n", a.FileName, w)
		}
		if !IsMediaFile(a.FileName) {
			warnf("WARN: skipping %s: unsupported file type %q\n",
				a.FileName, strings.ToLower(filepath.Ext(a.FileName)))
			continue
		}
		kept = append(kept, a)
	}

	// Photos win a base-name clash: the still is the thing the album is for, and the
	// video in such a pair is almost always the Live Photo half of it.
	//
	// Both sides are decided by extension rather than SyncAsset.IsVideo, because the
	// extension is what collectPhotosRecursive and checkDuplicateIDs will go on later.
	// IsVideo is the provider's own belief and exists for the provider's own filtering.
	photoBases := make(map[string]string, len(kept))
	for _, a := range kept {
		if IsPhotoFile(a.FileName) {
			photoBases[syncBaseName(a.FileName)] = a.FileName
		}
	}
	result := make([]SyncAsset, 0, len(kept))
	for _, a := range kept {
		if IsVideoFile(a.FileName) {
			if photo, clash := photoBases[syncBaseName(a.FileName)]; clash {
				warnf("WARN: skipping video %s: it shares a base name with photo %s, "+
					"which photogen cannot publish as two separate items\n", a.FileName, photo)
				continue
			}
		}
		result = append(result, a)
	}
	return result
}

// syncBaseName reduces a file name to the ID photogen would give it: lowercase, extension
// stripped. It mirrors collectPhotosRecursive and loadPhotoDescriptions.
func syncBaseName(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
}

// syncOneAlbum downloads one album's media into its sync folder and records what it did.
//
// This is real work, and it runs whether -doit was given. A dry run's whole purpose
// is to show what would be built, which needs the source to exist; see docs/PHOTOGEN.md.
//
// Nothing is pruned and no metadata is written unless the listing completed and every
// download succeeded. A half-listed album that pruned against its partial list would
// delete photos the upstream still has.
func syncOneAlbum(ctx context.Context, cfg *Config, ac *AlbumConfig, index, total int) error {
	warnf := func(format string, args ...any) {
		cfg.Warn.Warnf("  "+strings.Replace(format, "WARN: ", "WARN: ["+ac.Slug+"] ", 1), args...)
	}

	fmt.Printf("\n[%d/%d] Syncing %s (%s)\n", index, total, ac.Slug, ac.Sync.Provider)

	provider, err := newSyncProvider(ac.Sync)
	if err != nil {
		return err
	}

	album, err := provider.Album(ctx, ac.Sync.AlbumID)
	if err != nil {
		return fmt.Errorf("read album %s: %w", ac.Sync.AlbumID, err)
	}
	assets, err := provider.Assets(ctx, ac.Sync.AlbumID)
	if err != nil {
		return fmt.Errorf("list album %s: %w", ac.Sync.AlbumID, err)
	}

	assets = filterSyncAssets(assets, warnf)
	if len(assets) > maxAlbumPhotos {
		return fmt.Errorf("album %q has %d assets, over the %d limit — "+
			"photogen will not publish half an album, so split it upstream",
			ac.Slug, len(assets), maxAlbumPhotos)
	}

	prev, err := loadSyncMetadata(ac.Path)
	if err != nil {
		return err
	}
	items := assignSyncFileNames(assets, prev)

	// Atomics because runPool hands these to several goroutines at once.
	var downloaded, skipped atomic.Int64
	err = runPool(items, syncDownloadWorkers, func(_ int, it syncItem) error {
		if haveSyncAsset(ac.Path, it) {
			skipped.Add(1)
			return nil
		}
		if err := downloadSyncAsset(ctx, provider, ac.Path, it); err != nil {
			return fmt.Errorf("download %s: %w", it.asset.FileName, err)
		}
		downloaded.Add(1)
		return nil
	})
	if err != nil {
		return err
	}

	// Before the metadata write, because the merge's baseline is what the *previous* run
	// recorded, and because a failure here must not leave a new record claiming captions
	// that were never merged.
	if ac.Sync.Captions {
		if err := mergeSyncCaptions(ac.Path, items, prev, warnf); err != nil {
			return err
		}
	}

	meta := &SyncMetadata{
		Provider:    ac.Sync.Provider,
		AlbumID:     ac.Sync.AlbumID,
		SyncedAt:    time.Now().UTC().Truncate(time.Second),
		Name:        album.Name,
		Description: escapeSyncText(album.Description),
		Photos:      make([]SyncPhotoMeta, 0, len(items)),
	}
	for _, it := range items {
		meta.Photos = append(meta.Photos, SyncPhotoMeta{
			AssetID:          it.asset.ID,
			File:             it.file,
			OriginalFileName: it.asset.FileName,
			Size:             it.asset.Size,
			Checksum:         it.asset.Checksum,
			UpdatedAt:        it.asset.UpdatedAt,
			Caption:          it.caption,
		})
	}
	if err := writeSyncMetadata(ac.Path, meta); err != nil {
		return err
	}

	pruned, err := pruneSyncFolder(ac.Path, items)
	if err != nil {
		return err
	}

	fmt.Printf("  %d downloaded, %d up to date, %d pruned (%d photos)\n",
		downloaded.Load(), skipped.Load(), pruned, len(items))
	return nil
}

// haveSyncAsset reports whether the local copy is already the upstream's current bytes.
//
// Checksum first, then size, then the upstream timestamp, taking whichever the provider
// actually supplies. A provider that supplies none of the three leaves the file's presence
// as the only signal, which is the best that can be done without re-fetching every run.
//
// Skipping must leave the file's mtime untouched: MetaCache keys on path + mtime + size,
// so a needless re-download would make photogen re-decode every photo on the next run.
func haveSyncAsset(dir string, it syncItem) bool {
	if it.prev == nil {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, it.file))
	if err != nil || st.IsDir() {
		return false
	}
	switch {
	case it.asset.Checksum != "" && it.prev.Checksum != "":
		return it.asset.Checksum == it.prev.Checksum
	case it.asset.Size > 0:
		// The on-disk size is checked too, so a file replaced or clobbered locally is
		// re-fetched rather than trusted because the record still looks right.
		return it.prev.Size == it.asset.Size && st.Size() == it.asset.Size
	case !it.asset.UpdatedAt.IsZero():
		return it.prev.UpdatedAt.Equal(it.asset.UpdatedAt)
	default:
		return true
	}
}

// downloadSyncAsset writes one asset to its local file.
//
// Through a temp file in the same folder and a rename, for the same reason TranscodeVideo
// does it: an interrupted or failed run must not leave a truncated photo that a later run's
// size check accepts as finished.
func downloadSyncAsset(ctx context.Context, p SyncProvider, dir string, it syncItem) error {
	rc, err := p.Fetch(ctx, it.asset)
	if err != nil {
		return err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer rc.Close() //nolint:errcheck

	tmp, err := os.CreateTemp(dir, it.file+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close() //nolint:errcheck
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes 0600; the project writes 0666 so files created inside a Docker
	// container as root stay usable by the host user that mounted the volume.
	if err := os.Chmod(tmpName, filePerms); err != nil {
		return err
	}
	fmt.Printf("  downloaded: %s\n", it.file)
	return os.Rename(tmpName, filepath.Join(dir, it.file))
}

// pruneSyncFolder deletes everything in the folder that is not a current asset, except the
// two files sync maintains. photogen.txt is the only file a user may edit; everything else
// here is managed, which is what keeps this rule simple.
//
// Subdirectories are left alone: nothing sync writes creates one, so anything found is the
// user's.
func pruneSyncFolder(dir string, items []syncItem) (int, error) {
	keep := make(map[string]struct{}, len(items)+2)
	keep[SyncMetadataFileName] = struct{}{}
	keep[photogenFileName] = struct{}{}
	for _, it := range items {
		keep[it.file] = struct{}{}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read sync folder %s: %w", dir, err)
	}
	var pruned int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := keep[e.Name()]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return pruned, fmt.Errorf("prune %s: %w", filepath.Join(dir, e.Name()), err)
		}
		fmt.Printf("  pruned: %s\n", e.Name())
		pruned++
	}
	return pruned, nil
}

// applySyncMetadata fills in a synced album's name and description from what the last sync
// recorded.
//
// It runs even with -no-sync, so a build from disk still gets the upstream name. The
// fallbacks slot below every existing source: ToAlbumConfigs has already resolved an inline
// description: over the descriptions file, so an empty value here means both were absent.
func applySyncMetadata(ac *AlbumConfig) error {
	if ac.Sync == nil {
		return nil
	}
	meta, err := loadSyncMetadata(ac.Path)
	if err != nil {
		return err
	}
	if ac.Name == "" {
		ac.Name = meta.Name
	}
	if ac.Name == "" {
		ac.Name = ac.Slug
	}
	if ac.Description == "" {
		ac.Description = meta.Description
	}
	return nil
}

// RunSync is the sync stage: it syncs every album that has a sync: block, then applies the
// recorded name and description to all of them.
//
// The name and description pass runs even when syncing is skipped, which is the whole
// reason the two are one function rather than two calls in main.
func RunSync(ctx context.Context, cfg *Config, albums []*AlbumConfig, skip bool) error {
	var synced []*AlbumConfig
	for _, a := range albums {
		if a.Sync != nil {
			synced = append(synced, a)
		}
	}
	if len(synced) == 0 {
		return nil
	}

	if skip {
		fmt.Printf("\n[SYNC] skipped (-no-sync): building %d album(s) from what is on disk\n", len(synced))
	} else {
		fmt.Printf("\n[SYNC] %d album(s) — downloads and prunes even without -doit\n", len(synced))
		for i, a := range synced {
			if exit.ExitRequested() {
				return ErrInterrupted
			}
			if err := syncOneAlbum(ctx, cfg, a, i+1, len(synced)); err != nil {
				return fmt.Errorf("syncing %s: %w", a.Slug, err)
			}
		}
	}

	for _, a := range synced {
		if err := applySyncMetadata(a); err != nil {
			return err
		}
	}
	return nil
}
