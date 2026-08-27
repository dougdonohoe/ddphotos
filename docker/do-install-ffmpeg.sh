#!/bin/bash
#
# Installs ffmpeg and ffprobe into DDPHOTOS_FFMPEG_DIR, which the ddphotos wrapper mounts
# as the named Docker volume 'ddphotos-ffmpeg' so the download survives --rm containers.
#
# Why on demand rather than baked into the image:
#
#   * Size. `apt-get install ffmpeg` on bookworm-slim pulls 200 packages and 448MB, almost
#     all of it SDL2/X11/Wayland dragged in by ffplay via libavdevice. There is no way to
#     opt out, it is a hard dependency of the Debian package. Every photo-only user would
#     pay that cost for a feature they never use.
#   * Licensing. H.264 encoding needs libx264, which is GPL. Shipping it inside the
#     published image would make us a redistributor of a GPL binary; downloading it on the
#     user's own machine at first use sidesteps that entirely.
#
# This mirrors how wrangler and surge already work here: `npx --yes` downloading into the
# ddphotos-npm-cache volume on first run. Unlike npx, the download is checksum-verified.
#
set -euo pipefail

# The rolling "latest" release rather than a dated autobuild tag. BtbN prunes autobuilds to
# the last 14 dailies plus one snapshot per month for two years, so a dated tag 404s about two
# weeks after it is set, which is exactly how this broke once already. "latest" is the only
# URL BtbN guarantees stays live. The n8.1 assets under it track the 8.1 release branch, so we
# pick up bugfixes without jumping a major version.
#
# BtbN over the smaller johnvansickle builds because its assets are GitHub-hosted and it
# publishes a SHA-256 sidecar; johnvansickle publishes MD5 only.
FFMPEG_TAG="latest"
FFMPEG_BASE="https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_TAG}"

# GPL builds: the LGPL variants omit libx264, which is exactly what we need to encode H.264.
case "$(uname -m)" in
    x86_64|amd64)
        FFMPEG_FILE="ffmpeg-n8.1-latest-linux64-gpl-8.1.tar.xz"
        ;;
    aarch64|arm64)
        FFMPEG_FILE="ffmpeg-n8.1-latest-linuxarm64-gpl-8.1.tar.xz"
        ;;
    *)
        echo "Error: no ffmpeg build published for architecture $(uname -m)." >&2
        echo "       Install ffmpeg yourself and set DDPHOTOS_FFMPEG_DIR to its directory." >&2
        exit 1
        ;;
esac

DEST="${DDPHOTOS_FFMPEG_DIR:-/opt/ddphotos/ffmpeg}"
FORCE=false
[ "${1:-}" = "--force" ] && FORCE=true

if [ "$FORCE" = false ] && [ -x "$DEST/ffmpeg" ] && [ -x "$DEST/ffprobe" ]; then
    echo "ffmpeg is already installed in $DEST"
    "$DEST/ffmpeg" -version | head -1
    echo
    echo "Use 'ddphotos install-ffmpeg --force' to reinstall."
    exit 0
fi

mkdir -p "$DEST"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Installing ffmpeg for video support (one time, cached in the ddphotos-ffmpeg volume)."
echo "  source: $FFMPEG_BASE/$FFMPEG_FILE"
echo "  target: $DEST"
echo

curl -fSL --progress-bar -o "$TMP/ffmpeg.tar.xz" "$FFMPEG_BASE/$FFMPEG_FILE"

# The assets under "latest" are rebuilt in place, so a hardcoded checksum would go stale the
# same way a dated tag does. Verify against the sidecar BtbN publishes next to them instead:
# that catches a truncated or corrupted download, though it no longer pins one exact build.
echo "Verifying checksum ..."
curl -fsSL -o "$TMP/checksums.sha256" "$FFMPEG_BASE/checksums.sha256"
FFMPEG_SHA256=$(awk -v f="$FFMPEG_FILE" '$2 == f { print $1 }' "$TMP/checksums.sha256")
if [ -z "$FFMPEG_SHA256" ]; then
    echo "Error: $FFMPEG_FILE is not listed in checksums.sha256. Refusing to install." >&2
    exit 1
fi
echo "$FFMPEG_SHA256  $TMP/ffmpeg.tar.xz" | sha256sum -c - || {
    echo "Error: checksum mismatch. Refusing to install." >&2
    exit 1
}

echo "Extracting ..."
# Only the two executables; the archive also carries ffplay, docs and headers we never use.
tar -xJf "$TMP/ffmpeg.tar.xz" -C "$TMP" --strip-components=2 --wildcards \
    '*/bin/ffmpeg' '*/bin/ffprobe'
install -m 0755 "$TMP/ffmpeg" "$TMP/ffprobe" "$DEST/"

echo
echo "Installed:"
"$DEST/ffmpeg" -version | head -1
"$DEST/ffprobe" -version | head -1
