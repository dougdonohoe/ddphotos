package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
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
	photoIndex  int
	totalCount  int
}

// ResizePhotos generates resized variants for all photos in the album using
// concurrent goroutines. The number of workers is determined by Config.Workers().
// Output: outputRoot/albums/[album-slug]/[size]/[filename].webp
// Videos additionally produce outputRoot/albums/[album-slug]/video/[filename].mp4
func (ap *AlbumProcessor) ResizePhotos() error {
	// Build list of work, skipping variants that already exist. ResizeImage would skip
	// them anyway, but filtering here avoids dispatching a goroutine and printing a line
	// for every up-to-date file, which is the bulk of the work on a re-run.
	sizes := AllSizes()
	items := make([]resizeWork, 0, len(ap.Photos)*len(sizes))
	videos := make([]videoWork, 0)
	upToDate := 0

	for i, photo := range ap.Photos {
		if photo.IsVideo {
			vw := videoWork{
				photo: photo,
				// Derived from the source filename, not from VideoFileName's .mp4 form,
				// so an encrypted album hashes the same stem as the poster stills.
				videoPath:   ap.OutputPath(VideoDirName, ap.Config.PhotoOutputName(ap.AlbumConfig.Slug, photo.FileName, ".mp4")),
				posterPaths: make(map[ImageSize]string, len(sizes)),
				photoIndex:  i + 1,
				totalCount:  len(ap.Photos),
			}
			// Tracked whether they need writing, so -clean keeps existing files.
			ap.Config.TrackFile(vw.videoPath)
			need := ap.Config.Force
			if _, err := os.Stat(vw.videoPath); err != nil {
				need = true
			}
			for _, size := range sizes {
				p := ap.OutputPath(string(size), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, photo.FileName))
				ap.Config.TrackFile(p)
				vw.posterPaths[size] = p
				if _, err := os.Stat(p); err != nil {
					need = true
				}
			}
			if need {
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
			if !ap.Config.Force {
				if _, err := os.Stat(outPath); err == nil {
					upToDate++
					continue
				}
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
	if len(items) == 0 {
		return nil
	}

	// Pre-fill a buffered channel with all work items and close it so
	// goroutines drain it naturally with no further coordination needed.
	work := make(chan resizeWork, len(items))
	for _, item := range items {
		work <- item
	}
	close(work)

	// Start each goroutine; use WaitGroup to detect end
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for i := range numWorkers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for item := range work {
				if exit.ExitRequested() {
					return
				}
				result, err := ResizeImage(
					item.photo.AbsolutePath,
					item.outputPath,
					item.size,
					ap.Config.Force,
					ap.Config.DryRun,
				)
				if err != nil {
					errOnce.Do(func() {
						firstErr = fmt.Errorf("resize %s to %s: %w", item.photo.AbsolutePath, item.size, err)
					})
					return
				}
				fmt.Printf("    [w%d] %d/%d %s\n", id, item.photoIndex, item.totalCount, result.Message)
			}
		}(i + 1)
	}

	wg.Wait()
	return firstErr
}

// runVideoWorkers transcodes videos with its own, lower concurrency cap.
//
// ffmpeg is internally multithreaded and a transcode is orders of magnitude heavier than
// a WebP resize, so running Workers() of them saturates every core several times over and
// ends up slower than running fewer. Half the photo worker count, minimum one, keeps the
// machine responsive without leaving cores idle.
func (ap *AlbumProcessor) runVideoWorkers(videos []videoWork, numWorkers int) error {
	if len(videos) == 0 {
		return nil
	}

	videoWorkers := min(max(numWorkers/2, 1), len(videos))

	work := make(chan videoWork, len(videos))
	for _, v := range videos {
		work <- v
	}
	close(work)

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for i := range videoWorkers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for item := range work {
				if exit.ExitRequested() {
					return
				}
				if err := ap.processVideo(id, item); err != nil {
					errOnce.Do(func() { firstErr = err })
					return
				}
			}
		}(i + 1)
	}

	wg.Wait()
	return firstErr
}

// processVideo produces the MP4 and the poster stills for one source video.
func (ap *AlbumProcessor) processVideo(workerID int, item videoWork) error {
	result, err := TranscodeVideo(item.photo.AbsolutePath, item.videoPath, ap.Config.Force, ap.Config.DryRun)
	if err != nil {
		return fmt.Errorf("transcode %s: %w", item.photo.AbsolutePath, err)
	}
	fmt.Printf("    [v%d] %d/%d %s\n", workerID, item.photoIndex, item.totalCount, result.Message)

	if warn := VideoOversizeWarning(item.videoPath); warn != "" {
		ap.warnf("  WARN: %s\n", warn)
	}

	// The poster is extracted once to a temporary JPEG and then run through the ordinary
	// image pipeline, so it picks up the same WebP quality ladder and metadata stripping
	// as every other image in the album rather than needing a parallel implementation.
	needPoster := false
	for _, p := range item.posterPaths {
		if ap.Config.Force {
			needPoster = true
			break
		}
		if _, err := os.Stat(p); err != nil {
			needPoster = true
			break
		}
	}
	if !needPoster {
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
	if err := ExtractPoster(item.photo.AbsolutePath, posterJPEG, duration); err != nil {
		return err
	}

	for _, size := range AllSizes() {
		outPath := item.posterPaths[size]
		res, err := ResizeImage(posterJPEG, outPath, size, true, false)
		if err != nil {
			return fmt.Errorf("poster %s to %s: %w", item.photo.AbsolutePath, size, err)
		}
		fmt.Printf("    [v%d] %d/%d %s\n", workerID, item.photoIndex, item.totalCount, res.Message)
	}
	return nil
}
