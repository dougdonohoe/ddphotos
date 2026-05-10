#!/usr/bin/env bash
# Deploy sample sites to static hosting providers.
#
# Deploys two sites:
#   - init:   the ddphotos 'init' example site
#   - sample: the sample site in this repo (sample/)
#
# Usage:
#   bin/deploy-sample-sites.sh [--init] [--sample] [--surge] [--cloudflare] [--dev] [--doit]
#
# Sites:
#   Surge:
#     Init:     https://ddphotos-init.surge.sh (old, now redirects https://ddphotos-test-docker.surge.sh/)
#     Sample:   https://ddphotos-sample.surge.sh (old, now redirects https://ddphotos-test-sample.surge.sh/)
#   Cloudflare:
#     Init:     https://ddphotos-init.pages.dev
#     Sample:   https://ddphotos-sample.pages.dev
#     Sample 2: https://my-unique-site.pages.dev/
#
# With no site flags, both sites are deployed.
# With no provider flags, both providers are used.
# Flags combine: --cloudflare --sample deploys only sample to Cloudflare.
# Without --doit, everything runs except the final surge/wrangler upload.
#
# Options:
#   --init        Deploy init site only
#   --sample      Deploy sample site only
#   --surge       Deploy to surge.sh only
#   --cloudflare  Deploy to Cloudflare Pages only
#   --dev         Use local 'ddphotos' image (default: dougdonohoe/ddphotos:latest)
#   --doit        Actually upload to surge/wrangler (default: dry-run, skips upload)
#   --no-photogen Skip the photogen step
#   --no-build    Skip the build step
#   --verify      Instead of deploying, verify each site via test-photos-server.sh
#   --help        Show this usage message

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

IMAGE="dougdonohoe/ddphotos:latest"
PULL_FLAG=(--pull always)
DOIT=false
VERIFY=false
DO_PHOTOGEN=true
DO_BUILD=true
INIT_EXPLICIT=false
SAMPLE_EXPLICIT=false
SURGE_EXPLICIT=false
CLOUDFLARE_EXPLICIT=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --init)        INIT_EXPLICIT=true;             shift ;;
        --sample)      SAMPLE_EXPLICIT=true;           shift ;;
        --surge)       SURGE_EXPLICIT=true;            shift ;;
        --cloudflare)  CLOUDFLARE_EXPLICIT=true;       shift ;;
        --dev)         IMAGE="ddphotos"; PULL_FLAG=(); shift ;;
        --doit)        DOIT=true;                      shift ;;
        --no-photogen) DO_PHOTOGEN=false;              shift ;;
        --no-build)    DO_BUILD=false;                 shift ;;
        --verify)      VERIFY=true;                    shift ;;
        --help|-h)
            echo "Usage: bin/deploy-sample-sites.sh [--init] [--sample] [--surge] [--cloudflare] [--dev] [--doit]"
            echo ""
            echo "  --init        Deploy init site only (default: both sites)"
            echo "  --sample      Deploy sample site only (default: both sites)"
            echo "  --surge       Deploy to surge.sh only (default: both providers)"
            echo "  --cloudflare  Deploy to Cloudflare Pages only (default: both providers)"
            echo "  --dev         Use local 'ddphotos' image (default: dougdonohoe/ddphotos:latest)"
            echo "  --doit        Actually upload to surge/wrangler (default: dry-run, skips upload)"
            echo "  --no-photogen Skip the photogen step"
            echo "  --no-build    Skip the build step"
            echo "  --verify      Instead of deploying, verify each site via test-photos-server.sh"
            exit 0
            ;;
        *) echo "Unknown flag: $1" >&2; exit 1 ;;
    esac
done

# Resolve which sites and providers to use
if $INIT_EXPLICIT || $SAMPLE_EXPLICIT; then
    DO_INIT=$INIT_EXPLICIT
    DO_SAMPLE=$SAMPLE_EXPLICIT
else
    DO_INIT=true
    DO_SAMPLE=true
fi

if $SURGE_EXPLICIT || $CLOUDFLARE_EXPLICIT; then
    DO_SURGE=$SURGE_EXPLICIT
    DO_CLOUDFLARE=$CLOUDFLARE_EXPLICIT
else
    DO_SURGE=true
    DO_CLOUDFLARE=true
fi

DEPLOY_DIR="$HOME/junk/ddphotos-deploy"
mkdir -p "$DEPLOY_DIR"

# Both wrangler and surge are bundled in the Docker image via npx; no local install needed.

