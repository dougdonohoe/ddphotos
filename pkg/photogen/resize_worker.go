package photogen

import (
	"fmt"
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
	// Build list of work
	sizes := AllSizes()
	items := make([]resizeWork, 0, len(ap.Photos)*len(sizes))
	for i, photo := range ap.Photos {
		for _, size := range sizes {
			items = append(items, resizeWork{
				photo:      photo,
				size:       size,
				outputPath: ap.OutputPath(string(size), WebPFileName(photo.FileName)),
				photoIndex: i + 1,
				totalCount: len(ap.Photos),
			})
		}
	}

	// Do work using numWorkers goroutines
	numWorkers := ap.Config.Workers()
	fmt.Printf("  Resizing %d photos (%d items, %d workers)...\n",
		len(ap.Photos), len(items), numWorkers)

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
						firstErr = fmt.Errorf("resize %s to %s: %w", item.photo.FileName, item.size, err)
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
