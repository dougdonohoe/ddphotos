#!/bin/sh
set -e

cmd="${1:-help}"
shift 2>/dev/null || true

# Verify the mounted ddphotos script matches the image (skip for init/upgrade)
if [ "$cmd" != "init" ] && [ "$cmd" != "upgrade" ]; then
    if [ -f /ddphotos-script-dir/ddphotos ] && ! diff -q /docker/ddphotos /ddphotos-script-dir/ddphotos > /dev/null 2>&1; then
        echo "Error: local ddphotos script does not match the image."
        echo "Run: ./ddphotos upgrade"
        exit 1
    fi
fi

case "$cmd" in
    init)     exec /docker/do-init.sh "$@" ;;
    photogen) exec /docker/do-photogen.sh "$@" ;;
    build)    exec /docker/do-build.sh "$@" ;;
    serve)    exec /docker/do-serve.sh "$@" ;;
    run)      exec /docker/do-run.sh "$@" ;;
    deploy)   exec /docker/do-deploy.sh "$@" ;;
    upgrade)
        SCRIPT=/ddphotos-script-dir/ddphotos
        if diff -q /docker/ddphotos "$SCRIPT" > /dev/null 2>&1; then
            echo "ddphotos is up to date."
        else
            /bin/cp /docker/ddphotos "${SCRIPT}.new"
            chmod +x "${SCRIPT}.new"
            /bin/mv -f "${SCRIPT}.new" "$SCRIPT"
            echo "ddphotos script upgraded."
        fi
        ;;
    *)
        echo "Usage: docker run ddphotos {init|photogen|build|serve|run|deploy|upgrade}"
        echo ""
        echo "Commands:"
        echo "  init      Create config scaffold (--script-only to install 'ddphotos' script only)"
        echo "  photogen  Process source photos into albums output"
        echo "  build     Build the static site"
        echo "  serve     Preview the site via Apache on port 80"
        echo "  run       Preview the site via Vite dev server on port 5173"
        echo "  deploy    Rsync build and albums to a remote host"
        echo "  upgrade   Update the ddphotos script to match this image"
        exit 1
        ;;
esac
