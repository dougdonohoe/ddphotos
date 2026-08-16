package photogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireVideoTools skips the test when ffmpeg is unavailable, so `make test` still passes
// on a machine that has never needed video. CI installs ffmpeg so these actually run.
func requireVideoTools(t *testing.T) {
	t.Helper()
	if _, err := ensureVideoTools(); err != nil {
		t.Skipf("skipping: %v", err)
	}
}

func TestIsMediaFile(t *testing.T) {
	tests := []struct {
		name         string
		photo, video bool
	}{
		{"IMG_1234.jpg", true, false},
		{"IMG_1234.JPG", true, false},
		{"photo.heic", true, false},
		{"photo.webp", true, false},
		{"clip.mov", false, true},
		{"clip.MOV", false, true},
		{"clip.mp4", false, true},
		{"clip.m4v", false, true},
		{"notes.txt", false, false},
		{"photogen.txt", false, false},
		{"Ski 2007", false, false},
		{"no-extension", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.photo, IsPhotoFile(tc.name), "IsPhotoFile")
			assert.Equal(t, tc.video, IsVideoFile(tc.name), "IsVideoFile")
			assert.Equal(t, tc.photo || tc.video, IsMediaFile(tc.name), "IsMediaFile")
		})
	}
}

// TestHeroRejectsVideo pins the deliberate asymmetry: the scan accepts video, but a hero
// image does not. A hero is a hard-cropped still produced by libvips, which cannot open a
// video container at all.
func TestHeroRejectsVideo(t *testing.T) {
	af := &AlbumsFile{
		Settings: AlbumsSettings{
			ID: "test", SiteName: "n", SiteDescription: "d",
			CopyrightOwner: "o", CopyrightYear: 2026,
			Hero: &HeroEntry{Image: "clip.mov"},
		},
		Albums: []AlbumEntry{{Slug: "a", Name: "A", Source: "/tmp"}},
	}
	err := af.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported extension")
	assert.True(t, IsMediaFile("clip.mov"), "sanity: the scan does accept this file")
}

func TestReadVideoMetadata(t *testing.T) {
	requireVideoTools(t)

	tests := []struct {
		name       string
		filename   string
		wantWidth  int
		wantHeight int
		wantOrient string
		wantDate   string // empty means zero time expected
	}{
		{
			name:       "landscape hevc with creation time",
			filename:   "landscape.mov",
			wantWidth:  320,
			wantHeight: 240,
			wantOrient: "landscape",
			wantDate:   "2019-12-29",
		},
		{
			// The whole point of this fixture: ffprobe reports the stored 320x240, but a
			// 90-degree display matrix means it is really shown as 240x320. Reporting the
			// raw numbers lays the clip out as a landscape box in the justified grid.
			name:       "rotated portrait reports display dimensions",
			filename:   "portrait-rotated.mov",
			wantWidth:  240,
			wantHeight: 320,
			wantOrient: "portrait",
			wantDate:   "",
		},
		{
			name:       "no creation time yields zero date",
			filename:   "no-date.mp4",
			wantWidth:  320,
			wantHeight: 240,
			wantOrient: "landscape",
			wantDate:   "",
		},
		{
			name:       "silent video still reads",
			filename:   "silent.mp4",
			wantWidth:  320,
			wantHeight: 240,
			wantOrient: "landscape",
			wantDate:   "2020-06-15",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := ReadVideoMetadata(filepath.Join("testdata", tc.filename))
			require.NoError(t, err)

			assert.Equal(t, tc.wantWidth, meta.Width)
			assert.Equal(t, tc.wantHeight, meta.Height)
			assert.Equal(t, tc.wantOrient, meta.Orientation)
			assert.Greater(t, meta.Duration, 0.0, "duration should be populated")

			if tc.wantDate == "" {
				assert.True(t, meta.DateTaken.IsZero(), "expected zero date, got %s", meta.DateTaken)
			} else {
				require.False(t, meta.DateTaken.IsZero(), "expected a date")
				assert.Equal(t, tc.wantDate, meta.DateTaken.Format("2006-01-02"))
			}
		})
	}
}

// TestReadMediaMetadataDispatch confirms a video never reaches libvips, which would
// otherwise fail with an opaque load error.
func TestReadMediaMetadataDispatch(t *testing.T) {
	requireVideoTools(t)

	video, err := ReadMediaMetadata(filepath.Join("testdata", "landscape.mov"))
	require.NoError(t, err)
	assert.Equal(t, 320, video.Width)
	assert.Greater(t, video.Duration, 0.0)

	photo, err := ReadMediaMetadata(filepath.Join("testdata", "no-exif.jpg"))
	require.NoError(t, err)
	assert.Zero(t, photo.Duration, "stills carry no duration")
}

