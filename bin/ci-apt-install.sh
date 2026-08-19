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
# The `timeout` wrapper is the part that actually catches the common case, and it is not
# redundant with Acquire::*::Timeout. A degraded Azure mirror does not fail, it *trickles*:
# a PR run measured here spent 199s on this step with a dozen 8-15s stalls mid-download,
# every one of which recovered. apt returned success, so retries and per-connection
# timeouts both stayed dormant while the step ran 7x long. Bounding total wall time is the
# only thing that converts that into a mirror swap instead of a 30-minute job timeout.
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

# GitHub's runner images do not name the mirror host in ubuntu.sources at all. They point
# it at `mirror+file:/etc/apt/apt-mirrors.txt`, a priority-ordered list with Azure first
# and archive.ubuntu.com already present below it. That fallback is useless to us on its
# own: apt's mirror method only advances down the list on a hard failure, never on
# slowness, which is the mode that actually bites here. So the Azure line has to be
# removed outright rather than merely deprioritised.
MIRRORLIST=/etc/apt/apt-mirrors.txt

# Older images (and anything not using a mirrorlist) do name the host directly.
SOURCES_FILES=(/etc/apt/sources.list.d/ubuntu.sources /etc/apt/sources.list)

sudo tee /etc/apt/apt.conf.d/99-ci-flaky-mirror >/dev/null <<'EOF'
Acquire::Retries "5";
Acquire::http::Timeout "20";
Acquire::https::Timeout "20";
Acquire::ForceIPv4 "true";
EOF

# Per phase, not for the script as a whole. Override with APT_TIMEOUT.
#
# 120s is deliberately aggressive. It started at 240s, chosen to ride out a sluggish mirror
# rather than swap, but measurement showed that reasoning was backwards: when the fallback
# fired in CI, archive.ubuntu.com finished the whole install in 47-100s, i.e. *faster* than
# a degraded Azure manages. Swapping is cheap, so waiting to avoid it is the expensive
# choice. 120s still leaves 4x headroom over a healthy run (~20-30s).
#
# `sudo timeout` rather than `timeout sudo`: the signal has to reach apt-get, not sudo.
APT_TIMEOUT="${APT_TIMEOUT:-120}"

apt_install() {
    sudo timeout "$APT_TIMEOUT" apt-get update
    sudo timeout "$APT_TIMEOUT" apt-get install -y "$@"
}

if apt_install "$@"; then
    exit 0
fi

echo "::warning::apt failed or exceeded ${APT_TIMEOUT}s against azure.archive.ubuntu.com; retrying via archive.ubuntu.com"

# A timeout can land mid-unpack, which leaves dpkg wedged and makes the retry fail with
# "dpkg was interrupted" before it ever reaches the network. Cheap to run, no-op otherwise.
sudo dpkg --configure -a || true

if [[ -f "$MIRRORLIST" ]] && grep -q 'azure\.archive\.ubuntu\.com' "$MIRRORLIST"; then
    echo "--- $MIRRORLIST before ---"
    cat "$MIRRORLIST"
    # Only drop the Azure entry if the list still has somewhere else to fetch from.
    if grep -v 'azure\.archive\.ubuntu\.com' "$MIRRORLIST" | grep -q '://'; then
        sudo sed -i '/azure\.archive\.ubuntu\.com/d' "$MIRRORLIST"
    else
        echo 'http://archive.ubuntu.com/ubuntu/' | sudo tee "$MIRRORLIST" >/dev/null
    fi
    echo "--- $MIRRORLIST after ---"
    cat "$MIRRORLIST"
fi

for f in "${SOURCES_FILES[@]}"; do
    if [[ -f "$f" ]]; then
        sudo sed -i 's|azure\.archive\.ubuntu\.com|archive.ubuntu.com|g' "$f"
    fi
done

# Force the retry's `apt-get update` to genuinely re-fetch its indexes from the new mirror
# rather than reporting Hit off the old ones. The lists are keyed by the mirrorlist URI,
# not the mirror it resolves to, so editing the list alone would not invalidate them, and a
# timeout during the update phase can leave a half-written index behind.
#
# Deliberately no `apt-get clean`: already-downloaded .debs are checksum-verified against
# the new indexes, so reusing them is safe and saves re-fetching them over the slower mirror.
sudo rm -rf /var/lib/apt/lists/*

apt_install "$@"
