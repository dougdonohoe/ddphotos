#!/bin/sh
set -e

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"
export DDPHOTOS_SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"

exec /app/bin/search-cover.sh --from-docker "$@"
