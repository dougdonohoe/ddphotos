#!/bin/bash
set -e

if [ ! -d "/ddphotos" ]; then
    echo "Error: /ddphotos is not mounted. Add: -v ~/my-ddphotos:/ddphotos" >&2
    exit 1
fi

_mp=$(grep ' /ddphotos ' /proc/self/mountinfo 2>/dev/null | tail -1)
_named=false
# Linux: named volume path appears in mountinfo source
echo "$_mp" | grep -q 'docker/volumes' && _named=true
# Docker Desktop Mac: bind mounts use virtiofs/grpcfuse/osxfs; named volumes don't
if [ "$_named" = "false" ] && [ -n "$_mp" ]; then
    if grep -qE 'virtiofs|grpcfuse|osxfs' /proc/mounts 2>/dev/null; then
        echo "$_mp" | grep -qE 'virtiofs|grpcfuse|osxfs' || _named=true
    fi
fi
if [ "$_named" = "true" ]; then
    echo "Error: /ddphotos is mounted as a Docker named volume, not a host directory." >&2
    echo "Files written inside the container will not appear on your filesystem." >&2
    echo "Use a bind mount with a relative or absolute path:" >&2
    echo "  docker run -v ./my-dir:/ddphotos ddphotos init" >&2
    exit 1
fi

SCRIPT_ONLY=""
SITE_ID="my-photos"
WINDOWS=""
while [[ "${1:-}" == --* ]]; do
    case "$1" in
        --script-only) SCRIPT_ONLY=1; shift ;;
        --site-id) SITE_ID="$2"; shift 2 ;;
        --windows) WINDOWS=1; shift ;;
        *)
            echo "Unknown option: $1" >&2
            echo "" >&2
            echo "Usage: ddphotos init [--script-only] [--site-id ID] [--windows]" >&2
            echo "" >&2
            echo "  --script-only    Install just the 'ddphotos' wrapper script, skip config scaffold" >&2
            echo "  --site-id ID     Site ID written into albums.yaml (default: my-photos)" >&2
            echo "  --windows        Also install the 'ddphotos.cmd' Windows launcher" >&2
            exit 1
            ;;
    esac
done

# Always copy script
cp /docker/ddphotos /ddphotos/ddphotos
chmod +x /ddphotos/ddphotos

# On Windows hosts, also install the .cmd launcher (the host can't be detected
# from inside the container, so the caller signals it with --windows).
if [ -n "$WINDOWS" ]; then
    cp /docker/ddphotos.cmd /ddphotos/ddphotos.cmd
fi

# --script-only: install just the ddphotos wrapper script, skip config scaffold
if [ -n "$SCRIPT_ONLY" ]; then
    echo "Standalone 'ddphotos' script installed."
    echo
    echo "Use --dir to specify your DD Photos directory (the one with config|albums|build|export):"
    echo
    echo "  ddphotos --dir ~/my-ddphotos [command]"
    echo
    exit 0
fi

# Create config files
CONFIG="/ddphotos/config"

if [ -f "$CONFIG/albums.yaml" ]; then
    echo "Error: $CONFIG/albums.yaml already exists. Remove it to re-initialize." >&2
    exit 1
fi

# /docker/init holds the whole starter site (config/ and sample-photos/) laid out exactly
# as it should appear on the user's disk, so installing it is one recursive copy. The
# sample photos land next to config/ as ordinary files the user can see, edit and delete;
# albums.yaml reaches them via the relative base 'sample-base'.
cp -r /docker/init/. /ddphotos/

# Everything just copied is root-owned with the image's restrictive modes (files
# rw-r--r--, dirs rwxr-xr-x). entrypoint.sh's umask 0000 does not help: it applies to
# the container's own filesystem, while the /ddphotos bind mount imposes its own default
# mode (644/755) on new entries. So widen the mode explicitly - it is the only thing
# letting the host user edit config, recaption sample photos, and delete either.
# Driving the loop off the source keeps this correct as the starter site grows.
# 'X' adds +x for directories only, leaving file bits alone.
for _entry in /docker/init/*; do
    chmod -R a+rwX "/ddphotos/${_entry##*/}"
done

# Generated output directories - not part of the starter site, and albums/ honors
# DDPHOTOS_ALBUMS_DIR, so these are created rather than baked into the image.
mkdir -p "$DDPHOTOS_ALBUMS_DIR" /ddphotos/build /ddphotos/export

sed -i "s/__SITE_ID__/$SITE_ID/g" "$CONFIG/albums.yaml"

echo "Initialized ddphotos (site-id=$SITE_ID)!"
echo
echo "Next steps - generate, run, build and serve the example site:"
echo
echo "  1. cd [your-ddphotos-dir] # the same directory in your -v parameter to 'docker run'"
echo "  2. ./ddphotos photogen    # to resize images and create index files"
echo "  3. ./ddphotos run         # to run dev server"
echo "  4. ./ddphotos build       # to build static site"
echo "  5. ./ddphotos serve       # to serve static site via Apache"
echo
echo "The starter albums use the photos in sample-photos/ (see sample-photos/README.md)."
echo
echo "Then build your site:"
echo
echo "  1. Edit config/albums.yaml to define your own albums"
echo "  2. Repeat photogen, run, build, serve"
echo "  3. Delete sample-photos/ and the starter albums once you no longer need them"
echo
echo "When ready to deploy, configure your config/site.env for rsync or s3 and deploy, or export"
echo
echo "  1. ./ddphotos deploy"
echo "  2. ./ddphotos export [--copy] [--cloudflare]"
echo
echo "Docs:  https://github.com/dougdonohoe/ddphotos/blob/main/docs/DOCKER.md"
echo
