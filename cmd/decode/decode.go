// decode decrypts an enc.json file produced by photogen and prints the plaintext JSON.
//
// Usage:
//
//	go run cmd/decode/decode.go <path.enc.json>
//	go run cmd/decode/decode.go -passwords <pw-file> <path.enc.json>
//
// If -passwords is not given, the passwords file path is read from the pwFile
// field embedded in the enc.json by photogen. The correct password is determined
// automatically from the filename:
//
//   - albums.enc.json  → site-wide password (site.password)
//   - html.enc.json    → site-wide password (site.password)
//   - index.enc.json   → per-album password for the parent directory slug
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dougdonohoe/ddphotos/pkg/photogen"
)

func main() {
	passwords := flag.String("passwords", "", "path to passwords file (overrides pwFile stored in enc.json)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: decode [-passwords <file>] <path.enc.json>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	encPath := flag.Arg(0)

	pwFile := *passwords
	if pwFile == "" {
		var err error
		pwFile, err = photogen.ReadPwFile(encPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", encPath, err)
			os.Exit(1)
		}
		if pwFile == "" {
			fmt.Fprintf(os.Stderr, "No passwords file found in %s — use -passwords flag\n", encPath)
			os.Exit(1)
		}
	}

	ec, err := photogen.LoadEncryptConfig(pwFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading passwords from %s: %v\n", pwFile, err)
		os.Exit(1)
	}

	password := passwordForFile(ec, encPath)
	if password == "" {
		fmt.Fprintf(os.Stderr, "No password found for %s in %s\n", encPath, pwFile)
		os.Exit(1)
	}

	data, err := os.ReadFile(encPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", encPath, err)
		os.Exit(1)
	}

	plaintext, err := photogen.DecryptJSON(data, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Decrypt failed: %v\n", err)
		os.Exit(1)
	}

	// Pretty-print the decrypted JSON
	var out any
	if err := json.Unmarshal(plaintext, &out); err != nil {
		// Not valid JSON (shouldn't happen) — print raw
		fmt.Println(string(plaintext))
		return
	}
	pretty, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Println(string(plaintext))
		return
	}
	fmt.Println(string(pretty))
}

// passwordForFile returns the appropriate password for the given enc.json path:
//   - albums.enc.json → site-wide password
//   - html.enc.json   → site-wide password
//   - index.enc.json  → per-album password for the parent directory (album slug)
//
// html.enc.json used to fall through to the album branch and work only by accident: the
// parent directory of a site-level file is the site ID, the site ID is normally absent from
// AlbumPasswords, and AlbumPassword falls back to SitePassword when a slug is missing. That
// accident breaks as soon as an album's slug equals the site ID and the album has its own
// password, which nothing forbids, since validSiteID and slugPattern both accept the same
// strings. Measured before this change: with settings.id "uganda" and an album "uganda"
// carrying its own password, decoding uganda/html.enc.json failed with "cipher: message
// authentication failed" while uganda/albums.enc.json decoded fine.
//
// These three names come from jsonNames in pkg/photogen/json.go, which is where the set is
// decided; a fourth encrypted artifact would need adding here too.
func passwordForFile(ec *photogen.EncryptConfig, encPath string) string {
	switch filepath.Base(encPath) {
	case "albums.enc.json", "html.enc.json":
		return ec.SitePassword
	}
	slug := filepath.Base(filepath.Dir(encPath))
	return ec.AlbumPassword(slug)
}
