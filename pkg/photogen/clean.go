package photogen

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanOutputDir removes stale files in siteDir that are not in expectedFiles.
// Only the site root (non-directory files) and directories named in processedSlugs
// are examined; unprocessed album directories are left untouched.
func CleanOutputDir(siteDir string, processedSlugs []string, expectedFiles map[string]bool, dryRun bool) error {
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
			if err := cleanDir(fullPath, expectedFiles, dryRun); err != nil {
				return err
			}
		} else if !expectedFiles[fullPath] {
			removeStale(fullPath, dryRun)
		}
	}
	return nil
}

func cleanDir(dir string, expectedFiles map[string]bool, dryRun bool) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !expectedFiles[path] {
			removeStale(path, dryRun)
		}
		return nil
	})
}

func removeStale(path string, dryRun bool) {
	if dryRun {
		fmt.Printf("DRYRUN: would remove %s\n", path)
		return
	}
	if err := os.Remove(path); err != nil {
		fmt.Printf("WARN: failed to remove %s: %v\n", path, err)
		return
	}
	fmt.Printf("removed: %s\n", path)
}
