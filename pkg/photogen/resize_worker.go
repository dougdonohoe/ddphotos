package photogen

import (
	"fmt"
	"os"
	"path/filepath"
)

// resizeWork represents a single resize operation: one photo at one size.
type resizeWork struct {
	photo      *Photo
	size       ImageSize
	outputPath string
	photoIndex int
	totalCount int
}

// videoWork represents the full set of outputs for one source video: the transcoded MP4
// plus the poster stills. Unlike a photo, whose variants are independent, a video's poster
// frame must be extracted once and then fed to every image size, so the three outputs are
// kept together as one unit of work rather than fanned out.
type videoWork struct {
	photo       *Photo
	videoPath   string
	posterPaths map[ImageSize]string
	needVideo   bool // the MP4 is missing or stale
	needPoster  bool // at least one poster is missing or stale
	photoIndex  int
	totalCount  int
}

// ResizePhotos generates resized variants for all photos in the album using
// concurrent goroutines. The number of workers is determined by Config.Workers().
// Output: outputRoot/albums/[album-slug]/[size]/[filename].webp
// Videos additionally produce outputRoot/albums/[album-slug]/video/[filename].mp4
func (ap *AlbumProcessor) ResizePhotos() error {
	// Build list of work, skipping variants that are up to date. Filtering here avoids
	// dispatching a goroutine and printing a line for every up-to-date file, which is the
	// bulk of the work on a re-run. Up to date means the output exists and was made from
	// the source's current bytes (OutputUpToDate), so a source replaced under the same
	// name is redone.
	cache := ap.Config.MetaCache
	upToDateFor := func(outPath, sourcePath string) bool {
		return !ap.Config.Force && cache.OutputUpToDate(outPath, sourcePath)
	}
	sizes := AllSizes()
	items := make([]resizeWork, 0, len(ap.Photos)*len(sizes))
	videos := make([]videoWork, 0)
	upToDate := 0

	for i, photo := range ap.Photos {
		if photo.IsVideo {
			vw := videoWork{
				photo: photo,
				// Derived from the source filename rather than from an already-.mp4 name,
				// so an encrypted album hashes the same stem as the poster stills.
				videoPath:   ap.OutputPath(VideoDirName, ap.Config.PhotoOutputName(ap.AlbumConfig.Slug, photo.FileName, ".mp4")),
				posterPaths: make(map[ImageSize]string, len(sizes)),
				photoIndex:  i + 1,
				totalCount:  len(ap.Photos),
			}
			// Tracked whether they need writing, so -clean keeps existing files.
			ap.Config.TrackFile(vw.videoPath)
			vw.needVideo = !upToDateFor(vw.videoPath, photo.AbsolutePath)
			for _, size := range sizes {
				p := ap.OutputPath(string(size), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, photo.FileName))
				ap.Config.TrackFile(p)
				vw.posterPaths[size] = p
				// Every poster is checked, not just until one is stale, so each one without
				// a stamp gets adopted on this run.
				if !upToDateFor(p, photo.AbsolutePath) {
					vw.needPoster = true
				}
			}
			if vw.needVideo || vw.needPoster {
				videos = append(videos, vw)
			} else {
				upToDate += len(sizes) + 1
			}
			continue
		}

		for _, size := range sizes {
			outPath := ap.OutputPath(string(size), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, photo.FileName))
			// Tracked whether it needs writing, so -clean keeps existing files.
			ap.Config.TrackFile(outPath)
			if upToDateFor(outPath, photo.AbsolutePath) {
				upToDate++
				continue
			}
			items = append(items, resizeWork{
				photo:      photo,
				size:       size,
				outputPath: outPath,
				photoIndex: i + 1,
				totalCount: len(ap.Photos),
			})
		}
	}

	// Do work using numWorkers goroutines
	numWorkers := ap.Config.Workers()
	fmt.Printf("  Resizing %d photos (%d items, %d videos, %d workers, %d up to date)...\n",
		len(ap.Photos), len(items), len(videos), numWorkers, upToDate)

	if len(items) == 0 && len(videos) == 0 {
		return nil
	}

	if err := ap.runResizeWorkers(items, numWorkers); err != nil {
		return err
	}
	return ap.runVideoWorkers(videos, numWorkers)
}

