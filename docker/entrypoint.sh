#!/bin/sh
set -e

cmd="${1:-help}"
shift 2>/dev/null || true

# Verify the mounted ddphotos script matches the image (skip for init/upgrade/version)
if [ "$cmd" != "init" ] && [ "$cmd" != "upgrade" ] && [ "$cmd" != "version" ]; then
    if [ -f /ddphotos-script-dir/ddphotos ] && ! diff -q /docker/ddphotos /ddphotos-script-dir/ddphotos > /dev/null 2>&1; then
        echo "WARNING:  The local 'ddphotos' script does not match the image."
        echo "          Run: 'ddphotos upgrade' to fix this."
        echo ""
    fi
fi

case "$cmd" in
    init)     exec /docker/do-init.sh "$@" ;;
    photogen) exec /docker/do-photogen.sh "$@" ;;
    decode)   exec /docker/do-decode.sh "$@" ;;
    build)    exec /docker/do-build.sh "$@" ;;
    serve)    exec /docker/do-serve.sh "$@" ;;
    run)      exec /docker/do-run.sh "$@" ;;
    export)   exec /docker/do-export.sh "$@" ;;
    deploy)   exec /docker/do-deploy.sh "$@" ;;
    version)
        echo "Version:  $(cat /docker/VERSION 2>/dev/null || echo unknown)"
        echo "Git:      $(cat /docker/GIT_DESCRIBE 2>/dev/null || echo unknown)"
        ;;
    upgrade)
        SCRIPT=/ddphotos-script-dir/ddphotos
        VERSION=$(cat /docker/VERSION 2>/dev/null || echo "dev")
        if diff -q /docker/ddphotos "$SCRIPT" > /dev/null 2>&1; then
            echo "ddphotos is up to date ($VERSION)."
        else
            /bin/cp /docker/ddphotos "${SCRIPT}.new"
            chmod +x "${SCRIPT}.new"
            /bin/mv -f "${SCRIPT}.new" "$SCRIPT"
            echo "ddphotos script upgraded ($VERSION)."
        fi
        ;;
    *)
        echo "Usage: docker run ddphotos {init|photogen|decode|build|serve|run|export|deploy|upgrade}"
        echo ""
        echo "Commands:"
        echo "  init      Create config scaffold (--script-only to install 'ddphotos' script only)"
        echo "  photogen  Process source photos into albums output"
        echo "  decode    Decrypt an .enc.json file and print the contents"
        echo "  build     Build the static site"
        echo "  serve     Preview the site via Apache on port 80"
        echo "  run       Preview the site via Vite dev server on port 5173"
        echo "  export    Export site to export/<site-id>/ for local serving"
        echo "  deploy    Rsync build and albums to a remote host"
        echo "  upgrade   Update the ddphotos script to match this image"
        exit 1
        ;;
esac
