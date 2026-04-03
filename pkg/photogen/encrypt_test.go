package photogen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEncryptConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "passwords.yaml")

	content := `# WARNING: demo only
key: my-hmac-key
site:
  password: site-pass
  hint: Remember the site
albums:
  uganda:
    password: uganda-pass
    hint: Think of gorillas
  antarctica:
    password: antar-pass
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ec, err := LoadEncryptConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "my-hmac-key", ec.HMACKey)
	assert.Equal(t, "site-pass", ec.SitePassword)
	assert.Equal(t, "Remember the site", ec.SiteHint)
	assert.Equal(t, "uganda-pass", ec.AlbumPasswords["uganda"])
	assert.Equal(t, "Think of gorillas", ec.AlbumHints["uganda"])
	assert.Equal(t, "antar-pass", ec.AlbumPasswords["antarctica"])
	assert.Empty(t, ec.AlbumHints["antarctica"])
}

func TestLoadEncryptConfig_KeyOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "passwords.yaml")
	require.NoError(t, os.WriteFile(path, []byte("key: secret\n"), 0644))

	ec, err := LoadEncryptConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "secret", ec.HMACKey)
	assert.False(t, ec.IsSiteEncrypted())
}

func TestEncryptConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("key present with site password is valid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key", SitePassword: "passw0rd"}
		assert.NoError(t, ec.Validate())
	})

	t.Run("key present with per-album password is valid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key", AlbumPasswords: map[string]string{"uganda": "passw0rd"}}
		assert.NoError(t, ec.Validate())
	})

	t.Run("key only (no passwords) is valid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key"}
		assert.NoError(t, ec.Validate())
	})

	t.Run("site password without key is invalid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{SitePassword: "pass"}
		assert.ErrorContains(t, ec.Validate(), "key")
	})

	t.Run("per-album password without key is invalid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{AlbumPasswords: map[string]string{"uganda": "pass"}}
		assert.ErrorContains(t, ec.Validate(), "key")
	})

	t.Run("site password too short is invalid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key", SitePassword: "ab"}
		assert.ErrorContains(t, ec.Validate(), "site")
	})

	t.Run("site password exactly minimum length is valid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key", SitePassword: "abcde"}
		assert.NoError(t, ec.Validate())
	})

	t.Run("per-album password too short is invalid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key", AlbumPasswords: map[string]string{"uganda": "ab"}}
		assert.ErrorContains(t, ec.Validate(), "uganda")
	})

	t.Run("empty per-album password is invalid", func(t *testing.T) {
		t.Parallel()
		// Empty value (e.g. "uganda:" in passwords file) silently overrides site.password with no
		// encryption for that album — reject it explicitly.
		ec := &EncryptConfig{HMACKey: "key", SitePassword: "globalpass", AlbumPasswords: map[string]string{"uganda": ""}}
		assert.ErrorContains(t, ec.Validate(), "uganda")
	})

	t.Run("per-album password exactly minimum length is valid", func(t *testing.T) {
		t.Parallel()
		ec := &EncryptConfig{HMACKey: "key", AlbumPasswords: map[string]string{"uganda": "abcde"}}
		assert.NoError(t, ec.Validate())
	})
}

func TestAlbumPassword(t *testing.T) {
	t.Parallel()

	ec := &EncryptConfig{
		SitePassword:   "site-pass",
		AlbumPasswords: map[string]string{"uganda": "uganda-pass"},
	}
	assert.Equal(t, "uganda-pass", ec.AlbumPassword("uganda"))
	assert.Equal(t, "site-pass", ec.AlbumPassword("antarctica"))
	assert.Equal(t, "site-pass", ec.AlbumPassword("unknown"))
	assert.True(t, ec.IsAlbumEncrypted("uganda"))
	assert.True(t, ec.IsAlbumEncrypted("antarctica"))
}

func TestPhotoWebPName_WithKey(t *testing.T) {
	t.Parallel()

	ec := &EncryptConfig{HMACKey: "test-key"}

	name1 := ec.PhotoWebPName("IMG_3961.jpg")
	name2 := ec.PhotoWebPName("IMG_3961.jpg")
	name3 := ec.PhotoWebPName("IMG_3962.jpg")

	// Deterministic
	assert.Equal(t, name1, name2)
	// Different inputs → different outputs
	assert.NotEqual(t, name1, name3)
	// UUID format with .webp extension
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.webp$`, name1)
}

func TestPhotoWebPName_WithoutKey(t *testing.T) {
	t.Parallel()

	ec := &EncryptConfig{}
	assert.Equal(t, "IMG_3961.webp", ec.PhotoWebPName("IMG_3961.jpg"))

	var nilEc *EncryptConfig
	assert.Equal(t, "photo.webp", nilEc.PhotoWebPName("photo.jpg"))
}

func TestPhotoWebPName_DifferentKeys(t *testing.T) {
	t.Parallel()

	ec1 := &EncryptConfig{HMACKey: "key1"}
	ec2 := &EncryptConfig{HMACKey: "key2"}
	assert.NotEqual(t, ec1.PhotoWebPName("photo.jpg"), ec2.PhotoWebPName("photo.jpg"))
}

func TestEncryptJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	original := []byte(`{"slug":"test","title":"Test Album"}`)
	password := "test-password"

	encrypted, err := EncryptJSON(original, password, "")
	require.NoError(t, err)

	// Verify it's valid JSON with expected fields
	var payload encryptedPayload
	require.NoError(t, json.Unmarshal(encrypted, &payload))
	assert.NotEmpty(t, payload.Salt)
	assert.NotEmpty(t, payload.IV)
	assert.NotEmpty(t, payload.Data)

	// Verify it's not the original plaintext
	assert.NotContains(t, string(encrypted), "Test Album")
}

func TestDecryptJSON(t *testing.T) {
	t.Parallel()

	original := []byte(`{"slug":"test","title":"Test Album"}`)

	t.Run("round-trip produces original plaintext", func(t *testing.T) {
		t.Parallel()
		encrypted, err := EncryptJSON(original, "correct-password", "")
		require.NoError(t, err)

		plaintext, err := DecryptJSON(encrypted, "correct-password")
		require.NoError(t, err)
		assert.Equal(t, original, plaintext)
	})

	t.Run("wrong password returns error", func(t *testing.T) {
		t.Parallel()
		encrypted, err := EncryptJSON(original, "correct-password", "")
		require.NoError(t, err)

		_, err = DecryptJSON(encrypted, "wrong-password")
		assert.Error(t, err)
	})
}

func TestReadPwFile(t *testing.T) {
	t.Parallel()

	original := []byte(`{"slug":"test"}`)

	t.Run("returns stored pwFile path", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.enc.json")

		encrypted, err := EncryptJSON(original, "passw0rd", "sample/config/passwords.yaml")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, encrypted, 0o644))

		got, err := ReadPwFile(path)
		require.NoError(t, err)
		assert.Equal(t, "sample/config/passwords.yaml", got)
	})

	t.Run("returns empty string when no pwFile stored", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.enc.json")

		encrypted, err := EncryptJSON(original, "passw0rd", "")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, encrypted, 0o644))

		got, err := ReadPwFile(path)
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})
}

func TestEncryptJSON_NonDeterministic(t *testing.T) {
	t.Parallel()

	data := []byte(`{"test":true}`)
	enc1, err := EncryptJSON(data, "password", "")
	require.NoError(t, err)
	enc2, err := EncryptJSON(data, "password", "")
	require.NoError(t, err)
	// Each encryption uses a random salt+nonce, so outputs differ
	assert.NotEqual(t, string(enc1), string(enc2))
}