func TestTranscodeVideo(t *testing.T) {
	requireVideoTools(t)

	t.Run("produces a playable H.264 mp4", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out.mp4")
		res, err := TranscodeVideo(filepath.Join("testdata", "landscape.mov"), out, false, false)
		require.NoError(t, err)
		assert.True(t, res.Written)

		meta, err := ReadVideoMetadata(out)
		require.NoError(t, err)
		assert.Equal(t, 320, meta.Width)
		assert.Equal(t, 240, meta.Height)
	})

	t.Run("rotated source transcodes upright", func(t *testing.T) {
		// The scale filter sees the frame after the display matrix is applied, so this
		// comes out portrait with no rotation handling in our code. Pinned because a
		// well-meaning "fix" that rotates explicitly would double-rotate it.
		out := filepath.Join(t.TempDir(), "out.mp4")
		_, err := TranscodeVideo(filepath.Join("testdata", "portrait-rotated.mov"), out, false, false)
		require.NoError(t, err)

		meta, err := ReadVideoMetadata(out)
		require.NoError(t, err)
		assert.Equal(t, 240, meta.Width, "should be portrait")
		assert.Equal(t, 320, meta.Height)
		assert.Equal(t, "portrait", meta.Orientation)
	})

	t.Run("silent source does not fail the optional audio mapping", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out.mp4")
		_, err := TranscodeVideo(filepath.Join("testdata", "silent.mp4"), out, false, false)
		require.NoError(t, err)
		assert.FileExists(t, out)
	})

	t.Run("downscales to the long-edge maximum", func(t *testing.T) {
		// The fixtures are deliberately tiny, so build a big one to prove the cap works
		// and that the aspect ratio survives it.
		src := filepath.Join(t.TempDir(), "big.mp4")
		tools, err := ensureVideoTools()
		require.NoError(t, err)
		require.NoError(t, runCommand(tools.ffmpeg, "-v", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=10:duration=1",
			"-c:v", "libx264", "-preset", "ultrafast", src))

		out := filepath.Join(t.TempDir(), "out.mp4")
		_, err = TranscodeVideo(src, out, false, false)
		require.NoError(t, err)

		meta, err := ReadVideoMetadata(out)
		require.NoError(t, err)
		assert.Equal(t, videoMaxDimension, meta.Width)
		assert.Equal(t, 720, meta.Height)
	})

	t.Run("skips an existing output unless forced", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out.mp4")
		_, err := TranscodeVideo(filepath.Join("testdata", "landscape.mov"), out, false, false)
		require.NoError(t, err)

		res, err := TranscodeVideo(filepath.Join("testdata", "landscape.mov"), out, false, false)
		require.NoError(t, err)
		assert.True(t, res.Skipped, "second run should skip")

		res, err = TranscodeVideo(filepath.Join("testdata", "landscape.mov"), out, true, false)
		require.NoError(t, err)
		assert.True(t, res.Written, "force should rewrite")
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out.mp4")
		res, err := TranscodeVideo(filepath.Join("testdata", "landscape.mov"), out, false, true)
		require.NoError(t, err)
		assert.True(t, res.DryRun)
		assert.NoFileExists(t, out)
	})

	t.Run("leaves no temp file behind on failure", func(t *testing.T) {
		dir := t.TempDir()
		out := filepath.Join(dir, "out.mp4")
		_, err := TranscodeVideo(filepath.Join("testdata", "does-not-exist.mov"), out, false, false)
		require.Error(t, err)

		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		assert.Empty(t, entries, "a failed transcode must not leave a partial .tmp.mp4 that the next run would accept")
	})
}

func TestExtractPoster(t *testing.T) {
	requireVideoTools(t)

	t.Run("poster from a rotated source is already upright", func(t *testing.T) {
		// ffmpeg applies the display matrix on decode, so no rotation belongs in
		// ExtractPoster. Pinned so nobody adds one.
		out := filepath.Join(t.TempDir(), "poster.jpg")
		require.NoError(t, ExtractPoster(filepath.Join("testdata", "portrait-rotated.mov"), out, 1.0))

		meta, err := ReadPhotoMetadata(out)
		require.NoError(t, err)
		assert.Equal(t, 240, meta.Width)
		assert.Equal(t, 320, meta.Height)
		assert.Equal(t, "portrait", meta.Orientation)
	})

	t.Run("poster is a real decodable image", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "poster.jpg")
		require.NoError(t, ExtractPoster(filepath.Join("testdata", "landscape.mov"), out, 1.0))

		meta, err := ReadPhotoMetadata(out)
		require.NoError(t, err)
		assert.Equal(t, 320, meta.Width)
		assert.Equal(t, 240, meta.Height)
	})
}

func TestPosterOffset(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		want     float64
	}{
		{"long clip uses the one second default", 30, 1.0},
		{"exactly two seconds uses the default", 2, 1.0},
		{"short clip uses its midpoint", 1.0, 0.5},
		{"very short clip stays inside the clip", 0.4, 0.2},
		{"unknown duration falls back to the first frame", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PosterOffset(tc.duration)
			assert.Equal(t, tc.want, got)
			if tc.duration > 0 {
				assert.Less(t, got, tc.duration, "offset must land inside the clip")
			}
		})
	}
}

func TestIsQuarterTurn(t *testing.T) {
	// Phone video commonly reports -90; upside-down footage reports 180, which does not
	// swap the axes.
	for _, deg := range []float64{90, -90, 270, -270, 450} {
		assert.True(t, isQuarterTurn(deg), "%v should swap width and height", deg)
	}
	for _, deg := range []float64{0, 180, -180, 360} {
		assert.False(t, isQuarterTurn(deg), "%v should not swap width and height", deg)
	}
}

