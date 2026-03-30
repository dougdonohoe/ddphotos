package photogen

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// EncryptConfig holds the keys and passwords loaded from a passwords file.
// See LoadEncryptConfig for the file format.
type EncryptConfig struct {
	// HMACKey is used to derive deterministic UUID filenames for images (_key_: entry).
	// If empty, original WebP filenames are used unchanged.
	HMACKey string
	// SitePassword encrypts albums.json for the whole site (_all_: entry).
	// If empty, albums.json is written unencrypted.
	SitePassword string
	// AlbumPasswords holds per-album passwords keyed by slug.
	// An album with no entry here falls back to SitePassword.
	AlbumPasswords map[string]string
}

// LoadEncryptConfig reads a passwords file and returns an EncryptConfig.
//
// Format (blank lines and lines starting with # are ignored):
//
//	_key_:hmac-secret
//	_all_:site-password
//	album-slug:album-password
func LoadEncryptConfig(path string) (*EncryptConfig, error) {
	ec := &EncryptConfig{AlbumPasswords: map[string]string{}}
	err := scanLines(path, func(line string) {
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			return
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "_key_":
			ec.HMACKey = val
		case "_all_":
			ec.SitePassword = val
		default:
			ec.AlbumPasswords[key] = val
		}
	})
	if err != nil {
		return nil, fmt.Errorf("load encrypt config %s: %w", path, err)
	}
	return ec, nil
}

const minPasswordLen = 5

// Validate checks that the EncryptConfig is consistent:
//   - _key_ is required whenever any album is encrypted (UUID filenames depend on it)
//   - passwords must be at least minPasswordLen characters
//   - empty per-album password values are rejected (they silently override _all_ with no encryption)
func (ec *EncryptConfig) Validate() error {
	if ec.HMACKey == "" && (ec.IsSiteEncrypted() || len(ec.AlbumPasswords) > 0) {
		return fmt.Errorf("passwords file: _key_ is required when any album is encrypted")
	}
	if ec.SitePassword != "" && len(ec.SitePassword) < minPasswordLen {
		return fmt.Errorf("passwords file: _all_ password must be at least %d characters", minPasswordLen)
	}
	for slug, p := range ec.AlbumPasswords {
		if len(p) < minPasswordLen {
			return fmt.Errorf("passwords file: password for album %q must be at least %d characters", slug, minPasswordLen)
		}
	}
	return nil
}

// AlbumPassword returns the effective password for the given album slug.
// Returns the per-album password if set, otherwise falls back to SitePassword.
// Returns "" if neither is configured (album is not encrypted).
func (ec *EncryptConfig) AlbumPassword(slug string) string {
	if p, ok := ec.AlbumPasswords[slug]; ok {
		return p
	}
	return ec.SitePassword
}

// IsAlbumEncrypted reports whether the given album has a password.
func (ec *EncryptConfig) IsAlbumEncrypted(slug string) bool {
	return ec.AlbumPassword(slug) != ""
}

// IsSiteEncrypted reports whether albums.json is encrypted (i.e., _all_ is set).
func (ec *EncryptConfig) IsSiteEncrypted() bool {
	return ec.SitePassword != ""
}

// PhotoWebPName returns the output WebP filename for a source photo.
// If HMACKey is set, returns a deterministic UUID-format name derived via
// HMAC-SHA256 so that original filenames (e.g. IMG_3961.jpg) cannot be guessed.
// If HMACKey is empty, falls back to the standard WebP filename.
func (ec *EncryptConfig) PhotoWebPName(filename string) string {
	if ec == nil || ec.HMACKey == "" {
		return WebPFileName(filename)
	}
	mac := hmac.New(sha256.New, []byte(ec.HMACKey))
	mac.Write([]byte(strings.ToLower(filename)))
	sum := mac.Sum(nil)
	return fmt.Sprintf("%x-%x-%x-%x-%x.webp",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// encryptedPayload is the on-disk JSON format for encrypted files.
type encryptedPayload struct {
	Salt string `json:"salt"`
	IV   string `json:"iv"`
	Data string `json:"data"`
}

const pbkdf2Iterations = 100_000
const aesKeyLen = 32 // AES-256

// EncryptJSON encrypts plaintext JSON data with the given password using
// PBKDF2 (SHA-256, 100k iterations) for key derivation and AES-256-GCM for
// encryption. Returns a JSON blob containing the salt, IV, and ciphertext,
// all base64-encoded. The format is compatible with the Web Crypto API.
func EncryptJSON(data []byte, password string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, aesKeyLen, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	payload := encryptedPayload{
		Salt: base64.StdEncoding.EncodeToString(salt),
		IV:   base64.StdEncoding.EncodeToString(nonce),
		Data: base64.StdEncoding.EncodeToString(ciphertext),
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal encrypted payload: %w", err)
	}
	return out, nil
}
