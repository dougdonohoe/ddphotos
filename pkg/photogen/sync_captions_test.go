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

// metaFor builds the previous run's record, which is the merge's baseline. Records carry
// no asset ID unless a test sets one; see attachPrev.
func metaFor(pairs ...[2]string) *SyncMetadata {
	m := &SyncMetadata{}
	for _, p := range pairs {
		m.Photos = append(m.Photos, SyncPhotoMeta{File: p[0], Caption: p[1]})
	}
	return m
}

// attachPrev does what assignSyncFileNames does for the merge: it gives each item the
// previous run's record for the same asset. A record with no asset ID is taken to be the
// item's own when the file name matches, which keeps most tests down to file/caption pairs;
// a test about a different asset holding the name sets the record's AssetID.
func attachPrev(items []syncItem, prev *SyncMetadata) {
	for i := range items {
		for j := range prev.Photos {
			rec := &prev.Photos[j]
			if rec.File == items[i].file && (rec.AssetID == "" || rec.AssetID == items[i].asset.ID) {
				items[i].prev = rec
			}
		}
	}
}

func TestMergeSyncCaptions(t *testing.T) {
	t.Parallel()

	// run merges into a temp dir seeded with the given photogen.txt and returns the file
	// as written plus any warnings raised.
	// subdirs are created in the album folder first, as a user would add them.
	run := func(t *testing.T, existing string, items []syncItem, prev *SyncMetadata, subdirs ...string) (string, []string) {
		t.Helper()
		dir := t.TempDir()
		for _, sd := range subdirs {
			require.NoError(t, os.Mkdir(filepath.Join(dir, sd), 0o755))
		}
		if existing != "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, photogenFileName), []byte(existing), 0o644))
		}
		var warnings []string
		warnf := func(format string, args ...any) {
			warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
		}
		attachPrev(items, prev)
		require.NoError(t, mergeSyncCaptions(dir, items, warnf))
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

	// The file is the user's to edit, so what sync does not own is written back as found.
	t.Run("comments and blank lines keep their place", func(t *testing.T) {
		t.Parallel()
		existing := "# Trip notes\n\nb.jpg Second\n  # indented, kept as typed\na.jpg First\n"
		got, warnings := run(t, existing,
			syncItemsFor([2]string{"a.jpg", "First"}, [2]string{"b.jpg", "Second"}),
			metaFor([2]string{"a.jpg", "First"}, [2]string{"b.jpg", "Second"}))
		assert.Equal(t, existing, got)
		assert.Empty(t, warnings)
	})

	t.Run("a removed photo's line goes but the comments around it stay", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "# header\na.jpg First\ngone.jpg Old\n# footer\n",
			syncItemsFor([2]string{"a.jpg", "First"}, [2]string{"c.jpg", "Third"}),
			metaFor([2]string{"a.jpg", "First"}, [2]string{"gone.jpg", "Old"}))
		assert.Equal(t, "# header\na.jpg First\n# footer\nc.jpg Third\n", got)
	})

	// Prune leaves subdirectories alone, so a subfolder entry, which is how
	// manual_sort_order places a subfolder, has to survive too.
	t.Run("a subfolder entry keeps its place, written as found", func(t *testing.T) {
		t.Parallel()
		existing := "a.jpg First\n\"Extra Shots\"   Bonus\nb.jpg Second\n"
		got, _ := run(t, existing,
			syncItemsFor([2]string{"a.jpg", "First"}, [2]string{"b.jpg", "Second"}),
			metaFor([2]string{"a.jpg", "First"}, [2]string{"b.jpg", "Second"}),
			"Extra Shots")
		assert.Equal(t, existing, got)
	})

	t.Run("an entry naming neither a photo nor a subfolder is dropped", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "a.jpg First\nextras Bonus\n",
			syncItemsFor([2]string{"a.jpg", "First"}),
			metaFor([2]string{"a.jpg", "First"}))
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

	// metadata.yaml records upstream captions even with captions: false, so turning captions
	// on (or deleting photogen.txt) finds a baseline but no line. A missing line is not a
	// local edit to an empty caption; a line with no text is.
	t.Run("a photo with no line takes the upstream text", func(t *testing.T) {
		t.Parallel()
		items := syncItemsFor([2]string{"a.jpg", "Sunset"}, [2]string{"b.jpg", "Sunrise"})
		prev := metaFor([2]string{"a.jpg", "Sunset"}, [2]string{"b.jpg", "Sunrise"})

		got, warnings := run(t, "", items, prev)
		assert.Equal(t, "a.jpg Sunset\nb.jpg Sunrise\n", got)
		assert.Empty(t, warnings)

		got, warnings = run(t, "b.jpg\n", items, prev)
		assert.Equal(t, "b.jpg\na.jpg Sunset\n", got)
		assert.Empty(t, warnings)
	})

	// A new asset can be given a removed asset's name. The line under that name belongs to
	// the removed asset, so the new one must not inherit its caption or its baseline.
	t.Run("a new asset reusing a removed asset's name takes only its own caption", func(t *testing.T) {
		t.Parallel()
		// The old asset had no upstream caption, and the user wrote one locally.
		prev := metaFor([2]string{"IMG_0001.jpg", ""})
		prev.Photos[0].AssetID = "old-asset"

		got, warnings := run(t, "IMG_0001.jpg Written for the old photo\n",
			syncItemsFor([2]string{"IMG_0001.jpg", ""}), prev)
		assert.Equal(t, "IMG_0001.jpg\n", got)
		assert.Empty(t, warnings)

		got, warnings = run(t, "IMG_0001.jpg Written for the old photo\n",
			syncItemsFor([2]string{"IMG_0001.jpg", "New upstream"}), prev)
		assert.Equal(t, "IMG_0001.jpg New upstream\n", got)
		assert.Empty(t, warnings, "a different asset's edit is not a conflict")
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
		require.NoError(t, mergeSyncCaptions(dir, items, func(string, ...any) {}))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"img_1583", "sue and bob", "plain"}, pd.order)
		assert.Equal(t, "So wide open!", pd.descriptions["img_1583"])
		assert.Equal(t, "A &amp; B", pd.descriptions["sue and bob"])
		assert.Equal(t, "", pd.descriptions["plain"])
	})

	// parsePhotogenLine does no unescaping, so a quoted name has to be written verbatim.
	// The emoji here is joined by U+200D, which is not printable and which %q escaped to a
	// literal ‍ that no photo's key ever matched.
	t.Run("a quoted name with a non-printable rune is written verbatim", func(t *testing.T) {
		t.Parallel()
		name := "Family \U0001F468‍\U0001F469‍\U0001F467.jpg"
		dir := t.TempDir()
		items := syncItemsFor([2]string{name, "Reunion"}, [2]string{"b.jpg", "Other"})
		require.NoError(t, mergeSyncCaptions(dir, items, func(string, ...any) {}))

		data, err := os.ReadFile(filepath.Join(dir, photogenFileName))
		require.NoError(t, err)
		assert.Equal(t, `"`+name+`" Reunion`+"\nb.jpg Other\n", string(data))

		pd, err := loadPhotoDescriptions(dir)
		require.NoError(t, err)
		assert.Equal(t, "Reunion", pd.descriptions[photogenID(name)])

		// A re-sync must find the line again, so it keeps its place rather than being
		// dropped and re-appended after b.jpg.
		items = syncItemsFor([2]string{"b.jpg", "Other"}, [2]string{name, "Reunion"})
		attachPrev(items, metaFor([2]string{name, "Reunion"}, [2]string{"b.jpg", "Other"}))
		require.NoError(t, mergeSyncCaptions(dir, items, func(string, ...any) {}))
		data, err = os.ReadFile(filepath.Join(dir, photogenFileName))
		require.NoError(t, err)
		assert.Equal(t, `"`+name+`" Reunion`+"\nb.jpg Other\n", string(data))
	})

	t.Run("a hand-written entry with no extension is matched and canonicalized", func(t *testing.T) {
		t.Parallel()
		got, _ := run(t, "img_1583 Written by hand\n",
			syncItemsFor([2]string{"IMG_1583.jpeg", "Upstream"}),
			metaFor([2]string{"IMG_1583.jpeg", "Upstream"}))
		assert.Equal(t, "IMG_1583.jpeg Written by hand\n", got)
	})
}
