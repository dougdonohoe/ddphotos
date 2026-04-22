#!/usr/bin/env bash
# Run Playwright tests against one variant of the sample site.
#
# Usage:
#   bin/run-tests.sh [--passwords <file>] [--css <file>] [--mode dev|apache|nginx|all] [--ci]
#
# --passwords  Path to a passwords file (e.g. sample/config/passwords-all.yaml).
#              Omit for the no-password variant.
# --css        Path to a custom CSS file (e.g. sample/config/custom.css).
#              Omit for the no-CSS variant.
# --mode       Which server to test against: dev, apache, nginx, or all (default: all).
#              dev    — Vite dev server on port 5174
#              apache — static build + Docker/Apache on port 8083
#              nginx  — static build + Docker/nginx on port 8084
#              all    — dev, apache, and nginx
# --ci         Skip photogen if albums/<site-id> already exists; skip npm build if build/ exists.
#              Speeds up CI by reusing output from a prior run or step.

set -eo pipefail

PASSWORDS_FILE=""
CSS_FILE=""
MODE="all"
CI_MODE=false

usage() {
    echo "Usage: bin/run-tests.sh [--passwords <file>] [--css <file>] [--mode dev|apache|nginx|all] [--ci]"
    echo ""
    echo "Options:"
    echo "  --passwords <file>  Path to a passwords file (e.g. sample/config/passwords-all.yaml)."
    echo "                      Omit for the no-password variant."
    echo "  --css <file>        Path to a custom CSS file (e.g. sample/config/custom.css)."
    echo "                      Omit for the no-CSS variant."
    echo "  --mode <mode>       Server to test against: dev, apache, nginx, or all (default: all)."
    echo "                        dev    — Vite dev server on port 5174"
    echo "                        apache — static build + Docker/Apache on port 8083"
    echo "                        nginx  — static build + Docker/nginx on port 8084"
    echo "                        all    — dev, apache, and nginx"
    echo "  --ci                Skip photogen if albums/<site-id> exists; skip npm build if build/ exists."
    echo "  --help, -?          Show this help message and exit."
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --passwords)   PASSWORDS_FILE="$2"; shift 2 ;;
        --passwords=*) PASSWORDS_FILE="${1#*=}"; shift ;;
        --css)         CSS_FILE="$2"; shift 2 ;;
        --css=*)       CSS_FILE="${1#*=}"; shift ;;
        --mode)        MODE="$2"; shift 2 ;;
        --mode=*)      MODE="${1#*=}"; shift ;;
        --ci)          CI_MODE=true; shift ;;
        --help|-\?)    usage; exit 0 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

# cd to repo root
SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

# Node.js init: source nvm if node is not already on PATH
if ! command -v node &>/dev/null; then
    NVM_SH="${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    if [ ! -f "$NVM_SH" ]; then
        echo "Error: node not found and nvm not found at $NVM_SH" >&2
        echo "Install Node.js or nvm before running tests." >&2
        exit 1
    fi
    # shellcheck source=/dev/null
    source "$NVM_SH"
    # Activate the version specified in web/.nvmrc
    (cd web && nvm use --silent)
fi

# Resolve passwords file to absolute path
if [ -n "$PASSWORDS_FILE" ]; then
    PASSWORDS_FILE="$(cd "$(dirname "$PASSWORDS_FILE")" && pwd)/$(basename "$PASSWORDS_FILE")"
    [ -f "$PASSWORDS_FILE" ] || { echo "Error: passwords file not found: $PASSWORDS_FILE" >&2; exit 1; }
fi

# Resolve CSS file to absolute path
if [ -n "$CSS_FILE" ]; then
    CSS_FILE="$(cd "$(dirname "$CSS_FILE")" && pwd)/$(basename "$CSS_FILE")"
    [ -f "$CSS_FILE" ] || { echo "Error: CSS file not found: $CSS_FILE" >&2; exit 1; }
fi

# Derive site-id from flags.
# Convention: passwords-all.yaml -> site-id "sample-pw-all"
#             passwords-uganda.yaml -> site-id "sample-pw-uganda"
#             --css <file> -> site-id "sample-css"
#             (no flags) -> site-id "sample"
SITE_ID="sample"
PHOTOGEN_FLAGS="-config-dir sample/config -resize -index -clean -doit"
if [ -n "$PASSWORDS_FILE" ]; then
    BASENAME=$(basename "$PASSWORDS_FILE" .yaml)  # e.g. "passwords-all"
    VARIANT="${BASENAME#passwords-}"               # e.g. "all"
    SITE_ID="sample-pw-${VARIANT}"
    PHOTOGEN_FLAGS="-config-dir sample/config -resize -index -clean -passwords $PASSWORDS_FILE -site-id $SITE_ID -doit"
fi
if [ -n "$CSS_FILE" ]; then
    SITE_ID="sample-css"
    PHOTOGEN_FLAGS="-config-dir sample/config -resize -index -clean -css $CSS_FILE -site-id $SITE_ID -doit"
fi

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
if $CI_MODE && [ -d "$ALBUMS_DIR/$SITE_ID" ]; then
    echo ""
    echo "=== [--ci] Skipping photogen: $ALBUMS_DIR/$SITE_ID already exists ==="
else
    echo ""
    echo "=== Generating sample data (site-id: $SITE_ID) ==="
    # shellcheck disable=SC2086
    go run cmd/photogen/photogen.go $PHOTOGEN_FLAGS
fi

# --- helper: run Playwright against a base URL ---
run_playwright() {
    local base_url="$1"
    (
        cd web
        export PLAYWRIGHT_BASE_URL="$base_url"
        [ -n "$PASSWORDS_FILE" ] && export PLAYWRIGHT_PASSWORDS_FILE="$PASSWORDS_FILE"
        [ -n "$CSS_FILE" ] && export PLAYWRIGHT_CUSTOM_CSS="true"
        npx playwright test
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
    if $CI_MODE && [ -d "$(pwd)/build/$SITE_ID" ]; then
        echo ""
        echo "=== [apache] [--ci] Skipping npm build: build/$SITE_ID already exists ==="
    else
        echo ""
        echo "=== [apache] Building static site '$SITE_ID' ==="
        # Explicit error check: set -e is suppressed inside functions called via ||
        # (see run_apache || OVERALL_EXIT=$? below), so failures must be caught manually.
        (cd web && DDPHOTOS_ALBUMS_DIR="$ALBUMS_DIR" DDPHOTOS_SITE_ID="$SITE_ID" npm run build) || return 1
    fi

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
    if $CI_MODE && [ -d "$(pwd)/build/$SITE_ID" ]; then
        echo ""
        echo "=== [nginx] [--ci] Skipping npm build: build/$SITE_ID already exists ==="
    else
        echo ""
        echo "=== [nginx] Building static site '$SITE_ID' ==="
        (cd web && DDPHOTOS_ALBUMS_DIR="$ALBUMS_DIR" DDPHOTOS_SITE_ID="$SITE_ID" npm run build) || return 1
    fi

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
