#!/usr/bin/env bash
#
# Test the rsync deploy path of deploy-photos.sh against a local Docker container.
#
# Spins up an Apache+SSH (or nginx+SSH) container, builds a temporary config with siteUrl
# pointing at localhost, then runs deploy-photos.sh (photogen → build → rsync → post-deploy
# server routing tests + Playwright) against it.
#
# Usage: bin/rsync-test.sh [--server apache|nginx]
#
#   --server  Which server to deploy into (default: apache)

set -eo pipefail

SERVER="apache"
while [[ $# -gt 0 ]]; do
    case "$1" in
        --server)   SERVER="$2"; shift 2 ;;
        --server=*) SERVER="${1#*=}"; shift ;;
        *) echo "Usage: $0 [--server apache|nginx]" >&2; exit 1 ;;
    esac
done

if [[ "$SERVER" != "apache" && "$SERVER" != "nginx" ]]; then
    echo "Error: --server must be 'apache' or 'nginx' (got '$SERVER')" >&2
    exit 1
fi

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

HTTP_PORT=8083
SSH_PORT=2222
IMAGE="photos-$SERVER-ssh"
CONTAINER="rsync-test-$SERVER"

# Document root differs per server; RSYNC_DEST must match what the server actually serves.
if [ "$SERVER" = "apache" ]; then
    DOC_ROOT=/usr/local/apache2/htdocs/
else
    DOC_ROOT=/usr/share/nginx/html/
fi
TEST_KEY="$(pwd)/web/testdata/rsync-test-key"
# SSH refuses keys that are world-readable; git checkouts default to 0644.
chmod 600 "$TEST_KEY"
TEMP_CONFIG=$(mktemp -d /tmp/rsync-config.XXXXXX)

cleanup() {
    docker stop "$CONTAINER" 2>/dev/null || true
    /bin/rm -rf "$TEMP_CONFIG"
}
trap cleanup EXIT

# Build Docker image if missing
docker image inspect "$IMAGE" >/dev/null 2>&1 || {
    echo "=== Building $IMAGE ==="
    docker build --pull -t "$IMAGE" -f "web/$SERVER-ssh.dockerfile" web/
}

# Build temp config dir.
# Patch site_url so photogen writes http://localhost:$HTTP_PORT into config.json,
# which deploy-photos.sh then uses as the base URL for post-deploy tests.
awk '/site_url:/{print "  site_url: http://localhost:'"$HTTP_PORT"'"; next} {print}' \
    sample/config/albums.yaml > "$TEMP_CONFIG/albums.yaml"
/bin/cp sample/config/descriptions.txt "$TEMP_CONFIG/descriptions.txt"
cat > "$TEMP_CONFIG/site.env" <<EOF
RSYNC_HOST=root@localhost
RSYNC_DEST=$DOC_ROOT
EOF

# Start container (empty htdocs — rsync fills it)
echo "=== Starting $IMAGE on HTTP :$HTTP_PORT, SSH :$SSH_PORT ==="
docker run -d --rm --name "$CONTAINER" -p "$HTTP_PORT:80" -p "$SSH_PORT:22" "$IMAGE"
echo "Waiting for $SERVER..."
until curl -s -o /dev/null "http://localhost:$HTTP_PORT"; do sleep 1; done

# Run full deploy: photogen → build → rsync → post-deploy server tests + Playwright.
# RSYNC_RSH tells rsync which SSH command to use (custom port + test key).
export RSYNC_RSH="ssh -i $TEST_KEY -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
bin/deploy-photos.sh --no-pre-deploy-tests --server "$SERVER" --config-dir "$TEMP_CONFIG"
