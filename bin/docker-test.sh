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
SITE_ID="docker-test-id"  # passed to init via --site-id, verified against albums.yaml after
RUN_PORT=5173             # Vite host port; matches container default to keep port-mapping consistent
SERVE_PORT=8090           # Apache host port (maps to container port 80; avoids conflicts with run-tests.sh)
DO_BUILD=true
TEST_DIR=""
TEST_DIR2=""
TEMP_DECODE_DIR=""
EXT_CONFIG_DIR=""
SC_TEST_DIR=""
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
    if [ -n "$RUN_PID" ];   then kill "$RUN_PID"   2>/dev/null || true; fi
    if [ -n "$SERVE_PID" ]; then kill "$SERVE_PID" 2>/dev/null || true; fi
    # Belt-and-suspenders: stop any containers using the local image still running on our ports
    docker ps --filter publish="$RUN_PORT"   -q | xargs docker stop &>/dev/null || true
    docker ps --filter publish="$SERVE_PORT" -q | xargs docker stop &>/dev/null || true
    # !! IMPORTANT: Docker containers run as root, so any files/dirs they create inside a
    # !! mounted volume (e.g. TEST_DIR/albums/, TEST_DIR/build/) are root-owned with 700
    # !! permissions — unreadable and unwritable from the host. This has bitten us 3-4 times.
    # !!
    # !! On Mac, Docker's VM layer masks this (writes may appear to succeed locally), but CI
    # !! runs on Linux where host and container share the real filesystem — failures are real.
    # !! Always write tests to pass on Linux / CI, not just on Mac.
    # !!
    # !! Rules to avoid it:
    # !!   1. Never write test files into subdirs that Docker created (e.g. TEST_DIR/albums/...).
    # !!   2. For test fixtures, use TEMP_DECODE_DIR or a fresh mktemp -d (user-owned).
    # !!   3. If you need Docker output in a specific dir layout, copy it there after the fact.
    # !!   4. Cleanup below must use Docker to remove root-owned files before /bin/rm -rf works.
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
    if [ -n "$TEMP_DECODE_DIR" ]; then /bin/rm -rf "$TEMP_DECODE_DIR"; fi
    if [ -n "$EXT_CONFIG_DIR" ]; then /bin/rm -rf "$EXT_CONFIG_DIR"; fi
    if [ -n "$SC_TEST_DIR" ];   then /bin/rm -rf "$SC_TEST_DIR";   fi
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
docker run --rm -v "$TEST_DIR":/ddphotos "$IMAGE" init --site-id "$SITE_ID"
[ -x "$TEST_DIR/ddphotos" ]              || fail "ddphotos script not installed"
[ -f "$TEST_DIR/config/albums.yaml" ]    || fail "config/albums.yaml not created"
[ -f "$TEST_DIR/config/passwords.yaml" ] || fail "config/passwords.yaml not created"
[ -f "$TEST_DIR/config/site.env" ]       || fail "config/site.env not created"
[ -f "$TEST_DIR/config/passwords.yaml" ] || fail "config/passwords.yaml not created"
pass "ddphotos script and config created at $TEST_DIR"

VALIDATE_SITE_ID=$(awk '/^settings:/{f=1} f && /[[:space:]]id:/{gsub(/.*id:[[:space:]]*/,""); print; exit}' "$TEST_DIR/config/albums.yaml")
[ "$VALIDATE_SITE_ID" = "$SITE_ID" ] || fail "SITE_ID mismatch: expected '$SITE_ID', got '$VALIDATE_SITE_ID' in config/albums.yaml"
pass "site ID '$SITE_ID' written correctly to config/albums.yaml"

DDPHOTOS=("$TEST_DIR/ddphotos" --show-mounts)
DDPHOTOS_QUIET=("$TEST_DIR/ddphotos")
PASSWORDS_FILE="$TEST_DIR/config/passwords.yaml"

# ── 3. Error handling ─────────────────────────────────────────────────────────
step "Error handling: commands reject unexpected args"
out=$("${DDPHOTOS_QUIET[@]}" build extra-arg 2>&1) || true
echo "$out" | grep -q "takes no arguments" || fail "build: expected 'takes no arguments' error"
pass "build rejects unexpected args"

out=$("$TEST_DIR/ddphotos" --non-interactive serve --foo 2>&1) || true
echo "$out" | grep -q "takes no arguments" || fail "serve: expected 'takes no arguments' error"
pass "serve rejects unexpected args"

