package photogen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Use permissive modes so files written inside a Docker container (as root) remain
// accessible to the host user that mounted the volume.
const (
	dirPerms  os.FileMode = 0777
	filePerms os.FileMode = 0666
)

// loadJSON reads a JSON file at path and unmarshals it into a value of type T.
func loadJSON[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &zero); err != nil {
		return zero, fmt.Errorf("parse %s: %w", path, err)
	}
	return zero, nil
}

// scanLines opens path, reads it line by line, and calls fn for each non-blank,
// non-comment line (lines beginning with '#' are treated as comments).
// Returns any open or scanner error unwrapped; callers should wrap as needed.
func scanLines(path string, fn func(line string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fn(line)
	}
	return scanner.Err()
}
