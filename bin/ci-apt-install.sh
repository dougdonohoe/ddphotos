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
# So: bound the wait, retry a few times, and if the runner's mirror is still unusable, fall
# back to archive.ubuntu.com. The fallback is deliberately *only* a fallback. The cloud
# mirror is much faster than archive.ubuntu.com from inside its cloud when it is healthy, so
# switching unconditionally would slow down every green run to fix the rare red one.
#
# The mirror is not always Azure. Since 11/2026 some runners come up with
# us-east-2.ec2.archive.ubuntu.com instead, and every CI failure in this step on 11/18 was
# one of those: the fallback only knew to remove Azure, so the retry went back to the same
# stalled EC2 mirror and timed out a second time. The fallback now replaces whatever mirror
# the runner was given.
#
# --no-install-recommends roughly halves the download. libvips-dev's Recommends pull in
# libvips-doc (and with it mkdocs, numpy, nltk), libvips-tools, ghostscript and the AVIF
# encoder plugin, none of which a cgo build of photogen uses. Fewer bytes over a degraded
# mirror means fewer runs that hit the timeout at all.
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
# it at `mirror+file:/etc/apt/apt-mirrors.txt`, a priority-ordered list with the cloud
# mirror first and archive.ubuntu.com already present below it. That fallback is useless to
# us on its own: apt's mirror method only advances down the list on a hard failure, never
# on slowness, which is the mode that actually bites here. So the list is rewritten to
# archive.ubuntu.com alone rather than merely reordered.
MIRRORLIST=/etc/apt/apt-mirrors.txt
FALLBACK_MIRROR=http://archive.ubuntu.com/ubuntu/

# Any regional or cloud-specific mirror: azure.archive.ubuntu.com,
# us-east-2.ec2.archive.ubuntu.com, us.archive.ubuntu.com and so on. Never matches the bare
# archive.ubuntu.com, which is the fallback itself.
MIRROR_HOST_RE='[a-z0-9.-]+\.archive\.ubuntu\.com'

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
    sudo timeout "$APT_TIMEOUT" apt-get install -y --no-install-recommends "$@"
}

# For the warning only: which mirror the runner was given.
PRIMARY_MIRROR=$(grep -ohE "$MIRROR_HOST_RE" "$MIRRORLIST" "${SOURCES_FILES[@]}" 2>/dev/null | head -1 || true)

if apt_install "$@"; then
    exit 0
fi

echo "::warning::apt failed or exceeded ${APT_TIMEOUT}s against ${PRIMARY_MIRROR:-the default mirror}; retrying via archive.ubuntu.com"

# A timeout can land mid-unpack, which leaves dpkg wedged and makes the retry fail with
# "dpkg was interrupted" before it ever reaches the network. Cheap to run, no-op otherwise.
sudo dpkg --configure -a || true

if [[ -f "$MIRRORLIST" ]]; then
    echo "--- $MIRRORLIST before ---"
    cat "$MIRRORLIST"
    # Replaced wholesale rather than filtered. Other entries in the list (security.ubuntu.com,
    # say) would still be tried first if they sat above archive.ubuntu.com, and the point is
    # one known-good mirror. security.ubuntu.com is its own sources entry, unaffected.
    echo "$FALLBACK_MIRROR" | sudo tee "$MIRRORLIST" >/dev/null
    echo "--- $MIRRORLIST after ---"
    cat "$MIRRORLIST"
fi

for f in "${SOURCES_FILES[@]}"; do
    if [[ -f "$f" ]]; then
        sudo sed -i -E "s#${MIRROR_HOST_RE}#archive.ubuntu.com#g" "$f"
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
