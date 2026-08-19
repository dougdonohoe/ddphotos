#!/bin/bash
#
# Installs apt packages on a GitHub-hosted runner, defensively.
#
# GitHub's Ubuntu runner images pin their apt mirror to azure.archive.ubuntu.com, which
# every GitHub-hosted Linux runner shares and which degrades periodically. Two of apt's
# defaults turn that into a job-killing stall rather than a quick error:
#
#   * Acquire::Retries defaults to 0, so a single failed fetch fails the whole run.
#   * Acquire::*::Timeout is an *inactivity* timeout, so a mirror that trickles a few
#     bytes at a time never trips it.
#
# Observed here: this step normally takes ~20s, but has taken 312s, and once 3943s (66
# minutes) before the job hit its timeout-minutes and was killed.
#
# So: bound the wait, retry a few times, and if the Azure mirror is still unusable, fall
# back to archive.ubuntu.com. The fallback is deliberately *only* a fallback. The Azure
# mirror is much faster than archive.ubuntu.com from inside Azure when it is healthy, so
# switching unconditionally would slow down every green run to fix the rare red one.
#
# The apt.conf.d drop-in is written globally rather than passed as -o flags, so that
# anything else in the job that shells out to apt inherits it too (`npx playwright
# install-deps` did, before it was dropped from the workflow).
#
set -euo pipefail

if [[ $# -eq 0 ]]; then
    echo "usage: $0 <package> [package ...]" >&2
    exit 1
fi

# Ubuntu 24.04 runners use the deb822 format; older ones use the one-line sources.list.
SOURCES_FILES=(/etc/apt/sources.list.d/ubuntu.sources /etc/apt/sources.list)

sudo tee /etc/apt/apt.conf.d/99-ci-flaky-mirror >/dev/null <<'EOF'
Acquire::Retries "5";
Acquire::http::Timeout "20";
Acquire::https::Timeout "20";
Acquire::ForceIPv4 "true";
EOF

apt_install() {
    sudo apt-get update
    sudo apt-get install -y "$@"
}

if apt_install "$@"; then
    exit 0
fi

echo "::warning::apt failed against azure.archive.ubuntu.com; retrying via archive.ubuntu.com"
for f in "${SOURCES_FILES[@]}"; do
    if [[ -f "$f" ]]; then
        sudo sed -i 's|azure\.archive\.ubuntu\.com|archive.ubuntu.com|g' "$f"
    fi
done

apt_install "$@"
