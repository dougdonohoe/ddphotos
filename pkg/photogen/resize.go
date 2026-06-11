package photogen

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/davidbyttow/govips/v2/vips"
)

// ImageSize represents the target size variant for resizing.
type ImageSize string

const (
	SizeGrid ImageSize = "grid" // Medium grid display
	SizeFull ImageSize = "full" // Large full view
)

// ImageSizeConfig holds dimension constraints for each size.
type ImageSizeConfig struct {
	MaxDimension int // Maximum width or height (maintains aspect ratio)
	Quality      int // WebP quality (1-100)
}

var sizeConfigs = map[ImageSize]ImageSizeConfig{
	SizeGrid: {MaxDimension: 600, Quality: 85},
	SizeFull: {MaxDimension: 1600, Quality: 90},
}

// ResizeResult contains information about a resize operation.
type ResizeResult struct {
	Written bool   // true if file was written
	Skipped bool   // true if file already existed
	DryRun  bool   // true if this was a dry run
	Message string // human-readable status message
}

// openImage handles the skip/dryrun/load/rotate steps common to all resize operations.
// Returns (img, nil, nil) when the image is loaded and ready, (nil, result, nil) when
// short-circuited (skip or dry run), or (nil, nil, err) on error.
// The caller is responsible for calling img.Close() when a non-nil image is returned.
func openImage(inputPath, outputPath, dryRunLabel string, force, dryRun bool) (*vips.ImageRef, *ResizeResult, error) {
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return nil, &ResizeResult{Skipped: true, Message: fmt.Sprintf("exists: %s", outputPath)}, nil
		}
	}

	if dryRun {
		return nil, &ResizeResult{DryRun: true, Message: fmt.Sprintf("DRYRUN: would write %s (%s)", outputPath, dryRunLabel)}, nil
	}

	params := vips.NewImportParams()
	params.FailOnError.Set(false)
	img, err := vips.LoadImageFromFile(inputPath, params)
	if err != nil {
		return nil, nil, fmt.Errorf("load image %s: %w", inputPath, err)
	}

	if err := img.AutoRotate(); err != nil {
		img.Close()
		return nil, nil, fmt.Errorf("auto-rotate: %w", err)
	}

	return img, nil, nil
}

