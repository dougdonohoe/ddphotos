#!/usr/bin/env bash
set -e

SDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SDIR/.."

REPO="dougdonohoe/ddphotos"

DOIT=false
for arg in "$@"; do
    [ "$arg" = "--doit" ] && DOIT=true
done

# Determine version tag
if ! git diff --quiet || ! git diff --cached --quiet; then
    VERSION="dev"
else
    VERSION=$(git tag --points-at HEAD | grep '^v' | sort -V | tail -1)
    VERSION="${VERSION:-dev}"
fi

# Confirm before pushing
# IMAGE_TAG is baked into the script via DDPHOTOS_IMAGE so `ddphotos` knows which image to pull.
# Use :latest for releases (auto-updates on docker pull) and :dev for dev builds.
if [ "$VERSION" = "dev" ]; then
    TAGS=("-t" "$REPO:dev")
    IMAGE_TAG="$REPO:dev"
    echo "Pushing: $REPO:dev  (dirty or untagged)"
else
    TAGS=("-t" "$REPO:$VERSION" "-t" "$REPO:latest")
    IMAGE_TAG="$REPO:$VERSION"
    echo "Pushing: $REPO:$VERSION  +  $REPO:latest"
fi
echo ""
if ! $DOIT; then
    read -r -p "Continue? [y/N] " answer
    [[ "$answer" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 1; }
    echo ""
fi

# Ensure buildx builder exists
if ! docker buildx inspect ddphotos-builder > /dev/null 2>&1; then
    echo "Creating buildx builder..."
    docker buildx create --name ddphotos-builder --use
else
    docker buildx use ddphotos-builder
fi

GIT_DESCRIBE=$(git describe --tags --long --dirty --always 2>/dev/null || echo "unknown")
NODE_VERSION=$(cat web/.nvmrc)

docker buildx build \
    --platform linux/amd64,linux/arm64 \
    --build-arg NODE_VERSION="$NODE_VERSION" \
    --build-arg DDPHOTOS_VERSION="$VERSION" \
    --build-arg DDPHOTOS_GIT_DESCRIBE="$GIT_DESCRIBE" \
    --build-arg DDPHOTOS_IMAGE="$IMAGE_TAG" \
    "${TAGS[@]}" \
    -f docker/Dockerfile \
    --push \
    .

# pull what we just built (skipped in CI — runner is ephemeral)
if [ "${CI}" != "true" ]; then
    echo ""
    docker pull "$IMAGE_TAG"
fi

echo ""
echo "Done: https://hub.docker.com/r/$REPO/tags"
