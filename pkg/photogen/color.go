package photogen

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/davidbyttow/govips/v2/vips"
)

// Every output is converted to sRGB and then stripped of its profile, because a browser
// shows an untagged image as sRGB. Before this, the profile was stripped without the
// conversion, so a wide-gamut source (Adobe RGB, Display P3) came out visibly flat.
//
// PhotoMetadata.ColorProfile records what a source carries, in one of these forms:
//
//	"none"                  untagged; already treated as sRGB
//	"sRGB"                  an ICC profile with sRGB colorants, or an nclx tag with BT.709 primaries
//	"ICC: <description>"    an RGB ICC profile that is converted to sRGB
//	"nclx: Display P3"      an AVIF/HEIF nclx tag converted through a bundled profile
//	"nclx: Rec.2020"        likewise
//	"other: <detail>"       left alone: a non-RGB ICC profile, an HDR or unrecognized nclx tag
//
// "" means not read yet (a cache entry written before this field existed) or a video.
// needsColorConversion is the only place that interprets the forms.
const (
	colorProfileNone   = "none"
	colorProfileSRGB   = "sRGB"
	colorPrefixICC     = "ICC: "
	colorPrefixNCLX    = "nclx: "
	colorPrefixOther   = "other: "
	colorNCLXP3        = colorPrefixNCLX + "Display P3"
	colorNCLXRec2020   = colorPrefixNCLX + "Rec.2020"
	colorInputEmbedded = "embedded" // convert from the image's own ICC profile
)

// colorPipelineVersion is stamped on every output written by RecordDerived. An output whose
// source needs conversion and whose stamp predates this version is regenerated (see
// MetaCache.colorCurrent). Bump it when a change to the conversion should redo the outputs
// of the sources it affects, and only those.
const colorPipelineVersion = 1

// needsColorConversion reports whether a source with this ColorProfile is converted to sRGB
// on output.
func needsColorConversion(profile string) bool {
	return strings.HasPrefix(profile, colorPrefixICC) || strings.HasPrefix(profile, colorPrefixNCLX)
}

// colorProfile returns the photo's ColorProfile, or "" when its metadata was never read.
func (p *Photo) colorProfile() string {
	if p.PhotoMetadata == nil {
		return ""
	}
	return p.ColorProfile
}

// bundledProfiles are the input profiles for nclx tags, which libvips cannot use. They are
// the CC0 profiles from github.com/saucecontrol/Compact-ICC-Profiles. libvips has a
// built-in "p3" only in newer releases than Debian bookworm ships, and none for Rec.2020.
//
//go:embed icc/DisplayP3-v4.icc icc/Rec2020-v4.icc
var bundledProfiles embed.FS

var bundledProfileFiles = map[string]string{
	colorNCLXP3:      "icc/DisplayP3-v4.icc",
	colorNCLXRec2020: "icc/Rec2020-v4.icc",
}

var (
	bundledProfileOnce  sync.Once
	bundledProfilePaths map[string]string
	bundledProfileErr   error
)

// bundledProfilePath returns a file holding the bundled profile for an nclx ColorProfile.
// libvips takes profiles by path, so they are written once per process to a temp directory.
func bundledProfilePath(profile string) (string, error) {
	bundledProfileOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ddphotos-icc-")
		if err != nil {
			bundledProfileErr = fmt.Errorf("create icc temp dir: %w", err)
			return
		}
		bundledProfilePaths = map[string]string{}
		for name, file := range bundledProfileFiles {
			data, err := bundledProfiles.ReadFile(file)
			if err != nil {
				bundledProfileErr = err
				return
			}
			p := filepath.Join(dir, filepath.Base(file))
			if err := os.WriteFile(p, data, filePerms); err != nil {
				bundledProfileErr = fmt.Errorf("write %s: %w", p, err)
				return
			}
			bundledProfilePaths[name] = p
		}
	})
	if bundledProfileErr != nil {
		return "", bundledProfileErr
	}
	p, ok := bundledProfilePaths[profile]
	if !ok {
		return "", fmt.Errorf("no bundled profile for %q", profile)
	}
	return p, nil
}

