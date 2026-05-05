#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-site-id-undefined}"
SERVE_PORT="${SERVE_PORT:-8000}"

if [ $# -gt 0 ]; then
    echo "Error: 'serve' takes no arguments; got: $*" >&2
    echo "Did you mean to pass options before the command? e.g.: ddphotos $* serve" >&2
    exit 1
fi

if [ ! -d "/ddphotos/build/$SITE_ID" ]; then
    echo "Error: /ddphotos/build/$SITE_ID not found. Run 'build' first."
    exit 1
fi

BUILD_ROOT=/ddphotos/build \
ALBUMS_DIR=/ddphotos/albums/$SITE_ID \
DDPHOTOS_SITE_ID=$SITE_ID \
/docker/setup-htdocs.sh /htdocs

echo "  Serving $SITE_ID at:   http://localhost:${SERVE_PORT}"
echo ""

. /etc/apache2/envvars
mkdir -p "$APACHE_RUN_DIR" "$APACHE_LOCK_DIR" "$APACHE_LOG_DIR"
exec apache2 -D FOREGROUND
