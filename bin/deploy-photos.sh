#!/usr/bin/env bash

set -eo pipefail

# Parse flags
SKIP_PHOTOGEN=false
SKIP_RSYNC=false
SKIP_PLAYWRIGHT=false
SKIP_APACHE_TEST=false
DRY_RUN=false
CONFIG_DIR=""
SITE_ENV_ARG=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-photogen)  SKIP_PHOTOGEN=true; shift ;;
        --no-rsync)     SKIP_RSYNC=true; shift ;;
        --no-playwright) SKIP_PLAYWRIGHT=true; shift ;;
        --no-apache-test) SKIP_APACHE_TEST=true; shift ;;
        --dry-run)      DRY_RUN=true; shift ;;
        --config-dir)   CONFIG_DIR="$2"; shift 2 ;;
        --config-dir=*) CONFIG_DIR="${1#*=}"; shift ;;
        --site-env)     SITE_ENV_ARG="$2"; shift 2 ;;
        --site-env=*)   SITE_ENV_ARG="${1#*=}"; shift ;;
        *) echo "Unknown flag: $1"; exit 1 ;;
    esac
done

# cd to root of repo
SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."
REPO_ROOT="$(pwd)"

# Resolve CONFIG_DIR and SITE_ENV_ARG to absolute paths (relative paths break after subsequent cd's)
[ -n "$CONFIG_DIR" ]    && CONFIG_DIR="$(cd "$CONFIG_DIR" && pwd)"
[ -n "$SITE_ENV_ARG" ]  && SITE_ENV_ARG="$(cd "$(dirname "$SITE_ENV_ARG")" && pwd)/$(basename "$SITE_ENV_ARG")"

# Resolve site.env: --site-env takes priority, then --config-dir/site.env, then config/site.env
if [ -n "$SITE_ENV_ARG" ]; then
    SITE_ENV="$SITE_ENV_ARG"
elif [ -n "$CONFIG_DIR" ]; then
    SITE_ENV="$CONFIG_DIR/site.env"
else
    SITE_ENV="config/site.env"
fi
[ -f "$SITE_ENV" ] || { echo "Error: $SITE_ENV not found"; exit 1; }
source "$SITE_ENV"

