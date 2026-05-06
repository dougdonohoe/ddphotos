#!/bin/bash
set -e

if [ $# -gt 0 ]; then
    echo "Error: 'run' takes no arguments; got: $*" >&2
    echo "Did you mean to pass options before the command? e.g.: ddphotos $* run" >&2
    exit 1
fi

SITE_ID="${DDPHOTOS_SITE_ID:-site-id-undefined}"

if [ ! -d "$DDPHOTOS_ALBUMS_DIR/$SITE_ID" ]; then
    echo "Error: $DDPHOTOS_ALBUMS_DIR/$SITE_ID not found. Run 'photogen' first."
    exit 1
fi

export DDPHOTOS_SITE_ID="$SITE_ID"

# npm commands run from web dir
cd /app/web

RUN_PORT="${RUN_PORT:-5173}"
echo "  Running dev server for $SITE_ID at: http://localhost:${RUN_PORT}"

# set -m puts background jobs in their own process group, so Ctrl-C (SIGINT) goes
# only to this shell — not npm — letting us kill it cleanly without npm's error output.
set -m
npm run dev -- --port "$RUN_PORT" &
NPM_PID=$!
trap 'kill -- -$NPM_PID 2>/dev/null; exit 0' INT TERM
wait "$NPM_PID" || true
