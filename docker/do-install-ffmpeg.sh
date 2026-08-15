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
# ddphotos-npm-cache volume on first run. Unlike npx, the download is pinned and verified.
#
set -euo pipefail

# Pinned to an immutable dated BtbN release tag rather than a rolling "latest" asset, so
# the checksums below stay valid. BtbN over the smaller johnvansickle builds because its
# assets are GitHub-hosted under permanent tags; johnvansickle's versioned URLs disappear
# when a new release supersedes them, and it publishes MD5 sidecars only.
FFMPEG_TAG="autobuild-2026-08-12-13-15"
FFMPEG_BASE="https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_TAG}"

# GPL builds: the LGPL variants omit libx264, which is exactly what we need to encode H.264.
case "$(uname -m)" in
    x86_64|amd64)
        FFMPEG_FILE="ffmpeg-n8.1.2-34-g9b6c8969e0-linux64-gpl-8.1.tar.xz"
        FFMPEG_SHA256="980d92b6365c0bd242cbcc7a9c7a951acbae92a64285ca9db7c18999e62155a2"
        ;;
    aarch64|arm64)
        FFMPEG_FILE="ffmpeg-n8.1.2-34-g9b6c8969e0-linuxarm64-gpl-8.1.tar.xz"
        FFMPEG_SHA256="fbffa55263830f010355a44b2eb25dde32feb7b3870f1ec18737bfcd5e74df2b"
        ;;
    *)
        echo "Error: no pinned ffmpeg build for architecture $(uname -m)." >&2
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

echo "Verifying checksum ..."
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
