#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
COPY=""
CLOUDFLARE=""
EXPORT_SITE_ID="$SITE_ID"

while [ "${1#--}" != "$1" ]; do
    case "$1" in
        --copy)             COPY=1;                shift ;;
        --cloudflare)       CLOUDFLARE=1;          shift ;;
        --export-site-id)   EXPORT_SITE_ID="$2";   shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

EXPORT_DIR="/ddphotos/export/$EXPORT_SITE_ID"

if [ ! -d "/ddphotos/albums/$SITE_ID" ]; then
    echo "Error: /ddphotos/albums/$SITE_ID not found. Run 'photogen' first."
    exit 1
fi

if [ ! -d "/ddphotos/build/$SITE_ID" ]; then
    echo "Error: /ddphotos/build/$SITE_ID not found. Run 'build' first."
    exit 1
fi

if [ -n "$COPY" ]; then
    LINK_DIR=$(mktemp -d)
    RELATIVE_LINKS=1 \
    BUILD_ROOT=/ddphotos/build \
    ALBUMS_DIR=/ddphotos/albums/$SITE_ID \
    DDPHOTOS_SITE_ID=$SITE_ID \
    /docker/setup-htdocs.sh "$LINK_DIR"
    mkdir -p "$EXPORT_DIR"
    rsync -rLtv --delete "$LINK_DIR/" "$EXPORT_DIR/"
    /bin/rm -rf "$LINK_DIR"
else
    /bin/rm -rf "$EXPORT_DIR"
    mkdir -p "$EXPORT_DIR"
    RELATIVE_LINKS=1 \
    BUILD_ROOT=/ddphotos/build \
    ALBUMS_DIR=/ddphotos/albums/$SITE_ID \
    DDPHOTOS_SITE_ID=$SITE_ID \
    /docker/setup-htdocs.sh "$EXPORT_DIR"
fi

if [ -n "$CLOUDFLARE" ]; then
    /bin/cp /docker/cloudflare-worker.js "$EXPORT_DIR/_worker.js"
fi

echo "  Exported $SITE_ID to export/$EXPORT_SITE_ID"
echo "  Serve with: python3 -m http.server 8000 --directory export/$EXPORT_SITE_ID"
echo ""
