#!/bin/bash
#
# Reports when the Node or npm version this repo pins has fallen behind upstream.
#
# web/.nvmrc and web/.npm-version hold *exact* versions on purpose (see CLAUDE.md). A
# floating tag like `node:24` means the same commit builds a different image depending on
# the day you built it, and an upstream regression then arrives with no commit to bisect.
# Node 24.2.0 broke recursive fs.cpSync onto Docker bind mounts exactly that way.
#
# The cost of pinning exactly is that nothing tells you when a bugfix or security release
# ships. Dependabot cannot fill that gap: it has no .nvmrc ecosystem (requested since
# 2020), and its docker ecosystem cannot read docker/Dockerfile's
# `FROM node:${NODE_VERSION}-bookworm-slim` because there is no literal tag there to bump.
# So this script does the comparison, and .github/workflows/version-drift.yml runs it
# nightly and opens an issue.
#
# It deliberately does not edit anything. Bumping Node is a decision that CI then tests,
# not something that lands unattended.
#
# Exit codes:
#   0  every pin is current
#   1  at least one pin is behind (the markdown report on stdout says which)
#   2  the check itself could not run (missing tool, network, unexpected upstream shape)
#
# Usage: bin/check-versions.sh

set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

fail() {
    echo "Error: $*" >&2
    exit 2
}

command -v jq >/dev/null 2>&1 || fail "jq is required (brew install jq)"

NODE_PINNED=$(tr -d '[:space:]' < web/.nvmrc) || fail "cannot read web/.nvmrc"
NPM_PINNED=$(tr -d '[:space:]' < web/.npm-version) || fail "cannot read web/.npm-version"
ENGINES_NODE=$(jq -r '.engines.node // empty' web/package.json) || fail "cannot read web/package.json"
[ -n "$NODE_PINNED" ] || fail "web/.nvmrc is empty"
[ -n "$NPM_PINNED" ] || fail "web/.npm-version is empty"

NODE_MAJOR=${NODE_PINNED%%.*}
case "$NODE_MAJOR" in
    ''|*[!0-9]*) fail "web/.nvmrc does not start with a major version: '$NODE_PINNED'" ;;
esac

# A full template containing a path, not GNU's -p or a bare -t prefix: macOS ships BSD
# mktemp, and this is the spelling both accept (same reasoning as bin/docker-test.sh).
DIST=$(mktemp "${TMPDIR:-/tmp}/node-dist-index.XXXXXXXX") || fail "mktemp failed"
trap '/bin/rm -f "$DIST"' EXIT

# The full release index, ~1500 entries back to v0.1.14. Small enough (~250KB) that
# filtering locally is simpler than hunting for a per-line endpoint.
curl -fsSL --retry 3 --retry-delay 2 --max-time 60 -o "$DIST" \
    https://nodejs.org/dist/index.json || fail "could not fetch https://nodejs.org/dist/index.json"

# Newest release on the pinned major's line, with its release date. index.json happens to
# arrive newest-first, but sort numerically rather than depend on that: a string sort puts
# 24.9.0 above 24.20.0.
read -r NODE_NEWEST NODE_NEWEST_DATE <<<"$(jq -r --argjson major "$NODE_MAJOR" '
    [ .[]
      | { version: (.version | ltrimstr("v")), date: .date }
      | . + { parts: (.version | split(".") | map(tonumber)) }
      | select(.parts[0] == $major) ]
    | sort_by(.parts) | last
    | if . == null then "" else "\(.version) \(.date)" end' "$DIST")" \
    || fail "could not parse the Node release index"
[ -n "$NODE_NEWEST" ] || fail "no Node $NODE_MAJOR.x releases found in the release index"

# Highest major that carries an LTS codename, so a new LTS line does not go unnoticed.
NEWEST_LTS_MAJOR=$(jq -r '
    [ .[] | select(.lts != false) | .version | ltrimstr("v") | split(".") | .[0] | tonumber ]
    | max // empty' "$DIST") || fail "could not parse LTS lines from the Node release index"

NPM_NEWEST=$(curl -fsSL --retry 3 --retry-delay 2 --max-time 60 \
    https://registry.npmjs.org/npm/latest | jq -r '.version // empty') \
    || fail "could not fetch the current npm version from the registry"
[ -n "$NPM_NEWEST" ] || fail "the npm registry returned no version"

drift=0
report=""
add() { report+="$1"$'\n'; }

add "### Node (\`web/.nvmrc\`)"
add ""
if [ "$NODE_PINNED" = "$NODE_NEWEST" ]; then
    add "Current: pinned \`$NODE_PINNED\`, newest on the ${NODE_MAJOR}.x line."
else
    drift=1
    add "**Behind.** Pinned \`$NODE_PINNED\`; newest on the ${NODE_MAJOR}.x line is"
    add "\`$NODE_NEWEST\` (released $NODE_NEWEST_DATE)."
    add ""
    add "Changelog: https://github.com/nodejs/node/blob/main/doc/changelogs/CHANGELOG_V${NODE_MAJOR}.md"
    add ""
    add "To take it: edit \`web/.nvmrc\`, run \`make web-nvm-install\`, and let CI test it."
    add "Nothing else hardcodes the version (Makefile, \`bin/node-init.sh\`, \`bin/docker-push.sh\`,"
    add "\`docker/Dockerfile\` and the workflows all read that file)."
fi
add ""

if [ -n "$NEWEST_LTS_MAJOR" ] && [ "$NEWEST_LTS_MAJOR" -gt "$NODE_MAJOR" ]; then
    drift=1
    add "### Node major line"
    add ""
    add "**A newer LTS line exists.** Pinned on ${NODE_MAJOR}.x; ${NEWEST_LTS_MAJOR}.x is now LTS."
    add ""
    add "A major bump also needs \`engines.node\` in \`web/package.json\` (currently \`$ENGINES_NODE\`)"
    add "changed by hand; it is a major range on purpose so patch bumps do not need a second edit."
    add ""
fi

add "### npm (\`web/.npm-version\`)"
add ""
if [ "$NPM_PINNED" = "$NPM_NEWEST" ]; then
    add "Current: pinned \`$NPM_PINNED\`, which is \`npm@latest\`."
else
    drift=1
    add "**Behind.** Pinned \`$NPM_PINNED\`; \`npm@latest\` is \`$NPM_NEWEST\`."
    add ""
    add "To take it: edit \`web/.npm-version\` and run \`make web-nvm-install\`."
fi

printf '%s' "$report"
exit "$drift"
