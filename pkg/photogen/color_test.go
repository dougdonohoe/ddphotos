package photogen

import (
	"bytes"
	"log"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nclxFixture is a 64x48 swatch of P3 (200, 60, 40), tagged only with an nclx box
// (primaries 12, transfer 13), as Lightroom's AVIF export writes it. Made with:
//
//	heif-enc -A -q 90 --colour_primaries 12 --transfer_characteristic 13 \
//	  --matrix_coefficients 6 --full_range_flag 1 -o nclx-p3.avif swatch.png
var nclxFixture = filepath.Join("testdata", "color", "nclx-p3.avif")

func TestNeedsColorConversion(t *testing.T) {
	for profile, want := range map[string]bool{
		"":                       false,
		colorProfileNone:         false,
		colorProfileSRGB:         false,
		"ICC: Adobe RGB (1998)":  true,
		colorNCLXP3:              true,
		colorNCLXRec2020:         true,
		"other: nclx HDR 9/16/9": false,
		"other: ICC CMYK U.S. Web Coated (SWOP) v2": false,
	} {
		assert.Equal(t, want, needsColorConversion(profile), profile)
	}
}

func TestClassifyNCLX(t *testing.T) {
	tests := []struct {
		n           nclx
		wantProfile string
		wantInput   string
	}{
		{nclx{1, 13, 6}, colorProfileSRGB, ""},
		{nclx{2, 2, 2}, colorProfileSRGB, ""},
		{nclx{12, 13, 6}, colorNCLXP3, colorNCLXP3},
		{nclx{12, 2, 6}, colorNCLXP3, colorNCLXP3},
		{nclx{9, 1, 9}, colorNCLXRec2020, colorNCLXRec2020},
		{nclx{9, 14, 9}, colorNCLXRec2020, colorNCLXRec2020},
		{nclx{9, 16, 9}, "other: nclx HDR 9/16/9", ""},   // PQ
		{nclx{12, 18, 6}, "other: nclx HDR 12/18/6", ""}, // HLG
		{nclx{12, 1, 6}, "other: nclx 12/1/6", ""},       // P3 with the BT.709 curve
		{nclx{9, 13, 9}, "other: nclx 9/13/9", ""},
	}
	for _, tt := range tests {
		profile, input := classifyNCLX(tt.n)
		assert.Equal(t, tt.wantProfile, profile, "%v", tt.n)
		assert.Equal(t, tt.wantInput, input, "%v", tt.n)
	}
}

func TestReadNCLX(t *testing.T) {
	t.Run("nclx fixture", func(t *testing.T) {
		n, ok := readNCLX(nclxFixture)
		require.True(t, ok)
		assert.Equal(t, nclx{12, 13, 6}, n)
	})

	t.Run("an ICC colr on the primary item is not reported", func(t *testing.T) {
		_, ok := readNCLX(filepath.Join("testdata", "landscape-1.avif"))
		assert.False(t, ok)
	})

	t.Run("malformed and missing files are not reported", func(t *testing.T) {
		dir := t.TempDir()
		data, err := os.ReadFile(nclxFixture)
		require.NoError(t, err)
		for name, content := range map[string][]byte{
			"empty.avif":     {},
			"garbage.avif":   []byte("this is not an ISO BMFF file at all"),
			"truncated.avif": data[:len(data)/3],
			"bad-size.avif":  append([]byte{0, 0, 0, 4, 'f', 't', 'y', 'p'}, data...),
		} {
			p := filepath.Join(dir, name)
			require.NoError(t, os.WriteFile(p, content, 0644))
			assert.NotPanics(t, func() { readNCLX(p) }, name)
		}
		_, ok := readNCLX(filepath.Join(dir, "missing.avif"))
		assert.False(t, ok)
	})

	t.Run("primary item without colr falls back to the first nclx", func(t *testing.T) {
		colr := box("colr", append([]byte("nclx"), 0, 12, 0, 13, 0, 6, 0x80)...)
		ipco := box("ipco", colr...)
		// ipma v0, flags 0, one entry: item 2 -> property 1. The primary (item 1) has none.
		ipma := box("ipma", 0, 0, 0, 0, 0, 0, 0, 1, 0, 2, 1, 0x81)
		pitm := box("pitm", 0, 0, 0, 0, 0, 1)
		n, ok := nclxFromMeta(append(pitm, box("iprp", append(ipco, ipma...)...)...))
		require.True(t, ok)
		assert.Equal(t, nclx{12, 13, 6}, n)
	})
}

// box builds an ISO BMFF box for tests.
func box(typ string, payload ...byte) []byte {
	size := 8 + len(payload)
	return append([]byte{byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size), typ[0], typ[1], typ[2], typ[3]}, payload...)
}

