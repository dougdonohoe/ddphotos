package photogen

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
)

// ErrInterrupted reports that a worker pool stopped before its queue was empty because a
// graceful exit was requested (Ctrl-C). It has to be an error rather than a quiet return:
// Process keys off the resize result, and a nil there means it goes on to write an
// index.json listing photos whose WebPs were never generated, while photogen still exits
// 0. A wrapper script of the "photogen && deploy" shape then ships the broken album.
var ErrInterrupted = errors.New("interrupted before all photos were processed")

// took formats the time since start for a worker's "done" line, to the hundredth of a
// second, so a slow item can be told apart from a hung one.
func took(start time.Time) string {
	return fmt.Sprintf("%.2fs", time.Since(start).Seconds())
}

// runPool runs fn over every item with at most workers goroutines and returns the first
// error, or nil once the queue is empty.
//
// The queue is a buffered channel pre-filled with every item and closed before any worker
// starts, so workers drain it with no further coordination and the pool ends by itself.
// Workers are capped at len(items), since a goroutine with nothing to take is pure
// overhead, and floored at 1: no caller can currently pass less, but a zero would start no
// goroutines at all and return nil having processed nothing, which is exactly the
// half-finished run reported as complete that ErrInterrupted exists to prevent.
//
// Two conditions end the pool early, and both matter:
//
// A graceful exit request yields ErrInterrupted rather than a quiet stop, so a caller
// cannot mistake a half-finished run for a complete one.
//
// The first error from fn closes done, which every worker checks at the top of each
// iteration. Without it the siblings would work through the whole pre-filled queue before
// the error surfaced, which on a video-heavy album is minutes of transcoding whose output
// is discarded.
//
// workerID is 1-based and exists only so fn can label its log lines.
func runPool[T any](items []T, workers int, fn func(workerID int, item T) error) error {
	if len(items) == 0 {
		return nil
	}
	if workers > len(items) {
		workers = len(items)
	}
	if workers < 1 {
		workers = 1
	}

	work := make(chan T, len(items))
	for _, item := range items {
		work <- item
	}
	close(work)

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	done := make(chan struct{})

	for i := range workers {
		wg.Go(func() {
			workerID := i + 1
			for item := range work {
				select {
				case <-done:
					return
				default:
				}
				if exit.ExitRequested() {
					errOnce.Do(func() { firstErr = ErrInterrupted })
					return
				}
				if err := fn(workerID, item); err != nil {
					errOnce.Do(func() {
						firstErr = err
						close(done)
					})
					return
				}
			}
		})
	}

	wg.Wait()
	return firstErr
}
