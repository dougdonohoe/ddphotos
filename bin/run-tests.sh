#!/usr/bin/env bash
# Run Playwright tests against one variant of the sample site.
#
# Usage:
#   bin/run-tests.sh [--passwords <file>] [--css <file>] [--customization <file>] [--mode dev|apache|nginx|all] [--test <file>] [--site-id <id>]
#
# --passwords  Path to a passwords file (e.g. sample/config/passwords-all.yaml).
#              Omit for the no-password variant.
# --css        Path to a custom CSS file (e.g. sample/config/custom-example.css).
#              Omit for the no-CSS variant.
# --customization
#              Path to a customization file (e.g. sample/config/customization-album-nav.yaml).
#              Omit for the no-customization variant.
# --mode       Which server to test against: dev, apache, nginx, or all (default: all).
#              dev    — Vite dev server on port 5174
#              apache — static build + Docker/Apache on port 8083
#              nginx  — static build + Docker/nginx on port 8084
#              all    — dev, apache, and nginx
# --test       Run only the specified test file (passed directly to Playwright).
#              e.g. --test tests/privacy.spec.ts
# --site-id    Albums directory to generate into, overriding the derived one. See the
#              "Site ID" comment below for why bin/test-all.sh shares one across variants.
# --skip-video Set PLAYWRIGHT_SKIP_VIDEO, which skips web/tests/video.spec.ts. For a
#              variant whose transcoded MP4s are byte-identical to a variant that already
#              covers them; see the note above the run_variant calls in bin/test-all.sh.
#
# Note for anyone editing .github/workflows/ci.yml: with a shared --site-id this script
# rewrites albums/sample and build/sample, so the rsync and S3 deploy steps, which assume
# the plain sample site, must run *before* the step that calls bin/test-all.sh.

set -eo pipefail

PASSWORDS_FILE=""
CSS_FILE=""
CUSTOMIZATION_FILE=""
MODE="all"
SITE_ID_OVERRIDE=""
SKIP_VIDEO=false

usage() {
    echo "Usage: bin/run-tests.sh [--passwords <file>] [--css <file>] [--customization <file>] [--mode dev|apache|nginx|all] [--site-id <id>] [--skip-video]"
    echo ""
    echo "Options:"
    echo "  --passwords <file>  Path to a passwords file (e.g. sample/config/passwords-all.yaml)."
    echo "                      Omit for the no-password variant."
    echo "  --css <file>        Path to a custom CSS file (e.g. sample/config/custom-example.css)."
    echo "                      Omit for the no-CSS variant."
    echo "  --customization <file>"
    echo "                      Path to a customization file (e.g. sample/config/customization-album-nav.yaml)."
    echo "                      Omit for the no-customization variant."
    echo "  --mode <mode>       Server to test against: dev, apache, nginx, or all (default: all)."
    echo "                        dev    — Vite dev server on port 5174"
    echo "                        apache — static build + Docker/Apache on port 8083"
    echo "                        nginx  — static build + Docker/nginx on port 8084"
    echo "                        all    — dev, apache, and nginx"
    echo "  --test <file>       Run only a specific test file (e.g. tests/privacy.spec.ts)."
    echo "  --site-id <id>      Albums directory to generate into, overriding the derived one."
    echo "  --skip-video        Skip video.spec.ts (its MP4s are covered by another variant)."
    echo "  --help, -?          Show this help message and exit."
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --passwords)   PASSWORDS_FILE="$2"; shift 2 ;;
        --passwords=*) PASSWORDS_FILE="${1#*=}"; shift ;;
        --css)         CSS_FILE="$2"; shift 2 ;;
        --css=*)       CSS_FILE="${1#*=}"; shift ;;
        --customization)   CUSTOMIZATION_FILE="$2"; shift 2 ;;
        --customization=*) CUSTOMIZATION_FILE="${1#*=}"; shift ;;
        --mode)        MODE="$2"; shift 2 ;;
        --mode=*)      MODE="${1#*=}"; shift ;;
        --site-id)     SITE_ID_OVERRIDE="$2"; shift 2 ;;
        --site-id=*)   SITE_ID_OVERRIDE="${1#*=}"; shift ;;
        --skip-video)  SKIP_VIDEO=true; shift ;;
        --test)        TEST_FILTER="$2"; shift 2 ;;
        --test=*)      TEST_FILTER="${1#*=}"; shift ;;
        --help|-\?)    usage; exit 0 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

# cd to repo root
SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

# Node.js init (see bin/node-init.sh)
# shellcheck source=/dev/null
source "$SDIR/node-init.sh"

# Resolve passwords file to absolute path
if [ -n "$PASSWORDS_FILE" ]; then
    PASSWORDS_FILE="$(cd "$(dirname "$PASSWORDS_FILE")" && pwd)/$(basename "$PASSWORDS_FILE")"
    [ -f "$PASSWORDS_FILE" ] || { echo "Error: passwords file not found: $PASSWORDS_FILE" >&2; exit 1; }