// p3ToSRGB converts an 8-bit Display P3 color to 8-bit sRGB by the standard math, so the
// conversion test does not depend on the profile it is testing.
func p3ToSRGB(r, g, b float64) [3]float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	enc := func(v float64) float64 {
		v = math.Max(0, math.Min(1, v))
		if v <= 0.0031308 {
			return 255 * 12.92 * v
		}
		return 255 * (1.055*math.Pow(v, 1/2.4) - 0.055)
	}
	lr, lg, lb := lin(r), lin(g), lin(b)
	return [3]float64{
		enc(1.2249*lr - 0.2247*lg),
		enc(-0.0420*lr + 1.0419*lg),
		enc(-0.0197*lr - 0.0786*lg + 1.0979*lb),
	}
}

// meanRGB returns the mean of each band of an image file.
func meanRGB(t *testing.T, path string) [3]float64 {
	t.Helper()
	img, err := loadImage(path)
	require.NoError(t, err)
	defer img.Close()
	var out [3]float64
	for i := range out {
		band, err := img.Copy()
		require.NoError(t, err)
		require.NoError(t, band.ExtractBand(i, 1))
		out[i], err = band.Average()
		require.NoError(t, err)
		band.Close()
	}
	return out
}

func TestResizeImage_ConvertsToSRGB(t *testing.T) {
	t.Run("nclx Display P3", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "swatch.webp")
		_, err := ResizeImage(nclxFixture, out, SizeGrid, false, false)
		require.NoError(t, err)

		want := p3ToSRGB(200, 60, 40)
		got := meanRGB(t, out)
		for i := range want {
			// AV1 and WebP are both lossy, and the encoder's YCbCr round trip moves a flat
			// swatch a few levels. Unconverted, red would be 200, not ~219.
			assert.InDelta(t, want[i], got[i], 4, "band %d: want %v, got %v", i, want, got)
		}
	})

	t.Run("embedded Adobe RGB is more saturated than unconverted", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join("testdata", "no-create-date.jpg")

		// The old behavior, reproduced: the same pixels with the profile gone, which every
		// step after it then reads as sRGB. Both go through ResizeImage, since the WebP
		// encode alone lowers mean chroma by several percent.
		raw, err := loadImage(src)
		require.NoError(t, err)
		require.NoError(t, raw.RemoveICCProfile())
		ep := vips.NewJpegExportParams()
		ep.Quality = 100
		buf, _, err := raw.ExportJpeg(ep)
		raw.Close()
		require.NoError(t, err)
		untagged := filepath.Join(dir, "untagged.jpg")
		require.NoError(t, os.WriteFile(untagged, buf, 0644))

		chroma := func(source string) float64 {
			out := filepath.Join(dir, filepath.Base(source)+".webp")
			_, err := ResizeImage(source, out, SizeGrid, false, false)
			require.NoError(t, err)
			img, err := loadImage(out)
			require.NoError(t, err)
			defer img.Close()
			require.NoError(t, img.ToColorSpace(vips.InterpretationLCH))
			require.NoError(t, img.ExtractBand(1, 1))
			avg, err := img.Average()
			require.NoError(t, err)
			return avg
		}

		converted, unconverted := chroma(src), chroma(untagged)
		t.Logf("mean chroma: converted %.2f, unconverted %.2f", converted, unconverted)
		assert.Greater(t, converted, unconverted*1.08, "Adobe RGB read as sRGB is visibly flatter")
	})

	t.Run("output carries no profile", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "p3.webp")
		_, err := ResizeImage(filepath.Join("testdata", "landscape-1.heic"), out, SizeGrid, false, false)
		require.NoError(t, err)
		img, err := loadImage(out)
		require.NoError(t, err)
		defer img.Close()
		assert.False(t, img.HasICCProfile())
	})
}

func TestVipsLogHandler(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	vipsLogHandler("VIPS", vips.LogLevelWarning, "heifload: ignoring nclx profile")
	assert.Empty(t, buf.String(), "the nclx message is dropped")

	vipsLogHandler("VIPS", vips.LogLevelWarning, "something else")
	assert.Contains(t, buf.String(), "[VIPS.warning] something else")
}
