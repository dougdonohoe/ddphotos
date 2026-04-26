#!/bin/sh
set -e

cmd="${1:-help}"
shift 2>/dev/null || true

case "$cmd" in
    init)     exec /docker/do-init.sh "$@" ;;
    photogen) exec /docker/do-photogen.sh "$@" ;;
    build)    exec /docker/do-build.sh "$@" ;;
    serve)    exec /docker/do-serve.sh "$@" ;;
    deploy)   exec /docker/do-deploy.sh "$@" ;;
    *)
        echo "Usage: docker run ddphotos {init|photogen|build|serve|deploy}"
        echo ""
        echo "Commands:"
        echo "  init      Create ~/my-ddphotos config scaffold"
        echo "  photogen  Process source photos into albums output"
        echo "  build     Build the static site"
        echo "  serve     Preview the site via Apache on port 80"
        echo "  deploy    Rsync build and albums to a remote host"
        exit 1
        ;;
esac
