#!/bin/bash
set -e

# Cloudflare CLI via npx. On first run wrangler downloads into the npm cache
# (mounted as a named Docker volume by the ddphotos wrapper so it persists).
# Credentials via 'wrangler login' (port 8976 is exposed by the wrapper for the
# OAuth callback) or CLOUDFLARE_API_TOKEN. Stored in ddphotos-wrangler-config volume.

# For 'wrangler pages deploy', verify the export was run with --cloudflare.
# The deploy directory is the first non-flag arg after 'pages deploy'.
if [[ "${1:-}" == "pages" && "${2:-}" == "deploy" ]]; then
    DEPLOY_DIR=""
    skip_next=false
    for arg in "${@:3}"; do
        if $skip_next; then skip_next=false; continue; fi
        if [[ "$arg" == --* ]]; then
            # If the flag doesn't embed its value (--flag=value), the next arg is the value
            [[ "$arg" != *=* ]] && skip_next=true
            continue
        fi
        DEPLOY_DIR="$arg"
        break
    done
    if [ -n "$DEPLOY_DIR" ]; then
        if [ ! -d "$DEPLOY_DIR" ]; then
            echo "Error: $DEPLOY_DIR not found. Run 'export --cloudflare' first."
            exit 1
        elif [ ! -f "$DEPLOY_DIR/_worker.js" ]; then
            echo "Error: $DEPLOY_DIR/_worker.js not found. Run 'export --cloudflare' (not just 'export')."
            exit 1
        fi
    fi
fi

exec npx --yes wrangler "$@"
