#!/bin/sh
set -e

CONFIG="/ddphotos/config"

if [ ! -d "/ddphotos" ]; then
    echo "Error: /ddphotos is not mounted. Add: -v ~/my-ddphotos:/ddphotos"
    exit 1
fi

if [ -f "$CONFIG/albums.yaml" ]; then
    echo "Error: $CONFIG/albums.yaml already exists. Remove it to re-initialize."
    exit 1
fi

mkdir -p "$CONFIG" /ddphotos/albums /ddphotos/build

cp /docker/init/albums.yaml "$CONFIG/albums.yaml"
cp /docker/init/description.txt "$CONFIG/description.txt"

echo "Initialized $CONFIG"
echo ""
echo "Next steps:"
echo "  1. Edit ~/my-ddphotos/config/albums.yaml to define your albums"
echo "  2. Run: docker run -v ~/photos:/photos:ro -v ~/my-ddphotos:/ddphotos ddphotos photogen"
echo "  3. Run: docker run -v ~/my-ddphotos:/ddphotos ddphotos build"
echo "  4. Run: docker run -v ~/my-ddphotos:/ddphotos -p 8000:80 ddphotos serve"
