#!/usr/bin/env bash

set -eo pipefail

# Parse flags
SKIP_PHOTOGEN=false
SKIP_RSYNC=false
CONFIG_DIR=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-photogen)  SKIP_PHOTOGEN=true; shift ;;
        --no-rsync)     SKIP_RSYNC=true; shift ;;
        --config-dir)   CONFIG_DIR="$2"; shift 2 ;;
        --config-dir=*) CONFIG_DIR="${1#*=}"; shift ;;
        *) echo "Unknown flag: $1"; exit 1 ;;
    esac
done

# cd
SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

# Resolve CONFIG_DIR to absolute path (relative paths break after subsequent cd's)
[ -n "$CONFIG_DIR" ] && CONFIG_DIR="$(cd "$CONFIG_DIR" && pwd)"

# Load site config
if [ -n "$CONFIG_DIR" ]; then
    CONFIG="$CONFIG_DIR/site.env"
else
    CONFIG="config/site.env"
fi
[ -f "$CONFIG" ] || { echo "Error: $CONFIG not found"; exit 1; }
source "$CONFIG"

# Generate photos
if [ "$SKIP_PHOTOGEN" = true ]; then
    echo "Skipping photogen (--no-photogen)"
else
    PHOTOGEN_ARGS="-resize -index -doit"
    [ -n "$CONFIG_DIR" ] && PHOTOGEN_ARGS="--config-dir $CONFIG_DIR $PHOTOGEN_ARGS"
    go run ./cmd/photogen $PHOTOGEN_ARGS
fi

# Build static site
cd web
source "$HOME/.nvm/nvm.sh"
[ -n "$CONFIG_DIR" ] && export SITE_ENV="$CONFIG_DIR/site.env"
npm run build

# Local Apache test before deploying.
# Start Docker container if not already running on port 8080, and stop it on exit.
DOCKER_STARTED=false
_docker_cleanup() {
    if [ "$DOCKER_STARTED" = true ]; then
        echo "Stopping local Docker container..."
        docker stop "$(docker ps -q --filter publish=8080)" 2>/dev/null || true
    fi
}
trap _docker_cleanup EXIT

DOCKER_RUNNING=$(docker ps -q --filter publish=8080)
if [ -n "$DOCKER_RUNNING" ]; then
    echo "Docker already running on port 8080, using existing container..."
else
    echo "Starting local Docker container for testing..."
    docker run -d --rm -p 8080:80 -v "$PWD":/usr/local/apache2/htdocs:ro photos-apache > /dev/null
    DOCKER_STARTED=true
    sleep 1
fi

echo "Running local Apache tests..."
TEST_ARGS=(--local 8080)
[ -n "$CONFIG_DIR" ] && TEST_ARGS+=(--config-dir "$CONFIG_DIR")
"$SDIR/test-photos-apache.sh" "${TEST_ARGS[@]}"

echo "Running Playwright e2e tests..."
npx playwright test

if [ "$SKIP_RSYNC" = true ]; then
    echo "Skipping rsync, CloudFront invalidation, and post-deploy test (--no-rsync)"
else
    # Deploy (--checksum so rsync skips files with matching content, since
    # Vite resets timestamps on static/ files copied into build/)
    rsync -avz --checksum --delete build/ "$AWS_APACHE":"$RSYNC_DEST"

    # Clear cache
    aws cloudfront create-invalidation --distribution-id "$CLOUDFRONT_ID" --paths "/*"

    # Wait, run test
    echo "Sleeping 5 to allow cache to clear..."
    sleep 5
    PROD_ARGS=()
    [ -n "$CONFIG_DIR" ] && PROD_ARGS+=(--config-dir "$CONFIG_DIR")
    "$SDIR/test-photos-apache.sh" "${PROD_ARGS[@]}"

    echo "Running Playwright e2e tests against production..."
    PLAYWRIGHT_BASE_URL="$VITE_SITE_URL" npx playwright test
fi