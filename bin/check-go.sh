#!/bin/bash
#
# Reports when this repo's Go toolchain pin has fallen behind, and when a known
# vulnerability is reachable from cmd/ or pkg/.
#
# The sibling of bin/check-versions.sh, which does the same for the Node and npm pins, and
# run by the same nightly workflow.
#
# Deliberately NOT here: module drift. .github/dependabot.yml has a gomod ecosystem, and a
# PR with the edit already made and CI already run beats an issue saying a bump exists. The
# two things below are the ones Dependabot cannot do:
#
#   1. The `go` directive in go.mod. No updater reads it -- Dependabot's gomod ecosystem
#      bumps `require` lines only -- and all three setup-go steps in the workflows use
#      `go-version-file: go.mod`, so that directive is what CI actually runs on. Go supports
#      only the two most recent major releases, so a directive two releases back means CI is
#      testing on a Go that no longer gets security fixes. This is the .nvmrc problem in a
#      different file, which is why it lives in a script rather than in an updater.
#
#   2. Reachable vulnerabilities. Dependabot alerts match manifest versions against GitHub's
#      Advisory Database and say nothing about whether the vulnerable function is ever
#      called. govulncheck walks the call graph, so it stays quiet about the x/crypto/ssh and
#      openpgp advisories that sit in the module graph but that nothing here enters, and a
#      reachability-blind tool would nag about forever. It also covers advisories GitHub does
#      not carry: GO-2026-6222 (x/image) has a CVE but no GHSA, and showed up here while
#      Dependabot had no open alert for it.
#
# This deliberately does not edit go.mod. Bumping the Go version is a decision CI then tests,
# the same rule bin/check-versions.sh follows for Node.
#
# Exit codes:
#   0  the pin is current and nothing is reachable
#   1  something to report (the markdown on stdout says what)
#   2  the check itself could not run (missing tool, network, unexpected output)
#
# Usage: bin/check-go.sh [title-file]
#
# When there is something to report and title-file is given, a one-line summary is written
# there for use as an issue title. A reachable vulnerability leads that title, so the issue
# list shows the urgent thing first. The path is resolved before the cd below, so a relative
# one means the caller's directory.

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

command -v go >/dev/null 2>&1 || fail "go is required"

# govulncheck is not part of the Go distribution. Installed on demand so this works the same
# on a laptop and on a runner.
#
# @latest, not a pin, and that is deliberate in a repo that pins everything else exactly.
# The pins exist so a build is reproducible; this is not a build input, it is a scanner
# reporting on the outside world, and it fetches its vulnerability database at run time
# regardless. A pinned scanner would go stale in exactly the way this job exists to prevent.
if ! command -v govulncheck >/dev/null 2>&1; then
    echo "Installing govulncheck..." >&2
    go install golang.org/x/vuln/cmd/govulncheck@latest >&2 \
        || fail "could not install govulncheck"
    PATH="$(go env GOPATH)/bin:$PATH"
    command -v govulncheck >/dev/null 2>&1 || fail "govulncheck is not on PATH after install"
fi

report=""
add() { report+="$1"$'\n'; }

drift=0
vuln_title=""
drift_title=""

#
# 1. The Go toolchain pin
#
GO_PINNED=$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' go.mod | head -1)
[ -n "$GO_PINNED" ] || fail "no 'go <version>' directive found in go.mod"

# The same release index go.dev/dl uses. Stable releases only, newest first.
GO_NEWEST=$(curl -fsSL --retry 3 --retry-delay 2 --max-time 60 'https://go.dev/dl/?mode=json' \
    | sed -n 's/.*"version": *"go\([0-9][0-9.]*\)".*/\1/p' | head -1) \
    || fail "could not fetch the Go release index from https://go.dev/dl/"
[ -n "$GO_NEWEST" ] || fail "the Go release index returned no version"

add "### Go toolchain (\`go.mod\`)"
add ""
if [ "$GO_PINNED" = "$GO_NEWEST" ]; then
    add "Current: \`go $GO_PINNED\`, the newest stable release."
else
    drift=1
    drift_title="go $GO_PINNED to $GO_NEWEST"
    add "**Behind.** \`go.mod\` says \`go $GO_PINNED\`; the newest stable release is \`$GO_NEWEST\`."
    add ""
    add "This is what CI runs on: every \`setup-go\` step uses \`go-version-file: go.mod\`."
    add "Go supports the two most recent major releases, so a directive two majors back is"
    add "testing on a toolchain that no longer receives security fixes."
    add ""
    add "Release notes: https://go.dev/doc/devel/release"
    add ""
    add "To take it: edit the \`go\` directive in \`go.mod\`, then \`make build test vet\`."
fi
add ""

#
# 2. Reachable vulnerabilities
#
VULN_OUT=$(govulncheck ./cmd/... ./pkg/... 2>&1)
VULN_STATUS=$?

add "### Vulnerabilities (\`govulncheck\`)"
add ""
case "$VULN_STATUS" in
    0)
        add "None reachable from \`cmd/\` or \`pkg/\`."
        # govulncheck's own count of what it looked at and dismissed. Worth keeping: it is
        # the difference between this and a reachability-blind scanner, and it explains why
        # Dependabot can be quiet while advisories exist in the module graph.
        # The summary wraps across three lines and the last one carries the verb, so take
        # the paragraph from "This scan also found" up to its full stop rather than the
        # lines that happen to match a keyword.
        DISMISSED=$(printf '%s\n' "$VULN_OUT" \
            | sed -n '/^This scan also found/,/vulnerabilities\.$/p')
        if [ -n "$DISMISSED" ]; then
            add ""
            add "Present in the module graph but not on any call path from this code:"
            add ""
            while IFS= read -r line; do
                [ -n "$line" ] && add "> $line"
            done <<<"$DISMISSED"
        fi
        ;;
    3)
        # govulncheck uses 3 for "vulnerabilities found", not 1.
        drift=1
        ids=$(printf '%s\n' "$VULN_OUT" \
            | sed -n 's/^Vulnerability #[0-9]*: \(GO-[0-9][0-9-]*\).*/\1/p' | sort -u | tr '\n' ' ')
        ids=${ids% }
        count=$(printf '%s' "$ids" | wc -w | tr -d ' ')
        noun="vulnerabilities"
        [ "$count" = "1" ] && noun="vulnerability"
        vuln_title="$count reachable $noun (${ids// /, })"
        add "**$count reachable $noun: ${ids// /, }**"
        add ""
        add "Reachable means govulncheck traced a call path from this code into the affected"
        add "function, so this is actionable rather than advisory."
        add ""
        add '```'
        while IFS= read -r line; do add "$line"; done <<<"$VULN_OUT"
        add '```'
        ;;
    *)
        fail "govulncheck exited $VULN_STATUS: $VULN_OUT"
        ;;
esac

printf '%s' "$report"

# The vulnerability leads, so the issue title escalates on its own when one appears and
# drops back when it is fixed. ci-open-issue.sh retitles the open issue on every run.
if [ -n "$TITLE_FILE" ]; then
    title=""
    [ -n "$vuln_title" ] && title="$vuln_title"
    if [ -n "$drift_title" ]; then
        [ -n "$title" ] && title="$title, $drift_title" || title="$drift_title"
    fi
    if [ -n "$title" ]; then
        printf 'Go: %s\n' "$title" > "$TITLE_FILE" || fail "cannot write title file '$TITLE_FILE'"
    fi
fi

exit "$drift"