out=$("${DDPHOTOS_QUIET[@]}" export --no-such-flag 2>&1) || true
echo "$out" | grep -q "Unknown option" || fail "export: expected 'Unknown option' error"
pass "export rejects unknown flags"

step "Error handling: unknown pre-command option"
out=$("$TEST_DIR/ddphotos" --no-such-flag build 2>&1) || true
echo "$out" | grep -q "Unknown option" || fail "ddphotos: expected 'Unknown option' for unknown pre-command flag"
pass "ddphotos rejects unknown pre-command options"

step "Help command"
out=$("${DDPHOTOS_QUIET[@]}" help 2>&1)
echo "$out" | grep -q "photogen" || fail "help: missing expected content"
pass "help exits 0 and shows usage"

# ── 4. Photogen ────────────────────────────────────────────────────────────────
step "Photogen"
"${DDPHOTOS[@]}" photogen
[ -d "$TEST_DIR/albums/$SITE_ID" ] || fail "albums/$SITE_ID not created"
ALBUM_COUNT=$(find "$TEST_DIR/albums/$SITE_ID" -maxdepth 1 -mindepth 1 -type d | wc -l | tr -d ' ')
pass "albums/$SITE_ID created ($ALBUM_COUNT albums)"

# ── 4. Decode ──────────────────────────────────────────────────────────────────
step "Decode"
ENC_FILE="albums/$SITE_ID/secret/index.enc.json"
[ -f "$TEST_DIR/$ENC_FILE" ] || fail "$ENC_FILE not found after photogen"
decoded=$("${DDPHOTOS[@]}" decode "$ENC_FILE")
echo "$decoded" | grep -q '"photos"' || fail "decoded output missing 'photos' key"
pass "decode: $ENC_FILE decrypted OK"

# Test 2: files outside DDPHOTOS_DIR — exercises the external-mount path in ddphotos decode.
# The enc.json is placed in a secret/ subdirectory so the album slug is preserved in the
# container path (/ddphotos-args/arg-N/secret/index.enc.json), which decode needs to find
# the right per-album password.
TEMP_DECODE_DIR=$(mktemp -d)
mkdir -p "$TEMP_DECODE_DIR/secret"
/bin/cp "$TEST_DIR/config/passwords.yaml"  "$TEMP_DECODE_DIR/passwords.yaml"
/bin/cp "$TEST_DIR/$ENC_FILE"              "$TEMP_DECODE_DIR/secret/index.enc.json"

# (a) explicit --passwords pointing outside DDPHOTOS_DIR
decoded=$("${DDPHOTOS[@]}" decode --passwords "$TEMP_DECODE_DIR/passwords.yaml" "$TEMP_DECODE_DIR/secret/index.enc.json")
echo "$decoded" | grep -q '"photos"' || fail "decode --passwords (external): decoded output missing 'photos' key"
pass "decode --passwords: files outside DDPHOTOS_DIR OK"

# (b) replace embedded pwFile with the temp path; decode should mount it automatically
sed "s|\"pwFile\":\"[^\"]*\"|\"pwFile\":\"$TEMP_DECODE_DIR/passwords.yaml\"|" \
    "$TEMP_DECODE_DIR/secret/index.enc.json" > "$TEMP_DECODE_DIR/secret/index.enc.json.tmp"
mv "$TEMP_DECODE_DIR/secret/index.enc.json.tmp" "$TEMP_DECODE_DIR/secret/index.enc.json"
decoded=$("${DDPHOTOS[@]}" decode "$TEMP_DECODE_DIR/secret/index.enc.json")
echo "$decoded" | grep -q '"photos"' || fail "decode (external pwFile): decoded output missing 'photos' key"
pass "decode: both enc.json and pwFile outside DDPHOTOS_DIR OK"

# ── 5. Search-Cover ────────────────────────────────────────────────────────────
step "Search-Cover"
# Derive the URL from the decoded index so we don't hardcode the UUID.
SC_DECODED=$("${DDPHOTOS_QUIET[@]}" decode "$ENC_FILE")
SC_GRID=$(echo "$SC_DECODED" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['photos'][0]['src']['grid'])")
SC_URL="http://localhost:5173/albums/secret/$SC_GRID"
SC_OUT=$("${DDPHOTOS[@]}" search-cover "$SC_URL")
echo "$SC_OUT" | grep -q "cover: 2024-The-Way-21.jpg" || fail "search-cover: 'cover: 2024-The-Way-21.jpg' not in output"
pass "search-cover: found cover file for secret album"

