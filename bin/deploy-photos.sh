#!/usr/bin/env bash

set -eo pipefail

echo "Deploy starting with $* ..."

# Parse flags
SKIP_PHOTOGEN=${NO_PHOTOGEN:+true}; SKIP_PHOTOGEN=${SKIP_PHOTOGEN:-false}
SKIP_BUILD=false
SKIP_PRE_DEPLOY=false
SKIP_RSYNC=false
SKIP_PLAYWRIGHT=false
SKIP_SERVER_TEST=false
DRY_RUN=false
S3_MODE=false
CONFIG_DIR="config"
SITE_ENV_ARG=""
SITE_ID_ARG=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-photogen)       SKIP_PHOTOGEN=true; shift ;;
        --no-build)          SKIP_BUILD=true; shift ;;
        --no-pre-deploy-tests) SKIP_PRE_DEPLOY=true; shift ;;
        --no-rsync)          SKIP_RSYNC=true; shift ;;
        --no-playwright)     SKIP_PLAYWRIGHT=true; shift ;;
        --no-server-test)    SKIP_SERVER_TEST=true; shift ;;
        --dry-run)           DRY_RUN=true; shift ;;
        --s3)                S3_MODE=true; shift ;;
        --config-dir)        CONFIG_DIR="$2"; shift 2 ;;
        --config-dir=*)      CONFIG_DIR="${1#*=}"; shift ;;
        --site-env)          SITE_ENV_ARG="$2"; shift 2 ;;
        --site-env=*)        SITE_ENV_ARG="${1#*=}"; shift ;;
        --site-id)           SITE_ID_ARG="$2"; shift 2 ;;
        --site-id=*)         SITE_ID_ARG="${1#*=}"; shift ;;
        *)
            echo "Unknown flag: $1" >&2
            echo "" >&2
            echo "Usage: deploy-photos.sh [options]" >&2
            echo "" >&2
            echo "  --dry-run              Show what would be deployed without transferring files" >&2
            echo "  --s3                   Deploy to S3 (default: rsync; auto-set if S3_BUCKET in site.env)" >&2
            echo "  --config-dir DIR       Config directory (default: config/)" >&2
            echo "  --site-env FILE        Path to site.env (default: config-dir/site.env)" >&2
            echo "  --site-id ID           Site ID (overrides albums.yaml)" >&2
            echo "  --no-photogen          Skip photogen step" >&2
            echo "  --no-build             Skip build step" >&2
            echo "  --no-pre-deploy-tests  Skip pre-deploy server and Playwright tests" >&2
            echo "  --no-rsync             Skip rsync/S3 sync and post-deploy steps" >&2
            echo "  --no-playwright        Skip Playwright tests (pre- and post-deploy)" >&2
            echo "  --no-server-test       Skip server tests (pre- and post-deploy)" >&2
            exit 1
            ;;
    esac
done

# cd to root of repo (REPO_ROOT may be pre-set by caller, e.g. docker/do-deploy.sh)
SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
if [ -z "$REPO_ROOT" ]; then
    cd "$SDIR/.."
    REPO_ROOT="$(pwd)"
fi
cd "$REPO_ROOT"

# Verify CONFIG_DIR exists
[ -d "$CONFIG_DIR" ] || { echo "Error: config dir '$CONFIG_DIR' not found" >&2; exit 1; }

# Resolve CONFIG_DIR and SITE_ENV_ARG to absolute paths (relative paths break after subsequent cd's)
CONFIG_DIR="$(cd "$CONFIG_DIR" && pwd)"
[ -n "$SITE_ENV_ARG" ] && SITE_ENV_ARG="$(cd "$(dirname "$SITE_ENV_ARG")" && pwd)/$(basename "$SITE_ENV_ARG")"

# Resolve and source site.env: --site-env takes priority, then $CONFIG_DIR/site.env
if [ -n "$SITE_ENV_ARG" ]; then
    SITE_ENV="$SITE_ENV_ARG"
else
    SITE_ENV="$CONFIG_DIR/site.env"
fi
[ -f "$SITE_ENV" ] || { echo "Error: site environment file '$SITE_ENV' not found" >&2; exit 1; }
source "$SITE_ENV"

