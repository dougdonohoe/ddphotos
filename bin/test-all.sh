#!/usr/bin/env bash
# Run Playwright tests against all sample site variants:
#   1. No passwords (plain site)
#   2. passwords-all.yaml (entire site encrypted + Uganda per-album)
#   3. config-extras: passwords-keyonly.yaml + custom CSS + album-nav customization
#   4. passwords-uganda.yaml (Uganda album only)
#
# Usage:
#   bin/test-all.sh [--mode dev|apache|nginx|all]
#
# Passes --mode through to bin/run-tests.sh (default: all).
#
#
# --- Why four variants and not six ---
#
# keyonly, custom-css and album-nav used to be three separate runs. Diffing their generated
# sites against the plain one shows why they no longer are: each differs from plain by a
# single config.json field, and nothing else. Every WebP, MP4 and index.json is identical.
#
#   variant             entries differing from the plain site
#   passwords-keyonly   1   config.json gains "keyId"
#   custom-css          2   config.json gains "customCss", plus custom.css itself
#   album-nav           1   config.json gains "albumNav"
#   passwords-uganda    89  Uganda HMAC'd, no cover.jpg, albums.json differs
#   passwords-all       278 all three albums HMAC'd, albums.enc.json, no covers
#
# The three fields are orthogonal, so one run with all three set covers what the three
# separate runs did. Measured: the union of the three runs was 69 tests, the combined run
# is 67, and the only two it drops are the negative assertions ("CSS link is NOT present
# when not configured", "default back link when album_nav is not configured"), which still
# run in every other variant. That took ~114s off each e2e job.
#
# What it gives up: "css alone" and "album-nav alone" are no longer exercised. Plain covers
# neither-configured and this covers both-configured, so only the exactly-one combinations
# are untested. Split it back out if that ever matters.
#
#
# --- Site IDs ---
#
# Assigned per variant rather than derived, so three of the four share one albums directory
# and photogen has no media left to generate for them. What decides the grouping is which
# albums each variant encrypts, and nothing else (see the "Site ID" comment in
# bin/run-tests.sh):
#
#   variant              the-way  uganda  antarctica   site ID
#   no passwords          plain    plain    plain      sample
#   config-extras         plain    plain    plain      sample
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
    echo "  3. config-extras (passwords-keyonly.yaml + custom CSS + album-nav customization)"
    echo "  4. passwords-uganda.yaml (Uganda album only)"
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

# passwords-uganda runs last of the three sharing "sample". It is the only one of them
# whose media set differs (HMAC'd Uganda rather than plain), so running it anywhere else
# in the order would have -clean delete Uganda's WebPs and the next variant regenerate
# them, paying that twice instead of once.
#
# PLAYWRIGHT_SKIP_VIDEO on the last two: their video outputs are byte-identical to the
# plain site's (same sources, same encode settings, same filenames), so the video suite is
# re-testing files it already tested. passwords-all skips it on its own, because a locked
# site publishes no discoverable video. See web/tests/video.spec.ts.
run_variant "no passwords"          --site-id sample
run_variant "passwords-all.yaml"    --site-id sample-pw-all --passwords sample/config/passwords-all.yaml
run_variant "config-extras"         --site-id sample --skip-video \
    --passwords sample/config/passwords-keyonly.yaml \
    --css sample/config/custom-example.css \
    --customization sample/config/customization-album-nav.yaml
run_variant "passwords-uganda.yaml" --site-id sample --skip-video --passwords sample/config/passwords-uganda.yaml

echo ""
echo "All variants passed."