fi

# Resolve customization file to absolute path
if [ -n "$CUSTOMIZATION_FILE" ]; then
    CUSTOMIZATION_FILE="$(cd "$(dirname "$CUSTOMIZATION_FILE")" && pwd)/$(basename "$CUSTOMIZATION_FILE")"
    [ -f "$CUSTOMIZATION_FILE" ] || { echo "Error: customization file not found: $CUSTOMIZATION_FILE" >&2; exit 1; }
fi

# Resolve CSS file to absolute path
if [ -n "$CSS_FILE" ]; then
    CSS_FILE="$(cd "$(dirname "$CSS_FILE")" && pwd)/$(basename "$CSS_FILE")"
    [ -f "$CSS_FILE" ] || { echo "Error: CSS file not found: $CSS_FILE" >&2; exit 1; }
fi

# --- Site ID ---
#
# The albums directory this variant's media lands in. Variants that encrypt the same set
# of albums can share one, which is what bin/test-all.sh does via --site-id, and it is the
# single biggest saving in CI: a shared directory already holds every WebP and MP4, so
# photogen skips the resize and the ffmpeg transcode entirely and only rewrites the JSON.
#
# Safe to share because media bytes never depend on encryption, CSS or customization.
# What encryption changes is output *filenames*: Config.PhotoOutputName
# (pkg/photogen/config.go) HMACs the name only for albums that actually have a password,
# so two variants encrypting the same albums produce identically named, byte-identical
# media. It stays safe across runs because -clean normalizes the directory to the current
# variant every time: everything photogen writes is registered with TrackFile, and
# CleanOutputDir removes whatever the previous variant left behind, top-level files
# (custom.css, albums.enc.json) included. Anything ever written *without* a TrackFile call
# would survive into the next variant and be served, which is the one way this breaks.
#
# The derivation below is the fallback for a direct invocation, where a self-describing
# directory is more useful than a shared one:
#   passwords-all.yaml -> "sample-pw-all", passwords-uganda.yaml -> "sample-pw-uganda",
#   --css -> "sample-css", customization-album-nav.yaml -> "sample-album-nav",
#   no flags -> "sample"
SITE_ID="sample"
PHOTOGEN_FLAGS=(-config-dir sample/config -resize -index -clean -doit)
if [ -n "$PASSWORDS_FILE" ]; then
    BASENAME=$(basename "$PASSWORDS_FILE" .yaml)  # e.g. "passwords-all"
    SITE_ID="sample-pw-${BASENAME#passwords-}"     # e.g. "sample-pw-all"
    PHOTOGEN_FLAGS+=(-passwords "$PASSWORDS_FILE")
fi
if [ -n "$CSS_FILE" ]; then
    SITE_ID="sample-css"
    PHOTOGEN_FLAGS+=(-css "$CSS_FILE")
fi
if [ -n "$CUSTOMIZATION_FILE" ]; then
    BASENAME=$(basename "$CUSTOMIZATION_FILE" .yaml)  # e.g. "customization-album-nav"
    SITE_ID="sample-${BASENAME#customization-}"        # e.g. "sample-album-nav"
    PHOTOGEN_FLAGS+=(-customization "$CUSTOMIZATION_FILE")
fi
if [ -n "$SITE_ID_OVERRIDE" ]; then
    SITE_ID="$SITE_ID_OVERRIDE"
fi
PHOTOGEN_FLAGS+=(-site-id "$SITE_ID")

ALBUMS_DIR="$(pwd)/albums"

DEV_PORT=5174
DOCKER_PORT=8083
DOCKER_PORT_NGINX=8084
DOCKER_CONTAINER_APACHE="ddphotos-playwright-test-apache"
DOCKER_CONTAINER_NGINX="ddphotos-playwright-test-nginx"
DEV_PID=""

