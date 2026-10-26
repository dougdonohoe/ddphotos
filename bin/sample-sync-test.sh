#!/usr/bin/env bash
# Test that the sync demo in sample/config-sync/ still builds.
#
# Usage:
#   bin/sample-sync-test.sh
#
# Runs `make sample-photogen-sync` (mock provider, offline) and checks that it exits 0, that
# both albums were built, and that the two warnings the demo exists to show were printed: a
# skipped RAW file and the video half of a Live Photo pair. The Go tests and docker-test.sh
# cover the sync framework with their own fixtures; nothing else touches this config, its
# listing JSON files or its ../source/... paths.
#
# Needs libvips, and ffmpeg (DDPHOTOS_FFMPEG_DIR) because both albums carry a video.

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

fail() { echo "  FAIL: $*" >&2; exit 1; }

out=$(make sample-photogen-sync 2>&1) || { echo "$out"; fail "make sample-photogen-sync failed"; }

echo "$out" | grep -q "WARN: \[the-way\] skipping DSC_0001.arw: unsupported file type" \
    || { echo "$out"; fail "no warning for the skipped RAW file"; }
echo "$out" | grep -q "WARN: \[the-way\] skipping video IMG_9728.mov: it shares a base name with photo" \
    || { echo "$out"; fail "no warning for the Live Photo video"; }

for slug in antarctica the-way; do
    [ -f "albums/sample-sync/$slug/index.json" ] || fail "albums/sample-sync/$slug/index.json not built"
done

echo "  PASS: sample sync demo OK (2 albums built, RAW and Live Photo warnings printed)"
