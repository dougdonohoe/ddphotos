package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// captionLine is one physical photogen.txt line.
type captionLine struct {
	raw   string // the line as written, for lines the merge writes back untouched
	entry bool   // false for a blank line or a # comment
	id    string // entries only: lowercase name with any media extension stripped
	desc  string // entries only
}

// readCaptionLines parses photogen.txt in file order, every line included.
//
// It exists alongside loadPhotoDescriptions rather than reusing it because the merge has to
// write the file back: it needs the blank and comment lines loadPhotoDescriptions skips, in
// place. The parts that have to agree are shared outright: both decide what is an entry
// with isBlankOrComment and parse it with parsePhotogenLine, and both key on photogenID,
// which is what lets a line written here be found again when the album is built.
func readCaptionLines(path string) ([]captionLine, error) {
	var lines []captionLine
	err := scanRawLines(path, func(raw string) {
		if isBlankOrComment(raw) {
			lines = append(lines, captionLine{raw: raw})
			return
		}
		name, desc := parsePhotogenLine(strings.TrimSpace(raw))
		lines = append(lines, captionLine{raw: raw, entry: true, id: photogenID(name), desc: desc})
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
	local := make(map[string]string, len(existing))
	for _, l := range existing {
		if l.entry {
			local[l.id] = l.desc
		}
	}
	subdirs, err := subdirIDs(dir)
	if err != nil {
		return err
	}
	byID := make(map[string]syncItem, len(items))
	for _, it := range items {
		byID[photogenID(it.file)] = it
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
		localDesc, present := local[photogenID(it.file)]
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
	seen := make(map[string]struct{}, len(items))
	kept := 0 // subfolder entries, for the count printed below
	for _, l := range existing {
		if !l.entry {
			b.WriteString(l.raw + "\n")
			continue
		}
		it, ok := byID[l.id]
		if !ok {
			if _, sub := subdirs[l.id]; sub {
				b.WriteString(l.raw + "\n")
				kept++
			}
			continue
		}
		if _, dup := seen[l.id]; dup {
			continue
		}
		seen[l.id] = struct{}{}
		writeCaptionLine(&b, it.file, merged(it))
	}
	for _, it := range items {
		id := photogenID(it.file)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		writeCaptionLine(&b, it.file, merged(it))
	}

	if err := writeFileAtomic(path, []byte(b.String())); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("  wrote: %s (%d entries)\n", path, len(seen)+kept)
	return nil
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
