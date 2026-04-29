#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
EXPORT_DIR="/ddphotos/export/$SITE_ID"

if [ ! -d "/ddphotos/build/$SITE_ID" ]; then
    echo "Error: /ddphotos/build/$SITE_ID not found. Run 'build' first."
    exit 1
fi

mkdir -p "$EXPORT_DIR"

RELATIVE_LINKS=1 \
BUILD_ROOT=/ddphotos/build \
ALBUMS_DIR=/ddphotos/albums/$SITE_ID \
DDPHOTOS_SITE_ID=$SITE_ID \
/docker/setup-htdocs.sh "$EXPORT_DIR"

echo ""
echo "  Exported $SITE_ID to /ddphotos/export/$SITE_ID"
echo "  On the host, serve with: python3 -m http.server 8000 --directory export/$SITE_ID"
echo ""
