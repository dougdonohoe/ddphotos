#!/bin/sh
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
SERVE_PORT="${SERVE_PORT:-8000}"

if [ ! -d "/ddphotos/build/$SITE_ID" ]; then
    echo "Error: /ddphotos/build/$SITE_ID not found. Run 'build' first."
    exit 1
fi

/docker/setup-htdocs.sh /htdocs "$SITE_ID"

echo ""
echo "  Local:   http://localhost:${SERVE_PORT}"
echo ""

. /etc/apache2/envvars
mkdir -p "$APACHE_RUN_DIR" "$APACHE_LOCK_DIR" "$APACHE_LOG_DIR"
exec apache2 -D FOREGROUND
