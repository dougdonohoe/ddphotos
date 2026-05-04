#!/usr/bin/env bash

#
# Verify the photos site is being served correctly.
# Tests URL routing, redirects, and error handling.
# Works against any web server (Apache or nginx).
#
# Usage:
#   bin/test-photos-server.sh --local          # test local Docker on port 8080
#   bin/test-photos-server.sh --local 9090     # test local Docker on port 9090
#   bin/test-photos-server.sh --remote URL     # test a remote site at URL
#   bin/test-photos-server.sh --remote URL --cloudflare  # test Cloudflare Pages deployment
#   bin/test-photos-server.sh --remote URL --surge       # test Surge deployment
#
# Note: In production, Apache is behind CloudFront, so:
#   - Redirect locations use http:// (Apache sees HTTP from CloudFront)
#   - CloudFront upgrades to HTTPS on the next hop
#   - We check redirect locations with http:// to match what Apache returns
#
# Note: Cloudflare Pages (_worker.js) uses 308 (not 301) for trailing slash redirects
#   and always returns https:// in Location headers.
#

set -e

echo "test-photos-server.sh $* starting ..."

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

LOCAL=0
PORT=8080
REMOTE_URL=""
S3_MODE=0
CLOUDFLARE_MODE=0
SURGE_MODE=0

while [[ $# -gt 0 ]]; do
    case $1 in
        --local)
            LOCAL=1
            if [[ -n "${2:-}" && "$2" =~ ^[0-9]+$ ]]; then
                PORT="$2"
                shift
            fi
            shift
            ;;
        --remote)      REMOTE_URL="$2"; shift 2 ;;
        --remote=*)    REMOTE_URL="${1#*=}"; shift ;;
        --s3)          S3_MODE=1; shift ;;
        --cloudflare)  CLOUDFLARE_MODE=1; shift ;;
        --surge)       SURGE_MODE=1; shift ;;
        *) shift ;;
    esac
done

if [ "$LOCAL" -eq 1 ]; then
    BASE="http://localhost:$PORT"
    REDIRECT_BASE="http://localhost:$PORT"
    REDIRECT_STATUS=301
else
    [ -n "$REMOTE_URL" ] || { echo "Error: --remote URL is required when not using --local"; exit 1; }
    BASE="$REMOTE_URL"
    if [ "$CLOUDFLARE_MODE" -eq 1 ]; then
        # Cloudflare Pages (_worker.js): returns https:// in Location and uses 308
        REDIRECT_BASE="$REMOTE_URL"
        REDIRECT_STATUS=308
    elif [ "$S3_MODE" -eq 1 ] || [ "$SURGE_MODE" -eq 1 ]; then
        # S3+CloudFront and Surge: terminate HTTPS themselves, so Location headers use https://
        REDIRECT_BASE="$REMOTE_URL"
        REDIRECT_STATUS=301
    else
        # Apache behind CloudFront: Apache sees HTTP and returns http:// in Location headers
        REDIRECT_BASE="$(echo "$REMOTE_URL" | sed 's|^https://|http://|')"
        REDIRECT_STATUS=301
    fi
fi

# Dynamically pick the first album slug from albums.json.
# If the fetch fails (server down) or returns non-JSON (encrypted site), ALBUM stays empty
# and album-specific tests are skipped.
ALBUM=""
_albums_json=$(curl -sf "$BASE/albums/albums.json" 2>/dev/null) && {
    if command -v jq &>/dev/null; then
        ALBUM=$(echo "$_albums_json" | jq -r '.[0].slug // empty' 2>/dev/null)
    else
        ALBUM=$(echo "$_albums_json" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0]['slug'])" 2>/dev/null)
    fi
}

PASS=0
FAIL=0

# check_status URL EXPECTED_STATUS [DESCRIPTION]
check_status() {
    local url="$1"
    local expected="$2"
    local desc="${3:-$url}"
    local actual

    actual=$(curl -s -o /dev/null -w '%{http_code}' --max-redirs 0 "$url" 2>/dev/null)

    if [ "$actual" = "$expected" ]; then
        echo "  PASS  $desc (HTTP $actual)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $desc (expected $expected, got $actual)"
        FAIL=$((FAIL + 1))
    fi
}

# check_redirect URL EXPECTED_STATUS EXPECTED_LOCATION [DESCRIPTION]
check_redirect() {
    local url="$1"
    local expected_status="$2"
    local expected_location="$3"
    local desc="${4:-$url}"

    local actual_status actual_location
    actual_status=$(curl -s -o /dev/null -w '%{http_code}' --max-redirs 0 "$url" 2>/dev/null)
    actual_location=$(curl -s -o /dev/null -w '%{redirect_url}' --max-redirs 0 "$url" 2>/dev/null)

    if [ "$actual_status" = "$expected_status" ] && [ "$actual_location" = "$expected_location" ]; then
        echo "  PASS  $desc (HTTP $actual_status -> $expected_location)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $desc (expected $expected_status -> $expected_location, got $actual_status -> $actual_location)"
        FAIL=$((FAIL + 1))
    fi
}

