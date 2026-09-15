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
# A new Node version is not actionable the moment nodejs.org has the tarballs. The Docker
# image trails it: the version goes through nodejs/docker-node and then a
# docker-library/official-images PR before the tag is built and pushed, which has run 0-2
# days behind the release. docker/Dockerfile builds `FROM node:${NODE_VERSION}-...`, so a
# bump taken during that window fails CI with `node:<version>-bookworm-slim: not found`.
# That happened on 24.21.0 (released Mon 2026-09-07; tag still unpublished Wed 2026-09-09,
# official-images#22231 open). So a newer Node whose image does not exist yet is reported
# but not counted as drift, and the issue arrives a day later when the bump can build.
#
# Exit codes:
#   0  every pin is current
#   1  at least one pin is behind (the markdown report on stdout says which)
#   2  the check itself could not run (missing tool, network, unexpected upstream shape)
#
# Usage: bin/check-versions.sh [title-file]
#
# When drift is found and title-file is given, a one-line summary naming what is behind
# (e.g. "Bump Node 24.20.0 to 24.21.0") is written there for use as an issue title. The
# path is resolved before the cd below, so a relative one means the caller's directory.

set -uo pipefail

TITLE_FILE=""
if [ $# -gt 0 ]; then
    case "$1" in
        /*) TITLE_FILE=$1 ;;
        *) TITLE_FILE="$PWD/$1" ;;
    esac
fi

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

# Read the variant off `FROM node:${NODE_VERSION}-bookworm-slim` instead of repeating
# "-bookworm-slim" here, so the Dockerfile stays the only place naming the base image.
# shellcheck disable=SC2016  # ${NODE_VERSION} is matched literally, not expanded
NODE_IMAGE_SUFFIX=$(sed -n 's/^FROM node:\${NODE_VERSION}\(-[A-Za-z0-9._-]*\).*/\1/p' \
    docker/Dockerfile | head -1) || fail "cannot read docker/Dockerfile"
[ -n "$NODE_IMAGE_SUFFIX" ] \
    || fail "no 'FROM node:\${NODE_VERSION}-<variant>' line found in docker/Dockerfile"

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

# Has the Docker image for a given Node version been published yet?
#
# Anonymous pull scope is enough for a manifest HEAD, so no Docker Hub account or secret is
# involved. The Accept headers matter: official images publish a multi-arch index, and the
# registry answers 404 for a tag that exists if you do not ask for the index media types.
# No -L, because following a redirect would hand the bearer token to another host.
node_image_published() {
    local tag="$1$NODE_IMAGE_SUFFIX" token code
    token=$(curl -fsSL --retry 3 --retry-delay 2 --max-time 60 \
        'https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/node:pull' \
        | jq -r '.token // empty') || fail "could not fetch a Docker Hub pull token"
    [ -n "$token" ] || fail "Docker Hub returned no pull token"

    code=$(curl -sS -o /dev/null -w '%{http_code}' --retry 3 --retry-delay 2 --max-time 60 -I \
        -H "Authorization: Bearer $token" \
        -H 'Accept: application/vnd.oci.image.index.v1+json' \
        -H 'Accept: application/vnd.docker.distribution.manifest.list.v2+json' \
        "https://registry-1.docker.io/v2/library/node/manifests/$tag") \
        || fail "could not query the Docker registry for node:$tag"

    case "$code" in
        200) return 0 ;;
        404) return 1 ;;
        # Anything else is the check failing, not an answer about the tag. Exit 2 rather
        # than guess: reporting drift would send someone to a bump that may not build, and
        # reporting current would hide a real one.
        *) fail "unexpected HTTP $code from the Docker registry for node:$tag" ;;
    esac
}

drift=0
report=""
add() { report+="$1"$'\n'; }
titles=()

add "### Node (\`web/.nvmrc\`)"
add ""
if [ "$NODE_PINNED" = "$NODE_NEWEST" ]; then
    add "Current: pinned \`$NODE_PINNED\`, newest on the ${NODE_MAJOR}.x line."
elif ! node_image_published "$NODE_NEWEST"; then
    # Newer upstream, but nothing to take yet: the bump would fail the Docker build. Say so
    # and leave drift at 0, so no issue is opened until the image lands.
    add "Current: pinned \`$NODE_PINNED\`, newest *installable* on the ${NODE_MAJOR}.x line."
    add ""
    add "Node \`$NODE_NEWEST\` (released $NODE_NEWEST_DATE) is out, but"
    add "\`node:${NODE_NEWEST}${NODE_IMAGE_SUFFIX}\` has not been published to Docker Hub yet, so"
    add "\`docker/Dockerfile\` cannot build it. The image trails the release by 0-2 days while it"
    add "goes through nodejs/docker-node and docker-library/official-images. Holding the pin"
    add "until the tag exists; this will report as drift on a later run."
else
    drift=1
    titles+=("Node $NODE_PINNED to $NODE_NEWEST")
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
    titles+=("Node ${NODE_MAJOR}.x to ${NEWEST_LTS_MAJOR}.x LTS")
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
    titles+=("npm $NPM_PINNED to $NPM_NEWEST")
    add "**Behind.** Pinned \`$NPM_PINNED\`; \`npm@latest\` is \`$NPM_NEWEST\`."
    add ""
    add "To take it: edit \`web/.npm-version\` and run \`make web-nvm-install\`."
fi

if [ -n "$TITLE_FILE" ] && [ ${#titles[@]} -gt 0 ]; then
    title="Bump ${titles[0]}"
    for t in "${titles[@]:1}"; do title+=", $t"; done
    printf '%s\n' "$title" > "$TITLE_FILE" || fail "cannot write title file '$TITLE_FILE'"
fi

printf '%s' "$report"
exit "$drift"
