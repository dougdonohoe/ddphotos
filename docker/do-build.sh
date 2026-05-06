#!/bin/bash
set -e

if [ $# -gt 0 ]; then
    echo "Error: 'build' takes no arguments; got: $*" >&2
    echo "Did you mean to pass options before the command? e.g.: ddphotos $* build" >&2
    exit 1
fi

SITE_ID="${DDPHOTOS_SITE_ID:-site-id-undefined}"

if [ ! -d "$DDPHOTOS_ALBUMS_DIR/$SITE_ID" ]; then
    echo "Error: $DDPHOTOS_ALBUMS_DIR/$SITE_ID not found. Run 'photogen' first."
    exit 1
fi

export VITE_GIT_DESCRIBE=$(cat /docker/GIT_DESCRIBE 2>/dev/null || echo "unknown")
export VITE_DOCKER_IMAGE=$(cat /docker/IMAGE 2>/dev/null || echo "")
export VITE_GIT_BRANCH=""
export VITE_GIT_REPO_SLUG="dougdonohoe/ddphotos"
export VITE_GIT_REPO_URL="https://github.com/dougdonohoe/ddphotos"

export DDPHOTOS_SITE_ID="$SITE_ID"

# npm commands run from web dir
cd /app/web

# Ensure build dir exists
mkdir -p /ddphotos/build

# svelte.config.js (from /app/web) writes to ../build/$SITE_ID; redirect /app/build => /ddphotos/build
ln -sfn /ddphotos/build /app/build

echo "Building $DDPHOTOS_SITE_ID ..."

exec npm run build
