#!/usr/bin/env bash
# Test that docker/ddphotos passes credentials into containers by name only.
#
# Usage:
#   bin/test-docker-env.sh
#
# `docker run -e NAME=value` puts the value in docker's argv, where `ps` shows it to anyone
# who can list the process for as long as the container runs. `docker run -e NAME` has
# docker read the value from its own environment instead. This runs docker/ddphotos against
# a stub `docker` first on PATH that records the argv it was given and the environment it
# inherited, so no image or daemon is needed, and checks every command that forwards one.

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DDPHOTOS="$REPO_ROOT/docker/ddphotos"

step() { echo; echo "=== $* ==="; }
pass() { echo "  PASS: $*"; }
fail() { echo "  FAIL: $*" >&2; exit 1; }

WORK=$(mktemp -d)
trap '/bin/rm -rf "$WORK"' EXIT

# A throwaway HOME: surge creates ~/.netrc and the script creates ~/.config/ddphotos.
export HOME="$WORK/home"
mkdir -p "$HOME" "$WORK/bin" "$WORK/site/config"
# photogen checks for albums.yaml before it launches docker.
printf 'settings:\n  id: env-test\n' > "$WORK/site/config/albums.yaml"

# The stub answers `docker info` (ensure_docker_running) and logs every other call: one
# ARG line per argument, then the value of each checked variable as docker inherited it.
LOG="$WORK/docker.log"
cat > "$WORK/bin/docker" <<'EOF'
#!/usr/bin/env bash
[ "$1" = info ] && exit 0
{
    for a in "$@"; do printf 'ARG %s\n' "$a"; done
    for v in $CHECK_VARS; do printf 'ENV %s=%s\n' "$v" "${!v}"; done
} >> "$DOCKER_LOG"
EOF
chmod +x "$WORK/bin/docker"
export PATH="$WORK/bin:$PATH" DOCKER_LOG="$LOG"

# check <command...> -- <var...>: run the command with each var set to a unique value, then
# require that no argument contains a value, that `-e VAR` names each one, and that docker
# inherited each value, which is what makes the name-only form work.
check() {
    local cmd=() vars=() v
    while [ "$1" != "--" ]; do cmd+=("$1"); shift; done
    shift
    vars=("$@")

    : > "$LOG"
    export CHECK_VARS="${vars[*]}"
    for v in "${vars[@]}"; do export "$v=secret-value-$v"; done
    "$DDPHOTOS" --dir "$WORK/site" --non-interactive "${cmd[@]}" >/dev/null 2>&1 \
        || fail "${cmd[*]}: exited non-zero"
    for v in "${vars[@]}"; do unset "$v"; done

    grep -q '^ARG run$' "$LOG" || fail "${cmd[*]}: docker run was never called"
    for v in "${vars[@]}"; do
        if grep '^ARG ' "$LOG" | grep -qF "secret-value-$v"; then
            grep '^ARG ' "$LOG" | grep -F "secret-value-$v" >&2
            fail "${cmd[*]}: the value of $v is on docker's command line"
        fi
        grep -qx "ARG $v" "$LOG"                     || fail "${cmd[*]}: $v is not passed as -e $v"
        grep -qx "ENV $v=secret-value-$v" "$LOG"     || fail "${cmd[*]}: docker did not inherit $v"
    done
    pass "${cmd[*]}: ${vars[*]} passed by name"

    # Unset is not passed at all, so a container-side default (config/immich.env, ~/.aws)
    # still applies.
    : > "$LOG"
    "$DDPHOTOS" --dir "$WORK/site" --non-interactive "${cmd[@]}" >/dev/null 2>&1 \
        || fail "${cmd[*]}: exited non-zero with nothing set"
    for v in "${vars[@]}"; do
        grep -qx "ARG $v" "$LOG" && fail "${cmd[*]}: $v is passed although it is unset"
    done
    pass "${cmd[*]}: nothing passed when unset"
}

step "photogen"
check photogen -- IMMICH_API_KEY IMMICH_INSTANCE_URL

step "deploy"
check deploy -- AWS_PROFILE AWS_DEFAULT_PROFILE AWS_DEFAULT_REGION AWS_REGION \
    AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN

step "wrangler"
check wrangler whoami -- CLOUDFLARE_API_TOKEN CLOUDFLARE_ACCOUNT_ID CLOUDFLARE_EMAIL CLOUDFLARE_API_KEY

step "surge"
check surge list -- SURGE_LOGIN SURGE_TOKEN

echo
echo "All docker env tests passed."