// detectColorProfile returns the source's ColorProfile and the input profile that converts
// it to sRGB: "" for none, colorInputEmbedded for its own ICC profile, or an nclx
// ColorProfile whose bundled profile applies.
//
// An embedded ICC profile wins over an nclx tag, as it does in libvips. A profile counts as
// sRGB by its colorants rather than its description, which varies by vendor ("sRGB
// IEC61966-2.1", "sRGB built-in", ...); its tone curve is not compared, since a profile with
// sRGB primaries and a non-sRGB curve is vanishingly rare in photos.
func detectColorProfile(img *vips.ImageRef, path string) (profile, input string) {
	if data := img.GetICCProfile(); len(data) > 0 {
		info := parseICC(data)
		switch {
		case info.space != "RGB":
			return colorPrefixOther + "ICC " + strings.TrimSpace(info.space+" "+info.desc), ""
		case info.srgb:
			return colorProfileSRGB, ""
		default:
			desc := info.desc
			if desc == "" {
				desc = "unnamed"
			}
			return colorPrefixICC + desc, colorInputEmbedded
		}
	}
	if isHEIFFile(path) {
		if n, ok := readNCLX(path); ok {
			return classifyNCLX(n)
		}
	}
	return colorProfileNone, ""
}

// convertToSRGB converts img to sRGB in place when its source needs it. A failed conversion
// is reported and the image left as it was: off colors are better than a build that fails
// on a photo it used to accept.
func convertToSRGB(img *vips.ImageRef, path string) {
	profile, input := detectColorProfile(img, path)
	var err error
	switch input {
	case "":
		return
	case colorInputEmbedded:
		err = img.TransformICCProfile(vips.SRGBIEC6196621ICCProfilePath)
	default:
		var p string
		if p, err = bundledProfilePath(input); err == nil {
			// The image has no ICC profile, so the "fallback" is the input profile used.
			err = img.TransformICCProfileWithFallback(vips.SRGBIEC6196621ICCProfilePath, p)
		}
	}
	if err != nil {
		fmt.Printf("  WARN: %s: could not convert %s to sRGB, colors may be off: %v\n", path, profile, err)
	}
}

// isHEIFFile reports whether path is read by libvips' heifload, the only loader that can
// meet an nclx tag.
func isHEIFFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".heic", ".heif", ".avif":
		return true
	}
	return false
}

// iccInfo is what detectColorProfile needs from an ICC profile.
type iccInfo struct {
	space string // header data color space, e.g. "RGB", "GRAY", "CMYK"
	desc  string
	srgb  bool // rXYZ/gXYZ/bXYZ match sRGB's
}

// srgbColorants are sRGB's D50-adapted primaries, as in the sRGB IEC61966-2.1 profile.
// The tolerance admits the slightly different values of profiles derived from the Rec.709
// primaries, and is far tighter than the gap to Display P3 or Adobe RGB.
var srgbColorants = map[string][3]float64{
	"rXYZ": {0.4361, 0.2225, 0.0139},
	"gXYZ": {0.3851, 0.7169, 0.0971},
	"bXYZ": {0.1431, 0.0606, 0.7141},
}

const srgbColorantTolerance = 0.005

// parseICC reads the color space, description and colorants of an ICC profile. A
// malformed profile yields whatever could be read; space "" is then reported as non-RGB.
func parseICC(data []byte) iccInfo {
	var info iccInfo
	if len(data) < 132 {
		return info
	}
	info.space = strings.TrimSpace(string(data[16:20]))
	tags := map[string][]byte{}
	count := int(binary.BigEndian.Uint32(data[128:132]))
	for i := range count {
		e := 132 + i*12
		if e+12 > len(data) {
			break
		}
		off := int(binary.BigEndian.Uint32(data[e+4 : e+8]))
		size := int(binary.BigEndian.Uint32(data[e+8 : e+12]))
		if off < 0 || size < 0 || off+size > len(data) || off+size < off {
			continue
		}
		tags[string(data[e:e+4])] = data[off : off+size]
	}
	info.desc = iccDescription(tags["desc"])

	info.srgb = true
	for sig, want := range srgbColorants {
		got, ok := iccXYZ(tags[sig])
		if !ok {
			info.srgb = false
			break
		}
		for i := range want {
			if math.Abs(got[i]-want[i]) > srgbColorantTolerance {
				info.srgb = false
			}
		}
	}
	return info
}

// iccXYZ decodes an XYZType tag.
func iccXYZ(tag []byte) ([3]float64, bool) {
	var xyz [3]float64
	if len(tag) < 20 || string(tag[0:4]) != "XYZ " {
		return xyz, false
	}
	for i := range xyz {
		xyz[i] = float64(int32(binary.BigEndian.Uint32(tag[8+i*4:]))) / 65536
	}
	return xyz, true
}

