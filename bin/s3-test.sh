#!/usr/bin/env bash
#
# Test the S3 deploy path of deploy-photos.sh against Garage.
#
# Verifies that the three-pass aws s3 sync logic places files at the correct
# S3 keys with the correct Cache-Control headers, and that the Pass 1
# --exclude "albums/*" filter protects album data from accidental deletion.
#
# The deployed site is then served over HTTP and checked by deploy-photos.sh's normal
# post-deploy steps: bin/test-photos-server.sh --s3 and the @deploy Playwright tests.
# Garage (https://garagehq.deuxfleurs.fr) is the local S3 server; its s3_web endpoint
# serves the bucket, and bin/s3-edge-proxy.js supplies the URL routing that CloudFront
# does in production.
#
# Requires: Docker (for Garage), AWS CLI v2, Node (via bin/node-init.sh)
#
# Usage: bin/s3-test.sh

set -eo pipefail

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

# Pinned exactly: Garage publishes no 'latest' or floating major tag, and an exact
# version keeps the same commit testing against the same server.
GARAGE_VERSION=v2.4.1
GARAGE_PORT=3900
GARAGE_WEB_PORT=3902
EDGE_PORT=3903
GARAGE_URL="http://localhost:$GARAGE_PORT"
SITE_URL="http://localhost:$EDGE_PORT"
BUCKET="ddphotos-test"
CONTAINER="garage-s3-test"
SITE_ID="sample"
BUILD_DIR="$(pwd)/build"
ALBUMS_DIR="$(pwd)/albums"
TEMP_CONFIG=$(mktemp -d /tmp/s3-config.XXXXXX)
GARAGE_DIR=$(mktemp -d /tmp/s3-garage.XXXXXX)

# Throwaway credentials. Garage has no fixed root credentials, so they are imported
# into it below, which lets this script hardcode them.
KEY_NAME="ddphotos-test-key"
KEY_ID="GKddphotostest000000000000"
SECRET_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

# All aws commands target Garage. Both region variables are set: Garage validates the
# SigV4 credential scope against its own s3_region, so an AWS_REGION inherited from the
# developer's environment would otherwise fail with AuthorizationHeaderMalformed.
export AWS_ACCESS_KEY_ID="$KEY_ID"
export AWS_SECRET_ACCESS_KEY="$SECRET_KEY"
export AWS_DEFAULT_REGION=us-east-1
export AWS_REGION=us-east-1
export AWS_ENDPOINT_URL="$GARAGE_URL"

PASS=0
FAIL=0

EDGE_PID=""

cleanup() {
    [ -n "$EDGE_PID" ] && kill "$EDGE_PID" 2>/dev/null
    docker stop "$CONTAINER" 2>/dev/null || true
    /bin/rm -rf "$TEMP_CONFIG" "$GARAGE_DIR"
}
trap cleanup EXIT

# --- helpers ---

garage() {
    docker exec "$CONTAINER" /garage "$@"
}

check_present() {
    local key="$1" desc="${2:-$1}"
    if aws s3api head-object --bucket "$BUCKET" --key "$key" >/dev/null 2>&1; then
        echo "  PASS  $desc (present)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $desc (missing)"
        FAIL=$((FAIL + 1))
    fi
}

check_absent() {
    local key="$1" desc="${2:-$1}"
    if aws s3api head-object --bucket "$BUCKET" --key "$key" >/dev/null 2>&1; then
        echo "  FAIL  $desc (unexpectedly present)"
        FAIL=$((FAIL + 1))
    else
        echo "  PASS  $desc (correctly absent)"
        PASS=$((PASS + 1))
    fi
}

check_cache_control() {
    local key="$1" expected="$2" desc="${3:-$1}"
    local actual
    actual=$(aws s3api head-object --bucket "$BUCKET" --key "$key" \
        --query 'CacheControl' --output text 2>/dev/null || true)
    if [ "$actual" = "$expected" ]; then
        echo "  PASS  $desc (Cache-Control: $actual)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $desc (expected '$expected', got '$actual')"
        FAIL=$((FAIL + 1))
    fi
}

# Wait for a command to succeed, or give up and dump the container log
wait_for() {
    local desc="$1"; shift
    local _
    for _ in $(seq 1 60); do
        "$@" >/dev/null 2>&1 && return 0
        sleep 1
    done
    echo "Error: timed out waiting for $desc" >&2
    docker logs "$CONTAINER" 2>&1 | tail -30 >&2
    exit 1
}

# --- start Garage ---

echo "=== Starting Garage $GARAGE_VERSION ==="

# rpc_secret is required even for a single node; this is a throwaway cluster.
cat > "$GARAGE_DIR/garage.toml" <<EOF
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"

replication_factor = 1

rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "0000000000000000000000000000000000000000000000000000000000000001"

[s3_api]
s3_region = "$AWS_REGION"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage.localhost"

[s3_web]
bind_addr = "[::]:3902"
root_domain = ".web.garage.localhost"
index = "index.html"
EOF

docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d --rm --name "$CONTAINER" \
    -p "$GARAGE_PORT:3900" \
    -p "$GARAGE_WEB_PORT:3902" \
    -v "$GARAGE_DIR/garage.toml:/etc/garage.toml:ro" \
    "dxflrs/garage:$GARAGE_VERSION" /garage server

# Garage is not usable straight out of the image: even a single-node cluster needs a
# storage layout, an access key and a bucket before the S3 API will serve anything.
echo "Waiting for Garage node..."
wait_for "Garage node" garage status

