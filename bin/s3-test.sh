#!/usr/bin/env bash
#
# Test the S3 deploy path of deploy-photos.sh against MinIO.
#
# Verifies that the three-pass aws s3 sync logic places files at the correct
# S3 keys with the correct Cache-Control headers, and that the Pass 1
# --exclude "albums/*" filter protects album data from accidental deletion.
#
# Requires: Docker (for MinIO), AWS CLI v2
#
# Usage: bin/s3-test.sh

set -eo pipefail

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

MINIO_PORT=9000
MINIO_URL="http://localhost:$MINIO_PORT"
BUCKET="ddphotos-test"
CONTAINER="minio-s3-test"
SITE_ID="sample"
BUILD_DIR="$(pwd)/build"
ALBUMS_DIR="$(pwd)/albums"
TEMP_CONFIG=$(mktemp -d /tmp/s3-config.XXXXXX)

# All aws commands target MinIO
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_DEFAULT_REGION=us-east-1
export AWS_ENDPOINT_URL="$MINIO_URL"

PASS=0
FAIL=0

cleanup() {
    docker stop "$CONTAINER" 2>/dev/null || true
    /bin/rm -rf "$TEMP_CONFIG"
}
trap cleanup EXIT

# --- helpers ---

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

# --- start MinIO ---

echo "=== Starting MinIO ==="
docker run -d --rm --name "$CONTAINER" \
    -p "$MINIO_PORT:9000" \
    -e MINIO_ROOT_USER=minioadmin \
    -e MINIO_ROOT_PASSWORD=minioadmin \
    minio/minio server /data

echo "Waiting for MinIO S3..."
until aws s3 ls >/dev/null 2>&1; do sleep 1; done
aws s3 mb "s3://$BUCKET"

# --- build temp config ---

# Patch site_url so photogen writes a local URL into config.json
awk '/site_url:/{print "  site_url: http://localhost:'"$MINIO_PORT"'"; next} {print}' \
    sample/config/albums.yaml > "$TEMP_CONFIG/albums.yaml"
/bin/cp sample/config/descriptions.txt "$TEMP_CONFIG/descriptions.txt"
cat > "$TEMP_CONFIG/site.env" <<EOF
S3_BUCKET=$BUCKET
EOF

# --- run deploy ---

echo ""
echo "=== Running S3 deploy ==="
# Post-deploy tests are skipped: MinIO serves S3 API only, not HTTP.
bin/deploy-photos.sh --s3 --no-server-test --no-playwright --config-dir "$TEMP_CONFIG"

# --- assertions ---

echo ""
echo "Pass 1 — build files at root (expect present):"
check_present "index.html"                        "index.html"
check_present "favicon.ico"                       "favicon.ico"
check_present ".htaccess"                         ".htaccess"

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
