#!/bin/sh
# Populate a document root with symlinks so a web server can serve the site.
#
# Usage: setup-htdocs.sh <htdocs-dir>
#
# Environment variables (with defaults for the web/test container use case):
#   DDPHOTOS_SITE_ID  — site ID (default: sample)
#   BUILD_ROOT        — directory containing per-site build dirs (default: /build)
#   ALBUMS_DIR        — directory containing album data for the active site (default: /albums)
#
# Strategy:
#   1. Symlink everything from $BUILD_ROOT/<site-id>/ into <htdocs>/, except albums/.
#   2. Create <htdocs>/albums/ as a real directory, then populate it with:
#        - symlinks to *.html files from $BUILD_ROOT/<site-id>/albums/  (pre-rendered pages)
#        - symlinks to everything in $ALBUMS_DIR/                        (image dirs, JSON, etc.)
#
# All symlinks live inside the container — nothing dangling is left on the host.

set -e

HTDOCS="$1"
if [ -z "$HTDOCS" ]; then
    echo "Usage: setup-htdocs.sh <htdocs-dir>" >&2
    exit 1
fi

SITE_ID="${DDPHOTOS_SITE_ID:-sample}"
BUILD_ROOT="${BUILD_ROOT:-/build}"
ALBUMS_DIR="${ALBUMS_DIR:-/albums}"
BUILD_SITE="$BUILD_ROOT/$SITE_ID"

# When RELATIVE_LINKS=1, compute a relative symlink target using GNU realpath.
# Only set this when running inside the Linux container (realpath --relative-to
# is a GNU coreutils extension not available on macOS).
make_link() {
    local target="$1"
    local link="$2"
    if [ -n "${RELATIVE_LINKS:-}" ]; then
        target=$(realpath --relative-to="$(dirname "$link")" "$target")
    fi
    ln -sf "$target" "$link"
}

# 1. Symlink build output into htdocs/ (skip albums/ — handled separately below).
# Use find rather than glob so dotfiles like .htaccess are included.
find "$BUILD_SITE" -maxdepth 1 -mindepth 1 | while IFS= read -r item; do
    name=$(basename "$item")
    [ "$name" = "albums" ] && continue
    make_link "$item" "$HTDOCS/$name"
done

# 2. Create htdocs/albums/ and populate with symlinks
mkdir -p "$HTDOCS/albums"

# Pre-rendered album HTML pages from the build
for item in "$BUILD_SITE/albums"/*.html; do
    [ -e "$item" ] || continue
    make_link "$item" "$HTDOCS/albums/$(basename "$item")"
done

# Album data (image dirs, albums.json, sitemap.xml, etc.) from the photogen mount
for item in "$ALBUMS_DIR"/*; do
    [ -e "$item" ] || continue
    make_link "$item" "$HTDOCS/albums/$(basename "$item")"
done
