#!/bin/bash
set -e

# Surge CLI via npx. On first run surge downloads into the npm cache
# (mounted as a named Docker volume by the ddphotos wrapper so it persists).
# Credentials are stored in ~/.netrc (mounted read-write from the host by the wrapper).
# Surge does not follow symlinks — the export directory must be created with --copy.

# Find the deploy directory: first non-flag arg (skip --flag value pairs).
DEPLOY_DIR=""
skip_next=false
for arg in "$@"; do
    if $skip_next; then skip_next=false; continue; fi
    if [[ "$arg" == --* ]]; then
        [[ "$arg" != *=* ]] && skip_next=true
        continue
    fi
    DEPLOY_DIR="$arg"
    break
done

# Only validate if DEPLOY_DIR looks like a path (contains / or is . or ..).
# Bare words like 'list', 'whoami', 'login' are surge subcommands, not directories.
if [ -n "$DEPLOY_DIR" ] && [[ "$DEPLOY_DIR" == */* || "$DEPLOY_DIR" == "." || "$DEPLOY_DIR" == ".." ]]; then
    if [ ! -d "$DEPLOY_DIR" ]; then
        echo "Error: $DEPLOY_DIR not found. Run 'ddphotos export --copy' first." >&2
        exit 1
    elif [ -L "$DEPLOY_DIR/index.html" ]; then
        echo "Error: $DEPLOY_DIR contains symlinks. Run 'ddphotos export --copy' (not just 'export') — Surge does not follow symlinks." >&2
        exit 1
    fi
fi

exec npx --yes surge "$@"
