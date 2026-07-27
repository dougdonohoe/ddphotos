package photogen

import (
	"fmt"
	"os"
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

// ResizePhotos generates resized variants for all photos in the album using
// concurrent goroutines. The number of workers is determined by Config.Workers().
// Output: outputRoot/albums/[album-slug]/[size]/[filename].webp
func (ap *AlbumProcessor) ResizePhotos() error {
	// Build list of work, skipping variants that already exist. ResizeImage would skip
	// them anyway, but filtering here avoids dispatching a goroutine and printing a line
	// for every up-to-date file, which is the bulk of the work on a re-run.
	sizes := AllSizes()
	items := make([]resizeWork, 0, len(ap.Photos)*len(sizes))
	upToDate := 0
	for i, photo := range ap.Photos {
		for _, size := range sizes {
			outPath := ap.OutputPath(string(size), ap.Config.PhotoWebPName(ap.AlbumConfig.Slug, photo.FileName))
			// Tracked whether or not it needs writing, so -clean keeps existing files.
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
	fmt.Printf("  Resizing %d photos (%d items, %d workers, %d up to date)...\n",
		len(ap.Photos), len(items), numWorkers, upToDate)

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
