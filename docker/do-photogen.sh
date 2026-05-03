#!/bin/sh
set -e

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"

# Relative base paths in albums.yaml are anchored to cwd; cd to the album dir so they resolve correctly.
cd /ddphotos

if [ "$1" = "--" ]; then
    shift
    exec /usr/local/bin/photogen -config-dir "${DDPHOTOS_CONFIG_DIR:-/ddphotos/config}" "$@"
fi

exec /usr/local/bin/photogen \
    -config-dir "${DDPHOTOS_CONFIG_DIR:-/ddphotos/config}" \
    -resize -index -clean -doit \
    "$@"
