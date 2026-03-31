#!/usr/bin/env bash
# Run Playwright tests against all sample site variants:
#   1. No passwords (plain site)
#   2. passwords-all.yaml (entire site encrypted + Uganda per-album)
#   3. passwords-uganda.yaml (Uganda album only)
#
# Usage:
#   bin/test-all.sh [--mode dev|apache|both]
#
# Passes --mode through to bin/run-tests.sh (default: both).

set -eo pipefail

MODE="both"

usage() {
    echo "Usage: bin/test-all.sh [--mode dev|apache|both]"
    echo ""
    echo "Runs Playwright tests against all password variants of the sample site:"
    echo "  1. No passwords (plain site)"
    echo "  2. passwords-all.yaml (entire site + Uganda album encrypted)"
    echo "  3. passwords-uganda.yaml (Uganda album only)"
    echo ""
    echo "Options:"
    echo "  --mode <mode>  Server to test against: dev, apache, or both (default: both)."
    echo "                   dev    — Vite dev server on port 5174"
    echo "                   apache — static build + Docker/Apache on port 8083"
    echo "                   both   — dev first, then apache"
    echo "  --help, -?     Show this help message and exit."
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode)        MODE="$2"; shift 2 ;;
        --mode=*)      MODE="${1#*=}"; shift ;;
        --help|-\?)    usage; exit 0 ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

SDIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")
cd "$SDIR/.."

OVERALL_EXIT=0
trap 'exit 130' INT TERM

run_variant() {
    local label="$1"
    shift
    echo ""
    echo "###############################################################"
    echo "# Variant: $label"
    echo "###############################################################"
    bin/run-tests.sh "$@" --mode "$MODE" || OVERALL_EXIT=$?
}

run_variant "no passwords"
run_variant "passwords-all.yaml"    --passwords sample/config/passwords-all.yaml
run_variant "passwords-uganda.yaml" --passwords sample/config/passwords-uganda.yaml

if [ "$OVERALL_EXIT" -eq 0 ]; then
    echo ""
    echo "All variants passed."
else
    echo ""
    echo "One or more variants failed." >&2
fi

exit $OVERALL_EXIT
