package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// captionLine is one physical photogen.txt line.
type captionLine struct {
	raw   string // the line as written, for lines the merge writes back untouched
	entry bool   // false for a blank line or a # comment
	name  string // entries only: the entry name, lowercased but otherwise as written
	desc  string // entries only
}

// readCaptionLines parses photogen.txt in file order, every line included.
//
// It exists alongside loadPhotoDescriptions rather than reusing it because the merge has to
// write the file back: it needs the blank and comment lines loadPhotoDescriptions skips, in
// place. The parts that have to agree are shared outright: both decide what is an entry
// with isBlankOrComment and parse it with parsePhotogenLine, and both resolve an entry by
// full file name first and by photogenID stem second (syncItemMatcher mirrors photoMatcher),
// which is what lets a line written here be found again when the album is built.
func readCaptionLines(path string) ([]captionLine, error) {
	var lines []captionLine
	err := scanRawLines(path, func(raw string) {
		if isBlankOrComment(raw) {
			lines = append(lines, captionLine{raw: raw})
			return
		}
		name, desc := parsePhotogenLine(strings.TrimSpace(raw))
		lines = append(lines, captionLine{raw: raw, entry: true, name: strings.ToLower(name), desc: desc})
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return lines, nil
}

// mergeCaption resolves one photo's caption from the three values a sync has to reconcile:
// what is in photogen.txt now (local), what the last sync recorded upstream as saying
// (base), and what upstream says now.
//
//	local == base                    → upstream (the user has not touched it)
//	local != base, upstream == base  → local    (only the user changed it)
//	local != base, upstream != base  → upstream, and the caller warns
//
// Empty is a value, so an upstream description that gets cleared clears an untouched local
// caption. base always tracks upstream and never the file, so a local edit that wins the
// second row keeps winning on every later run for as long as upstream stays put.
func mergeCaption(local, base, upstream string) (result string, conflict bool) {
	if local == base {
		return upstream, false
	}
	if upstream == base {
		return local, false
	}
	return upstream, true
}

// mergeSyncCaptions rewrites photogen.txt from the current assets, preserving what the user
// has done to it.
//
// Line order is preserved: photos already in the file keep their position and new photos
// are appended in upstream order. That matters because manual_sort_order: true reads its
// order from this file, so an ordering set by hand or through the DD Photos App survives a
// re-sync. Of the other lines:
//
//   - blank lines and comments are written back as found, in place;
//   - an entry naming a subfolder is too, since prune leaves subfolders alone and the entry
//     is how manual_sort_order places one;
//   - a photo entry is rewritten under the item's full file name, and a bare stem naming
//     several files becomes one line for each, in its place;
//   - any other entry is dropped: its photo was removed upstream, or it names nothing.
//
// The baseline is each item's own record from the *previous* run (syncItem.prev, found by
// asset ID), which syncOneAlbum replaces afterward. It is looked up by asset rather than by
// file name because a new asset can be given a removed asset's name.
func mergeSyncCaptions(dir string, items []syncItem, warnf func(string, ...any)) error {
	path := filepath.Join(dir, photogenFileName)
	existing, err := readCaptionLines(path)
	if err != nil {
		return err
	}
	subdirs, err := subdirIDs(dir)
	if err != nil {
		return err
	}
	match := newSyncItemMatcher(items)

	// What the file says about each item, by its lowercased file name. A line naming the
	// file wins over a bare stem naming it with its variants, as it does in the build.
	local := make(map[string]string, len(existing))
	for _, exact := range []bool{false, true} {
		for _, l := range existing {
			if !l.entry || match.isName(l.name) != exact {
				continue
			}
			for _, it := range match.items(l.name) {
				local[strings.ToLower(it.file)] = l.desc
			}
		}
	}

	merged := func(it syncItem) string {
		// A new asset has no baseline, and any line under its name was written for a
		// removed asset that had the name before it.
		if it.prev == nil {
			return it.caption
		}
		base := it.prev.Caption
		// No line is not a local edit: the file may never have been written (captions was
		// off, which still records the baseline) or was deleted. Treat it as untouched so
		// upstream wins. A deliberately cleared caption is a line with no text.
		localDesc, present := local[strings.ToLower(it.file)]
		if !present {
			localDesc = base
		}
		desc, conflict := mergeCaption(localDesc, base, it.caption)
		if conflict {
			warnf("WARN: caption for %s changed both locally and upstream; "+
				"the upstream text wins (use captions: false to keep local captions)\n", it.file)
		}
		return desc
	}

	var b strings.Builder
	seen := make(map[string]struct{}, len(items)) // lowercased file names written
	kept := 0                                     // subfolder entries, for the count printed below
	for _, l := range existing {
		if !l.entry {
			b.WriteString(l.raw + "\n")
			continue
		}
		// A bare stem naming several files is rewritten in place as one line per file,
		// which keeps its position for manual_sort_order and its meaning, since in the
		// build a stem captions every variant.
		its := match.items(l.name)
		if len(its) == 0 {
			if _, sub := subdirs[l.name]; sub {
				b.WriteString(l.raw + "\n")
				kept++
			}
			continue
		}
		for _, it := range its {
			key := strings.ToLower(it.file)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			writeCaptionLine(&b, it.file, merged(it))
		}
	}
	for _, it := range items {
		key := strings.ToLower(it.file)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		writeCaptionLine(&b, it.file, merged(it))
	}

	if err := writeFileAtomic(path, []byte(b.String())); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("  wrote: %s (%d entries)\n", path, len(seen)+kept)
	return nil
}

// syncItemMatcher resolves photogen.txt entries to sync items with the same two tiers as
// photoMatcher: a full file name names that file, and anything else names every file with
// its stem. Keeping the rules identical is what makes a line the merge writes mean the same
// thing to the build.
type syncItemMatcher struct {
	byName map[string]syncItem   // lowercased local file name
	byStem map[string][]syncItem // photogenID of the local file name, in file-name order
}

func newSyncItemMatcher(items []syncItem) *syncItemMatcher {
	m := &syncItemMatcher{
		byName: make(map[string]syncItem, len(items)),
		byStem: make(map[string][]syncItem, len(items)),
	}
	sorted := append([]syncItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].file < sorted[j].file })
	for _, it := range sorted {
		m.byName[strings.ToLower(it.file)] = it
		stem := photogenID(it.file)
		m.byStem[stem] = append(m.byStem[stem], it)
	}
	return m
}