// iccDescription decodes a 'desc' tag: textDescriptionType in v2 profiles, or
// multiLocalizedUnicodeType in v4, whose first record is used.
func iccDescription(tag []byte) string {
	if len(tag) < 12 {
		return ""
	}
	switch string(tag[0:4]) {
	case "desc":
		n := int(binary.BigEndian.Uint32(tag[8:12]))
		if n <= 0 || 12+n > len(tag) {
			return ""
		}
		return strings.TrimSpace(string(bytes.TrimRight(tag[12:12+n], "\x00")))
	case "mluc":
		if len(tag) < 28 || binary.BigEndian.Uint32(tag[8:12]) == 0 {
			return ""
		}
		n := int(binary.BigEndian.Uint32(tag[20:24]))
		off := int(binary.BigEndian.Uint32(tag[24:28]))
		if n <= 0 || off+n > len(tag) || n%2 != 0 {
			return ""
		}
		units := make([]uint16, n/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(tag[off+i*2:])
		}
		return strings.TrimSpace(strings.TrimRight(string(utf16.Decode(units)), "\x00"))
	}
	return ""
}

// nclx is the color description an AVIF or HEIF file can carry instead of an ICC profile,
// as code points from ITU-T H.273.
type nclx struct {
	primaries, transfer, matrix uint16
}

// classifyNCLX maps an nclx tag to a ColorProfile and the bundled input profile for it.
// Only the combinations photo exporters produce are converted; anything else is left as
// it was, which is what happened to every nclx file before this.
func classifyNCLX(n nclx) (profile, input string) {
	detail := fmt.Sprintf("%d/%d/%d", n.primaries, n.transfer, n.matrix)
	switch {
	case n.transfer == 16 || n.transfer == 18: // PQ, HLG
		return colorPrefixOther + "nclx HDR " + detail, ""
	case n.primaries == 1 || n.primaries == 2: // BT.709 (sRGB's primaries), unspecified
		return colorProfileSRGB, ""
	case n.primaries == 12 && (n.transfer == 13 || n.transfer == 2): // Display P3, sRGB curve
		return colorNCLXP3, colorNCLXP3
	case n.primaries == 9 && (n.transfer == 1 || n.transfer == 6 || n.transfer == 14 || n.transfer == 15):
		return colorNCLXRec2020, colorNCLXRec2020 // BT.2020 with its own (BT.709) curve
	}
	return colorPrefixOther + "nclx " + detail, ""
}

// maxMetaBoxSize caps how much of a HEIF 'meta' box readNCLX will load. Real ones are a few
// KB; the cap only stops a corrupt length from allocating the whole file.
const maxMetaBoxSize = 4 << 20

// readNCLX returns the nclx tag of a HEIF/AVIF file's primary image, read from the
// container directly because libvips discards it ("heifload: ignoring nclx profile"). It
// reports false when there is none, when the primary image has an ICC profile instead, or
// when the file cannot be parsed.
func readNCLX(path string) (nclx, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nclx{}, false
	}
	defer f.Close()

	meta, ok := findTopLevelBox(f, "meta")
	if !ok || len(meta) < 4 {
		return nclx{}, false
	}
	return nclxFromMeta(meta[4:]) // meta is a FullBox: skip version and flags
}

// findTopLevelBox returns the payload of the first top-level box of type typ.
func findTopLevelBox(r io.ReadSeeker, typ string) ([]byte, bool) {
	var hdr [16]byte
	for {
		if _, err := io.ReadFull(r, hdr[:8]); err != nil {
			return nil, false
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		hdrLen := int64(8)
		switch size {
		case 1:
			if _, err := io.ReadFull(r, hdr[8:16]); err != nil {
				return nil, false
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
			hdrLen = 16
		case 0: // extends to the end of the file
			cur, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, false
			}
			end, err := r.Seek(0, io.SeekEnd)
			if err != nil {
				return nil, false
			}
			if _, err := r.Seek(cur, io.SeekStart); err != nil {
				return nil, false
			}
			size = end - cur + hdrLen
		}
		if size < hdrLen {
			return nil, false
		}
		payload := size - hdrLen
		if string(hdr[4:8]) == typ {
			if payload > maxMetaBoxSize {
				return nil, false
			}
			buf := make([]byte, payload)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, false
			}
			return buf, true
		}
		if _, err := r.Seek(payload, io.SeekCurrent); err != nil {
			return nil, false
		}
	}
}

// isoBox is one ISO BMFF box inside an in-memory buffer.
type isoBox struct {
	typ     string
	payload []byte
}

