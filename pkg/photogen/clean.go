package photogen

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanOutputDir removes stale files in siteDir that are not in expectedFiles.
// Only the site root (non-directory files) and directories named in processedSlugs
// are examined; unprocessed album directories are left untouched.
//
// A file that cannot be removed is a warning, not an error: the rest of the clean still
// runs, and warn replays it in the end-of-run summary. That matters more here than for
// most warnings, because a stale file left behind is exactly what -clean exists to stop
// reaching the server. warn may be nil (it then only prints).
func CleanOutputDir(siteDir string, processedSlugs []string, expectedFiles map[string]bool, dryRun bool, warn *WarnCollector) error {
	slugSet := make(map[string]bool, len(processedSlugs))
	for _, s := range processedSlugs {
		slugSet[s] = true
	}

	entries, err := os.ReadDir(siteDir)
	if os.IsNotExist(err) {
		return nil // nothing to clean on first run
	}
	if err != nil {
		return fmt.Errorf("read site dir %s: %w", siteDir, err)
	}

	for _, e := range entries {
		fullPath := filepath.Join(siteDir, e.Name())
		if e.IsDir() {
			if !slugSet[e.Name()] {
				continue // leave unprocessed album directories alone
			}
			if err := cleanDir(fullPath, expectedFiles, dryRun, warn); err != nil {
				return err
			}
		} else if !expectedFiles[fullPath] {
			removeStale(fullPath, dryRun, warn)
		}
	}
	return nil
}

func cleanDir(dir string, expectedFiles map[string]bool, dryRun bool, warn *WarnCollector) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !expectedFiles[path] {
			removeStale(path, dryRun, warn)
		}
		return nil
	})
}

func removeStale(path string, dryRun bool, warn *WarnCollector) {
	if dryRun {
		fmt.Printf("DRYRUN: would remove %s\n", path)
		return
	}
	if err := os.Remove(path); err != nil {
		// err already reads "remove <path>: <reason>", so the path is not repeated.
		warn.Warnf("WARN: -clean could not delete a stale file, which stays in the output: %v\n", err)
		return
	}
	fmt.Printf("removed: %s\n", path)
}
