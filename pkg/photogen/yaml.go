package photogen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// selfValidating is implemented by a config type that can check itself once parsed.
// Unexported along with validate itself: validation is part of loading, not something a
// caller outside the package is expected to invoke.
type selfValidating interface {
	validate() error
}

// readYAML reads the YAML file at path and unmarshals it into a new T.
//
// The read and the parse are reported separately because they mean different things to
// whoever has to fix it: a missing or unreadable file is a path problem, a parse failure is
// a content problem. Both name the file, since one run loads several of these.
//
// KnownFields is on, so a key that matches no field is an error rather than being ignored.
// These files are hand-edited and every key is optional-looking, so a typo used to be
// invisible: `site_nmae:` parsed happily and the site fell back to a default, with nothing
// said. The cost is that a key photogen does not know is now fatal, which is the intended
// trade: there is nowhere in these files to put a note or a setting for another tool.
func readYAML[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	v := new(T)
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		// A file that is empty, or only comments, has no document to decode and the decoder
		// says io.EOF. yaml.Unmarshal treated that as the zero value and callers rely on it:
		// an empty customization.yaml means "no customizations", not a broken file.
		if errors.Is(err, io.EOF) {
			return v, nil
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}

// loadYAML is readYAML plus the type's own validate, for a config file that is usable
// exactly as parsed. A file that has to be transformed afterward uses readYAML and
// validates separately: the passwords file parses as passwordsFile but becomes an
// EncryptConfig, whose Validate needs the album list and so runs later, from Config.Validate.
//
// The two type parameters are the usual Go shape for "the pointer to T, not T, implements
// the interface". PT is pinned to *T and is what carries validate, so a caller writes
// loadYAML[AlbumsFile](path) and the rest is inferred.
func loadYAML[T any, PT interface {
	*T
	selfValidating
}](path string) (PT, error) {
	v, err := readYAML[T](path)
	if err != nil {
		return nil, err
	}
	// The path prefix only, with no verb: validates messages already read as statements
	// about the file's contents ("album %q: name is required").
	if err := PT(v).validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// writeYAMLAtomic marshals v and replaces path with the result, prefixed by header.
//
// Atomic for the same reason MetaCache.Save is: an interrupted or failed run must never
// leave a half-written record that the next run then parses as the truth. header is written
// verbatim ahead of the document and is expected to end in a newline; it is how a generated
// file says it is generated, since yaml.Marshal has nowhere to put a comment.
func writeYAMLAtomic(path, header string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return writeFileAtomic(path, append([]byte(header), data...))
}