// childBoxes splits b into consecutive boxes, stopping at the first malformed one.
func childBoxes(b []byte) []isoBox {
	var out []isoBox
	for len(b) >= 8 {
		size := uint64(binary.BigEndian.Uint32(b[0:4]))
		hdrLen := uint64(8)
		switch size {
		case 1:
			if len(b) < 16 {
				return out
			}
			size = binary.BigEndian.Uint64(b[8:16])
			hdrLen = 16
		case 0:
			size = uint64(len(b))
		}
		if size < hdrLen || size > uint64(len(b)) {
			return out
		}
		out = append(out, isoBox{typ: string(b[4:8]), payload: b[hdrLen:size]})
		b = b[size:]
	}
	return out
}

// nclxFromMeta finds the colr property associated with the primary item ('pitm') through
// 'ipma'. If the primary item has no colr at all, the first nclx in 'ipco' is used, which
// covers writers that attach it only to a grid's tiles.
func nclxFromMeta(meta []byte) (nclx, bool) {
	primary := uint32(0)
	hasPrimary := false
	var props []isoBox
	assoc := map[uint32][]int{}

	for _, box := range childBoxes(meta) {
		switch box.typ {
		case "pitm":
			p := box.payload
			if len(p) >= 6 && p[0] == 0 {
				primary, hasPrimary = uint32(binary.BigEndian.Uint16(p[4:6])), true
			} else if len(p) >= 8 {
				primary, hasPrimary = binary.BigEndian.Uint32(p[4:8]), true
			}
		case "iprp":
			for _, child := range childBoxes(box.payload) {
				switch child.typ {
				case "ipco":
					props = childBoxes(child.payload)
				case "ipma":
					parseIPMA(child.payload, assoc)
				}
			}
		}
	}

	var first *nclx
	for _, prop := range props {
		if n, ok := parseColrNCLX(prop); ok && first == nil {
			first = &n
		}
	}

	if hasPrimary {
		sawColr := false
		for _, idx := range assoc[primary] {
			if idx < 1 || idx > len(props) || props[idx-1].typ != "colr" {
				continue
			}
			sawColr = true
			if n, ok := parseColrNCLX(props[idx-1]); ok {
				return n, true
			}
		}
		if sawColr {
			return nclx{}, false // an ICC colr: libvips reads that itself
		}
	}
	if first != nil {
		return *first, true
	}
	return nclx{}, false
}

// parseIPMA adds each item's 1-based property indexes from an 'ipma' payload to assoc.
func parseIPMA(p []byte, assoc map[uint32][]int) {
	if len(p) < 8 {
		return
	}
	version := p[0]
	wideIndex := p[3]&1 == 1
	count := binary.BigEndian.Uint32(p[4:8])
	p = p[8:]
	for range count {
		var item uint32
		if version < 1 {
			if len(p) < 2 {
				return
			}
			item, p = uint32(binary.BigEndian.Uint16(p)), p[2:]
		} else {
			if len(p) < 4 {
				return
			}
			item, p = binary.BigEndian.Uint32(p), p[4:]
		}
		if len(p) < 1 {
			return
		}
		n := int(p[0])
		p = p[1:]
		for range n {
			if wideIndex {
				if len(p) < 2 {
					return
				}
				assoc[item] = append(assoc[item], int(binary.BigEndian.Uint16(p)&0x7fff))
				p = p[2:]
			} else {
				if len(p) < 1 {
					return
				}
				assoc[item] = append(assoc[item], int(p[0]&0x7f))
				p = p[1:]
			}
		}
	}
}

// parseColrNCLX decodes a 'colr' box of colour_type nclx.
func parseColrNCLX(box isoBox) (nclx, bool) {
	p := box.payload
	if box.typ != "colr" || len(p) < 10 || string(p[0:4]) != "nclx" {
		return nclx{}, false
	}
	return nclx{
		primaries: binary.BigEndian.Uint16(p[4:6]),
		transfer:  binary.BigEndian.Uint16(p[6:8]),
		matrix:    binary.BigEndian.Uint16(p[8:10]),
	}, true
}

// vipsLogHandler is govips' default handler minus heifload's "ignoring nclx profile",
// which libvips prints on every open of an nclx-tagged file (several per photo per run).
// photogen reads the tag itself (readNCLX), and ResizePhotos warns about the ones it
// cannot convert, so the message carries nothing actionable.
func vipsLogHandler(domain string, level vips.LogLevel, message string) {
	if strings.Contains(message, "ignoring nclx profile") {
		return
	}
	names := map[vips.LogLevel]string{
		vips.LogLevelError:    "error",
		vips.LogLevelCritical: "critical",
		vips.LogLevelWarning:  "warning",
		vips.LogLevelMessage:  "message",
		vips.LogLevelInfo:     "info",
		vips.LogLevelDebug:    "debug",
	}
	log.Printf("[%v.%v] %v", domain, names[level], message)
}
