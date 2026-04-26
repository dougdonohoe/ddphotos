#!/bin/sh
set -e

cmd="${1:-help}"
shift 2>/dev/null || true

case "$cmd" in
    init)     exec /docker/do-init.sh "$@" ;;
    photogen) exec /docker/do-photogen.sh "$@" ;;
    build)    exec /docker/do-build.sh "$@" ;;
    serve)    exec /docker/do-serve.sh "$@" ;;
    run)      exec /docker/do-run.sh "$@" ;;
    deploy)   exec /docker/do-deploy.sh "$@" ;;
    upgrade)
        if diff -q /docker/ddphotos /ddphotos/ddphotos > /dev/null 2>&1; then
            echo "ddphotos is up to date."
        else
            /bin/cp /docker/ddphotos /ddphotos/ddphotos.new
            chmod +x /ddphotos/ddphotos.new
            /bin/mv -f /ddphotos/ddphotos.new /ddphotos/ddphotos
            echo "ddphotos script upgraded."
        fi
        ;;
    *)
        echo "Usage: docker run ddphotos {init|photogen|build|serve|run|deploy|upgrade}"
        echo ""
        echo "Commands:"
        echo "  init      Create ~/my-ddphotos config scaffold"
        echo "  photogen  Process source photos into albums output"
        echo "  build     Build the static site"
        echo "  serve     Preview the site via Apache on port 80"
        echo "  run       Preview the site via Vite dev server on port 5173"
        echo "  deploy    Rsync build and albums to a remote host"
        echo "  upgrade   Update the ddphotos script to match this image"
        exit 1
        ;;
esac
