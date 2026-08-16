package photogen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProcessor(t *testing.T, photos []*Photo) *AlbumProcessor {
	t.Helper()
	cfg := &Config{
		OutputRoot: t.TempDir(),
		SiteID:     "test",
		Warn:       &WarnCollector{},
	}
	ac := &AlbumConfig{Slug: "test-album", Name: "Test Album"}
	ap := NewAlbumProcessor(cfg, ac)
	ap.Photos = photos
	return ap
}

func TestResizePhotos_Success(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
	})

	err := ap.ResizePhotos()
	require.NoError(t, err)

	// Both size variants should have been written.
	for _, size := range AllSizes() {
		outPath := ap.OutputPath(string(size), WebPFileName("landscape-1.jpg"))
		_, statErr := os.Stat(outPath)
		assert.NoError(t, statErr, "expected output file for size %s", size)
	}
}

func TestResizePhotos_SkipExisting(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
	})

	// First run writes files.
	require.NoError(t, ap.ResizePhotos())

	// Second run should skip without error.
	err := ap.ResizePhotos()
	require.NoError(t, err)
}

// TestResizePhotos_SkipExistingWithoutReadingSource proves the skip happens before any
// work is dispatched: the source file is removed after the first run, so a second run
// that still tried to open it would fail.
func TestResizePhotos_SkipExistingWithoutReadingSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "landscape-1.jpg")
	data, err := os.ReadFile(filepath.Join("testdata", "landscape-1.jpg"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, data, 0644))

	ap := newTestProcessor(t, []*Photo{{FileName: "landscape-1.jpg", AbsolutePath: src}})
	require.NoError(t, ap.ResizePhotos())

	require.NoError(t, os.Remove(src))
	assert.NoError(t, ap.ResizePhotos(), "existing outputs must be skipped without opening the source")
}

func TestResizePhotos_DryRun(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
	})
	ap.Config.DryRun = true

	err := ap.ResizePhotos()
	require.NoError(t, err)

	// No files should have been written.
	for _, size := range AllSizes() {
		outPath := ap.OutputPath(string(size), WebPFileName("landscape-1.jpg"))
		_, statErr := os.Stat(outPath)
		assert.True(t, os.IsNotExist(statErr), "expected no output file for size %s in dry-run", size)
	}
}

func TestResizePhotos_ErrorOnBadInput(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{
		{FileName: "missing.jpg", AbsolutePath: "/nonexistent/missing.jpg"},
	})

	err := ap.ResizePhotos()
	// require, not assert: assert records the failure and carries on, so a nil error here
	// would panic on the next line instead of reporting what actually went wrong.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resize")
}

func TestResizePhotos_Empty(t *testing.T) {
	ap := newTestProcessor(t, []*Photo{})
	err := ap.ResizePhotos()
	require.NoError(t, err)
}

// newTestVideo returns a Photo for a committed video fixture, shaped the way
// collectPhotosRecursive and fillMetadata would leave it.
func newTestVideo(t *testing.T, filename string) *Photo {
	t.Helper()
	path := filepath.Join("testdata", filename)
	meta, err := ReadMediaMetadata(path)
	require.NoError(t, err)
	return &Photo{FileName: filename, AbsolutePath: path, IsVideo: true, PhotoMetadata: meta}
}

// videoOutputs returns the MP4 and every poster path a video should produce.
//
// The names are built literally rather than by calling Config.PhotoOutputName: deriving
// the expectation from the function under test would only prove that ResizePhotos and
// this helper agree, not that either is right. newTestProcessor builds an unencrypted
// config, so the stem is simply the source name with its extension replaced.
func videoOutputs(ap *AlbumProcessor, filename string) (string, []string) {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	mp4 := ap.OutputPath(VideoDirName, stem+".mp4")
	posters := make([]string, 0, len(AllSizes()))
	for _, size := range AllSizes() {
		posters = append(posters, ap.OutputPath(string(size), stem+".webp"))
	}
	return mp4, posters
}

func TestResizePhotos_Video(t *testing.T) {
	requireVideoTools(t)

	ap := newTestProcessor(t, []*Photo{newTestVideo(t, "landscape.mov")})
	require.NoError(t, ap.ResizePhotos())

	// A video expands to three outputs, not the two a still produces: the transcoded MP4
	// plus a poster at every image size.
	mp4, posters := videoOutputs(ap, "landscape.mov")
	assert.FileExists(t, mp4)
	for _, p := range posters {
		assert.FileExists(t, p, "poster should be written for every size")
	}

	// The MP4 is a real transcode, not a copy of the .mov.
	meta, err := ReadVideoMetadata(mp4)
	require.NoError(t, err)
	assert.Positive(t, meta.Duration)
}

