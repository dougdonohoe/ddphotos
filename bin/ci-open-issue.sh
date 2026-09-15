#!/bin/bash
#
# Files the result of a scheduled workflow as a GitHub issue.
#
# A nightly run has nobody watching it. The only thing GitHub does on its own is email the
# repo owner, which is easy to lose, and the run itself scrolls off the Actions tab. An
# issue survives until someone closes it.
#
# Re-runs update the open issue carrying the same label rather than opening a second one,
# so a problem that persists for a week is one thread and not seven issues. They comment
# only when the report differs from the last one posted: a pin nobody can take for a month
# should not add thirty copies of the same text. The title is kept current either way.
#
# "Differs" is judged on the body file alone, before the run link and timestamp are
# appended. Each post carries the file's hash in a hidden HTML comment, and the newest hash
# on the issue (body or comments) is what the new report is compared against. So callers must
# keep anything per-run (run URLs, dates) out of the body file, or every run will comment.
#
# Usage: bin/ci-open-issue.sh <label> <title> <body-file>
#
# Requires the `gh` CLI (preinstalled on GitHub runners) with GH_TOKEN set, and
# `issues: write` on the job. The default GITHUB_TOKEN is enough -- no PAT and no secret --
# but this repo's default workflow permission is read-only, so the job must ask for it:
#
#     permissions:
#       contents: read
#       issues: write
#
# The repo is taken from the git remote that actions/checkout sets up, so this also works
# when run by hand from a local clone.

set -euo pipefail

if [ $# -ne 3 ]; then
    echo "Usage: $(basename "$0") <label> <title> <body-file>" >&2
    exit 2
fi

LABEL=$1
TITLE=$2
BODY_FILE=$3

[ -r "$BODY_FILE" ] || { echo "Error: cannot read body file '$BODY_FILE'" >&2; exit 2; }

BODY=$(cat "$BODY_FILE")

# git rather than sha256sum/shasum: the only hashing tool guaranteed on both the runner and
# a macOS clone. Any stable content hash will do; this is change detection, not security.
HASH=$(git hash-object "$BODY_FILE")
MARKER="<!-- ci-open-issue: $HASH -->"

# Link back to the run that produced this, when there is one.
if [ -n "${GITHUB_RUN_ID:-}" ]; then
    BODY="$BODY

---
From [\`${GITHUB_WORKFLOW:-workflow}\` run ${GITHUB_RUN_ID}](${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}) on $(date -u '+%Y-%m-%d %H:%M UTC')."
fi

BODY="$BODY

$MARKER"

# Idempotent: succeeds the first time, errors harmlessly every night after. Created here
# rather than assumed so a fork gets the same behavior with no manual repo setup.
gh label create "$LABEL" --color FBCA04 --description "Opened automatically by a scheduled workflow" >/dev/null 2>&1 || true

EXISTING=$(gh issue list --label "$LABEL" --state open --limit 1 --json number --jq '.[0].number // empty')

if [ -n "$EXISTING" ]; then
    # capture() emits nothing on a body without a marker (issues filed before markers
    # existed, human comments), so `last` is the most recent post this script made.
    read -r LAST_HASH CURRENT_TITLE < <(gh issue view "$EXISTING" --json title,body,comments --jq '
        ([.body, .comments[].body]
         | map(capture("<!-- ci-open-issue: (?<h>[0-9a-f]+) -->").h) | last // "none")
        + " " + .title')

    if [ "$LAST_HASH" = "$HASH" ]; then
        echo "Issue #$EXISTING already has this report; not commenting"
    else
        echo "Commenting on existing issue #$EXISTING"
        gh issue comment "$EXISTING" --body "$BODY"
    fi

    # A title can carry specifics (which version is behind) that change while the issue is
    # open, so keep it current. Compared first so an unchanged title adds no rename event.
    if [ "$CURRENT_TITLE" != "$TITLE" ]; then
        echo "Retitling issue #$EXISTING"
        gh issue edit "$EXISTING" --title "$TITLE"
    fi
else
    echo "Opening a new issue"
    gh issue create --label "$LABEL" --title "$TITLE" --body "$BODY"
fi
