#!/usr/bin/env bash

set -eo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

SITE_ID="${DDPHOTOS_SITE_ID:-$(sed -n 's/^DDPHOTOS_SITE_ID=//p' "$REPO_ROOT/config/defaults.env")}"
BUILD_DIR="$REPO_ROOT/build"
ALBUMS_DIR="$REPO_ROOT/albums"

COPY=""
CLOUDFLARE=""

while [[ "${1:-}" == --* ]]; do
    case "$1" in
        --copy)       COPY=1;          shift ;;
        --cloudflare) CLOUDFLARE=1; shift ;;
        --site-id)    SITE_ID="$2";    shift 2 ;;
        --build-dir)  BUILD_DIR="$2";  shift 2 ;;
        --albums-dir) ALBUMS_DIR="$2"; shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

SITE_ID="${SITE_ID:-sample}"

if [ ! -d "$ALBUMS_DIR/$SITE_ID" ]; then
    echo "Error: $ALBUMS_DIR/$SITE_ID not found." >&2
    if [[ "$SITE_ID" == "sample" ]]; then
        echo "Run 'make sample-photogen' first." >&2
    fi
    exit 1
fi

if [ ! -d "$BUILD_DIR/$SITE_ID" ]; then
    echo "Error: $BUILD_DIR/$SITE_ID not found." >&2
    if [[ "$SITE_ID" == "sample" ]]; then
        echo "Run 'make sample-build' first." >&2
    fi
    exit 1
fi

EXPORT_DIR="$REPO_ROOT/export/$SITE_ID"

if [ -n "$COPY" ]; then
    LINK_DIR=$(mktemp -d)
    BUILD_ROOT="$BUILD_DIR" \
    ALBUMS_DIR="$ALBUMS_DIR/$SITE_ID" \
    DDPHOTOS_SITE_ID="$SITE_ID" \
    "$REPO_ROOT/web/setup-htdocs.sh" "$LINK_DIR"
    mkdir -p "$EXPORT_DIR"
    rsync -rLtv --delete "$LINK_DIR/" "$EXPORT_DIR/"
    /bin/rm -rf "$LINK_DIR"
else
    /bin/rm -rf "$EXPORT_DIR"
    mkdir -p "$EXPORT_DIR"
    BUILD_ROOT="$BUILD_DIR" \
    ALBUMS_DIR="$ALBUMS_DIR/$SITE_ID" \
    DDPHOTOS_SITE_ID="$SITE_ID" \
    "$REPO_ROOT/web/setup-htdocs.sh" "$EXPORT_DIR"
fi

if [ -n "$CLOUDFLARE" ]; then
    /bin/cp "$REPO_ROOT/docker/cloudflare-worker.js" "$EXPORT_DIR/_worker.js"
fi

echo ""
echo "  Exported $SITE_ID to export/$SITE_ID"
echo "  Serve with: python3 -m http.server 8000 --directory export/$SITE_ID"
echo ""
