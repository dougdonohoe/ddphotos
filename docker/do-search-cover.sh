#!/bin/sh
set -e

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"
export DDPHOTOS_SITE_ID="${DDPHOTOS_SITE_ID:-site-id-undefined}"

exec /app/bin/search-cover.sh --from-docker "$@"