func TestParseVideoCreationTime(t *testing.T) {
	t.Run("rfc3339 with fractional seconds", func(t *testing.T) {
		got := parseVideoCreationTime("2019-12-29T23:27:41.000000Z")
		assert.Equal(t, "2019-12-29 23:27:41", got.Format("2006-01-02 15:04:05"))
	})
	t.Run("offset is normalized to utc", func(t *testing.T) {
		got := parseVideoCreationTime("2019-12-29T18:27:41-05:00")
		assert.Equal(t, "2019-12-29 23:27:41", got.Format("2006-01-02 15:04:05"))
		assert.Equal(t, "UTC", got.Location().String())
	})
	for _, s := range []string{"", "not a date", "0000-00-00T00:00:00Z"} {
		t.Run("unparseable: "+s, func(t *testing.T) {
			assert.True(t, parseVideoCreationTime(s).IsZero())
		})
	}
}

func TestVideoOutputNaming(t *testing.T) {
	assert.Equal(t, "clip.mp4", VideoFileName("clip.mov"))
	assert.Equal(t, "clip.mp4", VideoFileName("clip.MOV"))
	assert.Equal(t, "clip.mp4", VideoFileName("clip.mp4"))

	t.Run("encrypted albums share one stem across outputs", func(t *testing.T) {
		// The MP4 and its poster must hash to the same unguessable stem, or the pair is
		// trivially correlated by anyone who can list the directory.
		ec := &EncryptConfig{HMACKey: "test-key"}
		video := ec.PhotoOutputName("clip.mov", ".mp4")
		poster := ec.PhotoWebPName("clip.mov")

		assert.Equal(t, ".mp4", filepath.Ext(video))
		assert.Equal(t, ".webp", filepath.Ext(poster))
		assert.NotContains(t, video, "clip")
		assert.Equal(t,
			video[:len(video)-len(".mp4")],
			poster[:len(poster)-len(".webp")],
			"video and poster must share a stem")
	})

	t.Run("unencrypted albums keep the original name", func(t *testing.T) {
		ec := &EncryptConfig{}
		assert.Equal(t, "clip.mp4", ec.PhotoOutputName("clip.mov", ".mp4"))
		assert.Equal(t, "clip.webp", ec.PhotoWebPName("clip.mov"))
	})
}

// TestCoverImageSource covers a bug found in end-to-end testing: an album whose first (or
// configured) item is a video fed the .mov straight into libvips, failing the entire run
// with "unsupported image format". The cover must come from the generated poster instead.
func TestCoverImageSource(t *testing.T) {
	ap := &AlbumProcessor{
		Config:      &Config{OutputRoot: "/out", SiteID: "site"},
		AlbumConfig: &AlbumConfig{Slug: "trip"},
	}

	still := &Photo{FileName: "photo.jpg", AbsolutePath: "/src/photo.jpg"}
	assert.Equal(t, "/src/photo.jpg", ap.coverImageSource(still),
		"a still is read from its original source")

	video := &Photo{FileName: "clip.mov", AbsolutePath: "/src/clip.mov", IsVideo: true}
	got := ap.coverImageSource(video)
	assert.Equal(t, filepath.Join("/out", "site", "trip", "full", "clip.webp"), got)
	assert.NotContains(t, got, ".mov", "libvips can never be handed the video container")
}

// TestReadMetadataWithoutConfig pins the uncached branch of readMetadata. An
// AlbumProcessor built without a Config (as several unit tests do) bypasses the metadata
// cache and reads the file directly, and that branch has to dispatch on media kind just
// like the cached one — libvips cannot open a video container, so a .mov sent to
// ReadPhotoMetadata fails with "unsupported image format".
func TestReadMetadataWithoutConfig(t *testing.T) {
	requireVideoTools(t)

	ap := &AlbumProcessor{AlbumConfig: &AlbumConfig{Slug: "trip"}}
	require.Nil(t, ap.Config, "the point of this test is the nil-Config path")

	meta, err := ap.readMetadata(filepath.Join("testdata", "landscape.mov"))
	require.NoError(t, err)
	assert.Positive(t, meta.Width)
	assert.Positive(t, meta.Height)
	assert.Positive(t, meta.Duration, "video metadata carries a duration")
}

func TestVideoOversizeWarning(t *testing.T) {
	dir := t.TempDir()

	small := filepath.Join(dir, "small.mp4")
	require.NoError(t, os.WriteFile(small, make([]byte, 1024), 0o644))
	assert.Empty(t, VideoOversizeWarning(small))

	big := filepath.Join(dir, "big.mp4")
	require.NoError(t, os.WriteFile(big, make([]byte, videoSizeWarnBytes+1), 0o644))
	warn := VideoOversizeWarning(big)
	assert.Contains(t, warn, "big.mp4")
	assert.Contains(t, warn, "Cloudflare")

	assert.Empty(t, VideoOversizeWarning(filepath.Join(dir, "missing.mp4")))
}