# check_body URL PATTERN [DESCRIPTION]
# Fetches URL and checks that the response body contains PATTERN.
check_body() {
    local url="$1"
    local pattern="$2"
    local desc="${3:-$url}"
    local body

    body=$(curl -s -L "$url" 2>/dev/null)

    if echo "$body" | grep -q "$pattern"; then
        echo "  PASS  $desc (found '$pattern')"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $desc (pattern '$pattern' not found in response)"
        FAIL=$((FAIL + 1))
    fi
}

# check_final_status URL EXPECTED_STATUS [DESCRIPTION]
# Follows all redirects and checks the final HTTP status.
check_final_status() {
    local url="$1"
    local expected="$2"
    local desc="${3:-$url}"
    local actual

    actual=$(curl -s -o /dev/null -w '%{http_code}' -L "$url" 2>/dev/null)

    if [ "$actual" = "$expected" ]; then
        echo "  PASS  $desc (HTTP $actual)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $desc (expected $expected, got $actual)"
        FAIL=$((FAIL + 1))
    fi
}

if [ -n "$ALBUM" ]; then
  echo "Testing $BASE with album '$ALBUM' ..."
else
  echo "Testing $BASE (no album available - likely an encrypted site - album-specific tests will be skipped) ..."
fi
echo ""

echo "Pages (expect 200):"
check_status "$BASE"                              200 "Home page"
if [ -n "$ALBUM" ]; then
    check_status "$BASE/albums/$ALBUM"            200 "Album page ($ALBUM)"
fi

echo ""
echo "Static assets (expect 200):"
check_status "$BASE/favicon.ico"                  200 "Favicon"
check_status "$BASE/robots.txt"                   200 "Robots.txt"
check_status "$BASE/sitemap.xml"                  200 "Sitemap (root)"
check_status "$BASE/about.json"                   200 "About JSON"
check_status "$BASE/albums/config.json"           200 "Config JSON"
check_status "$BASE/albums/sitemap.xml"           200 "Sitemap (albums)"

echo ""
echo "About info:"
curl -sf "$BASE/about.json" 2>/dev/null || echo "  (could not fetch about.json)"
echo ""

echo ""
echo "Trailing slash redirects (expect $REDIRECT_STATUS -> no slash):"
if [ -n "$ALBUM" ]; then
    check_redirect "$BASE/albums/$ALBUM/"         "$REDIRECT_STATUS" "$REDIRECT_BASE/albums/$ALBUM"   "Album trailing slash redirect"
fi

echo ""
echo "Trailing slash -> final page (expect 200 after redirect):"
if [ -n "$ALBUM" ]; then
    check_final_status "$BASE/albums/$ALBUM/"     200 "Album trailing slash -> 200"
fi

echo ""
echo "/albums redirect (serves albums.html which redirects client-side):"
check_status "$BASE/albums"                       200 "/albums serves redirect page"
check_final_status "$BASE/albums"                 200 "/albums -> home after redirect"

if [ -n "$ALBUM" ]; then
    echo ""
    echo "Photo permalink URLs (expect 200):"
    check_status "$BASE/albums/$ALBUM/1"          200 "Photo permalink (first photo)"
    check_status "$BASE/albums/$ALBUM/10"         200 "Photo permalink (10th photo)"

    echo ""
    echo "Photo permalink asset paths (must be absolute for correct rendering at /albums/slug/N depth):"
    check_body "$BASE/albums/$ALBUM/1" "/_app/immutable" "Photo permalink HTML has absolute asset paths"

    echo ""
    if [ "$SURGE_MODE" -eq 1 ]; then
        # Surge has no worker, so trailing slash on photo permalinks is handled by the SPA router
        echo "Photo permalink trailing slash (Surge: SPA handles, expect 200):"
        check_final_status "$BASE/albums/$ALBUM/1/"  200 "Photo permalink trailing slash -> 200"
    else
        echo "Photo permalink trailing slash redirects (expect $REDIRECT_STATUS):"
        check_redirect "$BASE/albums/$ALBUM/1/"       "$REDIRECT_STATUS" "$REDIRECT_BASE/albums/$ALBUM/1" "Photo permalink trailing slash"
    fi
fi

echo ""
if [ "$SURGE_MODE" -eq 1 ]; then
    # Surge uses 200.html as SPA fallback for all missing paths — no server-side 404 routing
    echo "404s (Surge: SPA fallback returns 200 for all missing paths):"
    check_status "$BASE/albums/doesnotexist"      200 "Bad album slug (Surge SPA: 200, 404 handled client-side)"
    check_status "$BASE/albums/doesnotexist/1"    200 "Bad album slug with photo index (Surge SPA: 200)"
    check_status "$BASE/nope"                     200 "Unknown path serves SPA shell"
else
    echo "404s (expect 404):"
    check_status "$BASE/albums/doesnotexist"          404 "Bad album slug"
    check_status "$BASE/albums/doesnotexist/1"        404 "Bad album slug with photo index"
    # /nope returns 200 because .htaccess falls back to index.html (SPA handles 404 client-side)
    check_status "$BASE/nope"                         200 "Unknown path serves SPA shell"
    check_body   "$BASE/albums/doesnotexist"          "404 - Not Found" "Custom 404 page served for bad album slug"
fi

echo ""
echo "---"
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
