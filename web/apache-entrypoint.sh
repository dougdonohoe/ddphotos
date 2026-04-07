#!/bin/sh
set -e

HTDOCS="/usr/local/apache2/htdocs"

/setup-htdocs.sh "$HTDOCS"

exec httpd-foreground
