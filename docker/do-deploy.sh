#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
SITE_ENV="/ddphotos/config/site.env"

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

ALBUMS_CONFIG="/ddphotos/albums/$SITE_ID/config.json"
BUILD_INDEX="/ddphotos/build/$SITE_ID/index.html"

if [ ! -f "$BUILD_INDEX" ]; then
    echo "Error: /ddphotos/build/$SITE_ID not found. Run 'build' first."
    exit 1
fi

stale=$(
    find /ddphotos/config -maxdepth 1 -newer "$BUILD_INDEX" ! -name "site.env" 2>/dev/null
    [ -f "$ALBUMS_CONFIG" ] && find "$ALBUMS_CONFIG" -newer "$BUILD_INDEX" 2>/dev/null || true
)
if [ -n "$stale" ]; then
    echo "Error: config or album data is newer than build output. Run 'build' before 'deploy'."
    exit 1
fi

export REPO_ROOT="/ddphotos"
export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"
export DDPHOTOS_SITE_ID="$SITE_ID"

exec /docker/deploy-photos.sh \
    --no-photogen \
    --no-build \
    --no-pre-deploy-tests \
    --no-playwright \
    --config-dir /ddphotos/config \
    --site-env "$SITE_ENV" \
    "$@"
