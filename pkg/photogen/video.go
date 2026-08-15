package photogen

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// allowedVideoExtensions are the source video containers photogen will transcode.
// Kept separate from allowedPhotoExtensions because the two are not interchangeable:
// a hero image, for instance, accepts photos only.
var allowedVideoExtensions = map[string]struct{}{
	".mov": {},
	".mp4": {},
	".m4v": {},
}

// VideoDirName is the album subdirectory holding transcoded MP4s, alongside grid/ and full/.
const VideoDirName = "video"

// Transcode settings. One rendition only: the source is always downscaled to fit
// videoMaxDimension on its long edge and re-encoded to H.264 + AAC in MP4.
//
// Re-encoding is not an optimization, it is a correctness requirement. Phone video is
// routinely HEVC in a .mov container, which does not play in Chrome or Firefox. H.264
// baseline-compatible MP4 is the only combination that plays everywhere without a fallback.
const (
	videoMaxDimension = 1280
	videoCRF          = "23"
	videoPreset       = "medium"
	videoAudioBitrate = "128k"
)

// videoSizeWarnBytes is the per-asset limit imposed by Cloudflare Pages. Exceeding it does
// not fail the run (other targets have no such limit), but it is warned about because the
// resulting deploy failure happens far away from here and gives no hint of the cause.
const videoSizeWarnBytes = 25 * 1024 * 1024

// posterMaxOffset caps how far into a clip the poster frame is taken. Short clips use their
// midpoint instead, so a 0.5s video does not try to seek past its own end.
const posterMaxOffset = 1.0

// ExitVideoToolsMissing is returned to the shell when an album contains video but ffmpeg
// could not be found. The Docker wrapper watches for this specific code, installs ffmpeg
// into its cache volume, and retries once. Nothing has been written when it is returned,
// so the retry is a clean re-run rather than a resume.
const ExitVideoToolsMissing = 3

// ErrVideoToolsMissing is wrapped by every error caused by ffmpeg being unavailable, so
// that main can tell "install ffmpeg" apart from a genuine processing failure and exit
// with ExitVideoToolsMissing instead of the generic failure path.
var ErrVideoToolsMissing = errors.New("ffmpeg tools unavailable")

// IsPhotoFile reports whether name has a supported still-image extension.
func IsPhotoFile(name string) bool {
	_, ok := allowedPhotoExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// IsVideoFile reports whether name has a supported video extension.
func IsVideoFile(name string) bool {
	_, ok := allowedVideoExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// IsMediaFile reports whether name is either a supported photo or a supported video.
func IsMediaFile(name string) bool {
	return IsPhotoFile(name) || IsVideoFile(name)
}

// VideoFileName returns the transcoded output filename for a source video.
func VideoFileName(filename string) string {
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext) + ".mp4"
}

// videoTools holds resolved paths to the ffmpeg and ffprobe executables.
type videoTools struct {
	ffmpeg  string
	ffprobe string
}

var (
	videoToolsOnce sync.Once
	resolvedTools  videoTools
	resolvedErr    error
)

// ensureVideoTools locates ffmpeg and ffprobe, resolving them once per process and
// falling back to the on-demand installer when neither is present.
//
// Unexported along with videoTools: nothing outside this package needs the executable
// paths. What callers in cmd/ need is the failure, and they get it from the exported
// ErrVideoToolsMissing, VideoToolsHint and ExitVideoToolsMissing.
//
// Search order:
//  1. DDPHOTOS_FFMPEG / DDPHOTOS_FFPROBE, explicit paths for anyone pinning a build.
//  2. DDPHOTOS_FFMPEG_DIR, the cache directory the Docker wrapper mounts as a named
//     volume so a download survives the container.
//  3. PATH, which is what a native `brew install ffmpeg` or `apt install ffmpeg` gives.
//
// ffmpeg is deliberately not bundled in the Docker image: the Debian package pulls in
// roughly 450MB of SDL2/X11 dependencies via ffplay, which every photo-only user would
// pay for. Shipping it would also make us a redistributor of a GPL libx264 build.
func ensureVideoTools() (videoTools, error) {
	videoToolsOnce.Do(func() {
		resolvedTools, resolvedErr = resolveVideoTools()
		if resolvedErr == nil {
			return
		}
		// Nothing on the system, but the environment may offer an installer. In Docker
		// the wrapper points DDPHOTOS_FFMPEG_INSTALLER at a script that downloads a
		// pinned static build into a mounted volume, so the fetch happens once per
		// machine and only for users who actually have video. The decision lives here
		// rather than in the shell because this is the first point at which anything
		// knows a video exists at all: a photo-only run never reaches it.
		if installed := runFFmpegInstaller(); installed {
			resolvedTools, resolvedErr = resolveVideoTools()
		}
	})
	return resolvedTools, resolvedErr
}

// runFFmpegInstaller runs DDPHOTOS_FFMPEG_INSTALLER if it points at an executable.
// Returns whether it ran successfully. Output is streamed so the user sees the download
// progress and understands why the run paused.
func runFFmpegInstaller() bool {
	installer := os.Getenv("DDPHOTOS_FFMPEG_INSTALLER")
	if installer == "" || !isExecutableFile(installer) {
		return false
	}
	fmt.Printf("\n  Video files found, but ffmpeg is not installed yet.\n")
	cmd := exec.Command(installer)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  WARN: ffmpeg install failed: %v\n", err)
		return false
	}
	fmt.Println()
	return true
}