# Load defaults.env as a fallback for vars not already set (e.g. DDPHOTOS_ALBUMS_DIR, DDPHOTOS_SITE_ID)
DEFAULTS_ENV="$(dirname "$SDIR")/config/defaults.env"
if [ -f "$DEFAULTS_ENV" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
        [[ "$line" =~ ^#|^$ ]] && continue
        key="${line%%=*}"
        val="${line#*=}"
        [ -z "${!key+x}" ] && export "$key"="$val"
    done < "$DEFAULTS_ENV"
fi

# These must be set — either by the caller, site.env, or defaults.env
[ -n "$DDPHOTOS_ALBUMS_DIR" ] || { echo "Error: DDPHOTOS_ALBUMS_DIR not set (not in environment or config/defaults.env)"; exit 1; }
[ -n "$DDPHOTOS_SITE_ID" ]    || { echo "Error: DDPHOTOS_SITE_ID not set (not in environment or config/defaults.env)"; exit 1; }

# These must be set in site.env — guard early so a missing value can't
# cause rsync --delete to target the wrong (or empty) remote path.
[ -n "$AWS_APACHE" ]      || { echo "Error: AWS_APACHE not set in $SITE_ENV"; exit 1; }
[ -n "$RSYNC_DEST" ]      || { echo "Error: RSYNC_DEST not set in $SITE_ENV"; exit 1; }
[ -n "$CLOUDFRONT_ID" ]   || { echo "Error: CLOUDFRONT_ID not set in $SITE_ENV"; exit 1; }
[ -n "$VITE_SITE_URL" ]   || { echo "Error: VITE_SITE_URL not set in $SITE_ENV"; exit 1; }

# Ensure RSYNC_DEST ends with / so rsync targets an explicit directory path,
# never a bare or empty string that could default to the remote home directory.
[[ "$RSYNC_DEST" == */ ]] || RSYNC_DEST="${RSYNC_DEST}/"

# Resolve DDPHOTOS_ALBUMS_DIR to absolute path
DDPHOTOS_ALBUMS_DIR="$(cd "$DDPHOTOS_ALBUMS_DIR" && pwd)"

# Generate photos
if [ "$SKIP_PHOTOGEN" = true ]; then
    echo "Skipping photogen (--no-photogen)"
else
    PHOTOGEN_ARGS="-resize -index -clean -doit"
    [ -n "$CONFIG_DIR" ] && PHOTOGEN_ARGS="--config-dir $CONFIG_DIR $PHOTOGEN_ARGS"
    # shellcheck disable=SC2086
    go run ./cmd/photogen $PHOTOGEN_ARGS
fi

# Build static site
cd web
source "$HOME/.nvm/nvm.sh"
SITE_ENV="$SITE_ENV" DDPHOTOS_ALBUMS_DIR="$DDPHOTOS_ALBUMS_DIR" DDPHOTOS_SITE_ID="$DDPHOTOS_SITE_ID" npm run build

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
if [ "$SKIP_APACHE_TEST" = true ]; then
    echo "Skipping local Apache tests (--no-apache-test)"
else
    # Verify Docker image schema before running
    _schema=$(docker image inspect photos-apache --format '{{index .Config.Labels "ddphotos.schema"}}' 2>/dev/null || true)
    if [ "$_schema" != "2" ]; then
        echo "ERROR: photos-apache image is stale or missing (expected schema=2, got '$_schema')."
        echo "Run: make web-docker-build"
        exit 1
    fi

    DOCKER_RUNNING=$(docker ps -q --filter publish=8080)
    if [ -n "$DOCKER_RUNNING" ]; then
        echo "Docker already running on port 8080, using existing container..."
    else
        echo "Starting local Docker container for testing..."
        docker run -d --rm -p 8080:80 \
            -e DDPHOTOS_SITE_ID="$DDPHOTOS_SITE_ID" \
            -v "$REPO_ROOT/build":/build:ro \
            -v "$DDPHOTOS_ALBUMS_DIR/$DDPHOTOS_SITE_ID":/albums:ro \
            photos-apache > /dev/null
        DOCKER_STARTED=true
        sleep 1
    fi

    echo "Running local Apache tests..."
    TEST_ARGS=(--local 8080)
    [ -n "$CONFIG_DIR" ] && TEST_ARGS+=(--config-dir "$CONFIG_DIR")
    "$SDIR/test-photos-apache.sh" "${TEST_ARGS[@]}"
fi

if [ "$SKIP_PLAYWRIGHT" = true ]; then
    echo "Skipping Playwright tests (--no-playwright)"
else
    echo "Running Playwright e2e tests..."
    npx playwright test
fi

RSYNC_OPTS="-avz --checksum --delete"
[ "$DRY_RUN" = true ] && RSYNC_OPTS="$RSYNC_OPTS --dry-run"

if [ "$SKIP_RSYNC" = true ]; then
    echo "Skipping rsync, CloudFront invalidation, and post-deploy test (--no-rsync)"
else
    [ "$DRY_RUN" = true ] && echo "=== DRY RUN: rsync will not transfer any files ==="

    # Deploy app files + pre-rendered album HTML/JSON.
    # --checksum: rsync skips files with matching content (Vite resets timestamps on static/ files)
    # --filter='protect albums/**': prevent --delete from touching albums/ content (hero.jpg,
    #   sitemap.xml, images, JSON) — those files are managed by the second rsync pass below.
    #   Note: 'protect' only suppresses deletion; it still transfers new files from the source.
    # shellcheck disable=SC2086
    rsync $RSYNC_OPTS \
        --filter='protect albums/**' \
        "$REPO_ROOT/build/$DDPHOTOS_SITE_ID/" "$AWS_APACHE":"$RSYNC_DEST"

    # Deploy album data (images + JSON) independently.
    # --exclude=*.html: don't delete pre-rendered .html pages synced above
    # shellcheck disable=SC2086
    rsync $RSYNC_OPTS \
        --exclude=*.html \
        "$DDPHOTOS_ALBUMS_DIR/$DDPHOTOS_SITE_ID/" \
        "$AWS_APACHE":"${RSYNC_DEST}albums/"

    # Clear cache (skipped in dry-run mode)
    if [ "$DRY_RUN" = true ]; then
        echo "DRY RUN: skipping CloudFront invalidation"
    else
        aws cloudfront create-invalidation --distribution-id "$CLOUDFRONT_ID" --paths "/*"
    fi

    if [ "$SKIP_APACHE_TEST" = true ]; then
        echo "Skipping post-deploy Apache tests (--no-apache-test)"
    elif [ "$DRY_RUN" = true ]; then
        echo "DRY RUN: skipping post-deploy Apache tests"
    else
        # Wait, run test
        echo "Sleeping 5 to allow cache to clear..."
        sleep 5
        PROD_ARGS=()
        [ -n "$CONFIG_DIR" ] && PROD_ARGS+=(--config-dir "$CONFIG_DIR")
        "$SDIR/test-photos-apache.sh" "${PROD_ARGS[@]}"
    fi

    if [ "$SKIP_PLAYWRIGHT" = true ]; then
        echo "Skipping Playwright tests against production (--no-playwright)"
    elif [ "$DRY_RUN" = true ]; then
        echo "DRY RUN: skipping Playwright tests against production"
    else
        echo "Running Playwright e2e tests against production..."
        PLAYWRIGHT_BASE_URL="$VITE_SITE_URL" npx playwright test
    fi
fi