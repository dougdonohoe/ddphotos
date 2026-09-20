package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeCaption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		local        string
		base         string
		upstream     string
		want         string
		wantConflict bool
	}{
		{"untouched locally takes the upstream text", "old", "old", "new", "new", false},
		{"a local edit survives an unchanged upstream", "mine", "old", "old", "mine", false},
		{"both changed: upstream wins and warns", "mine", "old", "new", "new", true},
		{"nothing changed anywhere", "same", "same", "same", "same", false},
		{"a new photo takes the upstream text", "", "", "new", "new", false},
		// Empty is a value: clearing the description upstream clears the caption.
		{"a cleared upstream description clears an untouched caption", "old", "old", "", "", false},
		{"a locally cleared caption stays cleared", "", "old", "old", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, conflict := mergeCaption(tt.local, tt.base, tt.upstream)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantConflict, conflict)
		})
	}
}

func TestPhotogenID(t *testing.T) {
	t.Parallel()
	// Has to match loadPhotoDescriptions exactly, or a merged line would not be found
	// again when the album is built.
	assert.Equal(t, "img_001", photogenID("IMG_001.JPG"))
	assert.Equal(t, "img_001", photogenID("img_001"))
	assert.Equal(t, "clip", photogenID("clip.mov"))
	assert.Equal(t, "my.folder", photogenID("My.Folder"), "a non-media extension is left alone")
}

// syncItemsFor builds the items a merge operates on from file/caption pairs, in order.
func syncItemsFor(pairs ...[2]string) []syncItem {
	items := make([]syncItem, 0, len(pairs))
	for i, p := range pairs {
		items = append(items, syncItem{
			asset:   SyncAsset{ID: fmt.Sprintf("asset-%d", i)},
			file:    p[0],
			caption: p[1],
		})
	}
	return items
}

// metaFor builds the previous run's record, which is the merge's baseline.
func metaFor(pairs ...[2]string) *SyncMetadata {
	m := &SyncMetadata{}
	for _, p := range pairs {
		m.Photos = append(m.Photos, SyncPhotoMeta{File: p[0], Caption: p[1]})
	}
	return m
}

func TestMergeSyncCaptions(t *testing.T) {
	t.Parallel()

	// run merges into a temp dir seeded with the given photogen.txt and returns the file
	// as written plus any warnings raised.
	run := func(t *testing.T, existing string, items []syncItem, prev *SyncMetadata) (string, []string) {
		t.Helper()
		dir := t.TempDir()
		if existing != "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, photogenFileName), []byte(existing), 0o644))
		}
		var warnings []string
		warnf := func(format string, args ...any) {
			warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
		}
		require.NoError(t, mergeSyncCaptions(dir, items, prev, warnf))
		data, err := os.ReadFile(filepath.Join(dir, photogenFileName))
		require.NoError(t, err)
		return string(data), warnings
	}

	t.Run("a first sync writes every photo, captioned or not", func(t *testing.T) {
		t.Parallel()
		got, warnings := run(t, "",
			syncItemsFor([2]string{"a.jpg", "First"}, [2]string{"b.jpg", ""}),
			&SyncMetadata{})
		assert.Equal(t, "a.jpg First\nb.jpg\n", got)
		assert.Empty(t, warnings)
	})

	// manual_sort_order reads its order from this file, so an ordering set by hand or
	// through the DD Photos App has to survive a re-sync.
	t.Run("existing line order is preserved and new photos append in upstream order", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "c.jpg Third\na.jpg First\n",
			syncItemsFor([2]string{"a.jpg", "First"}, [2]string{"b.jpg", "Second"}, [2]string{"c.jpg", "Third"}),
			metaFor([2]string{"a.jpg", "First"}, [2]string{"c.jpg", "Third"}))
		assert.Equal(t, "c.jpg Third\na.jpg First\nb.jpg Second\n", got)
	})

	t.Run("a line for a pruned photo is dropped", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "a.jpg First\ngone.jpg Old\n",
			syncItemsFor([2]string{"a.jpg", "First"}),
			metaFor([2]string{"a.jpg", "First"}, [2]string{"gone.jpg", "Old"}))
		assert.Equal(t, "a.jpg First\n", got)
	})

	t.Run("a local edit survives when upstream has not changed", func(t *testing.T) {
		t.Parallel()
		got, warnings := run(t, "a.jpg My own words\n",
			syncItemsFor([2]string{"a.jpg", "Upstream text"}),
			metaFor([2]string{"a.jpg", "Upstream text"}))
		assert.Equal(t, "a.jpg My own words\n", got)
		assert.Empty(t, warnings)
	})

	t.Run("both changed: upstream wins and the photo is named in the warning", func(t *testing.T) {
		t.Parallel()
		got, warnings := run(t, "a.jpg My own words\n",
			syncItemsFor([2]string{"a.jpg", "Brand new upstream"}),
			metaFor([2]string{"a.jpg", "Original upstream"}))
		assert.Equal(t, "a.jpg Brand new upstream\n", got)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "a.jpg")
		assert.Contains(t, warnings[0], "captions: false")
	})

	t.Run("a name with a space is quoted so parsePhotogenLine reads it back whole", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "", syncItemsFor([2]string{"Sue and Bob.jpg", "A caption"}), &SyncMetadata{})
		assert.Equal(t, "\"Sue and Bob.jpg\" A caption\n", got)

		name, desc := parsePhotogenLine(strings.TrimSuffix(got, "\n"))
		assert.Equal(t, "Sue and Bob.jpg", name)
		assert.Equal(t, "A caption", desc)
	})

	// scanLines drops any line starting with #, so an unquoted name would vanish on the
	// next read and the caption with it.
	t.Run("a name starting with a hash is quoted", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "", syncItemsFor([2]string{"#1.jpg", "Number one"}), &SyncMetadata{})
		assert.Equal(t, "\"#1.jpg\" Number one\n", got)
	})

	// The file the merge writes has to be the file the album build reads.
	t.Run("what is written is what loadPhotoDescriptions reads back", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		items := syncItemsFor(
			[2]string{"IMG_1583.jpeg", "So wide open!"},
			[2]string{"Sue and Bob.jpg", "A &amp; B"},
			[2]string{"plain.jpg", ""},
		)
		require.NoError(t, mergeSyncCaptions(dir, items, &SyncMetadata{}, func(string, ...any) {}))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"img_1583", "sue and bob", "plain"}, pd.order)
		assert.Equal(t, "So wide open!", pd.descriptions["img_1583"])
		assert.Equal(t, "A &amp; B", pd.descriptions["sue and bob"])
		assert.Equal(t, "", pd.descriptions["plain"])
	})

	t.Run("a hand-written entry with no extension is matched and canonicalized", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "img_1583 Written by hand\n",
			syncItemsFor([2]string{"IMG_1583.jpeg", "Upstream"}),
			metaFor([2]string{"IMG_1583.jpeg", "Upstream"}))
		assert.Equal(t, "IMG_1583.jpeg Written by hand\n", got)
	})
}
