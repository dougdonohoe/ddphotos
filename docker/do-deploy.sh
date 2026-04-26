#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
SITE_ENV="/ddphotos/config/site.env"

if [ ! -f "$SITE_ENV" ]; then
    echo "Error: $SITE_ENV not found."
    echo "Create it with RSYNC_HOST, RSYNC_DEST (rsync) or S3_BUCKET (S3) and optional CLOUDFRONT_ID."
    exit 1
fi

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"
export DDPHOTOS_SITE_ID="$SITE_ID"

exec /docker/deploy-photos.sh \
    --no-photogen \
    --no-build \
    --no-pre-deploy-tests \
    --site-env "$SITE_ENV" \
    "$@"
