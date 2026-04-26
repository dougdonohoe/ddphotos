#!/bin/sh
set -e

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"
SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"

exec /usr/local/bin/photogen \
    -config-dir /ddphotos/config \
    -site-id "$SITE_ID" \
    -resize -index -clean -doit \
    "$@"
