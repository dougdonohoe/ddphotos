#!/bin/bash
set -e

# Relative base paths in albums.yaml are anchored to cwd; cd to the album dir so they resolve correctly.
cd /ddphotos

# Resolve env passed into container, with defaults
CONFIG_DIR="${DDPHOTOS_CONFIG_DIR:-/ddphotos/config}"
SITE_ID="${DDPHOTOS_SITE_ID}" # if no site-id, uses settings.id in $CONFIG_DIR/albums.yaml

photogen() {
    echo "Calling 'photogen $*' ..."
    echo ""
    exec /usr/local/bin/photogen "$@"
}

if [ "$1" = "--" ]; then
    shift
    photogen -config-dir "$CONFIG_DIR" -site-id "$SITE_ID" "$@"
fi

photogen \
    -config-dir "$CONFIG_DIR" \
    -site-id "$SITE_ID" \
    -resize -index -clean -doit \
    "$@"
