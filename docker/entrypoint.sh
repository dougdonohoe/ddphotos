#!/bin/sh
set -e

cmd="${1:-help}"
shift 2>/dev/null || true

# Verify the mounted ddphotos script matches the image (skip for init/upgrade/version)
if [ "$cmd" != "init" ] && [ "$cmd" != "upgrade" ] && [ "$cmd" != "version" ]; then
    if [ -f /ddphotos-script-dir/ddphotos ] && ! diff -q /docker/ddphotos /ddphotos-script-dir/ddphotos > /dev/null 2>&1; then
        echo "WARNING:  The local 'ddphotos' script does not match the image." >&2
        echo "          Run: 'ddphotos upgrade' to fix this." >&2
        echo "" >&2
    fi
fi

case "$cmd" in
    init)         exec /docker/do-init.sh "$@" ;;
    photogen)     exec /docker/do-photogen.sh "$@" ;;
    decode)       exec /docker/do-decode.sh "$@" ;;
    search-cover) exec /docker/do-search-cover.sh "$@" ;;
    build)        exec /docker/do-build.sh "$@" ;;
    serve)        exec /docker/do-serve.sh "$@" ;;
    run)          exec /docker/do-run.sh "$@" ;;
    export)       exec /docker/do-export.sh "$@" ;;
    deploy)       exec /docker/do-deploy.sh "$@" ;;
    version)
        echo "Version:  $(cat /docker/VERSION 2>/dev/null || echo unknown)"
        echo "Git:      $(cat /docker/GIT_DESCRIBE 2>/dev/null || echo unknown)"
        ;;
    upgrade)
        SCRIPT=/ddphotos-script-dir/ddphotos
        VERSION=$(cat /docker/VERSION 2>/dev/null || echo "dev")
        if diff -q /docker/ddphotos "$SCRIPT" > /dev/null 2>&1; then
            echo "The 'ddphotos' script is up to date ($VERSION)."
        else
            /bin/cp /docker/ddphotos "${SCRIPT}.new"
            chmod +x "${SCRIPT}.new"
            /bin/mv -f "${SCRIPT}.new" "$SCRIPT"
            echo "The 'ddphotos' script was upgraded ($VERSION)."
        fi
        ;;
    *)
        echo "Unknown command: '$cmd'" >&2
        echo >&2
        echo "This image is intended to be used via the 'ddphotos' wrapper script." >&2
        echo "See: https://github.com/dougdonohoe/ddphotos" >&2
        echo >&2
        exit 1
        ;;
esac
