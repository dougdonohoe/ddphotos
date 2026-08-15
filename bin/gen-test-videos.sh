#!/bin/bash
#
# Regenerates the video fixtures in pkg/photogen/testdata/.
#
# These are committed rather than generated at test time, deliberately. photogen needs an
# HEVC *decoder*, which every ffmpeg build has, but never an HEVC *encoder*, which many
# static builds omit. Generating the fixtures during `go test` would invent a dependency
# the production code does not have, and would skip the single most important test (can we
# read a phone's HEVC clip at all?) on exactly the builds where that question matters.
#
# So: this script needs libx265 and is run by hand when a fixture needs changing. The
# tests themselves only need a decoder.
#
# The fixtures are intentionally tiny (320x240, 1 second, tens of KB) since nothing about
# what they exercise depends on being large.
#
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="pkg/photogen/testdata"

command -v ffmpeg >/dev/null || { echo "Error: ffmpeg not found." >&2; exit 1; }
if ! ffmpeg -hide_banner -encoders 2>/dev/null | grep -q libx265; then
    echo "Error: this ffmpeg has no libx265 encoder, which is needed to build the HEVC fixture." >&2
    echo "       macOS: brew install ffmpeg" >&2
    exit 1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# landscape.mov -- the real-world phone shape: HEVC video, AAC audio, creation_time set.
# HEVC in a .mov container is what iPhones produce and what browsers refuse to play, so
# this is the fixture that proves the decode-and-transcode path works at all.
echo "Generating $OUT/landscape.mov ..."
ffmpeg -y -v error \
    -f lavfi -i testsrc2=size=320x240:rate=10:duration=1 \
    -f lavfi -i sine=frequency=440:duration=1 \
    -c:v libx265 -tag:v hvc1 -preset ultrafast \
    -c:a aac -b:a 32k -shortest \
    -metadata creation_time=2019-12-29T23:27:41Z \
    "$OUT/landscape.mov"

# portrait-rotated.mov -- the dimension trap. ffprobe reports 320x240 with a rotation of
# 90 in side_data, so the displayed size is really 240x320. Written in two steps because
# -display_rotation is an INPUT option; there is no output-side equivalent. Both
# '-metadata:s:v:0 rotate=90' and '-bsf:v h264_metadata=rotate=90' were tried and silently
# wrote no rotation at all on ffmpeg 8.1, so do not "simplify" this back to one command.
echo "Generating $OUT/portrait-rotated.mov ..."
ffmpeg -y -v error -f lavfi -i testsrc2=size=320x240:rate=10:duration=1 \
    -c:v libx264 -preset ultrafast -an "$TMP/base.mov"
ffmpeg -y -v error -display_rotation 90 -i "$TMP/base.mov" -c copy "$OUT/portrait-rotated.mov"

# no-date.mp4 -- no creation_time tag, so DateTaken must come back as the zero time
# rather than an error. Mirrors testdata/no-exif.jpg on the photo side.
echo "Generating $OUT/no-date.mp4 ..."
ffmpeg -y -v error -f lavfi -i testsrc2=size=320x240:rate=10:duration=1 \
    -c:v libx264 -preset ultrafast -an -map_metadata -1 "$OUT/no-date.mp4"

# silent.mp4 -- landscape H.264 with no audio stream at all, so the transcode's optional
# audio mapping ("-map 0:a:0?") is exercised. Without the '?' this file fails to transcode.
echo "Generating $OUT/silent.mp4 ..."
ffmpeg -y -v error -f lavfi -i testsrc2=size=320x240:rate=10:duration=2 \
    -c:v libx264 -preset ultrafast -an \
    -metadata creation_time=2020-06-15T10:00:00Z "$OUT/silent.mp4"

echo
ls -l "$OUT"/*.mov "$OUT"/*.mp4
