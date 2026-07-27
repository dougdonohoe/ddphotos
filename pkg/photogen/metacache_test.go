package photogen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realFixture returns the path to a testdata image with known metadata.
const realFixture = "landscape-1.jpg"

// copyFixture copies a testdata image into dir so tests can freely modify it.
func copyFixture(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func TestMetaCache_Metadata(t *testing.T) {
	t.Run("a hit returns the same values as a direct read", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		want, err := ReadPhotoMetadata(src)
		require.NoError(t, err)

		mc := NewMetaCache(filepath.Join(dir, MetaCacheFileName))

		first, err := mc.Metadata(src)
		require.NoError(t, err)
		assert.Equal(t, want, first, "miss must match a direct read")

		second, err := mc.Metadata(src)
		require.NoError(t, err)
		assert.Equal(t, want, second, "hit must match a direct read")
		assert.Equal(t, 1, mc.Len())
	})

	t.Run("the returned pointer does not alias cache state", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		mc := NewMetaCache(filepath.Join(dir, MetaCacheFileName))

		first, err := mc.Metadata(src)
		require.NoError(t, err)
		first.Width = -1

		second, err := mc.Metadata(src)
		require.NoError(t, err)
		assert.NotEqual(t, -1, second.Width, "mutating a result must not corrupt the cache")
	})

	t.Run("a changed modification time invalidates the entry", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		mc := NewMetaCache(filepath.Join(dir, MetaCacheFileName))

		_, err := mc.Metadata(src)
		require.NoError(t, err)

		// Replace the file contents with a different image but keep the same path.
		other, err := os.ReadFile(filepath.Join("testdata", "portrait-1.jpg"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(src, other, 0644))
		require.NoError(t, os.Chtimes(src, time.Now(), time.Now().Add(time.Second)))

		meta, err := mc.Metadata(src)
		require.NoError(t, err)
		assert.Equal(t, "portrait", meta.Orientation, "stale entry must not be served")
	})

	t.Run("a changed size invalidates the entry even at the same mtime", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		mc := NewMetaCache(filepath.Join(dir, MetaCacheFileName))

		_, err := mc.Metadata(src)
		require.NoError(t, err)
		stat, err := os.Stat(src)
		require.NoError(t, err)

		other, err := os.ReadFile(filepath.Join("testdata", "portrait-1.jpg"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(src, other, 0644))
		// Pin the modification time back so only the size differs.
		require.NoError(t, os.Chtimes(src, stat.ModTime(), stat.ModTime()))

		meta, err := mc.Metadata(src)
		require.NoError(t, err)
		assert.Equal(t, "portrait", meta.Orientation, "size change must invalidate")
	})

	t.Run("refresh mode ignores existing entries but still records them", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		mc := NewMetaCache(filepath.Join(dir, MetaCacheFileName))

		_, err := mc.Metadata(src)
		require.NoError(t, err)

		// Corrupt the cached value; refresh must re-read rather than serve it.
		mc.entries[src] = metaCacheEntry{
			ModTimeNano: mc.entries[src].ModTimeNano,
			Size:        mc.entries[src].Size,
			Meta:        &PhotoMetadata{Width: 1, Height: 1, Orientation: "square"},
		}
		mc.SetRefresh(true)

		meta, err := mc.Metadata(src)
		require.NoError(t, err)
		assert.Equal(t, 5028, meta.Width, "refresh must re-decode")
		assert.Equal(t, "landscape", mc.entries[src].Meta.Orientation, "refresh must rewrite the entry")
	})

	t.Run("a nil cache reads directly", func(t *testing.T) {
		var mc *MetaCache
		meta, err := mc.Metadata(filepath.Join("testdata", realFixture))
		require.NoError(t, err)
		assert.Equal(t, 5028, meta.Width)
		assert.Equal(t, 0, mc.Len())
		assert.NoError(t, mc.Save())
	})

	t.Run("an unreadable file reports the underlying error", func(t *testing.T) {
		mc := NewMetaCache(filepath.Join(t.TempDir(), MetaCacheFileName))
		_, err := mc.Metadata("/nonexistent/path/photo.jpg")
		assert.Error(t, err)
		assert.Equal(t, 0, mc.Len(), "a failed read must not be cached")
	})
}

