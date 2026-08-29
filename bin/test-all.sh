#!/usr/bin/env bash
# Run Playwright tests against all sample site variants:
#   1. No passwords (plain site)
#   2. passwords-all.yaml (entire site encrypted + Uganda per-album)
#   3. passwords-keyonly.yaml (passwords file with no effective password — nothing encrypted)
#   4. custom-css (sample/config/custom-example.css injected)
#   5. album-nav (customization.yaml replacing the album page's "← Albums" link)
#   6. passwords-uganda.yaml (Uganda album only)
#
# Usage:
#   bin/test-all.sh [--mode dev|apache|nginx|all]
#
# Passes --mode through to bin/run-tests.sh (default: all).
#
# Site IDs are assigned per variant rather than derived, so five of the six share one
# albums directory and photogen has no media left to generate for them. What decides the
# grouping is which albums each variant encrypts, and nothing else (see the "Site ID"
# comment in bin/run-tests.sh):
#
#   variant              the-way  uganda  antarctica   site ID
#   no passwords          plain    plain    plain      sample
#   passwords-keyonly     plain    plain    plain      sample     (protects nothing)
#   custom-css            plain    plain    plain      sample
#   album-nav             plain    plain    plain      sample
#   passwords-uganda      plain    HMAC     plain      sample     (runs last, see below)
#   passwords-all         HMAC     HMAC     HMAC       sample-pw-all
#
# passwords-all needs its own directory: a site-wide password encrypts every album, so
# every output filename is HMAC'd and none of the plain media is reusable.

set -eo pipefail

MODE="all"

usage() {
    echo "Usage: bin/test-all.sh [--mode dev|apache|nginx|all]"
    echo ""
    echo "Runs Playwright tests against all sample site variants:"
    echo "  1. No passwords (plain site)"
    echo "  2. passwords-all.yaml (entire site + Uganda album encrypted)"
    echo "  3. passwords-keyonly.yaml (passwords file with no effective password — nothing encrypted)"
    echo "  4. custom-css (sample/config/custom-example.css injected)"
    echo "  5. album-nav (customization.yaml replacing the album page's \"← Albums\" link)"
    echo "  6. passwords-uganda.yaml (Uganda album only)"
    echo ""
    echo "Options:"
    echo "  --mode <mode>  Server to test against: dev, apache, nginx, or all (default: all)."
    echo "                   dev    — Vite dev server on port 5174"
    echo "                   apache — static build + Docker/Apache on port 8083"
    echo "                   nginx  — static build + Docker/nginx on port 8084"
    echo "                   all    — dev, apache, and nginx"
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

trap 'exit 130' INT TERM

run_variant() {
    local label="$1"
    shift
    echo ""
    echo "###############################################################"
    echo "# Variant: $label"
    echo "###############################################################"
    bin/run-tests.sh "$@" --mode "$MODE"
}

# passwords-uganda runs last of the five sharing "sample". It is the only one of them
# whose media set differs (HMAC'd Uganda rather than plain), so running it anywhere else
# in the order would have -clean delete Uganda's WebPs and the next variant regenerate
# them, paying that twice instead of once.
run_variant "no passwords"           --site-id sample
run_variant "passwords-all.yaml"     --site-id sample-pw-all --passwords sample/config/passwords-all.yaml
run_variant "passwords-keyonly.yaml" --site-id sample        --passwords sample/config/passwords-keyonly.yaml
run_variant "custom-css"             --site-id sample        --css sample/config/custom-example.css
run_variant "album-nav"              --site-id sample        --customization sample/config/customization-album-nav.yaml
run_variant "passwords-uganda.yaml"  --site-id sample        --passwords sample/config/passwords-uganda.yaml

echo ""
echo "All variants passed."
