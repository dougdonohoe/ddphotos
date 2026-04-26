#!/bin/sh
# Populate the Apache document root with symlinks for the ddphotos serve command.
#
# Mounts (via /ddphotos):
#   /ddphotos/build/<site-id>  — SvelteKit static build output
#   /ddphotos/albums/<site-id> — photogen output (image dirs + JSON)
set -e

HTDOCS="$1"
SITE_ID="${2:-my-photos}"

if [ -z "$HTDOCS" ]; then
    echo "Usage: setup-htdocs.sh <htdocs-dir> [site-id]" >&2
    exit 1
fi

BUILD_SITE="/ddphotos/build/$SITE_ID"
ALBUMS_SITE="/ddphotos/albums/$SITE_ID"

# Symlink build output into htdocs/ (skip albums/ — handled separately below)
find "$BUILD_SITE" -maxdepth 1 -mindepth 1 | while IFS= read -r item; do
    name=$(basename "$item")
    [ "$name" = "albums" ] && continue
    ln -sf "$item" "$HTDOCS/$name"
done

# Create htdocs/albums/ and populate with symlinks
mkdir -p "$HTDOCS/albums"

# Pre-rendered album HTML pages from the build
for item in "$BUILD_SITE/albums"/*.html; do
    [ -e "$item" ] || continue
    ln -sf "$item" "$HTDOCS/albums/$(basename "$item")"
done

# Album data (image dirs, JSON, etc.) from photogen output
for item in "$ALBUMS_SITE"/*; do
    [ -e "$item" ] || continue
    ln -sf "$item" "$HTDOCS/albums/$(basename "$item")"
done
