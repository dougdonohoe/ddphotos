#!/usr/bin/env bash
# Verify (and optionally rebuild) a photos Docker image (apache or nginx).
#
# Usage:
#   bin/docker-check.sh [--server apache|nginx] [--build|--force]
#
#   --server  Which server image to manage (default: apache)
#   (no flag) check only; exit 1 if stale or missing
#   --build   rebuild if stale or missing, no-op if current
#   --force   always rebuild

set -eo pipefail

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

SERVER="apache"
MODE="check"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --server)
            SERVER="$2"
            if [[ "$SERVER" != "apache" && "$SERVER" != "nginx" ]]; then
                echo "Usage: $0 [--server apache|nginx] [--build|--force]" >&2
                exit 1
            fi
            shift 2
            ;;
        --build)  MODE=build; shift ;;
        --force)  MODE=force; shift ;;
        *) echo "Usage: $0 [--server apache|nginx] [--build|--force]" >&2; exit 1 ;;
    esac
done

if ! docker info >/dev/null 2>&1; then
    echo "ERROR: Docker daemon is not running."
    exit 1
fi

IMAGE="photos-$SERVER"
WEB="$SDIR/../web"

if [ "$SERVER" = "apache" ]; then
    DOCKERFILE="$WEB/apache.dockerfile"
    HASH_INPUTS="$WEB/apache.dockerfile $WEB/apache-entrypoint.sh $WEB/setup-htdocs.sh"
else
    DOCKERFILE="$WEB/nginx.dockerfile"
    HASH_INPUTS="$WEB/nginx.dockerfile $WEB/nginx-entrypoint.sh $WEB/nginx.conf $WEB/setup-htdocs.sh"
fi

expected=$(cat $HASH_INPUTS | shasum -a 256 | cut -d' ' -f1)

_build() {
    docker build -t "$IMAGE" -f "$DOCKERFILE" --label "ddphotos.hash=$expected" "$WEB/"
}

if [ "$MODE" = force ]; then
    _build
    exit 0
fi

stored=$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "ddphotos.hash"}}' 2>/dev/null || true)

if [ "$stored" = "$expected" ]; then
    exit 0
fi

if [ "$MODE" = build ]; then
    echo "=== Building Docker image $IMAGE (stale or missing) ==="
    _build
else
    echo "ERROR: $IMAGE image is stale or missing."
    if [ "$SERVER" = "apache" ]; then
        echo "Run: make web-docker-build-apache"
    else
        echo "Run: make web-docker-build-nginx"
    fi
    exit 1
fi
