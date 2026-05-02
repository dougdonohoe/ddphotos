#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
EXPORT_DIR="/ddphotos/export/$SITE_ID"
COPY=""
CLOUDFLARE=""

while [ "${1#--}" != "$1" ]; do
    case "$1" in
        --copy)       COPY=1;          shift ;;
        --cloudflare) CLOUDFLARE=1; shift ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

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
    #find "$EXPORT_DIR" -type l -delete 2>/dev/null || true
    #rsync -aLv --delete "$LINK_DIR/" "$EXPORT_DIR/"

      echo "=== LINK_DIR ==="
      ls -la "$LINK_DIR/albums/" 2>&1

      echo "=== EXPORT_DIR before rsync ==="
      ls -la "$EXPORT_DIR/" 2>&1

      rsync -aLv --delete "$LINK_DIR/" "$EXPORT_DIR/" 2>&1; RSYNC_RC=$?
      echo "=== rsync exit code: $RSYNC_RC ==="

      echo "=== EXPORT_DIR after rsync ==="
      ls -la "$EXPORT_DIR/" 2>&1
      ls -la "$EXPORT_DIR/albums/" 2>&1


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

echo ""
echo "  Exported $SITE_ID to export/$SITE_ID"
echo "  Serve with: python3 -m http.server 8000 --directory export/$SITE_ID"
echo ""