func TestMetaCache_LoadAndSave(t *testing.T) {
	t.Run("saved entries survive a round trip", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		path := filepath.Join(dir, ".build", MetaCacheFileName)

		mc := NewMetaCache(path)
		want, err := mc.Metadata(src)
		require.NoError(t, err)
		require.NoError(t, mc.Save())

		reloaded := LoadMetaCache(path)
		assert.Equal(t, 1, reloaded.Len())
		got, err := reloaded.Metadata(src)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a missing file loads as an empty cache", func(t *testing.T) {
		mc := LoadMetaCache(filepath.Join(t.TempDir(), "does-not-exist.json"))
		assert.Equal(t, 0, mc.Len())
	})

	t.Run("a malformed file loads as an empty cache", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), MetaCacheFileName)
		require.NoError(t, os.WriteFile(path, []byte("{not json"), 0644))
		mc := LoadMetaCache(path)
		assert.Equal(t, 0, mc.Len())
	})

	t.Run("a wrong version loads as an empty cache", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), MetaCacheFileName)
		body, err := json.Marshal(metaCacheFile{
			Version: metaCacheVersion + 1,
			Entries: map[string]metaCacheEntry{"/some/photo.jpg": {Meta: &PhotoMetadata{Width: 9}}},
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, body, 0644))

		mc := LoadMetaCache(path)
		assert.Equal(t, 0, mc.Len())
	})

	t.Run("save is a no-op when nothing changed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), MetaCacheFileName)
		mc := NewMetaCache(path)
		require.NoError(t, mc.Save())
		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err), "clean cache must not write a file")
	})

	t.Run("save drops entries whose source file is gone", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		path := filepath.Join(dir, MetaCacheFileName)

		mc := NewMetaCache(path)
		_, err := mc.Metadata(src)
		require.NoError(t, err)
		require.NoError(t, mc.Save())
		require.Equal(t, 1, LoadMetaCache(path).Len())

		require.NoError(t, os.Remove(src))
		// Touch something so the cache is dirty and actually rewrites.
		mc.RecordDerived(path, path, "")
		require.NoError(t, mc.Save())

		assert.Equal(t, 0, LoadMetaCache(path).Len(), "pruned entry must not persist")
	})

	t.Run("save leaves no temp files behind", func(t *testing.T) {
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		path := filepath.Join(dir, MetaCacheFileName)

		mc := NewMetaCache(path)
		_, err := mc.Metadata(src)
		require.NoError(t, err)
		require.NoError(t, mc.Save())

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, e := range entries {
			assert.NotContains(t, e.Name(), ".tmp", "temp file left behind: %s", e.Name())
		}
	})
}

