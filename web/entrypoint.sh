#!/bin/sh
set -e

# Symlink album data from /albums (the external mount) into build/albums/ so
# Apache can serve images alongside the pre-rendered .html pages.
#
# Only items not already present in build/albums/ get a symlink — this preserves
# pre-rendered files (antarctica.html, config.json, etc.) from the build step.
# Existing dangling symlinks (from a previous container run) are refreshed.

BUILD_ALBUMS=/usr/local/apache2/htdocs/build/albums

for item in /albums/*; do
    [ -e "$item" ] || continue          # skip if glob matched nothing
    name=$(basename "$item")
    target="$BUILD_ALBUMS/$name"
    if [ -d "$item" ]; then
        # Album slug directory: always replace with a symlink to /albums/<slug>.
        # The pre-rendered dir only has index.json; images live in /albums/<slug>.
        # The symlink serves index.json correctly (identical content).
        rm -rf "$target"
        ln -s "$item" "$target"
    elif [ -L "$target" ]; then
        ln -sf "$item" "$target"        # refresh existing file symlink
    elif [ ! -e "$target" ]; then
        ln -s "$item" "$target"         # create symlink for files not in build
    fi
    # real file already exists (pre-rendered .html / .json) — leave it alone
done

exec httpd-foreground