func resolveVideoTools() (videoTools, error) {
	var t videoTools

	find := func(envExact, base string) string {
		if p := os.Getenv(envExact); p != "" {
			if isExecutableFile(p) {
				return p
			}
		}
		if dir := os.Getenv("DDPHOTOS_FFMPEG_DIR"); dir != "" {
			p := filepath.Join(dir, base)
			if isExecutableFile(p) {
				return p
			}
		}
		if p, err := exec.LookPath(base); err == nil {
			return p
		}
		return ""
	}

	t.ffmpeg = find("DDPHOTOS_FFMPEG", "ffmpeg")
	t.ffprobe = find("DDPHOTOS_FFPROBE", "ffprobe")

	var missing []string
	if t.ffmpeg == "" {
		missing = append(missing, "ffmpeg")
	}
	if t.ffprobe == "" {
		missing = append(missing, "ffprobe")
	}
	if len(missing) > 0 {
		return t, fmt.Errorf("%w: %s not found", ErrVideoToolsMissing, strings.Join(missing, " and "))
	}
	return t, nil
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// VideoToolsHint returns the actionable message shown when video files were found but
// ffmpeg is unavailable. Kept as one string so the Docker and native paths stay identical.
func VideoToolsHint() string {
	return "video files require ffmpeg and ffprobe, which were not found\n" +
		"    Docker:  ddphotos install-ffmpeg   (one-time download, cached in a Docker volume)\n" +
		"    macOS:   brew install ffmpeg\n" +
		"    Linux:   sudo apt-get install ffmpeg\n" +
		"    Or set DDPHOTOS_FFMPEG_DIR to a directory containing ffmpeg and ffprobe."
}

// ffprobeOutput is the subset of `ffprobe -show_streams -show_format -of json` we consume.
type ffprobeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		SideData  []struct {
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Tags     struct {
			CreationTime string `json:"creation_time"`
		} `json:"tags"`
	} `json:"format"`
}

// ReadVideoMetadata extracts canonical dimensions, orientation, duration and creation date
// from a video file using ffprobe.
//
// Dimensions are rotation-corrected. This is the subtlest part of video support: phone
// video stores a display matrix rather than rotated pixels, so ffprobe reports a portrait
// clip as 1920x1080 with a rotation of -90. Everything downstream is already correct
// without help (ffmpeg applies the matrix before the filter graph, so both the transcode
// and the extracted poster frame come out upright), which is exactly what makes this easy
// to miss: only the numbers written into index.json are wrong, and the symptom is a
// portrait video laid out as a landscape box in the justified grid.
func ReadVideoMetadata(path string) (*PhotoMetadata, error) {
	tools, err := ensureVideoTools()
	if err != nil {
		return nil, fmt.Errorf("%w\n    %s", err, VideoToolsHint())
	}

	out, err := exec.Command(tools.ffprobe,
		"-v", "error",
		"-show_streams",
		"-show_format",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", path, describeExecErr(err))
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("parse ffprobe output for %s: %w", path, err)
	}

	width, height, found := 0, 0, false
	for _, s := range probe.Streams {
		if s.CodecType != "video" {
			continue
		}
		width, height, found = s.Width, s.Height, true
		for _, sd := range s.SideData {
			if sd.Rotation != nil && isQuarterTurn(*sd.Rotation) {
				width, height = height, width
			}
		}
		break
	}
	if !found {
		return nil, fmt.Errorf("no video stream in %s", path)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions %dx%d in %s", width, height, path)
	}

	return &PhotoMetadata{
		Width:       width,
		Height:      height,
		Orientation: deriveOrientation(width, height),
		DateTaken:   parseVideoCreationTime(probe.Format.Tags.CreationTime),
		Duration:    parseDuration(probe.Format.Duration),
	}, nil
}

// isQuarterTurn reports whether a display-matrix rotation swaps width and height.
// ffprobe reports the angle as a float that may be negative (-90 is the common phone case).
func isQuarterTurn(deg float64) bool {
	d := math.Mod(math.Abs(deg), 180)
	return math.Abs(d-90) < 1
}

// parseVideoCreationTime parses the container creation_time tag. Unlike EXIF, which carries
// no timezone and is assumed to be UTC in parseExifDateTime, this tag is RFC 3339 and
// already carries its own zone, so it only needs normalizing. Returns the zero time when
// absent or unparseable, matching how readDateTaken treats a photo with no EXIF date.
func parseVideoCreationTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func parseDuration(s string) float64 {
	d, err := strconv.ParseFloat(s, 64)
	if err != nil || d < 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return 0
	}
	return d
}