// isName reports whether an entry names one file in full.
func (m *syncItemMatcher) isName(entry string) bool {
	_, ok := m.byName[strings.ToLower(entry)]
	return ok
}

// items returns what an entry names, or nil when it names no item (it may be a subfolder).
func (m *syncItemMatcher) items(entry string) []syncItem {
	if it, ok := m.byName[strings.ToLower(entry)]; ok {
		return []syncItem{it}
	}
	return m.byStem[photogenID(entry)]
}

// subdirIDs returns the album folder's subfolders by lowercased name, which is how the build
// matches a photogen.txt entry to a subfolder (subdirActual in collectPhotosRecursive).
func subdirIDs(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	ids := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			ids[strings.ToLower(e.Name())] = struct{}{}
		}
	}
	return ids, nil
}

// writeCaptionLine emits one photogen.txt line.
//
// The name is quoted when parsePhotogenLine would otherwise mis-split it — a space makes
// everything after the first word look like the description — and also when the line would
// start with a character scanLines treats specially, so a file name beginning with # is not
// silently read back as a comment.
//
// The quotes go around the name verbatim, not via %q: parsePhotogenLine does no unescaping,
// so %q's ‍ for a zero-width joiner would be read back as literal text. Verbatim is
// safe because sanitizeSyncFileName strips any " from the name.
func writeCaptionLine(b *strings.Builder, name, desc string) {
	if strings.ContainsAny(name, " \t") || strings.HasPrefix(name, "#") || strings.HasPrefix(name, `"`) {
		b.WriteString(`"` + name + `"`)
	} else {
		b.WriteString(name)
	}
	if desc != "" {
		b.WriteString(" ")
		b.WriteString(desc)
	}
	b.WriteString("\n")
}