# Cleanup: kill dev server and stop Docker container on exit
# shellcheck disable=SC2317
cleanup() {
    if [ -n "$DEV_PID" ]; then kill "$DEV_PID" 2>/dev/null || true; fi
    docker stop "$DOCKER_CONTAINER_APACHE" 2>/dev/null || true
    docker stop "$DOCKER_CONTAINER_NGINX" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# --- photogen (done once, shared across all modes) ---
#
# Always run, even when the site directory already exists. It is cheap when it does (every
# output is present, so photogen stats and skips it), and skipping it outright would leave
# the previous variant's config.json, albums.json and custom.css in place, quietly testing
# the wrong site.
echo ""
echo "=== Generating sample data (site-id: $SITE_ID) ==="
go run cmd/photogen/photogen.go "${PHOTOGEN_FLAGS[@]}"

# --- helper: run Playwright against a base URL ---
run_playwright() {
    local base_url="$1"
    (
        cd web
        export PLAYWRIGHT_BASE_URL="$base_url"
        [ -n "$PASSWORDS_FILE" ] && export PLAYWRIGHT_PASSWORDS_FILE="$PASSWORDS_FILE"
        [ -n "$CSS_FILE" ] && export PLAYWRIGHT_CUSTOM_CSS="true"
        [ "$SKIP_VIDEO" = true ] && export PLAYWRIGHT_SKIP_VIDEO="true"
        npx playwright test ${TEST_FILTER:+"$TEST_FILTER"}
    )
}

# --- helper: wait for HTTP endpoint to respond ---
wait_for_http() {
    local url="$1"
    local label="$2"
    local tries=0
    echo "Waiting for $label..."
    until curl -s -o /dev/null "$url" 2>/dev/null; do
        sleep 1
        tries=$((tries + 1))
        if [ "$tries" -ge 30 ]; then
            echo "Error: $label did not become ready in time" >&2
            return 1
        fi
    done
}

# --- dev mode ---
run_dev() {
    echo ""
    echo "=== [dev] Starting Vite dev server for site '$SITE_ID' on port $DEV_PORT ==="
    (cd web && DDPHOTOS_ALBUMS_DIR="$ALBUMS_DIR" DDPHOTOS_SITE_ID="$SITE_ID" npx vite dev --port "$DEV_PORT" --clearScreen false) &
    DEV_PID=$!

    wait_for_http "http://localhost:$DEV_PORT" "dev server"

    local exit_code=0
    run_playwright "http://localhost:$DEV_PORT" || exit_code=$?

    kill "$DEV_PID" 2>/dev/null || true
    wait "$DEV_PID" 2>/dev/null || true
    DEV_PID=""

    return $exit_code
}

# --- apache mode ---
run_apache() {
    # Always rebuild, for the same reason photogen always runs: hooks.server.ts reads
    # /albums/*.json off disk during pre-rendering and bakes it into the HTML, so the
    # static build belongs to one variant even when the site ID is shared.
    echo ""
    echo "=== [apache] Building static site '$SITE_ID' ==="
    # Explicit error check: set -e is suppressed inside functions called via ||
    # (see run_apache || OVERALL_EXIT=$? below), so failures must be caught manually.
    (cd web && DDPHOTOS_ALBUMS_DIR="$ALBUMS_DIR" DDPHOTOS_SITE_ID="$SITE_ID" npm run build) || return 1

    # Build Docker image if missing or stale
    "$SDIR/docker-check.sh" --build || return 1

    echo "=== [apache] Starting Apache for site '$SITE_ID' on port $DOCKER_PORT ==="
    docker run -d --rm --name "$DOCKER_CONTAINER_APACHE" -p "$DOCKER_PORT:80" \
        -e DDPHOTOS_SITE_ID="$SITE_ID" \
        -v "$(pwd)/build":/build:ro \
        -v "$ALBUMS_DIR/$SITE_ID":/albums:ro \
        photos-apache

    wait_for_http "http://localhost:$DOCKER_PORT" "Apache"

    local exit_code=0
    run_playwright "http://localhost:$DOCKER_PORT" || exit_code=$?

    docker stop "$DOCKER_CONTAINER_APACHE" 2>/dev/null || true

    return $exit_code
}

# --- nginx mode ---
run_nginx() {
    # Always rebuild; see run_apache.
    echo ""
    echo "=== [nginx] Building static site '$SITE_ID' ==="
    (cd web && DDPHOTOS_ALBUMS_DIR="$ALBUMS_DIR" DDPHOTOS_SITE_ID="$SITE_ID" npm run build) || return 1

    # Build Docker image if missing or stale
    "$SDIR/docker-check.sh" --server nginx --build || return 1

    echo "=== [nginx] Starting nginx for site '$SITE_ID' on port $DOCKER_PORT_NGINX ==="
    docker run -d --rm --name "$DOCKER_CONTAINER_NGINX" -p "$DOCKER_PORT_NGINX:80" \
        -e DDPHOTOS_SITE_ID="$SITE_ID" \
        -v "$(pwd)/build":/build:ro \
        -v "$ALBUMS_DIR/$SITE_ID":/albums:ro \
        photos-nginx

    wait_for_http "http://localhost:$DOCKER_PORT_NGINX" "nginx"

    local exit_code=0
    run_playwright "http://localhost:$DOCKER_PORT_NGINX" || exit_code=$?

    docker stop "$DOCKER_CONTAINER_NGINX" 2>/dev/null || true

    return $exit_code
}

# --- run selected modes ---
OVERALL_EXIT=0

if [[ "$MODE" == "dev" || "$MODE" == "all" ]]; then
    run_dev || OVERALL_EXIT=$?
fi

if [[ "$MODE" == "apache" || "$MODE" == "all" ]]; then
    run_apache || OVERALL_EXIT=$?
fi

if [[ "$MODE" == "nginx" || "$MODE" == "all" ]]; then
    run_nginx || OVERALL_EXIT=$?
fi

exit $OVERALL_EXIT
