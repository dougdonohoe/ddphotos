#!/usr/bin/env bash
# Run Playwright tests against one variant of the sample site.
#
# Usage:
#   bin/run-tests.sh [--passwords <file>] [--css <file>] [--mode dev|apache|both]
#
# --passwords  Path to a passwords file (e.g. sample/config/passwords-all.yaml).
#              Omit for the no-password variant.
# --css        Path to a custom CSS file (e.g. sample/config/custom.css).
#              Omit for the no-CSS variant.
# --mode       Which server to test against: dev, apache, or both (default: both).
#              dev   — Vite dev server on port 5174
#              apache — static build + Docker/Apache on port 8083
#              both  — dev first, then apache

set -eo pipefail

PASSWORDS_FILE=""
CSS_FILE=""
MODE="both"

usage() {
    echo "Usage: bin/run-tests.sh [--passwords <file>] [--css <file>] [--mode dev|apache|both]"
    echo ""
    echo "Options:"
    echo "  --passwords <file>  Path to a passwords file (e.g. sample/config/passwords-all.yaml)."
    echo "                      Omit for the no-password variant."
    echo "  --css <file>        Path to a custom CSS file (e.g. sample/config/custom.css)."
    echo "                      Omit for the no-CSS variant."
    echo "  --mode <mode>       Server to test against: dev, apache, or both (default: both)."
    echo "                        dev    — Vite dev server on port 5174"
    echo "                        apache — static build + Docker/Apache on port 8083"
    echo "                        both   — dev first, then apache"
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

# Derive site-id and symlink target from flags.
# Convention: passwords-all.yaml -> site-id "sample-pw-all"
#             passwords-uganda.yaml -> site-id "sample-pw-uganda"
#             --css <file> -> site-id "sample-css"
#             (no flags) -> site-id "sample"
SITE_ID="sample"
SYMLINK_TARGET="../albums/sample"
PHOTOGEN_FLAGS="-config-dir sample/config -resize -index -clean -doit"
if [ -n "$PASSWORDS_FILE" ]; then
    BASENAME=$(basename "$PASSWORDS_FILE" .yaml)  # e.g. "passwords-all"
    VARIANT="${BASENAME#passwords-}"               # e.g. "all"
    SITE_ID="sample-pw-${VARIANT}"
    SYMLINK_TARGET="../albums/sample-pw-${VARIANT}"
    PHOTOGEN_FLAGS="-config-dir sample/config -resize -index -clean -passwords $PASSWORDS_FILE -site-id $SITE_ID -doit"
fi
if [ -n "$CSS_FILE" ]; then
    SITE_ID="sample-css"
    SYMLINK_TARGET="../albums/sample-css"
    PHOTOGEN_FLAGS="-config-dir sample/config -resize -index -clean -css $CSS_FILE -site-id $SITE_ID -doit"
fi

SITE_ENV="$(pwd)/sample/config/site.env"
DEV_PORT=5174
DOCKER_PORT=8083
DOCKER_CONTAINER="ddphotos-playwright-test"
DEV_PID=""

# Cleanup: kill dev server and stop Docker container on exit
cleanup() {
    [ -n "$DEV_PID" ] && kill "$DEV_PID" 2>/dev/null || true
    docker stop "$DOCKER_CONTAINER" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# --- photogen + symlink (done once, shared by both dev and apache runs) ---
echo ""
echo "=== Generating sample data (site-id: $SITE_ID) ==="
# shellcheck disable=SC2086
go run cmd/photogen/photogen.go $PHOTOGEN_FLAGS

echo "=== Setting symlink: web/static/albums -> $SYMLINK_TARGET ==="
ln -sfn "$SYMLINK_TARGET" web/static/albums

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
    echo "=== [dev] Starting Vite dev server on port $DEV_PORT ==="
    (cd web && SITE_ENV="$SITE_ENV" npx vite dev --port "$DEV_PORT" --clearScreen false) &
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
    echo ""
    echo "=== [apache] Building static site ==="
    # Explicit error check: set -e is suppressed inside functions called via ||
    # (see run_apache || OVERALL_EXIT=$? below), so failures must be caught manually.
    (cd web && SITE_ENV="$SITE_ENV" npm run build) || return 1

    # Build Docker image if it doesn't already exist
    if ! docker image inspect photos-apache &>/dev/null 2>&1; then
        echo "=== [apache] Building Docker image ==="
        docker build -t photos-apache web/ || return 1
    fi

    echo "=== [apache] Starting Apache on port $DOCKER_PORT ==="
    docker run -d --rm --name "$DOCKER_CONTAINER" -p "$DOCKER_PORT:80" \
        -v "$(pwd)/web":/usr/local/apache2/htdocs:ro photos-apache

    wait_for_http "http://localhost:$DOCKER_PORT" "Apache"

    local exit_code=0
    run_playwright "http://localhost:$DOCKER_PORT" || exit_code=$?

    docker stop "$DOCKER_CONTAINER" 2>/dev/null || true

    return $exit_code
}

# --- run selected modes ---
OVERALL_EXIT=0

if [[ "$MODE" == "dev" || "$MODE" == "both" ]]; then
    run_dev || OVERALL_EXIT=$?
fi

if [[ "$MODE" == "apache" || "$MODE" == "both" ]]; then
    run_apache || OVERALL_EXIT=$?
fi

exit $OVERALL_EXIT
