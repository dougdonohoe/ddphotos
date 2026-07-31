package photogen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultCustomizationFile is the filename looked for alongside albums.yaml in the config
// directory. It is optional: a config dir without one produces a site with default chrome.
const DefaultCustomizationFile = "customization.yaml"

// Customizations is the structure parsed from a customization file. It holds optional
// presentation overrides, all of which are purely cosmetic — an absent file leaves the site
// looking exactly as it does without one.
//
// It lives in its own file rather than a block in albums.yaml so that tools which own
// albums.yaml (notably the DD Photos desktop app) neither need to know about it nor risk
// reformatting it on save.
type Customizations struct {
	// AlbumNav replaces the default "← Albums" link in each album page header.
	AlbumNav []NavLink `yaml:"album_nav"`
}

// NavLink is a single configurable navigation link.
type NavLink struct {
	Label  string `yaml:"label" json:"label"`
	Href   string `yaml:"href" json:"href"`
	ID     string `yaml:"id" json:"id,omitempty"`          // optional HTML id, for custom CSS to target
	NewTab bool   `yaml:"new_tab" json:"newTab,omitempty"` // open in a new tab
}

// LoadCustomizations reads and validates a customization file.
func LoadCustomizations(path string) (*Customizations, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Customizations
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// ResolveCustomizations applies the precedence between the -no-customization flag, an explicit
// -customization path, and the default file in configDir. It always returns a non-nil value so
// callers never have to nil-check.
//
// An explicit overridePath that does not exist is an error; a missing default file is not.
func ResolveCustomizations(configDir, overridePath string, disabled bool) (*Customizations, error) {
	if disabled {
		return &Customizations{}, nil
	}
	if overridePath != "" {
		return LoadCustomizations(overridePath)
	}
	path := filepath.Join(configDir, DefaultCustomizationFile)
	if _, err := os.Stat(path); err != nil {
		return &Customizations{}, nil
	}
	return LoadCustomizations(path)
}

func (c *Customizations) validate() error {
	return validateNavLinks("album_nav", c.AlbumNav)
}

// navIDPattern matches ids that are safe to use both as an HTML id and as a CSS selector,
// so a site owner can style a link with "#my-id { ... }" in their custom CSS.
var navIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// validateNavLinks checks a configured list of navigation links. context names the YAML key
// for error messages.
func validateNavLinks(context string, links []NavLink) error {
	seen := map[string]bool{}
	for i, l := range links {
		if l.Label == "" {
			return fmt.Errorf("%s[%d]: label is required", context, i)
		}
		if l.Href == "" {
			return fmt.Errorf("%s[%d] (%q): href is required", context, i, l.Label)
		}
		// Either an absolute URL with a scheme, or a site-root-relative path. A bare
		// "albums/foo" would resolve differently depending on the current page.
		if !strings.Contains(l.Href, "://") && !strings.HasPrefix(l.Href, "/") &&
			!strings.HasPrefix(l.Href, "mailto:") {
			return fmt.Errorf("%s[%d] (%q): href %q must start with \"/\" or include a scheme such as https://",
				context, i, l.Label, l.Href)
		}
		if l.ID == "" {
			continue
		}
		if !navIDPattern.MatchString(l.ID) {
			return fmt.Errorf("%s[%d] (%q): id %q must start with a letter and contain only letters, digits, hyphens or underscores",
				context, i, l.Label, l.ID)
		}
		if seen[l.ID] {
			return fmt.Errorf("%s: duplicate id %q", context, l.ID)
		}
		seen[l.ID] = true
	}
	return nil
}
