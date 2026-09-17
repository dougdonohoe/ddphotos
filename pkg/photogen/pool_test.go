package photogen

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dougdonohoe/ddphotos/pkg/exit"
)

// seq returns 0..n-1, a queue of distinct items to hand the pool.
func seq(n int) []int {
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}
	return items
}

func TestRunPool(t *testing.T) {
	t.Parallel()

	t.Run("runs fn once per item", func(t *testing.T) {
		t.Parallel()
		var mu sync.Mutex
		seen := map[int]int{}
		require.NoError(t, runPool(seq(50), 4, func(_ int, item int) error {
			mu.Lock()
			defer mu.Unlock()
			seen[item]++
			return nil
		}))

		assert.Len(t, seen, 50, "every item must be processed")
		for item, count := range seen {
			assert.Equal(t, 1, count, "item %d processed more than once", item)
		}
	})

	t.Run("an empty queue is a no-op", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		require.NoError(t, runPool(nil, 4, func(_ int, _ int) error {
			calls.Add(1)
			return nil
		}))
		assert.Zero(t, calls.Load())
	})

	// A goroutine with nothing to take is pure overhead, and the worker IDs fn labels its
	// log lines with should not run past the number of items.
	t.Run("workers are capped at the number of items", func(t *testing.T) {
		t.Parallel()
		var mu sync.Mutex
		ids := map[int]bool{}
		require.NoError(t, runPool(seq(2), 16, func(workerID int, _ int) error {
			mu.Lock()
			defer mu.Unlock()
			ids[workerID] = true
			return nil
		}))
		for id := range ids {
			assert.LessOrEqual(t, id, 2, "worker id %d exceeds the item count", id)
		}
	})

	t.Run("worker ids are 1-based", func(t *testing.T) {
		t.Parallel()
		var mu sync.Mutex
		ids := map[int]bool{}
		require.NoError(t, runPool(seq(20), 3, func(workerID int, _ int) error {
			mu.Lock()
			defer mu.Unlock()
			ids[workerID] = true
			return nil
		}))
		assert.False(t, ids[0], "ids start at 1, not 0")
	})

	t.Run("returns the error from fn", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		err := runPool(seq(10), 2, func(_ int, item int) error {
			if item == 0 {
				return boom
			}
			return nil
		})
		assert.ErrorIs(t, err, boom)
	})

	// The queue is pre-filled and closed, so nothing else would stop the siblings: without
	// the cancel they work through every remaining item before the error surfaces.
	//
	// The per-item sleep is what makes this test honest, and it took a flake to find out.
	// Closing release frees the siblings, but a closed channel receives instantly, so
	// without the sleep they raced back for the next item with no back-pressure at all,
	// against a failing worker that had not yet reached errOnce.Do(close(done)). Each
	// sibling iteration was a few hundred nanoseconds, so one deschedule of the failing
	// goroutine was enough for three of them to drain the queue: observed at 89 and at
	// 499 processed, failing roughly one run in eight alongside the package's other
	// parallel tests.
	//
	// The sleep restores the back-pressure that closing the channel removed. Cancelling
	// takes microseconds, so 2ms per item is a margin of three orders of magnitude. It
	// costs nothing when the pool behaves, because only the siblings already in flight
	// ever sleep: a passing run processes about three items, a regressed one would need
	// 499 at 2ms across 3 workers, so this cannot quietly pass by being slow.
	t.Run("stops the remaining workers after the first error", func(t *testing.T) {
		t.Parallel()
		var processed atomic.Int32
		release := make(chan struct{})

		err := runPool(seq(500), 4, func(_ int, item int) error {
			if item == 0 {
				close(release) // let the others past their first item
				return errors.New("boom")
			}
			<-release
			time.Sleep(2 * time.Millisecond)
			processed.Add(1)
			return nil
		})

		require.Error(t, err)
		// Three siblings can each be inside fn when the cancel lands, so a few stragglers
		// are expected. Draining all 499 is the regression.
		assert.Less(t, int(processed.Load()), 50,
			"workers must stop after the first error, not drain the queue")
	})
}

// The interrupt cases live outside TestRunPool because exit.SetExitRequested writes a
// process-global flag that every runPool in the package reads.
//
// Nothing here calls t.Parallel, deliberately, and that includes the parent. A parallel
// parent does not protect a sequential subtest: the subtests run inline while the parent
// body executes, and that body itself runs concurrently with every other parallel test in
// the package. Demonstrated by holding the flag for two seconds, which made all three
// subtests of TestProcess_EncryptedAlbumRemovesStaleCoverJPEG fail with ErrInterrupted
// from an album that was never interrupted. Today the window is microseconds, so it had
// not bitten yet.
//
// This matches resize_worker_test.go, whose exit-flag tests have always been sequential.
func TestRunPoolInterrupt(t *testing.T) {
	t.Run("reports an interrupt rather than stopping quietly", func(t *testing.T) {
		exit.SetExitRequested()
		t.Cleanup(exit.ClearExitRequested)

		var calls atomic.Int32
		err := runPool(seq(10), 2, func(_ int, _ int) error {
			calls.Add(1)
			return nil
		})
		assert.ErrorIs(t, err, ErrInterrupted)
		assert.Zero(t, calls.Load(), "an already-requested exit must do no work at all")
	})

	// errOnce means the first recorded error wins, so an interrupt arriving after a real
	// failure cannot overwrite the more useful message. Single worker on purpose: with
	// siblings, whether one of them reaches the exit check before the failing worker
	// records its error is a genuine race, and the test would flake on it.
	t.Run("a real error is not overwritten by a later interrupt", func(t *testing.T) {
		boom := errors.New("boom")
		t.Cleanup(exit.ClearExitRequested)

		err := runPool(seq(20), 1, func(_ int, item int) error {
			if item == 0 {
				exit.SetExitRequested()
				return boom
			}
			return nil
		})
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrInterrupted)
	})
}