# ── 6. Decode + Search-Cover with external --config-dir ───────────────────────
# Regression test for: decode and search-cover failing to mount the config dir
# when --config-dir points outside DDPHOTOS_DIR.
#
# Simulates photogen having been run with an external --config-dir by rewriting
# the embedded pwFile path from /ddphotos/config/... to /ddphotos-config/...
# (the container path used when --config-dir is outside DDPHOTOS_DIR).
step "Decode + Search-Cover with external --config-dir"

# Simulate photogen having been run with an external --config-dir by rewriting
# the embedded pwFile path from /ddphotos/config/... to /ddphotos-config/...
EXT_CONFIG_DIR=$(mktemp -d)
/bin/cp "$TEST_DIR/config/passwords.yaml" "$EXT_CONFIG_DIR/"
/bin/cp "$TEST_DIR/config/albums.yaml"    "$EXT_CONFIG_DIR/"

# Write the modified enc.json to TEMP_DECODE_DIR (user-owned) — the album
# directory inside TEST_DIR is root-owned (created by Docker) and unwritable.
EXT_ENC_FILE="$TEMP_DECODE_DIR/secret/index-extconfig.enc.json"
sed 's|"pwFile":"/ddphotos/config/passwords.yaml"|"pwFile":"/ddphotos-config/passwords.yaml"|' \
    "$TEST_DIR/$ENC_FILE" > "$EXT_ENC_FILE"

decoded=$("${DDPHOTOS[@]}" --config-dir "$EXT_CONFIG_DIR" decode "$EXT_ENC_FILE")
echo "$decoded" | grep -q '"photos"' || fail "decode --config-dir: decoded output missing 'photos' key"
pass "decode --config-dir: external config dir mounted correctly"

# search-cover needs the modified enc.json at the album path. Use a fresh
# user-owned SC_TEST_DIR with --dir so we can write to it freely.
SC_TEST_DIR=$(mktemp -d)
chmod 755 "$SC_TEST_DIR"
mkdir -p "$SC_TEST_DIR/albums/$SITE_ID/secret"
/bin/cp "$EXT_ENC_FILE" "$SC_TEST_DIR/albums/$SITE_ID/secret/index.enc.json"
SC_OUT=$("${DDPHOTOS[@]}" --dir "$SC_TEST_DIR" --config-dir "$EXT_CONFIG_DIR" search-cover "$SC_URL")
echo "$SC_OUT" | grep -q "cover: 2024-The-Way-21.jpg" || fail "search-cover --config-dir: 'cover: 2024-The-Way-21.jpg' not in output"
pass "search-cover --config-dir: external config dir mounted correctly"

# ── 7. Run (Vite dev server) + Playwright ─────────────────────────────────────
step "Run — Vite dev server on port $RUN_PORT"
RUN_PORT="$RUN_PORT" "${DDPHOTOS[@]}" --non-interactive run &
RUN_PID=$!
wait_for_http "http://localhost:$RUN_PORT" "Vite dev server"
run_playwright "http://localhost:$RUN_PORT" "$PASSWORDS_FILE"
kill "$RUN_PID" 2>/dev/null || true; wait "$RUN_PID" 2>/dev/null || true; RUN_PID=""
pass "run + Playwright OK"

# ── 8. Build ───────────────────────────────────────────────────────────────────
step "Build"
"${DDPHOTOS[@]}" build
[ -d "$TEST_DIR/build/$SITE_ID" ] || fail "build/$SITE_ID not created"
pass "build/$SITE_ID created"

# ── 9. Serve (Apache) + Playwright + test-photos-server.sh ────────────────────
step "Serve — Apache on port $SERVE_PORT"
SERVE_PORT="$SERVE_PORT" "${DDPHOTOS[@]}" --non-interactive serve &
SERVE_PID=$!
wait_for_http "http://localhost:$SERVE_PORT" "Apache"
curl -s "http://localhost:$SERVE_PORT/albums/config.json" # sanity check before tests run
"$SCRIPT_DIR/test-photos-server.sh" --local "$SERVE_PORT"
run_playwright "http://localhost:$SERVE_PORT" "$PASSWORDS_FILE"
kill "$SERVE_PID" 2>/dev/null || true; wait "$SERVE_PID" 2>/dev/null || true; SERVE_PID=""
pass "serve + Playwright + test-photos-server.sh OK"