// TestFixedNameOutputs_Regeneration covers cover.jpg and hero.jpg, which have fixed
// output names and so used to be regenerated unconditionally on every run. They are now
// skipped when the cache says the source is unchanged, and this is what keeps that skip
// from going stale.
func TestFixedNameOutputs_Regeneration(t *testing.T) {
	// modTime returns the modification time of path, used to detect a rewrite.
	modTime := func(t *testing.T, path string) time.Time {
		t.Helper()
		stat, err := os.Stat(path)
		require.NoError(t, err)
		return stat.ModTime()
	}

	// age backdates a file so a rewrite is detectable regardless of clock resolution.
	age := func(t *testing.T, path string) {
		t.Helper()
		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(path, old, old))
	}

	t.Run("cover.jpg", func(t *testing.T) {
		newAP := func(t *testing.T) (*AlbumProcessor, string) {
			t.Helper()
			dir := t.TempDir()
			src := copyFixture(t, dir, realFixture)
			ap := newTestProcessor(t, []*Photo{{FileName: realFixture, AbsolutePath: src}})
			ap.Config.MetaCache = NewMetaCache(filepath.Join(dir, MetaCacheFileName))
			return ap, src
		}

		t.Run("is skipped when the source is unchanged", func(t *testing.T) {
			ap, _ := newAP(t)
			require.NoError(t, ap.WriteCoverJPEG())
			out := ap.OutputPath("cover.jpg")
			age(t, out)
			before := modTime(t, out)

			require.NoError(t, ap.WriteCoverJPEG())
			assert.Equal(t, before, modTime(t, out), "unchanged source must not be re-encoded")
		})

		t.Run("is regenerated when the cover points at a different photo", func(t *testing.T) {
			ap, src := newAP(t)
			require.NoError(t, ap.WriteCoverJPEG())
			out := ap.OutputPath("cover.jpg")
			age(t, out)
			before := modTime(t, out)

			other := copyFixture(t, filepath.Dir(src), "portrait-1.jpg")
			ap.Photos = []*Photo{{FileName: "portrait-1.jpg", AbsolutePath: other}}

			require.NoError(t, ap.WriteCoverJPEG())
			assert.NotEqual(t, before, modTime(t, out), "a new cover source must be re-encoded")
		})

		t.Run("is regenerated when the output is deleted", func(t *testing.T) {
			ap, _ := newAP(t)
			require.NoError(t, ap.WriteCoverJPEG())
			out := ap.OutputPath("cover.jpg")
			require.NoError(t, os.Remove(out))

			require.NoError(t, ap.WriteCoverJPEG())
			assert.FileExists(t, out)
		})

		t.Run("is regenerated under -force", func(t *testing.T) {
			ap, _ := newAP(t)
			require.NoError(t, ap.WriteCoverJPEG())
			out := ap.OutputPath("cover.jpg")
			age(t, out)
			before := modTime(t, out)

			ap.Config.Force = true
			require.NoError(t, ap.WriteCoverJPEG())
			assert.NotEqual(t, before, modTime(t, out))
		})

		t.Run("without a cache it always regenerates", func(t *testing.T) {
			ap, _ := newAP(t)
			ap.Config.MetaCache = nil
			require.NoError(t, ap.WriteCoverJPEG())
			out := ap.OutputPath("cover.jpg")
			age(t, out)
			before := modTime(t, out)

			require.NoError(t, ap.WriteCoverJPEG())
			assert.NotEqual(t, before, modTime(t, out), "no cache must preserve the old behaviour")
		})

		t.Run("dry-run records nothing", func(t *testing.T) {
			ap, _ := newAP(t)
			ap.Config.DryRun = true
			require.NoError(t, ap.WriteCoverJPEG())
			assert.NoFileExists(t, ap.OutputPath("cover.jpg"))
			assert.False(t, ap.Config.MetaCache.DerivedUpToDate(
				ap.OutputPath("cover.jpg"), ap.Photos[0].AbsolutePath, ""),
				"a dry run must not stamp an output it never wrote")
		})
	})

	t.Run("hero.jpg", func(t *testing.T) {
		newCfg := func(t *testing.T, crop string) (*Config, string) {
			t.Helper()
			dir := t.TempDir()
			src := copyFixture(t, dir, realFixture)
			cfg := &Config{
				OutputRoot: dir,
				SiteID:     "test",
				Warn:       &WarnCollector{},
				Hero:       &HeroConfig{ImagePath: src, Crop: crop},
				MetaCache:  NewMetaCache(filepath.Join(dir, MetaCacheFileName)),
			}
			return cfg, src
		}

		t.Run("is skipped when the source and crop are unchanged", func(t *testing.T) {
			cfg, _ := newCfg(t, "center")
			require.NoError(t, cfg.WriteHeroJPEG())
			out := cfg.SiteOutputPath("hero.jpg")
			age(t, out)
			before := modTime(t, out)

			require.NoError(t, cfg.WriteHeroJPEG())
			assert.Equal(t, before, modTime(t, out))
		})

		t.Run("is regenerated when the crop changes", func(t *testing.T) {
			cfg, _ := newCfg(t, "center")
			require.NoError(t, cfg.WriteHeroJPEG())
			out := cfg.SiteOutputPath("hero.jpg")
			age(t, out)
			before := modTime(t, out)

			cfg.Hero.Crop = "top"
			require.NoError(t, cfg.WriteHeroJPEG())
			assert.NotEqual(t, before, modTime(t, out), "a crop change must be re-encoded")
		})

		t.Run("is regenerated when the hero image changes", func(t *testing.T) {
			cfg, src := newCfg(t, "center")
			require.NoError(t, cfg.WriteHeroJPEG())
			out := cfg.SiteOutputPath("hero.jpg")
			age(t, out)
			before := modTime(t, out)

			cfg.Hero.ImagePath = copyFixture(t, filepath.Dir(src), "portrait-1.jpg")
			require.NoError(t, cfg.WriteHeroJPEG())
			assert.NotEqual(t, before, modTime(t, out))
		})
	})
}