// prepareImage extends openImage with scale-down-to-fit and mkdir, ready for export.
// Returns (img, nil, nil) when the image is ready to export,
// (nil, result, nil) when short-circuited (skip or dry run), or (nil, nil, err) on error.
// The caller is responsible for calling img.Close() when a non-nil image is returned.
func prepareImage(inputPath, outputPath string, maxDim int, dryRunLabel string, force, dryRun bool) (*vips.ImageRef, *ResizeResult, error) {
	img, result, err := openImage(inputPath, outputPath, dryRunLabel, force, dryRun)
	if err != nil || result != nil {
		return nil, result, err
	}

	var scale float64
	if img.Width() > img.Height() {
		scale = float64(maxDim) / float64(img.Width())
	} else {
		scale = float64(maxDim) / float64(img.Height())
	}
	if scale >= 1.0 {
		scale = 1.0
	}
	if scale < 1.0 {
		if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
			img.Close()
			return nil, nil, fmt.Errorf("resize: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), dirPerms); err != nil {
		img.Close()
		return nil, nil, fmt.Errorf("create output directory: %w", err)
	}

	return img, nil, nil
}

// ResizeImage resizes an image to the specified size variant and writes it as WebP.
// If outputPath exists and force is false, the operation is skipped.
// If dryRun is true, no file is written but the operation is simulated.
func ResizeImage(inputPath, outputPath string, size ImageSize, force, dryRun bool) (*ResizeResult, error) {
	config, ok := sizeConfigs[size]
	if !ok {
		return nil, fmt.Errorf("unknown image size: %s", size)
	}

	img, result, err := prepareImage(inputPath, outputPath, config.MaxDimension, string(size), force, dryRun)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	defer img.Close()

	// Export as WebP with all metadata stripped (smaller files, no GPS leak).
	// Photo metadata (dimensions, date, orientation) is preserved in the JSON index.
	ep := vips.NewWebpExportParams()
	ep.Quality = config.Quality
	ep.StripMetadata = true

	buf, _, err := img.ExportWebp(ep)
	if err != nil {
		return nil, fmt.Errorf("export webp: %w", err)
	}

	if err := os.WriteFile(outputPath, buf, filePerms); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ResizeResult{
		Written: true,
		Message: fmt.Sprintf("wrote: %s (%s, %dx%d)", outputPath, size, img.Width(), img.Height()),
	}, nil
}

// coverJPEGMaxDimension is the max dimension for the OG cover JPEG.
const coverJPEGMaxDimension = 1200

// ResizeCoverJPEG resizes an image and writes it as JPEG for use as an Open Graph image.
// JPEG is used for broad crawler compatibility (iMessage does not support WebP previews).
func ResizeCoverJPEG(inputPath, outputPath string, force, dryRun bool) (*ResizeResult, error) {
	img, result, err := prepareImage(inputPath, outputPath, coverJPEGMaxDimension, "cover jpeg", force, dryRun)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	defer img.Close()

	ep := vips.NewJpegExportParams()
	ep.Quality = 85
	ep.StripMetadata = true

	buf, _, err := img.ExportJpeg(ep)
	if err != nil {
		return nil, fmt.Errorf("export jpeg: %w", err)
	}

	if err := os.WriteFile(outputPath, buf, filePerms); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ResizeResult{
		Written: true,
		Message: fmt.Sprintf("wrote: %s (cover jpeg, %dx%d)", outputPath, img.Width(), img.Height()),
	}, nil
}

// heroWidth and heroHeight define the exact pixel dimensions of the output hero image.
const heroWidth = 1600
const heroHeight = 250

// ResizeHeroJPEG scales and crops an image to heroWidth × heroHeight and writes it as JPEG.
// crop controls the vertical anchor when the scaled image is taller than heroHeight:
// "top" keeps the top, "bottom" keeps the bottom, anything else (including "") centers.
// Horizontally the crop is always centered.
func ResizeHeroJPEG(inputPath, outputPath, crop string, force, dryRun bool) (*ResizeResult, error) {
	img, result, err := openImage(inputPath, outputPath, "hero jpeg", force, dryRun)
	if err != nil || result != nil {
		return result, err
	}
	defer img.Close()

	// Image must be at least as large as the hero target in both dimensions; smaller
	// images can't be cropped to heroWidth x heroHeight without upscaling artifacts.
	if img.Width() < heroWidth || img.Height() < heroHeight {
		return nil, fmt.Errorf("%s: image is %dx%d, too small for hero (minimum %dx%d)",
			inputPath, img.Width(), img.Height(), heroWidth, heroHeight)
	}

	// Scale so the image covers the target dimensions (both width >= heroWidth and height >= heroHeight).
	wScale := float64(heroWidth) / float64(img.Width())
	hScale := float64(heroHeight) / float64(img.Height())
	scale := wScale
	if hScale > scale {
		scale = hScale
	}
	if scale < 1.0 {
		if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
			return nil, fmt.Errorf("%s: resize: %w", inputPath, err)
		}
	}

	// Crop to exact target. Center horizontally; vertical position controlled by crop param.
	x := (img.Width() - heroWidth) / 2
	var y int
	switch crop {
	case "top":
		y = 0
	case "bottom":
		y = img.Height() - heroHeight
	default: // center
		y = (img.Height() - heroHeight) / 2
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if err := img.ExtractArea(x, y, heroWidth, heroHeight); err != nil {
		return nil, fmt.Errorf("%s: crop: %w", inputPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), dirPerms); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	ep := vips.NewJpegExportParams()
	ep.Quality = 85
	ep.StripMetadata = true

	buf, _, err := img.ExportJpeg(ep)
	if err != nil {
		return nil, fmt.Errorf("%s: export jpeg: %w", inputPath, err)
	}
	if err := os.WriteFile(outputPath, buf, filePerms); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ResizeResult{
		Written: true,
		Message: fmt.Sprintf("  wrote: %s (hero jpeg, %dx%d)", outputPath, heroWidth, heroHeight),
	}, nil
}

// AllSizes returns all defined image sizes.
func AllSizes() []ImageSize {
	return []ImageSize{SizeGrid, SizeFull}
}

// GetSizeConfig returns the configuration for a given size.
func GetSizeConfig(size ImageSize) (ImageSizeConfig, bool) {
	cfg, ok := sizeConfigs[size]
	return cfg, ok
}

// WebPFileName converts a filename to use .webp extension.
// Example: "photo.jpg" -> "photo.webp"
func WebPFileName(filename string) string {
	ext := filepath.Ext(filename)
	return filename[:len(filename)-len(ext)] + ".webp"
}