echo "Assigning cluster layout..."
NODE_ID=$(garage node id -q 2>/dev/null | cut -d@ -f1)
garage layout assign -z dc1 -c 1G "$NODE_ID" >/dev/null
# Version 1 is the first layout of a freshly created cluster
garage layout apply --version 1 >/dev/null

echo "Creating key and bucket..."
garage key import --yes -n "$KEY_NAME" "$KEY_ID" "$SECRET_KEY" >/dev/null
garage bucket create "$BUCKET" >/dev/null
garage bucket allow --read --write --owner "$BUCKET" --key "$KEY_NAME" >/dev/null

# Serve the bucket over HTTP. The error document is what CloudFront's custom error
# responses do in production: a missing key returns 404.html with a 404 status.
garage bucket website --allow --index-document index.html --error-document 404.html "$BUCKET" >/dev/null

echo "Waiting for Garage S3 API..."
wait_for "Garage S3 API" aws s3 ls "s3://$BUCKET"

# --- start the edge proxy ---

# Node.js init (see bin/node-init.sh)
# shellcheck source=/dev/null
source "$SDIR/node-init.sh"

echo "Starting edge proxy on $SITE_URL..."
node bin/s3-edge-proxy.js --port "$EDGE_PORT" --origin "localhost:$GARAGE_WEB_PORT" --bucket "$BUCKET" &
EDGE_PID=$!
# No -f: the bucket is still empty, so any answer at all means the proxy is listening
wait_for "edge proxy" curl -s -o /dev/null "$SITE_URL/"

# --- build temp config ---

# Patch site_url so photogen writes a local URL into config.json, which deploy-photos.sh
# then uses as the base URL for its post-deploy tests
awk '/site_url:/{print "  site_url: '"$SITE_URL"'"; next} {print}' \
    sample/config/albums.yaml > "$TEMP_CONFIG/albums.yaml"
/bin/cp sample/config/descriptions.txt "$TEMP_CONFIG/descriptions.txt"
cat > "$TEMP_CONFIG/site.env" <<EOF
S3_BUCKET=$BUCKET
EOF

# --- run deploy ---

echo ""
echo "=== Running S3 deploy ==="
# Post-deploy runs bin/test-photos-server.sh --s3 and the @deploy Playwright tests against
# the deployed site, served by Garage through the edge proxy.
bin/deploy-photos.sh --s3 --playwright-smoke --config-dir "$TEMP_CONFIG"

# --- assertions ---

echo ""
echo "Pass 1 — build files at root (expect present):"
check_present "index.html"                        "index.html"
check_present "favicon.ico"                       "favicon.ico"
check_present ".htaccess"                         ".htaccess"
check_present "sitemap.xml"                       "sitemap.xml"
check_present "robots.txt"                        "robots.txt"

echo ""
echo "Pass 1 — pre-rendered album HTML re-included via --include \"albums/*.html\":"
check_present "albums/antarctica.html"            "albums/antarctica.html"
check_present "albums/the-way.html"               "albums/the-way.html"
check_present "albums/uganda.html"                "albums/uganda.html"

echo ""
echo "Pass 2a — album metadata (Cache-Control: no-cache):"
check_present       "albums/albums.json"
check_cache_control "albums/albums.json"                        "no-cache"
check_present       "albums/sitemap.xml"
check_cache_control "albums/sitemap.xml"                        "no-cache"
check_present       "albums/antarctica/index.json"
check_cache_control "albums/antarctica/index.json"              "no-cache"
check_present       "albums/antarctica/cover.jpg"
check_cache_control "albums/antarctica/cover.jpg"               "no-cache"

echo ""
echo "Pass 2b — WebP images (Cache-Control: max-age=31536000,immutable):"
check_present       "albums/antarctica/grid/cuverville_is_03.webp"
check_cache_control "albums/antarctica/grid/cuverville_is_03.webp"  "max-age=31536000,immutable"
check_present       "albums/antarctica/full/cuverville_is_03.webp"
check_cache_control "albums/antarctica/full/cuverville_is_03.webp"  "max-age=31536000,immutable"

echo ""
echo "Pass 2a — .html excluded from album data sync (not deleted by Pass 2a --delete):"
check_present "albums/antarctica.html"            "albums/antarctica.html survives Pass 2a"

echo ""
echo "Boundary — Pass 1 --exclude \"albums/*\" protects album data from --delete:"
# Upload a sentinel file to albums/ that is NOT in the build source
SENTINEL=$(mktemp /tmp/sentinel.XXXXXX)
aws s3 cp "$SENTINEL" "s3://$BUCKET/albums/sentinel.webp" --content-type "image/webp" >/dev/null
/bin/rm -f "$SENTINEL"
# Run only Pass 1 (same options as deploy-photos.sh)
aws s3 sync "$BUILD_DIR/$SITE_ID/" "s3://$BUCKET/" \
    --delete --exclude "albums/*" --include "albums/*.html"
check_present "albums/sentinel.webp"              "albums/sentinel.webp survives Pass 1 --delete"
# Run Pass 2b — its --delete should remove files not in the album source
aws s3 sync "$ALBUMS_DIR/$SITE_ID/" "s3://$BUCKET/albums/" \
    --delete --exclude "*" --include "*.webp" \
    --cache-control "max-age=31536000,immutable"
check_absent  "albums/sentinel.webp"              "albums/sentinel.webp removed by Pass 2b --delete"

echo ""
echo "---"
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
