#!/usr/bin/env bash
# End-to-end test of the ddphotos Docker workflow.
#
# Usage:
#   bin/docker-test.sh              # build image then run all tests
#   bin/docker-test.sh --no-build   # skip 'make docker-build' (reuse existing image)

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE="ddphotos"
SITE_ID="my-photos"   # site-id created by 'init' template
RUN_PORT=5173          # Vite host port; matches container default to keep port-mapping consistent
SERVE_PORT=8090        # Apache host port (maps to container port 80; avoids conflicts with run-tests.sh)
DO_BUILD=true
TEST_DIR=""
TEST_DIR2=""
RUN_PID=""
SERVE_PID=""

# --- flags ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-build) DO_BUILD=false; shift ;;
        --help|-\?) echo "Usage: bin/docker-test.sh [--no-build]"; exit 0 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

# --- helpers ---
step()  { echo; echo "=== $* ==="; }
pass()  { echo "  PASS: $*"; }
fail()  { echo "  FAIL: $*" >&2; exit 1; }

cleanup() {
    [ -n "$RUN_PID" ]   && kill "$RUN_PID"   2>/dev/null || true
    [ -n "$SERVE_PID" ] && kill "$SERVE_PID" 2>/dev/null || true
    # Belt-and-suspenders: stop any containers using the local image still running on our ports
    docker ps --filter publish="$RUN_PORT"   -q | xargs docker stop &>/dev/null || true
    docker ps --filter publish="$SERVE_PORT" -q | xargs docker stop &>/dev/null || true
    # Docker creates root-owned files in TEST_DIRs; clear them via Docker before removing the dir
    if [ -n "$TEST_DIR" ]; then
        docker run --rm --entrypoint /bin/sh -v "$TEST_DIR":/target "$IMAGE" \
            -c 'find /target -mindepth 1 -delete' 2>/dev/null || true
        /bin/rm -rf "$TEST_DIR"
    fi
    if [ -n "$TEST_DIR2" ]; then
        docker run --rm --entrypoint /bin/sh -v "$TEST_DIR2":/target "$IMAGE" \
            -c 'find /target -mindepth 1 -delete' 2>/dev/null || true
        /bin/rm -rf "$TEST_DIR2"
    fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# Source nvm if node not on PATH
if ! command -v node &>/dev/null; then
    NVM_SH="${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    [ -f "$NVM_SH" ] || { echo "Error: node not found; install Node.js or nvm" >&2; exit 1; }
    # shellcheck source=/dev/null
    source "$NVM_SH"
fi

run_playwright() {
    local base_url="$1" passwords_file="${2:-}"
    (
        cd "$REPO_ROOT/web"
        export PLAYWRIGHT_BASE_URL="$base_url"
        export PLAYWRIGHT_IGNORE_CUSTOM_CSS=1
        [ -n "$passwords_file" ] && export PLAYWRIGHT_PASSWORDS_FILE="$passwords_file"
        npx playwright test
    )
}

wait_for_http() {
    local url="$1" label="$2" tries=0
    echo "  Waiting for $label..."
    until curl -s -o /dev/null "$url" 2>/dev/null; do
        sleep 1
        tries=$((tries + 1))
        if [ "$tries" -ge 60 ]; then echo "Error: $label did not become ready" >&2; return 1; fi
    done
}

cd "$REPO_ROOT"

# ── 1. Build image ─────────────────────────────────────────────────────────────
if $DO_BUILD; then
    step "Building Docker image"
    make docker-build
else
    step "Skipping Docker build (--no-build)"
fi

# ── 2. Init ────────────────────────────────────────────────────────────────────
step "Init"
TEST_DIR=$(mktemp -d)
chmod 755 "$TEST_DIR"
docker run --rm -v "$TEST_DIR":/ddphotos "$IMAGE" init
[ -x "$TEST_DIR/ddphotos" ]              || fail "ddphotos script not installed"
[ -f "$TEST_DIR/config/albums.yaml" ]    || fail "config/albums.yaml not created"
[ -f "$TEST_DIR/config/passwords.yaml" ] || fail "config/passwords.yaml not created"
[ -f "$TEST_DIR/config/site.env" ]       || fail "config/site.env not created"
[ -f "$TEST_DIR/config/passwords.yaml" ] || fail "config/passwords.yaml not created"
pass "ddphotos script and config created at $TEST_DIR"

PASSWORDS_FILE="$TEST_DIR/config/passwords.yaml"

# ── 3. Photogen ────────────────────────────────────────────────────────────────
step "Photogen"
"$TEST_DIR/ddphotos" photogen
[ -d "$TEST_DIR/albums/$SITE_ID" ] || fail "albums/$SITE_ID not created"
ALBUM_COUNT=$(find "$TEST_DIR/albums/$SITE_ID" -maxdepth 1 -mindepth 1 -type d | wc -l | tr -d ' ')
pass "albums/$SITE_ID created ($ALBUM_COUNT albums)"

# ── 4. Run (Vite dev server) + Playwright ─────────────────────────────────────
step "Run — Vite dev server on port $RUN_PORT"
RUN_PORT="$RUN_PORT" "$TEST_DIR/ddphotos" --non-interactive run &
RUN_PID=$!
wait_for_http "http://localhost:$RUN_PORT" "Vite dev server"
run_playwright "http://localhost:$RUN_PORT" "$PASSWORDS_FILE"
kill "$RUN_PID" 2>/dev/null || true; wait "$RUN_PID" 2>/dev/null || true; RUN_PID=""
pass "run + Playwright OK"

# ── 5. Build ───────────────────────────────────────────────────────────────────
step "Build"
"$TEST_DIR/ddphotos" build
[ -d "$TEST_DIR/build/$SITE_ID" ] || fail "build/$SITE_ID not created"
pass "build/$SITE_ID created"

# ── 6. Serve (Apache) + Playwright + test-photos-server.sh ────────────────────
step "Serve — Apache on port $SERVE_PORT"
SERVE_PORT="$SERVE_PORT" "$TEST_DIR/ddphotos" --non-interactive serve &
SERVE_PID=$!
wait_for_http "http://localhost:$SERVE_PORT" "Apache"
curl -s "http://localhost:$SERVE_PORT/albums/config.json" # sanity check before tests run
"$SCRIPT_DIR/test-photos-server.sh" --local "$SERVE_PORT"
run_playwright "http://localhost:$SERVE_PORT" "$PASSWORDS_FILE"
kill "$SERVE_PID" 2>/dev/null || true; wait "$SERVE_PID" 2>/dev/null || true; SERVE_PID=""
pass "serve + Playwright + test-photos-server.sh OK"

# ── 7. Export (symlink mode) ───────────────────────────────────────────────────
EXPORT_DIR="$TEST_DIR/export/$SITE_ID"

step "Export (symlinks)"
"$TEST_DIR/ddphotos" export
[ -d "$EXPORT_DIR" ]            || fail "export /$SITE_ID not created"
[ -f "$EXPORT_DIR/index.html" ] || fail "export/$SITE_ID/index.html missing"
broken=$(find "$EXPORT_DIR" -type l ! -exec test -e {} \; -print)
[ -z "$broken" ] || fail "broken symlinks in export/$SITE_ID: $broken"
ENTRY_COUNT=$(find "$EXPORT_DIR" \( -type f -o -type l \) | wc -l | tr -d ' ')
pass "export/$SITE_ID OK ($ENTRY_COUNT entries, no broken symlinks)"

step "Export --copy (resolved)"
"$TEST_DIR/ddphotos" export --copy
[ -d "$EXPORT_DIR" ]            || fail "export/$SITE_ID not created"
[ -f "$EXPORT_DIR/index.html" ] || fail "export/$SITE_ID/index.html missing"
symlinks=$(find "$EXPORT_DIR" -type l)
[ -z "$symlinks" ] || fail "export --copy still has symlinks: $symlinks"
FILE_COUNT=$(find "$EXPORT_DIR" -type f | wc -l | tr -d ' ')
pass "export --copy OK ($FILE_COUNT files, no symlinks)"

step "Export --cloudflare"
"$TEST_DIR/ddphotos" export --cloudflare
[ -d "$EXPORT_DIR" ]                || fail "export/$SITE_ID not created"
[ -f "$EXPORT_DIR/index.html" ]     || fail "export/$SITE_ID/index.html missing"
[ -f "$EXPORT_DIR/_worker.js" ]     || fail "export/$SITE_ID/_worker.js missing"
grep -q "ASSETS.fetch" "$EXPORT_DIR/_worker.js" || fail "_worker.js missing ASSETS.fetch"
pass "export --cloudflare OK (_worker.js present)"

# ── 8. Version ─────────────────────────────────────────────────────────────────
step "Version"
version_out=$("$TEST_DIR/ddphotos" version)
echo "$version_out"
echo "$version_out" | grep -qF "$TEST_DIR/ddphotos" || fail "version: Script path does not match $TEST_DIR/ddphotos"
pass "version: Script path OK"

version_image_out=$("$TEST_DIR/ddphotos" version --image)
echo "$version_image_out"
echo "$version_image_out" | grep -qF "$TEST_DIR/ddphotos" || fail "version --image: Script path does not match $TEST_DIR/ddphotos"
echo "$version_image_out" | grep -q "Git:" || fail "version --image: missing Git: line"
echo "$version_image_out" | grep -q "Version:.*dev" || fail "version --image: missing Version: dev"
pass "version --image: Script path OK, Git: and Version: dev present"

# ── 9. Init --script-only ──────────────────────────────────────────────────────
step "Init --script-only"
TEST_DIR2=$(mktemp -d)
chmod 755 "$TEST_DIR2"
docker run --rm -v "$TEST_DIR2":/ddphotos "$IMAGE" init --script-only
[ -x "$TEST_DIR2/ddphotos" ]   || fail "ddphotos script not installed"
[ ! -d "$TEST_DIR2/config" ]   || fail "--script-only should not create config/"
[ ! -d "$TEST_DIR2/albums" ]   || fail "--script-only should not create albums/"
pass "init --script-only OK (only ddphotos script installed)"

# ── 10. Skip ──────────────────────────────────────────────────────
# Note: decided to skip tests for deploy (s3/rsync) due to complexity
#       of setup.  S3 works (it is actively used by yours truly). I have faith
#       in rsync code, but if someone reports problems we can revisit it.
#       The existing rsync-test.sh could be repurposed, but the
#       $RSYNC_RSH var isn't passed on to the container, not is the temp
#       .ssh key.  The existing s3-test.sh could also be repurposed,
#       but it assumes the sample site contents.
#
# Note: Likewise, not testing upgrade logic since it depends on prod
#       images. I did test manually by editing the ddphotos script to
#       verify the detection logic works.

echo ""
echo "=== All docker tests passed ==="