// TestResizePhotos_VideoTracksFilesForClean is the highest-stakes test here. Every video
// output must go through Config.TrackFile whether it needed writing, because
// CleanOutputDir deletes anything under a processed album that is not in that set. A
// regression does not fail a run: it silently deletes the user's transcoded videos and
// posters on the next -clean, and the only symptom is a slow rebuild.
func TestResizePhotos_VideoTracksFilesForClean(t *testing.T) {
	requireVideoTools(t)

	ap := newTestProcessor(t, []*Photo{newTestVideo(t, "landscape.mov")})
	ap.Config.InitClean()
	require.NoError(t, ap.ResizePhotos())

	mp4, posters := videoOutputs(ap, "landscape.mov")
	expected := ap.Config.ExpectedFiles()
	assert.True(t, expected[mp4], "the mp4 must be tracked: %s", mp4)
	for _, p := range posters {
		assert.True(t, expected[p], "poster must be tracked: %s", p)
	}

	// Prove it end to end rather than trusting the map: a real clean must leave them.
	siteDir := filepath.Join(ap.Config.OutputRoot, ap.Config.SiteID)
	require.NoError(t, CleanOutputDir(siteDir, []string{ap.AlbumConfig.Slug}, expected, false))
	assert.FileExists(t, mp4, "-clean must not delete the transcoded video")
	for _, p := range posters {
		assert.FileExists(t, p, "-clean must not delete a poster")
	}
}

// A second run must track the same files even though it writes nothing. This is the case
// that actually bites: the skip path is the one taken on every run after the first, so a
// TrackFile call misplaced inside the "needs writing" branch would pass the test above and
// still delete everything on the next -clean.
func TestResizePhotos_VideoTracksFilesWhenSkipped(t *testing.T) {
	requireVideoTools(t)

	photo := newTestVideo(t, "landscape.mov")
	ap := newTestProcessor(t, []*Photo{photo})
	require.NoError(t, ap.ResizePhotos())

	mp4, posters := videoOutputs(ap, "landscape.mov")
	mp4Before, err := os.Stat(mp4)
	require.NoError(t, err)

	// Re-run against the same output directory with clean tracking enabled.
	ap2 := NewAlbumProcessor(ap.Config, ap.AlbumConfig)
	ap2.Photos = []*Photo{photo}
	ap2.Config.InitClean()
	require.NoError(t, ap2.ResizePhotos())

	mp4After, err := os.Stat(mp4)
	require.NoError(t, err)
	assert.Equal(t, mp4Before.ModTime(), mp4After.ModTime(), "an up-to-date video must not be re-transcoded")

	expected := ap2.Config.ExpectedFiles()
	assert.True(t, expected[mp4], "a skipped mp4 must still be tracked")
	for _, p := range posters {
		assert.True(t, expected[p], "a skipped poster must still be tracked: %s", p)
	}
}

// A video is all-or-nothing: unlike a still, whose sizes are skipped independently, a
// missing poster has to redo the whole item because the poster frame comes from the source.
func TestResizePhotos_VideoRedoneWhenPosterMissing(t *testing.T) {
	requireVideoTools(t)

	photo := newTestVideo(t, "landscape.mov")
	ap := newTestProcessor(t, []*Photo{photo})
	require.NoError(t, ap.ResizePhotos())

	_, posters := videoOutputs(ap, "landscape.mov")
	require.NoError(t, os.Remove(posters[0]))

	ap2 := NewAlbumProcessor(ap.Config, ap.AlbumConfig)
	ap2.Photos = []*Photo{photo}
	require.NoError(t, ap2.ResizePhotos())
	assert.FileExists(t, posters[0], "a missing poster must be regenerated")
}

func TestResizePhotos_VideoDryRun(t *testing.T) {
	requireVideoTools(t)

	ap := newTestProcessor(t, []*Photo{newTestVideo(t, "landscape.mov")})
	ap.Config.DryRun = true
	require.NoError(t, ap.ResizePhotos())

	mp4, posters := videoOutputs(ap, "landscape.mov")
	assert.NoFileExists(t, mp4, "dry run must not transcode")
	for _, p := range posters {
		assert.NoFileExists(t, p, "dry run must not write a poster")
	}
}

// TestResizePhotos_MixedAlbum covers the split in ResizePhotos: stills go to the resize
// workers and videos to their own lower-concurrency pool, and both must complete.
func TestResizePhotos_MixedAlbum(t *testing.T) {
	requireVideoTools(t)

	ap := newTestProcessor(t, []*Photo{
		{FileName: "landscape-1.jpg", AbsolutePath: filepath.Join("testdata", "landscape-1.jpg")},
		newTestVideo(t, "landscape.mov"),
	})
	require.NoError(t, ap.ResizePhotos())

	for _, size := range AllSizes() {
		assert.FileExists(t, ap.OutputPath(string(size), WebPFileName("landscape-1.jpg")))
	}
	mp4, posters := videoOutputs(ap, "landscape.mov")
	assert.FileExists(t, mp4)
	for _, p := range posters {
		assert.FileExists(t, p)
	}
}
