package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The six site writers used to print their error and drop it, so a run that could not write
// albums.json still exited 0 and a "photogen && deploy" wrapper shipped the result. These
// cover the contract that replaced that: report every failure, return the first one.
func TestRunSiteWrites(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	t.Run("all steps succeeding returns nil", func(t *testing.T) {
		t.Parallel()
		var ran []string
		err := runSiteWrites([]siteWriteStep{
			{"writing a", func() error { ran = append(ran, "a"); return nil }},
			{"writing b", func() error { ran = append(ran, "b"); return nil }},
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, ran)
	})

	// The regression. A single failure has to reach the caller, because that is what
	// decides the process exit status.
	t.Run("a failing step is returned, not swallowed", func(t *testing.T) {
		t.Parallel()
		err := runSiteWrites([]siteWriteStep{
			{"writing a", func() error { return nil }},
			{"writing b", func() error { return boom }},
		})
		assert.ErrorIs(t, err, boom)
	})

	// These are independent files. Stopping at the first problem would make the user
	// rediscover the rest one run at a time, so the later steps still run.
	t.Run("later steps still run after a failure", func(t *testing.T) {
		t.Parallel()
		var ran []string
		err := runSiteWrites([]siteWriteStep{
			{"writing a", func() error { ran = append(ran, "a"); return boom }},
			{"writing b", func() error { ran = append(ran, "b"); return nil }},
			{"writing c", func() error { ran = append(ran, "c"); return nil }},
		})
		require.Error(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, ran, "every step is attempted")
	})

	// First error wins, so the message names the step that failed earliest rather than
	// whichever happened to be last in the list.
	t.Run("the first error is the one returned", func(t *testing.T) {
		t.Parallel()
		second := errors.New("second")
		err := runSiteWrites([]siteWriteStep{
			{"writing a", func() error { return boom }},
			{"writing b", func() error { return second }},
		})
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, second)
	})

	t.Run("no steps is not an error", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, runSiteWrites(nil))
	})
}

// -clean deletes anything under a processed album that the run did not track. All three
// rules here exist because a flag combination leaves files untracked that are nonetheless
// real output, so the delete would take work the user still wants.
func TestValidateSyncFlags(t *testing.T) {
	t.Parallel()

	t.Run("either flag alone, or neither, is allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateSyncFlags(false, false))
		assert.NoError(t, validateSyncFlags(true, false))
		assert.NoError(t, validateSyncFlags(false, true))
	})

	// Together they download nothing, build nothing and exit 0, which reads as success.
	t.Run("both together are rejected", func(t *testing.T) {
		t.Parallel()
		err := validateSyncFlags(true, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-no-sync")
		assert.Contains(t, err.Error(), "-sync-only")
	})
}

func TestValidateCleanFlags(t *testing.T) {
	t.Parallel()

	const outPath = "/tmp/out/site"

	t.Run("clean with resize and index and no limit is allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateCleanFlags(true, true, true, 0, outPath))
	})

	t.Run("without clean nothing is checked", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateCleanFlags(false, false, false, 10, outPath))
	})

	t.Run("clean without resize is rejected", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, false, true, 0, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-resize")
		assert.Contains(t, err.Error(), outPath, "the message names the directory to remove by hand")
	})

	// Only the site writers and WriteAlbumIndex call TrackFile for the JSON, so without
	// -index none of it is expected and -clean removes albums.json, config.json,
	// html.json, sitemap.xml and every album's index.json, leaving images and no data.
	t.Run("clean without index is rejected", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, true, false, 0, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-index")
	})

	// LoadPhotos truncates ap.Photos before ResizePhotos calls TrackFile, so only the
	// first N outputs land in the expected set and -clean removes every photo past the
	// limit that an earlier full run had generated.
	t.Run("clean with limit is rejected", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, true, true, 10, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-limit")
	})

	// The resize rule is reported first: it is the most fundamental misuse, and a run with
	// several problems at once should not depend on check order for its message.
	t.Run("every problem at once reports the resize rule", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, false, false, 10, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-resize")
	})

	// Index is checked before limit, so a run missing both is told about the data loss
	// rather than the development-only flag.
	t.Run("missing index outranks limit", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, true, false, 10, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-index")
	})
}