// TranscodeVideo re-encodes a source video to a web-playable MP4, scaled to fit
// videoMaxDimension on its long edge. It mirrors ResizeImage's contract exactly (same
// skip/dry-run/force semantics, same ResizeResult) so the resize worker pool treats
// photos and videos identically.
func TranscodeVideo(inputPath, outputPath string, force, dryRun bool) (*ResizeResult, error) {
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return &ResizeResult{Skipped: true, Message: fmt.Sprintf("exists: %s", outputPath)}, nil
		}
	}
	if dryRun {
		return &ResizeResult{DryRun: true, Message: fmt.Sprintf("DRYRUN: would write %s (video)", outputPath)}, nil
	}

	tools, err := ensureVideoTools()
	if err != nil {
		return nil, fmt.Errorf("%w\n    %s", err, VideoToolsHint())
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), dirPerms); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	// Write to a temp file in the destination directory and rename on success, so an
	// interrupted run cannot leave a truncated MP4 that the next run's os.Stat would
	// happily accept as already done.
	tmp := outputPath + ".tmp.mp4"
	defer os.Remove(tmp)

	args := []string{
		"-v", "error",
		"-y",
		"-i", inputPath,
		// The filter sees the frame after the display matrix has been applied, so
		// comparing iw against ih here is already the displayed orientation. -2 keeps
		// the other axis even, which yuv420p requires.
		"-vf", fmt.Sprintf("scale='if(gt(iw,ih),min(%d,iw),-2)':'if(gt(iw,ih),-2,min(%d,ih))'",
			videoMaxDimension, videoMaxDimension),
		"-c:v", "libx264",
		"-profile:v", "high",
		"-crf", videoCRF,
		"-preset", videoPreset,
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", videoAudioBitrate,
		"-ac", "2",
		// Drop everything except the first video and (if present) first audio stream.
		// Phone video carries timed-metadata streams that MP4 cannot hold, and '?'
		// makes the audio mapping optional so silent clips do not fail.
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-map_metadata", "-1",
		"-movflags", "+faststart",
		tmp,
	}
	if err := runCommand(tools.ffmpeg, args...); err != nil {
		return nil, fmt.Errorf("transcode %s: %w", inputPath, err)
	}
	if err := os.Rename(tmp, outputPath); err != nil {
		return nil, fmt.Errorf("finalize %s: %w", outputPath, err)
	}

	msg := fmt.Sprintf("wrote: %s (video)", outputPath)
	if info, err := os.Stat(outputPath); err == nil {
		msg = fmt.Sprintf("wrote: %s (video, %.1f MB)", outputPath, float64(info.Size())/(1024*1024))
	}
	return &ResizeResult{Written: true, Message: msg}, nil
}

// VideoOversizeWarning returns a warning string when the transcoded file exceeds the
// Cloudflare Pages per-asset limit, or "" when it is within it.
func VideoOversizeWarning(outputPath string) string {
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() <= videoSizeWarnBytes {
		return ""
	}
	return fmt.Sprintf("%s is %.1f MB, over the %d MB Cloudflare Pages per-asset limit; "+
		"it will deploy fine to S3, rsync and Surge but will be rejected by Cloudflare Pages",
		filepath.Base(outputPath), float64(info.Size())/(1024*1024), videoSizeWarnBytes/(1024*1024))
}

// ExtractPoster writes a single frame from the video to outputPath as a JPEG, for use as
// the still shown in the grid and as the lightbox placeholder.
//
// The frame comes out already upright: ffmpeg applies the display matrix on decode, so a
// rotated source needs no special handling here. The JPEG is a temporary: callers hand it
// to ResizeImage so the poster goes through exactly the same WebP ladder, quality settings
// and metadata stripping as every other image in the album.
func ExtractPoster(inputPath, outputPath string, duration float64) error {
	tools, err := ensureVideoTools()
	if err != nil {
		return fmt.Errorf("%w\n    %s", err, VideoToolsHint())
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), dirPerms); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	offset := PosterOffset(duration)
	// -ss before -i seeks by keyframe, which is both far faster and, for the first
	// second of a clip, accurate enough for a thumbnail.
	err = runCommand(tools.ffmpeg,
		"-v", "error",
		"-y",
		"-ss", strconv.FormatFloat(offset, 'f', 3, 64),
		"-i", inputPath,
		"-frames:v", "1",
		"-q:v", "2",
		outputPath,
	)
	if err != nil {
		return fmt.Errorf("extract poster from %s: %w", inputPath, err)
	}
	return nil
}

// PosterOffset returns the timestamp to grab the poster frame from. It favors one second
// in, which skips the fade-in and autofocus hunting most clips open with, but falls back to
// the midpoint of anything shorter than two seconds so the seek stays inside the clip.
func PosterOffset(duration float64) float64 {
	if duration <= 0 {
		return 0
	}
	if half := duration / 2; half < posterMaxOffset {
		return half
	}
	return posterMaxOffset
}

// runCommand runs cmd and turns a non-zero exit into an error carrying its stderr, which
// is where ffmpeg reports the actual reason for a failure.
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// describeExecErr surfaces stderr from an *exec.ExitError, which Output() captures but
// whose Error() string omits.
func describeExecErr(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
