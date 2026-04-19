#!/usr/bin/env bash
# Run Playwright tests against all sample site variants:
#   1. No passwords (plain site)
#   2. passwords-all.yaml (entire site encrypted + Uganda per-album)
#   3. passwords-uganda.yaml (Uganda album only)
#   4. custom-css (sample/config/custom.css injected)
#
# Usage:
#   bin/test-all.sh [--mode dev|apache|nginx|all] [--ci]
#
# Passes --mode and --ci through to bin/run-tests.sh (default: all).

set -eo pipefail

MODE="all"
CI_FLAG=""

usage() {
    echo "Usage: bin/test-all.sh [--mode dev|apache|nginx|all] [--ci]"
    echo ""
    echo "Runs Playwright tests against all sample site variants:"
    echo "  1. No passwords (plain site)"
    echo "  2. passwords-all.yaml (entire site + Uganda album encrypted)"
    echo "  3. passwords-uganda.yaml (Uganda album only)"
    echo "  4. custom-css (sample/config/custom.css injected)"
    echo ""
    echo "Options:"
    echo "  --mode <mode>  Server to test against: dev, apache, nginx, or all (default: all)."
    echo "                   dev    — Vite dev server on port 5174"
    echo "                   apache — static build + Docker/Apache on port 8083"
    echo "                   nginx  — static build + Docker/nginx on port 8084"
    echo "                   all    — dev, apache, and nginx"
    echo "  --ci           Skip photogen if albums/<site-id> exists; skip npm build if build/ exists."
    echo "  --help, -?     Show this help message and exit."
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode)        MODE="$2"; shift 2 ;;
        --mode=*)      MODE="${1#*=}"; shift ;;
        --ci)          CI_FLAG="--ci"; shift ;;
        --help|-\?)    usage; exit 0 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

trap 'exit 130' INT TERM

run_variant() {
    local label="$1"
    shift
    echo ""
    echo "###############################################################"
    echo "# Variant: $label"
    echo "###############################################################"
    bin/run-tests.sh "$@" --mode "$MODE" $CI_FLAG
}

run_variant "no passwords"
run_variant "passwords-all.yaml"    --passwords sample/config/passwords-all.yaml
run_variant "passwords-uganda.yaml" --passwords sample/config/passwords-uganda.yaml
run_variant "custom-css"            --css sample/config/custom.css

echo ""
echo "All variants passed."
