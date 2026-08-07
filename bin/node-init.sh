#!/usr/bin/env bash
# Shared Node.js initialization. Sourced (not executed) by the bin/*.sh scripts that run
# npm/npx, so that PATH changes apply to the calling shell.
#
# Uses the node already on PATH only if its major version matches web/.nvmrc; otherwise
# sources nvm and switches to that version. Matching on the version, rather than mere
# presence, keeps a distro node at the wrong major (Ubuntu's apt 'nodejs' is commonly
# pulled in as a dependency of something else) from shadowing the repo's Node. Sourcing
# nvm alone is not enough either — that activates nvm's default alias, which is not
# necessarily the version this repo wants.
#
# This mirrors NODE_INIT in the Makefile; keep the two in sync.

_ni_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
_ni_wanted=$(cat "$_ni_root/web/.nvmrc")
# The '|| true' matters: the callers run under 'set -eo pipefail', where a missing node
# makes the pipeline exit 127 and takes the whole script down before the check below.
_ni_found=$(node -v 2>/dev/null | sed 's/^v\([0-9]*\).*/\1/' || true)

if [ "$_ni_found" != "$_ni_wanted" ]; then
    NVM_SH="${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    if [ ! -f "$NVM_SH" ]; then
        echo "Error: node $_ni_wanted required (found '${_ni_found:-none}') and nvm not found at $NVM_SH" >&2
        echo "Install Node.js or nvm before continuing." >&2
        exit 1
    fi
    # shellcheck source=/dev/null
    source "$NVM_SH"
    # Not in a subshell: nvm use only changes PATH for the shell it runs in.
    nvm use --silent "$_ni_wanted"
fi

unset _ni_root _ni_wanted _ni_found
