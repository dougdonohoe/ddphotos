#!/bin/bash
set -e

if [ $# -gt 0 ]; then
    echo "Error: 'serve' takes no arguments; got: $*" >&2
    echo "Did you mean to pass options before the command? e.g.: ddphotos $* serve" >&2
    exit 1
fi

SITE_ID="${DDPHOTOS_SITE_ID:-site-id-undefined}"

if [ ! -d "/ddphotos/build/$SITE_ID" ]; then
    echo "Error: /ddphotos/build/$SITE_ID not found. Run 'build' first."
    exit 1
fi

# setup htdocs symlinks in for Apache to serve
BUILD_ROOT=/ddphotos/build \
  ALBUMS_DIR=$DDPHOTOS_ALBUMS_DIR/$SITE_ID \
  DDPHOTOS_SITE_ID=$SITE_ID \
  /docker/setup-htdocs.sh /htdocs

SERVE_PORT="${SERVE_PORT:-8000}"
echo "  Serving $SITE_ID at: http://localhost:${SERVE_PORT}"
echo ""

. /etc/apache2/envvars
mkdir -p "$APACHE_RUN_DIR" "$APACHE_LOCK_DIR" "$APACHE_LOG_DIR"
exec apache2 -D FOREGROUND
