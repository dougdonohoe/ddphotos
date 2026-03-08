#!/usr/bin/env bash

#
# Verify Apache is serving the photos site correctly.
# Tests URL routing, redirects, and error handling.
#
# Usage:
#   bin/test-photos-apache.sh                  # test production ($VITE_SITE_URL)
#   bin/test-photos-apache.sh --local          # test local Docker on port 8080
#   bin/test-photos-apache.sh --local 9090     # test local Docker on port 9090
#
# Note: In production, Apache is behind CloudFront, so:
#   - Redirect locations use http:// (Apache sees HTTP from CloudFront)
#   - CloudFront upgrades to HTTPS on the next hop
#   - We check redirect locations with http:// to match what Apache returns
#

set -e

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

LOCAL=0
PORT=8080
CONFIG_DIR=""

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
        --config-dir)   CONFIG_DIR="$2"; shift 2 ;;
        --config-dir=*) CONFIG_DIR="${1#*=}"; shift ;;
        *) shift ;;
    esac
done

if [ -n "$CONFIG_DIR" ]; then
    CONFIG="$CONFIG_DIR/site.env"
else
    CONFIG="$SDIR/../config/site.env"
fi
[ -f "$CONFIG" ] || { echo "Error: $CONFIG not found"; exit 1; }
source "$CONFIG"

if [ "$LOCAL" -eq 1 ]; then
    BASE="http://localhost:$PORT"
    REDIRECT_BASE="http://localhost:$PORT"
    ALBUM="${TEST_ALBUM_LOCAL:-antarctica}"
else
    BASE="$VITE_SITE_URL"
    # Apache returns http:// in redirect Location headers since CloudFront
    # terminates SSL and forwards HTTP to the origin.
    REDIRECT_BASE="$(echo "$VITE_SITE_URL" | sed 's|^https://|http://|')"
    ALBUM="${TEST_ALBUM_PROD:-patagonia}"
fi

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

echo "Testing $BASE ..."
echo ""

echo "Pages (expect 200):"
check_status "$BASE"                              200 "Home page"
check_status "$BASE/albums/$ALBUM"               200 "Album page (no trailing slash)"
if [ "$LOCAL" -eq 0 ] && [ -n "$TEST_ALBUM_HYPHEN" ]; then
    check_status "$BASE/albums/$TEST_ALBUM_HYPHEN" 200 "Album page with hyphenated slug"
fi

echo ""
echo "Static assets (expect 200):"
check_status "$BASE/favicon.ico"                  200 "Favicon"
check_status "$BASE/robots.txt"                   200 "Robots.txt"
check_status "$BASE/albums/albums.json"           200 "Albums JSON"
check_status "$BASE/sitemap.xml"                  200 "Sitemap"

echo ""
echo "Trailing slash redirects (expect 301 -> no slash):"
check_redirect "$BASE/albums/$ALBUM/"            301 "$REDIRECT_BASE/albums/$ALBUM"    "Album trailing slash redirect"
if [ "$LOCAL" -eq 0 ] && [ -n "$TEST_ALBUM_HYPHEN" ]; then
    check_redirect "$BASE/albums/$TEST_ALBUM_HYPHEN/" 301 "$REDIRECT_BASE/albums/$TEST_ALBUM_HYPHEN" "Hyphenated album trailing slash"
fi

echo ""
echo "Trailing slash -> final page (expect 200 after redirect):"
check_final_status "$BASE/albums/$ALBUM/"        200 "Album trailing slash -> 200"

echo ""
echo "/albums redirect (serves albums.html which redirects client-side):"
check_status "$BASE/albums"                       200 "/albums serves redirect page"
check_final_status "$BASE/albums"                 200 "/albums -> home after redirect"

echo ""
echo "Photo permalink URLs (expect 200):"
check_status "$BASE/albums/$ALBUM/1"             200 "Photo permalink (first photo)"
check_status "$BASE/albums/$ALBUM/10"            200 "Photo permalink (10th photo)"

echo ""
echo "Photo permalink asset paths (must be absolute for correct rendering at /albums/slug/N depth):"
check_body "$BASE/albums/$ALBUM/1" "/_app/immutable" "Photo permalink HTML has absolute asset paths"

echo ""
echo "Photo permalink trailing slash redirects (expect 301):"
check_redirect "$BASE/albums/$ALBUM/1/"          301 "$REDIRECT_BASE/albums/$ALBUM/1"  "Photo permalink trailing slash"

echo ""
echo "404s (expect 404):"
check_status "$BASE/albums/doesnotexist"          404 "Bad album slug"
check_status "$BASE/albums/doesnotexist/1"        404 "Bad album slug with photo index"
# /nope returns 200 because .htaccess falls back to index.html (SPA handles 404 client-side)
check_status "$BASE/nope"                         200 "Unknown path serves SPA shell"
check_body   "$BASE/albums/doesnotexist"          "404 - Not Found" "Custom 404 page served for bad album slug"

echo ""
echo "---"
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