# Determine <site-id>
# --site-id takes highest precedence over all other sources
# Fall back to albums.yaml settings.id if SITE_ID not yet set
# NOTE: we explicitly *do not* use DDPHOTOS_SITE_ID env var.  It's either --site-id or from albums.yaml
[ -n "$SITE_ID_ARG" ] && SITE_ID="$SITE_ID_ARG"
if [ -z "$SITE_ID" ]; then
    ALBUMS_YAML="$CONFIG_DIR/albums.yaml"
    if [ -f "$ALBUMS_YAML" ]; then
        SITE_ID=$(awk '/^settings:/{f=1} f && /[[:space:]]id:/{gsub(/.*id:[[:space:]]*/,""); print; exit}' "$ALBUMS_YAML")
    fi
fi

# Load defaults.env as a fallback for vars not already set (e.g. DDPHOTOS_ALBUMS_DIR)
DEFAULTS_ENV="$(dirname "$SDIR")/config/defaults.env"
if [ -f "$DEFAULTS_ENV" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
        [[ "$line" =~ ^#|^$ ]] && continue
        key="${line%%=*}"
        val="${line#*=}"
        [ -z "${!key+x}" ] && export "$key"="$val"
    done < "$DEFAULTS_ENV"
fi

# These must be set:
#   DDPHOTOS_ALBUMS_DIR from environment or defaults.env
#   SITE_ID from --site-id or albums.yaml
[ -n "$DDPHOTOS_ALBUMS_DIR" ] || { echo "Error: DDPHOTOS_ALBUMS_DIR not set (not in environment or config/defaults.env)" >&2; exit 1; }
[ -n "$SITE_ID" ]    || { echo "Error: <site id> not set (not in --site-id or $CONFIG_DIR/albums.yaml)" >&2; exit 1; }

# Resolve DDPHOTOS_ALBUMS_DIR to absolute path
DDPHOTOS_ALBUMS_DIR="$(cd "$DDPHOTOS_ALBUMS_DIR" && pwd)"

# These must be set in site.env — guard early so a missing value can't
# cause rsync --delete to target the wrong (or empty) remote path.
if [ "$S3_MODE" = true ]; then
    [ -n "$S3_BUCKET" ] || { echo "Error: S3_BUCKET not set in $SITE_ENV" >&2; exit 1; }
else
    [ -n "$RSYNC_HOST" ]  || { echo "Error: RSYNC_HOST not set in $SITE_ENV" >&2; exit 1; }
    [ -n "$RSYNC_DEST" ]  || { echo "Error: RSYNC_DEST not set in $SITE_ENV" >&2; exit 1; }
    # Ensure RSYNC_DEST ends with / so rsync targets an explicit directory path,
    # never a bare or empty string that could default to the remote home directory.
    [[ "$RSYNC_DEST" == */ ]] || RSYNC_DEST="${RSYNC_DEST}/"
fi

###
### Begin deploy actions here
###

echo "Deploying $SITE_ID ..."
echo "  Config dir: $CONFIG_DIR"
echo "  Site env:   $SITE_ENV"
echo "  Albums dir: $DDPHOTOS_ALBUMS_DIR"
echo

# Generate photos
if [ "$SKIP_PHOTOGEN" = true ]; then
    echo "Skipping photogen (--no-photogen)"
else
    PHOTOGEN_ARGS=(-site-id "$SITE_ID" -resize -index -clean -doit)
    [ -n "$CONFIG_DIR" ] && PHOTOGEN_ARGS=(--config-dir "$CONFIG_DIR" "${PHOTOGEN_ARGS[@]}")
    go run ./cmd/photogen "${PHOTOGEN_ARGS[@]}"
fi

# Read site URL from config.json (written by photogen)
CONFIG_JSON="$DDPHOTOS_ALBUMS_DIR/$SITE_ID/config.json"
[ -f "$CONFIG_JSON" ] || { echo "Error: $CONFIG_JSON not found — run photogen first" >&2; exit 1; }
SITE_URL=$(python3 -c "import json; print(json.load(open('$CONFIG_JSON'))['siteUrl'])")
[ -n "$SITE_URL" ] || { echo "Error: siteUrl not found in $CONFIG_JSON" >&2; exit 1; }
DEFAULT_URL="https://your-ddphotos.example.com"
[ "$SITE_URL" = "$DEFAULT_URL" ] && {
    echo "Error: 'site_url' in albums.yaml is still the default '$DEFAULT_URL'." >&2
    echo "       Set it to your actual site URL before deploying or pass -site-url [ur] to photogen" >&2
    exit 1
}

# Build static site
if [ "$SKIP_BUILD" = true ]; then
    echo "Skipping build (--no-build)"
else
    cd web
    NVM_SH="${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    if ! command -v node &>/dev/null; then
        # shellcheck source=/dev/null
        source "$NVM_SH"
    fi
    DDPHOTOS_ALBUMS_DIR="$DDPHOTOS_ALBUMS_DIR" DDPHOTOS_SITE_ID="$SITE_ID" npm run build
fi

# Docker cleanup (used in tests)
DOCKER_STARTED=false
_docker_cleanup() {
    if [ "$DOCKER_STARTED" = true ]; then
        echo "Stopping local Docker container..."
        docker stop "$(docker ps -q --filter publish=8080)" 2>/dev/null || true
    fi
}
trap _docker_cleanup EXIT

# Pre-deploy: run local Docker server tests and Playwright (rsync mode only; skipped for S3).
_pre_deploy() {

    echo
    echo "Starting pre-deploy ..."

    if [ "$S3_MODE" = true ]; then
        echo "Skipping pre-deploy local tests (--s3 mode)"
    elif [ "$SKIP_PRE_DEPLOY" = true ]; then
        echo "Skipping pre-deploy tests (--no-pre-deploy-tests)"
    elif [ "$SKIP_SERVER_TEST" = true ]; then
        echo "Skipping local server tests (--no-server-test)"
    else
        # Verify Docker image is current before running
        "$SDIR/docker-check.sh"

        DOCKER_RUNNING=$(docker ps -q --filter publish=8080)
        if [ -n "$DOCKER_RUNNING" ]; then
            echo "Docker already running on port 8080, using existing container..."
        else
            echo "Starting local Docker container for testing..."
            docker run -d --rm -p 8080:80 \
                -e DDPHOTOS_SITE_ID="$SITE_ID" \
                -v "$REPO_ROOT/build":/build:ro \
                -v "$DDPHOTOS_ALBUMS_DIR/$SITE_ID":/albums:ro \
                photos-apache > /dev/null
            DOCKER_STARTED=true
            sleep 1
        fi

        echo "Running local server tests..."
        TEST_ARGS=(--local 8080)
        "$SDIR/test-photos-server.sh" "${TEST_ARGS[@]}"

        if [ "$SKIP_PLAYWRIGHT" = true ]; then
            echo "Skipping pre-deploy Playwright tests (--no-playwright)"
        else
            echo "Running pre-deploy Playwright e2e tests..."
            npx playwright test
        fi
    fi
}

# Post-deploy: invalidate CloudFront, run server tests, run Playwright against production.
# Argument: "s3" to pass --s3 to test-photos-server.sh (S3 mode), empty for rsync mode.
_post_deploy() {
    local mode="${1:-}"

    echo
    echo "Starting post-deploy ..."

    # Clear cache (skipped in dry-run mode or if CLOUDFRONT_ID is not set)
    if [ "$DRY_RUN" = true ]; then
        echo "DRY RUN: skipping CloudFront invalidation"
    elif [ -z "$CLOUDFRONT_ID" ]; then
        echo "Skipping CloudFront invalidation (CLOUDFRONT_ID not set in $SITE_ENV)"
    else
        aws cloudfront create-invalidation --distribution-id "$CLOUDFRONT_ID" --paths "/*" \
            --query 'Invalidation.Id' --output text
    fi

    if [ "$SKIP_SERVER_TEST" = true ]; then
        echo "Skipping post-deploy server tests (--no-server-test)"
    elif [ "$DRY_RUN" = true ]; then
        echo "DRY RUN: skipping post-deploy server tests"
    else
        if [ -n "$CLOUDFRONT_ID" ]; then
            echo "Sleeping 5 to allow CloudFront cache to clear..."
            sleep 5
        fi
        PROD_ARGS=(--remote "$SITE_URL")
        [ "$mode" = "s3" ] && PROD_ARGS=(--s3 "${PROD_ARGS[@]}")
        "$SDIR/test-photos-server.sh" "${PROD_ARGS[@]}"
    fi

    if [ "$SKIP_PLAYWRIGHT" = true ]; then
        echo "Skipping Playwright tests against production (--no-playwright)"
    elif [ "$DRY_RUN" = true ]; then
        echo "DRY RUN: skipping Playwright tests against production"
    else
        echo "Running Playwright e2e tests against production $SITE_URL..."
        PLAYWRIGHT_BASE_URL="$SITE_URL" npx playwright test
    fi
}

##
## Deployment
##

_pre_deploy

if [ "$SKIP_RSYNC" = true ]; then
    echo "Skipping deploy, CloudFront invalidation, and post-deploy tests (--no-rsync)"
elif [ "$S3_MODE" = true ]; then
    [ "$DRY_RUN" = true ] && echo "=== DRY RUN: aws s3 sync will not transfer any files ==="

    S3_SYNC_OPTS=(--delete)
    [ "$DRY_RUN" = true ] && S3_SYNC_OPTS+=(--dryrun)

    # Deploy app files + pre-rendered album HTML/JSON.
    # Pass 1: sync web build, protecting albums/ image/JSON data (managed by Pass 2 below).
    # --exclude "albums/*" prevents uploading or deleting album images/JSON from S3.
    # --include "albums/*.html" re-includes pre-rendered SvelteKit album pages (last rule wins).
    aws s3 sync "$REPO_ROOT/build/$SITE_ID/" "s3://$S3_BUCKET/" \
        "${S3_SYNC_OPTS[@]}" --exclude "albums/*" --include "albums/*.html"

    # Deploy album data — two passes to set different Cache-Control headers.
    #
    # Pass 2a: JSON, XML, JPEG covers — must revalidate on every request (content can change
    #   in-place, e.g. cover.jpg or index.enc.json when the encryption key changes).
    # No --size-only: JSON files always get a fresh timestamp when photogen runs, so
    #   default size+timestamp comparison reliably detects changes. --size-only would
    #   silently skip re-encrypted JSON files since AES-GCM output size is key-independent.
    # --exclude=*.html: don't delete pre-rendered .html pages synced above.
    aws s3 sync "$DDPHOTOS_ALBUMS_DIR/$SITE_ID/" "s3://$S3_BUCKET/albums/" \
        "${S3_SYNC_OPTS[@]}" --exclude "*.html" --exclude "*.webp" \
        --cache-control "no-cache"

    # Pass 2b: WebP photos — immutable; photogen gives them a deterministic UUID name
    #   derived from HMAC(key, filename), so key rotation renames all files.
    #   photogen skips existing WebP files (preserving timestamp), so unchanged files are
    #   never re-uploaded. Regenerated files (after manual delete) get a new timestamp.
    aws s3 sync "$DDPHOTOS_ALBUMS_DIR/$SITE_ID/" "s3://$S3_BUCKET/albums/" \
        "${S3_SYNC_OPTS[@]}" \
        --exclude "*" --include "*.webp" \
        --cache-control "max-age=31536000,immutable"

    _post_deploy s3
else
    RSYNC_OPTS=(-avz --checksum --delete)
    RSYNC_OPTS_ALBUMS=(-avz --delete)
    [ "$DRY_RUN" = true ] && RSYNC_OPTS+=(--dry-run)
    [ "$DRY_RUN" = true ] && RSYNC_OPTS_ALBUMS+=(--dry-run)

    [ "$DRY_RUN" = true ] && echo "=== DRY RUN: rsync will not transfer any files ==="

    # Deploy app files + pre-rendered album HTML/JSON.
    # --checksum: Vite resets timestamps on build output files every build, so size+time
    #   is not a reliable change signal; content comparison is needed.
    # --filter='protect albums/**': prevent --delete from touching albums/ content (hero.jpg,
    #   sitemap.xml, images, JSON) — those files are managed by the second rsync pass below.
    #   Note: 'protect' only suppresses deletion; it still transfers new files from the source.
    rsync "${RSYNC_OPTS[@]}" \
        --filter='protect albums/**' \
        "$REPO_ROOT/build/$SITE_ID/" "$RSYNC_HOST":"$RSYNC_DEST"

    # Deploy album data (images + JSON) independently.
    # No --checksum: photogen preserves timestamps on existing files, so size+time is a
    #   reliable change signal and content comparison is unnecessary overhead.
    # --exclude=*.html: don't delete pre-rendered .html pages synced above
    rsync "${RSYNC_OPTS_ALBUMS[@]}" \
        "--exclude=*.html" \
        "$DDPHOTOS_ALBUMS_DIR/$SITE_ID/" \
        "$RSYNC_HOST":"${RSYNC_DEST}albums/"

    _post_deploy
fi

echo
echo "Deploy done."
echo