#!/bin/bash
set -e

SITE_ID="${DDPHOTOS_SITE_ID:-my-photos}"
RUN_PORT="${RUN_PORT:-5173}"

if [ ! -d "/ddphotos/albums/$SITE_ID" ]; then
    echo "Error: /ddphotos/albums/$SITE_ID not found. Run 'photogen' first."
    exit 1
fi

export VITE_GIT_DESCRIBE=$(cat /docker/GIT_DESCRIBE 2>/dev/null || echo "unknown")
export VITE_DOCKER_IMAGE=$(cat /docker/IMAGE 2>/dev/null || echo "")
export VITE_GIT_BRANCH=""
export VITE_GIT_REPO_SLUG="dougdonohoe/ddphotos"
export VITE_GIT_REPO_URL="https://github.com/dougdonohoe/ddphotos"

echo "  Dev server for $SITE_ID at:   http://localhost:${RUN_PORT}"

export DDPHOTOS_ALBUMS_DIR=/ddphotos/albums
export DDPHOTOS_SITE_ID="$SITE_ID"
cd /app/web

# set -m puts background jobs in their own process group, so Ctrl-C (SIGINT) goes
# only to this shell — not npm — letting us kill it cleanly without npm's error output.
set -m
npm run dev -- --port "$RUN_PORT" &
NPM_PID=$!
trap "kill -- -$NPM_PID 2>/dev/null; exit 0" INT TERM
wait "$NPM_PID" || true
