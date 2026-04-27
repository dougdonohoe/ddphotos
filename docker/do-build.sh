#!/bin/sh
set -e

export DDPHOTOS_ALBUMS_DIR="/ddphotos/albums"
export DDPHOTOS_SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"

export VITE_GIT_DESCRIBE=$(cat /docker/IMAGE 2>/dev/null || echo "ddphotos")
export VITE_GIT_BRANCH=""
export VITE_GIT_REPO_SLUG="dougdonohoe/ddphotos"
export VITE_GIT_REPO_URL="https://github.com/dougdonohoe/ddphotos"

mkdir -p /ddphotos/build

# svelte.config.js writes to ../build/$SITE_ID relative to web/; redirect to /ddphotos/build
ln -sfn /ddphotos/build /app/build

cd /app/web
exec npm run build
