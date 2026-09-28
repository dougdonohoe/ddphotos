package photogen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSidecar(t *testing.T) {
	t.Parallel()

	sidecarDate := time.Date(2007, 10, 17, 10, 24, 19, 0, time.UTC)
	writeRaw := func(t *testing.T, path, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	t.Run("the name is the media file's full name plus the suffix", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "dir/IMG_1.png.photogen.json", sidecarPath("dir/IMG_1.png"))
	})

	t.Run("a sidecar date overrides the EXIF date", func(t *testing.T) {
		t.Parallel()
		src := copyFixture(t, t.TempDir(), realFixture)
		ap := &AlbumProcessor{}
		exif, err := ap.readMetadata(src)
		require.NoError(t, err)
		require.False(t, exif.DateTaken.IsZero(), "the fixture has an EXIF date to override")

		_, err = writeSidecar(src, Sidecar{DateTaken: &sidecarDate})
		require.NoError(t, err)
		meta, err := ap.readMetadata(src)
		require.NoError(t, err)
		assert.Equal(t, sidecarDate, meta.DateTaken)
		assert.True(t, meta.DateFromSidecar)
		assert.Equal(t, exif.Width, meta.Width, "only the date is overridden")
		assert.Contains(t, (&Photo{FileName: "a.jpg", SourcePath: "a.jpg", PhotoMetadata: meta}).String(),
			"2007-10-17 10:24 via sidecar")
	})

	t.Run("a file with no EXIF date gets one from its sidecar", func(t *testing.T) {
		t.Parallel()
		src := copyFixture(t, t.TempDir(), "no-exif.jpg")
		writeRaw(t, sidecarPath(src), `{"dateTaken": "2007-10-17T10:24:19Z"}`)
		meta, err := (&AlbumProcessor{}).readMetadata(src)
		require.NoError(t, err)
		assert.Equal(t, sidecarDate, meta.DateTaken)
	})

	// The cache keys on the photo's mtime and size, so a sidecar has to be read outside it
	// or a sidecar edited on its own would be ignored.
	t.Run("a sidecar change applies through a warm metadata cache", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		ap := &AlbumProcessor{Config: &Config{MetaCache: NewMetaCache(filepath.Join(dir, MetaCacheFileName))}}
		exif, err := ap.readMetadata(src)
		require.NoError(t, err)

		writeRaw(t, sidecarPath(src), `{"dateTaken": "2007-10-17T10:24:19Z"}`)
		meta, err := ap.readMetadata(src)
		require.NoError(t, err)
		assert.Equal(t, sidecarDate, meta.DateTaken)

		require.NoError(t, os.Remove(sidecarPath(src)))
		meta, err = ap.readMetadata(src)
		require.NoError(t, err)
		assert.Equal(t, exif.DateTaken, meta.DateTaken, "the override never reached the cache")
		assert.False(t, meta.DateFromSidecar)
	})

	t.Run("a malformed sidecar is an error naming it", func(t *testing.T) {
		t.Parallel()
		for name, body := range map[string]string{
			"bad json":      `{"dateTaken": `,
			"bad date":      `{"dateTaken": "yesterday"}`,
			"unknown field": `{"dateTakn": "2007-10-17T10:24:19Z"}`,
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				src := copyFixture(t, t.TempDir(), realFixture)
				writeRaw(t, sidecarPath(src), body)
				_, err := (&AlbumProcessor{}).readMetadata(src)
				require.Error(t, err)
				assert.Contains(t, err.Error(), sidecarPath(src))
			})
		}
	})

	// Older Google Takeout exports put a differently shaped IMG_1.jpg.json beside every
	// photo; the .photogen infix is what keeps those out.
	t.Run("a Takeout-style json beside the photo is ignored", func(t *testing.T) {
		t.Parallel()
		src := copyFixture(t, t.TempDir(), realFixture)
		writeRaw(t, src+".json", `{"photoTakenTime": {"timestamp": "1"}}`)
		_, err := (&AlbumProcessor{}).readMetadata(src)
		require.NoError(t, err)
	})

	t.Run("an identical sidecar is not rewritten", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "a.jpg")
		wrote, err := writeSidecar(src, Sidecar{DateTaken: &sidecarDate})
		require.NoError(t, err)
		assert.True(t, wrote)
		wrote, err = writeSidecar(src, Sidecar{DateTaken: &sidecarDate})
		require.NoError(t, err)
		assert.False(t, wrote)
	})
}
