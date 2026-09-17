package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -clean deletes anything under a processed album that the run did not track. Both rules
// here exist because a flag combination leaves files untracked that are nonetheless real
// output, so the delete would take work the user still wants.
func TestValidateCleanFlags(t *testing.T) {
	t.Parallel()

	const outPath = "/tmp/out/site"

	t.Run("clean with resize and no limit is allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateCleanFlags(true, true, 0, outPath))
	})

	t.Run("without clean nothing is checked", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateCleanFlags(false, false, 10, outPath))
	})

	t.Run("clean without resize is rejected", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, false, 0, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-resize")
		assert.Contains(t, err.Error(), outPath, "the message names the directory to remove by hand")
	})

	// LoadPhotos truncates ap.Photos before ResizePhotos calls TrackFile, so only the
	// first N outputs land in the expected set and -clean removes every photo past the
	// limit that an earlier full run had generated.
	t.Run("clean with limit is rejected", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, true, 10, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-limit")
	})

	// The resize rule is reported first: it is the more fundamental misuse, and a run with
	// neither -resize nor a sane limit should not depend on check order for its message.
	t.Run("both problems report the resize rule", func(t *testing.T) {
		t.Parallel()
		err := validateCleanFlags(true, false, 10, outPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "-resize")
	})
}
