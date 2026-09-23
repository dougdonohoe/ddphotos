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

// ParseEnvFile reads a KEY=VALUE file and returns its entries. scanLines has already dropped
// blank lines and comments; a line with no '=' is ignored, the way the shell would ignore it.
//
// Unlike loadDefaultsEnv in cmd/photogen, which this replaced the parsing half of, it does not
// touch the process environment. A caller wanting "a real environment variable wins over the
// file" reads os.LookupEnv first and falls back to the returned map. A missing file is returned
// as an error so the caller decides whether absence matters.
//
// One pair of surrounding quotes is stripped from a value. Nothing in this project writes them,
// but an env file holding a secret is hand-typed and quoting it is the obvious instinct, and a
// key with a stray quote on each end fails in a way that is very hard to see.
func ParseEnvFile(path string) (map[string]string, error) {
	out := map[string]string{}
	err := scanLines(path, func(line string) {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		out[strings.TrimSpace(key)] = val
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