step() { echo; echo "=== $* ==="; }

# ---------------------------------------------------------------------------
# Build and deploy one site to one or more targets.
# Usage: _deploy_site <site> "<wrangler-project>:<surge-domain>" [...]
#
# Each target is a colon-separated pair; surge-domain may be empty to skip surge.
# photogen and build run once, then all targets are deployed.
# ---------------------------------------------------------------------------
_deploy_site() {
    local site="$1"
    local site_url="$2"
    shift 2
    local site_dir="$DEPLOY_DIR/$site"

    step "Site: $site"

    if $VERIFY; then
        for target in "$@"; do
            local wrangler_project="${target%%:*}"
            local surge_domain="${target#*:}"

            if $DO_CLOUDFLARE; then
                local url="https://${wrangler_project}.pages.dev"
                echo
                echo "=== Validating $site @ cloudflare: $url ==="
                "$SCRIPT_DIR/test-photos-server.sh" --remote "$url" --cloudflare
            fi

            if $DO_SURGE && [ -n "$surge_domain" ]; then
                local url="https://${surge_domain}"
                echo
                echo "=== Validating $site @ surge: $url ==="
                "$SCRIPT_DIR/test-photos-server.sh" --remote "$url" --surge
            fi
        done
        return
    fi

    mkdir -p "$site_dir"

    if [ -z "$(ls -A "$site_dir")" ]; then
        echo "docker: init (create $site_dir)"
        docker run "${PULL_FLAG[@]}" --rm -v "$site_dir":/ddphotos "$IMAGE" init
    else
        echo "docker: init --script-only ($site_dir exists)"
        docker run "${PULL_FLAG[@]}" --rm -v "$site_dir":/ddphotos "$IMAGE" init --script-only
    fi

    if [ "$site" = "sample" ]; then
        echo "config: copy sample/config"
        /bin/rm -rf "$site_dir/config"
        /bin/cp -r "$REPO_ROOT/sample/config" "$site_dir/config"
        # Fix relative base path to absolute so Docker can mount the sample source directory
        sed -i.bak "s|sample: sample/source|sample: $REPO_ROOT/sample/source|" "$site_dir/config/albums.yaml"
        /bin/rm "$site_dir/config/albums.yaml.bak"
    fi

    if $DO_PHOTOGEN; then
        echo "photogen"
        local site_url_args=()
        [ -n "$site_url" ] && site_url_args=(-site-url "$site_url")
        "$site_dir/ddphotos" --show-mounts photogen "${site_url_args[@]}"
    else
        echo "photogen: skipping (--no-photogen set)"
    fi

    if $DO_BUILD; then
        echo "build"
        "$site_dir/ddphotos" --show-mounts build
    else
        echo "build: skipping (--no-build set)"
    fi

    # Export once per provider, then deploy to each target
    local cf_exported=false
    local surge_exported=false

    for target in "$@"; do
        local wrangler_project="${target%%:*}"
        local surge_domain="${target#*:}"

        if $DO_CLOUDFLARE; then
            if ! $cf_exported; then
                echo "export: cloudflare"
                "$site_dir/ddphotos" --show-mounts export --cloudflare --export-site-id cloudflare
                cf_exported=true
            fi
            if $DOIT; then
                echo "wrangler: deploy $wrangler_project"
                "$site_dir/ddphotos" wrangler pages deploy --project-name "$wrangler_project" export/cloudflare
            else
                echo "wrangler: skipping upload (--doit not set)"
            fi
        fi

        if $DO_SURGE && [ -n "$surge_domain" ]; then
            if ! $surge_exported; then
                echo "export: surge"
                "$site_dir/ddphotos" --show-mounts export --copy --export-site-id surge
                surge_exported=true
            fi
            if $DOIT; then
                echo "surge: deploy $surge_domain"
                "$site_dir/ddphotos" surge --domain "$surge_domain" export/surge
            else
                echo "surge: skipping upload (--doit not set)"
            fi
        fi
    done
}

##
## Deploy
##

INIT_TARGETS=(
    "ddphotos-init:ddphotos-init.surge.sh"
)
SAMPLE_TARGETS=(
    "ddphotos-sample:ddphotos-sample.surge.sh"
    "my-unique-site:"
)

if $DO_INIT;   then _deploy_site "init"   "https://ddphotos-test.donohoe.info" "${INIT_TARGETS[@]}";   fi
if $DO_SAMPLE; then _deploy_site "sample" ""                                   "${SAMPLE_TARGETS[@]}"; fi

echo
echo "Done."
