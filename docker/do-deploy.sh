#!/bin/bash
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-site-id-undefined}"
CONFIG_DIR="${DDPHOTOS_CONFIG_DIR:-/ddphotos/config}"
SITE_ENV="${DDPHOTOS_SITE_ENV:-$CONFIG_DIR/site.env}"

if [ ! -f "$SITE_ENV" ]; then
    echo "Error: $SITE_ENV not found."
    echo "Create it with RSYNC_HOST, RSYNC_DEST (rsync) or S3_BUCKET (S3) and optional CLOUDFRONT_ID."
    exit 1
fi

# Source site.env to detect deployment mode
. "$SITE_ENV"

# Auto-add --s3 if S3_BUCKET is set and not already passed
case " $* " in
    *\ --s3*) ;;
    *) [ -n "${S3_BUCKET:-}" ] && set -- --s3 "$@" ;;
esac

ALBUMS_CONFIG="$DDPHOTOS_ALBUMS_DIR/$SITE_ID/config.json"
BUILD_INDEX="/ddphotos/build/$SITE_ID/index.html"

if [ ! -f "$ALBUMS_CONFIG" ]; then
    echo "Error: album data not found at $ALBUMS_CONFIG. Run 'photogen' first."
    exit 1
fi

if [ -n "$(find "$CONFIG_DIR" -maxdepth 1 -type f -newer "$ALBUMS_CONFIG" ! -name "site.env" 2>/dev/null)" ]; then
    echo "Error: config is newer than album data. Run 'photogen' before 'deploy'."
    exit 1
fi

if [ ! -f "$BUILD_INDEX" ]; then
    echo "Error: build output not found. Run 'build' first."
    exit 1
fi

if [ -n "$(find "$ALBUMS_CONFIG" -newer "$BUILD_INDEX" 2>/dev/null)" ]; then
    echo "Error: album data is newer than build output. Run 'build' before 'deploy'."
    exit 1
fi

export REPO_ROOT="/ddphotos"
export DDPHOTOS_SITE_ID="$SITE_ID"

echo "Deploying: $SITE_ID"
echo "  Config:   $CONFIG_DIR"
echo "  Site env: $SITE_ENV"
echo ""

exec /docker/deploy-photos.sh \
    --no-photogen \
    --no-build \
    --no-pre-deploy-tests \
    --no-playwright \
    --config-dir "$CONFIG_DIR" \
    --site-env "$SITE_ENV" \
    "$@"
