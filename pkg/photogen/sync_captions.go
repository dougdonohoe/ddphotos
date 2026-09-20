package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// captionLine is one parsed photogen.txt entry.
type captionLine struct {
	id   string // the lookup key: lowercase name with any media extension stripped
	desc string
}

// readCaptionLines parses photogen.txt in file order.
//
// It exists alongside loadPhotoDescriptions rather than reusing it because the merge needs
// the entries in file order, where loadPhotoDescriptions hands back a map plus an order of
// IDs. The parts that have to agree are shared outright: both read through scanLines, so
// both skip blank and # lines the same way, and both key on photogenID — which is what
// lets a line written here be found again when the album is built.
func readCaptionLines(path string) ([]captionLine, error) {
	var lines []captionLine
	err := scanLines(path, func(line string) {
		name, desc := parsePhotogenLine(line)
		lines = append(lines, captionLine{id: photogenID(name), desc: desc})
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
// Line order is preserved: photos already in the file keep their position, new photos are
// appended in upstream order, and lines matching no current asset are dropped. That matters
// because manual_sort_order: true reads its order from this file, so an ordering set by
// hand or through the DD Photos App survives a re-sync.
//
// prev is the metadata record written by the *previous* run, which is what makes the
// baseline comparison possible; syncOneAlbum passes it in and replaces the file afterward.
func mergeSyncCaptions(dir string, items []syncItem, prev *SyncMetadata, warnf func(string, ...any)) error {
	path := filepath.Join(dir, photogenFileName)
	existing, err := readCaptionLines(path)
	if err != nil {
		return err
	}
	local := make(map[string]string, len(existing))
	for _, l := range existing {
		local[l.id] = l.desc
	}
	baseByFile := prev.captionByFile()

	byID := make(map[string]syncItem, len(items))
	for _, it := range items {
		byID[photogenID(it.file)] = it
	}

	merged := func(it syncItem) string {
		id := photogenID(it.file)
		localDesc, present := local[id]
		if !present {
			localDesc = ""
		}
		base := baseByFile[it.file]
		desc, conflict := mergeCaption(localDesc, base, it.caption)
		if conflict {
			warnf("WARN: caption for %s changed both locally and upstream; "+
				"the upstream text wins (use captions: false to keep local captions)\n", it.file)
		}
		return desc
	}

	var b strings.Builder
	seen := make(map[string]struct{}, len(items))
	for _, l := range existing {
		it, ok := byID[l.id]
		if !ok {
			continue // the photo this line named is no longer in the album
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
	fmt.Printf("  wrote: %s (%d entries)\n", path, len(seen))
	return nil
}

// writeCaptionLine emits one photogen.txt line.
//
// The name is quoted when parsePhotogenLine would otherwise mis-split it — a space makes
// everything after the first word look like the description — and also when the line would
// start with a character scanLines treats specially, so a file name beginning with # is not
// silently read back as a comment.
func writeCaptionLine(b *strings.Builder, name, desc string) {
	if strings.ContainsAny(name, " \t") || strings.HasPrefix(name, "#") || strings.HasPrefix(name, `"`) {
		fmt.Fprintf(b, "%q", name)
	} else {
		b.WriteString(name)
	}
	if desc != "" {
		b.WriteString(" ")
		b.WriteString(desc)
	}
	b.WriteString("\n")
}