func TestMetaCache_Derived(t *testing.T) {
	// setup returns a cache plus an existing source and output file.
	setup := func(t *testing.T) (*MetaCache, string, string) {
		t.Helper()
		dir := t.TempDir()
		src := copyFixture(t, dir, realFixture)
		out := filepath.Join(dir, "cover.jpg")
		require.NoError(t, os.WriteFile(out, []byte("generated"), 0644))
		return NewMetaCache(filepath.Join(dir, MetaCacheFileName)), src, out
	}

	t.Run("an unrecorded output is not up to date", func(t *testing.T) {
		mc, src, out := setup(t)
		assert.False(t, mc.DerivedUpToDate(out, src, ""))
	})

	t.Run("a recorded output is up to date", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "")
		assert.True(t, mc.DerivedUpToDate(out, src, ""))
	})

	t.Run("a different source invalidates", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "")
		other := copyFixture(t, filepath.Dir(src), "portrait-1.jpg")
		assert.False(t, mc.DerivedUpToDate(out, other, ""),
			"pointing the cover at a different photo must regenerate")
	})

	t.Run("a different variant invalidates", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "center")
		assert.False(t, mc.DerivedUpToDate(out, src, "top"),
			"changing the hero crop must regenerate")
	})

	t.Run("a modified source invalidates", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "")
		require.NoError(t, os.Chtimes(src, time.Now(), time.Now().Add(time.Hour)))
		assert.False(t, mc.DerivedUpToDate(out, src, ""))
	})

	t.Run("a deleted output invalidates", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "")
		require.NoError(t, os.Remove(out))
		assert.False(t, mc.DerivedUpToDate(out, src, ""))
	})

	t.Run("refresh mode invalidates", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "")
		mc.SetRefresh(true)
		assert.False(t, mc.DerivedUpToDate(out, src, ""))
	})

	t.Run("a nil cache always regenerates", func(t *testing.T) {
		var mc *MetaCache
		mc.RecordDerived("out.jpg", "src.jpg", "")
		assert.False(t, mc.DerivedUpToDate("out.jpg", "src.jpg", ""))
	})

	t.Run("derived stamps survive a round trip", func(t *testing.T) {
		mc, src, out := setup(t)
		mc.RecordDerived(out, src, "top")
		require.NoError(t, mc.Save())

		reloaded := LoadMetaCache(mc.path)
		assert.True(t, reloaded.DerivedUpToDate(out, src, "top"))
		assert.False(t, reloaded.DerivedUpToDate(out, src, "center"))
	})
}
