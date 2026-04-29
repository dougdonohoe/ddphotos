#!/bin/sh
set -e

if [ ! -d "/ddphotos" ]; then
    echo "Error: /ddphotos is not mounted. Add: -v ~/my-ddphotos:/ddphotos"
    exit 1
fi

# Always copy script
cp /docker/ddphotos /ddphotos/ddphotos
chmod +x /ddphotos/ddphotos

# --script-only: install just the ddphotos wrapper script, skip config scaffold
if [ "${1:-}" = "--script-only" ]; then
    echo "Standalone 'ddphotos' script installed."
    echo
    echo "Usage with a separate albums directory:"
    echo
    echo "  ddphotos --dir ~/my-ddphotos photogen|run|build|serve|upgrade|version"
    echo
    exit 0
fi

# Create config files
CONFIG="/ddphotos/config"

if [ -f "$CONFIG/albums.yaml" ]; then
    echo "Error: $CONFIG/albums.yaml already exists. Remove it to re-initialize."
    exit 1
fi

mkdir -p "$CONFIG" /ddphotos/albums /ddphotos/build
cp /docker/init/albums.yaml "$CONFIG/albums.yaml"
cp /docker/init/descriptions.txt "$CONFIG/descriptions.txt"
cp /docker/init/custom.css "$CONFIG/custom.css"
cp /docker/init/passwords.yaml "$CONFIG/passwords.yaml"

echo "Initialized ddphotos!"
echo
echo "Next steps - generate, run, build and serve the example site:"
echo
echo "  1. cd [your-ddphotos-dir] # the same directory in your -v parameter to 'docker run'"
echo "  2. ./ddphotos photogen    # to resize images and create index files"
echo "  3. ./ddphotos run         # to run dev server"
echo "  4. ./ddphotos build       # to build static site"
echo "  5. ./ddphotos serve       # to serve static site via Apache"
echo
echo "Then build your site:"
echo
echo "  1. Edit config/albums.yaml to define your own albums"
echo "  2. Repeat photogen, run, build, serve"
echo
echo "When ready to deploy, configure your   config/site.env for rsync or s3"
echo
echo "  1. ./ddphotos deploy"
echo
echo "Docs:  https://github.com/dougdonohoe/ddphotos"
echo
