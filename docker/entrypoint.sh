#!/bin/bash
set -e

# ensure files/dirs created by the container are world-writable so the host user can modify them
umask 0000

# Capture cmd, default to help, remove it from args
cmd="${1:-help}"
if [ "$#" -gt 0 ]; then shift; fi

# Verify the mounted ddphotos script matches the image (only for commands that use it)
case "$cmd" in
    photogen|decode|search-cover|build|serve|run|export|deploy)
        if [ -f /ddphotos-script-dir/ddphotos ] && ! diff -q /docker/ddphotos /ddphotos-script-dir/ddphotos > /dev/null 2>&1; then
            echo "WARNING:  The local 'ddphotos' script does not match the image." >&2
            echo "          Run: 'ddphotos upgrade' to fix this." >&2
            echo "" >&2
        fi
        ;;
esac

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
    help|*)
        [ "$cmd" != "help" ] && { echo "Unknown command: '$cmd'" >&2; echo >&2; }
        echo "Usage: ddphotos [OPTIONS] COMMAND"
        echo ""
        echo "Commands: init, photogen, decode, search-cover, build, serve, run, export, deploy, upgrade, version"
        echo ""
        echo "This image is intended to be used via the 'ddphotos' wrapper script."
        echo "See: https://github.com/dougdonohoe/ddphotos"
        echo ""
        [ "$cmd" = "help" ] || exit 1
        ;;
esac