func (ap *AlbumProcessor) runResizeWorkers(items []resizeWork, numWorkers int) error {
	return runPool(items, numWorkers, func(workerID int, item resizeWork) error {
		// Forced because ResizePhotos only queues what needs writing, which includes an
		// existing output whose source has been replaced.
		result, err := ResizeImage(
			item.photo.AbsolutePath,
			item.outputPath,
			item.size,
			true,
			ap.Config.DryRun,
		)
		if err != nil {
			return fmt.Errorf("resize %s to %s: %w", item.photo.AbsolutePath, item.size, err)
		}
		if result.Written {
			ap.Config.MetaCache.RecordDerived(item.outputPath, item.photo.AbsolutePath, "")
		}
		fmt.Printf("    [w%d] %d/%d %s\n", workerID, item.photoIndex, item.totalCount, result.Message)
		return nil
	})
}

// runVideoWorkers transcodes videos with its own, lower concurrency cap.
//
// ffmpeg is internally multithreaded and a transcode is orders of magnitude heavier than
// a WebP resize, so running Workers() of them saturates every core several times over and
// ends up slower than running fewer. Half the photo worker count, minimum one, keeps the
// machine responsive without leaving cores idle.
func (ap *AlbumProcessor) runVideoWorkers(videos []videoWork, numWorkers int) error {
	return runPool(videos, max(numWorkers/2, 1), ap.processVideo)
}

// processVideo produces the MP4 and the poster stills for one source video.
func (ap *AlbumProcessor) processVideo(workerID int, item videoWork) error {
	source := item.photo.AbsolutePath
	if item.needVideo {
		// Forced for the same reason as the photo resize: a stale MP4 still exists.
		result, err := TranscodeVideo(source, item.videoPath, true, ap.Config.DryRun)
		if err != nil {
			return fmt.Errorf("transcode %s: %w", source, err)
		}
		if result.Written {
			ap.Config.MetaCache.RecordDerived(item.videoPath, source, "")
		}
		fmt.Printf("    [v%d] %d/%d %s\n", workerID, item.photoIndex, item.totalCount, result.Message)

		if warn := VideoOversizeWarning(item.videoPath); warn != "" {
			ap.warnf("  WARN: %s\n", warn)
		}
	}

	// The poster is extracted once to a temporary JPEG and then run through the ordinary
	// image pipeline, so it picks up the same WebP quality ladder and metadata stripping
	// as every other image in the album rather than needing a parallel implementation.
	if !item.needPoster {
		return nil
	}

	if ap.Config.DryRun {
		for _, size := range AllSizes() {
			fmt.Printf("    [v%d] %d/%d DRYRUN: would write %s (%s poster)\n",
				workerID, item.photoIndex, item.totalCount, item.posterPaths[size], size)
		}
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "ddphotos-poster-")
	if err != nil {
		return fmt.Errorf("create temp dir for poster: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	duration := 0.0
	if item.photo.PhotoMetadata != nil {
		duration = item.photo.Duration
	}
	posterJPEG := filepath.Join(tmpDir, "poster.jpg")
	if err := ExtractPoster(source, posterJPEG, duration); err != nil {
		return err
	}

	for _, size := range AllSizes() {
		outPath := item.posterPaths[size]
		res, err := ResizeImage(posterJPEG, outPath, size, true, false)
		if err != nil {
			return fmt.Errorf("poster %s to %s: %w", source, size, err)
		}
		// Stamped with the video, not the temporary JPEG: the video is what a later run
		// compares against.
		if res.Written {
			ap.Config.MetaCache.RecordDerived(outPath, source, "")
		}
		fmt.Printf("    [v%d] %d/%d %s\n", workerID, item.photoIndex, item.totalCount, res.Message)
	}
	return nil
}
