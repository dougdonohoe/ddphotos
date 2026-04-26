#!/bin/sh
set -e

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"

if [ "$1" = "--" ]; then
    shift
    exec /usr/local/bin/photogen -config-dir /ddphotos/config "$@"
fi

exec /usr/local/bin/photogen \
    -config-dir /ddphotos/config \
    -resize -index -clean -doit \
    "$@"
