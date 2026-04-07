#!/bin/sh
set -e

HTDOCS="/usr/share/nginx/html"

/setup-htdocs.sh "$HTDOCS"

exec nginx -g 'daemon off;'