# ── 10. Export (symlink mode) ──────────────────────────────────────────────────
EXPORT_DIR="$TEST_DIR/export/$SITE_ID"

step "Export (symlinks)"
"${DDPHOTOS[@]}" export
[ -d "$EXPORT_DIR" ]            || fail "export /$SITE_ID not created"
[ -f "$EXPORT_DIR/index.html" ] || fail "export/$SITE_ID/index.html missing"
broken=$(find "$EXPORT_DIR" -type l ! -exec test -e {} \; -print)
[ -z "$broken" ] || fail "broken symlinks in export/$SITE_ID: $broken"
ENTRY_COUNT=$(find "$EXPORT_DIR" \( -type f -o -type l \) | wc -l | tr -d ' ')
pass "export/$SITE_ID OK ($ENTRY_COUNT entries, no broken symlinks)"

step "Export --export-site-id (alternate destination)"
"${DDPHOTOS[@]}" export --export-site-id alternate
EXPORT_ALT_DIR="$TEST_DIR/export/alternate"
[ -d "$EXPORT_ALT_DIR" ]            || fail "export/alternate not created"
[ -f "$EXPORT_ALT_DIR/index.html" ] || fail "export/alternate/index.html missing"
pass "export --export-site-id alternate OK"

step "Export --copy (resolved)"
"${DDPHOTOS[@]}" export --copy
[ -d "$EXPORT_DIR" ]            || fail "export/$SITE_ID not created"
[ -f "$EXPORT_DIR/index.html" ] || fail "export/$SITE_ID/index.html missing"
symlinks=$(find "$EXPORT_DIR" -type l)
[ -z "$symlinks" ] || fail "export --copy still has symlinks: $symlinks"
FILE_COUNT=$(find "$EXPORT_DIR" -type f | wc -l | tr -d ' ')
pass "export --copy OK ($FILE_COUNT files, no symlinks)"

step "Export --cloudflare"
"${DDPHOTOS[@]}" export --cloudflare
[ -d "$EXPORT_DIR" ]                || fail "export/$SITE_ID not created"
[ -f "$EXPORT_DIR/index.html" ]     || fail "export/$SITE_ID/index.html missing"
[ -f "$EXPORT_DIR/_worker.js" ]     || fail "export/$SITE_ID/_worker.js missing"
grep -q "ASSETS.fetch" "$EXPORT_DIR/_worker.js" || fail "_worker.js missing ASSETS.fetch"
pass "export --cloudflare OK (_worker.js present)"

# ── 11. Version ────────────────────────────────────────────────────────────────
step "Version"
version_out=$("${DDPHOTOS[@]}" version)
echo "$version_out"
echo "$version_out" | grep -qF "$TEST_DIR/ddphotos" || fail "version: Script path does not match $TEST_DIR/ddphotos"
pass "version: Script path OK"

version_image_out=$("${DDPHOTOS[@]}" version --image)
echo "$version_image_out"
echo "$version_image_out" | grep -qF "$TEST_DIR/ddphotos" || fail "version --image: Script path does not match $TEST_DIR/ddphotos"
echo "$version_image_out" | grep -q "Git:" || fail "version --image: missing Git: line"
echo "$version_image_out" | grep -q "Version:.*dev" || fail "version --image: missing Version: dev"
pass "version --image: Script path OK, Git: and Version: dev present"

echo "$version_out" | grep -q "Site ID:.*$SITE_ID" || fail "version: Site ID does not show $SITE_ID"
pass "version: Site ID '$SITE_ID' auto-detected from albums.yaml"

# ── 12. Init --script-only ─────────────────────────────────────────────────────
step "Init --script-only"
TEST_DIR2=$(mktemp -d)
chmod 755 "$TEST_DIR2"
docker run --rm -v "$TEST_DIR2":/ddphotos "$IMAGE" init --script-only
[ -x "$TEST_DIR2/ddphotos" ]   || fail "ddphotos script not installed"
[ ! -d "$TEST_DIR2/config" ]   || fail "--script-only should not create config/"
[ ! -d "$TEST_DIR2/albums" ]   || fail "--script-only should not create albums/"
pass "init --script-only OK (only ddphotos script installed)"

# ── 13. Skip ──────────────────────────────────────────────────────
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
