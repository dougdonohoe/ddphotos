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
cp /docker/ddphotos /ddphotos/ddphotos
chmod +x /ddphotos/ddphotos

echo "Initialized ~/my-ddphotos"
echo ""
echo "Next steps - generate, run, build and serve the example site:"
echo ""
echo "  1. cd ~/my-ddphotos"
echo "  2. ./ddphotos photogen"
echo "  3. ./ddphotos run"
echo "  4. ./ddphotos build"
echo "  5. ./ddphotos serve"
echo ""
echo "Then build your site:"
echo ""
echo "  1. Edit ~/my-ddphotos/config/albums.yaml to define your own albums"
echo "  2. Repeat photogen, run, build, serve"