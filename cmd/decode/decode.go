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
//   - albums.enc.json  → site-wide password (_all_)
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
//   - index.enc.json  → per-album password for the parent directory (album slug)
func passwordForFile(ec *photogen.EncryptConfig, encPath string) string {
	if filepath.Base(encPath) == "albums.enc.json" {
		return ec.SitePassword
	}
	slug := filepath.Base(filepath.Dir(encPath))
	return ec.AlbumPassword(slug)
}
