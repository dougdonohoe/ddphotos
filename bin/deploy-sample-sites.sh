#!/usr/bin/env bash
# Deploy sample sites to static hosting providers.
#
# Deploys two sites:
#   - init:   the ddphotos 'init' example site
#   - sample: the sample site in this repo (sample/)
#
# Usage:
#   bin/deploy-sample-sites.sh [--init] [--sample] [--surge] [--cloudflare] [--s3] [--dev] [--doit]
#
# Sites:
#   Surge:
#     Init:     https://ddphotos-init.surge.sh   (old, now redirects https://ddphotos-test-docker.surge.sh/)
#     Sample:   https://ddphotos-sample.surge.sh (old, now redirects https://ddphotos-test-sample.surge.sh/)
#   Cloudflare:
#     Init:     https://ddphotos-init.pages.dev
#     Sample:   https://ddphotos-sample.pages.dev
#     Sample 2: https://my-unique-site.pages.dev/
#   S3/CloudFront:
#     Init:     https://ddphotos-test.donohoe.info  (bucket: ddphotos-test-donohoe-info)
#     Sample:   https://ddphotos.donohoe.info       (bucket: ddphotos-donohoe-info)
#
# With no site flags, both sites are deployed.
# With no provider flags, all providers are used.
# Flags combine: --cloudflare --sample deploys only sample to Cloudflare.
# Without --doit, everything runs except the final surge/wrangler/S3 upload.
#
# S3 credentials: set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN in the
# environment, or configure ~/.aws. CloudFront invalidation requires DDPHOTOS_CF_ID
# (sample site) and DDPHOTOS_TEST_CF_ID (init site) to be set.
#
# Options:
#   --init        Deploy init site only
#   --sample      Deploy sample site only
#   --surge       Deploy to surge.sh only
#   --cloudflare  Deploy to Cloudflare Pages only
#   --s3          Deploy to S3/CloudFront only
#   --dev         Use local 'ddphotos' image (default: dougdonohoe/ddphotos:latest)
#   --doit        Actually upload to surge/wrangler/S3 (default: dry-run, skips upload)
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
S3_EXPLICIT=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --init)        INIT_EXPLICIT=true;             shift ;;
        --sample)      SAMPLE_EXPLICIT=true;           shift ;;
        --surge)       SURGE_EXPLICIT=true;            shift ;;
        --cloudflare)  CLOUDFLARE_EXPLICIT=true;       shift ;;
        --s3)          S3_EXPLICIT=true;               shift ;;
        --dev)         IMAGE="ddphotos"; PULL_FLAG=(); shift ;;
        --doit)        DOIT=true;                      shift ;;
        --no-photogen) DO_PHOTOGEN=false;              shift ;;
        --no-build)    DO_BUILD=false;                 shift ;;
        --verify)      VERIFY=true;                    shift ;;
        --help|-h)
            echo "Usage: bin/deploy-sample-sites.sh [--init] [--sample] [--surge] [--cloudflare] [--s3] [--dev] [--doit]"
            echo ""
            echo "  --init        Deploy init site only (default: both sites)"
            echo "  --sample      Deploy sample site only (default: both sites)"
            echo "  --surge       Deploy to surge.sh only (default: both providers)"
            echo "  --cloudflare  Deploy to Cloudflare Pages only (default: both providers)"
            echo "  --s3          Deploy to S3/CloudFront only (default: both providers)"
            echo "  --dev         Use local 'ddphotos' image (default: dougdonohoe/ddphotos:latest)"
            echo "  --doit        Actually upload to surge/wrangler/S3 (default: dry-run, skips upload)"
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

if $SURGE_EXPLICIT || $CLOUDFLARE_EXPLICIT || $S3_EXPLICIT; then
    DO_SURGE=$SURGE_EXPLICIT
    DO_CLOUDFLARE=$CLOUDFLARE_EXPLICIT
    DO_S3=$S3_EXPLICIT
else
    DO_SURGE=true
    DO_CLOUDFLARE=true
    DO_S3=true
fi

DEPLOY_DIR="$HOME/junk/ddphotos-deploy"
mkdir -p "$DEPLOY_DIR"

# Both wrangler and surge are bundled in the Docker image via npx; no local install needed.

step() { echo; echo "=== $* ==="; }

# Docker init, config copy, photogen, build — runs once per site before any provider deploy.
# Usage: _setup_site <site> <site_url>
#   site_url: passed to photogen -site-url (empty to use config default)
_setup_site() {
    local site="$1" site_url="$2"
    local site_dir="$DEPLOY_DIR/$site"

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
    else
        # Replace the default placeholder domain written by 'ddphotos init' with the real site URL
        if [ -n "$site_url" ]; then
            local domain="${site_url#https://}"
            sed -i.bak "s/your-ddphotos\.example\.com/$domain/" "$site_dir/config/albums.yaml"
            /bin/rm "$site_dir/config/albums.yaml.bak"
        fi
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
}

# Deploy or verify: S3/CloudFront
# Usage: _run_s3 <site> <s3_bucket> <s3_cf_id> <s3_url>
#   s3_cf_id: CloudFront distribution ID (empty skips invalidation)
_run_s3() {
    local site="$1" s3_bucket="$2" s3_cf_id="$3" s3_url="$4"
    local site_dir="$DEPLOY_DIR/$site"
    if $VERIFY; then
        echo; echo "=== Validating $site @ s3: $s3_url ==="
        "$SCRIPT_DIR/test-photos-server.sh" --remote "$s3_url" --s3
        return
    fi

    # TODO: Temp fix: Docker init creates site.env as root-owned rw-r--r--; delete so we can recreate it.
    #       Remove once do-init.sh's chmod a+w is in a released image.
    /bin/rm -f "$site_dir/config/site.env"
    {
        printf 'S3_BUCKET=%s\n' "$s3_bucket"
        [ -n "$s3_cf_id" ] && printf 'CLOUDFRONT_ID=%s\n' "$s3_cf_id"
    } > "$site_dir/config/site.env"
    if $DOIT; then
        echo "s3: deploy $s3_bucket"
        "$site_dir/ddphotos" deploy --no-server-test # server test done by --verify
    else
        echo "s3: skipping deploy to $s3_bucket (--doit not set)"
    fi
}

# Deploy or verify: Cloudflare Pages
# Usage: _run_cloudflare <site> <wrangler_project> [<wrangler_project2> ...]
_run_cloudflare() {
    local site="$1"; shift
    local site_dir="$DEPLOY_DIR/$site"
    if $VERIFY; then
        for project in "$@"; do
            echo; echo "=== Validating $site @ cloudflare: https://${project}.pages.dev ==="
            "$SCRIPT_DIR/test-photos-server.sh" --remote "https://${project}.pages.dev" --cloudflare
        done
        return
    fi
    local exported=false
    for project in "$@"; do
        if ! $exported; then
            echo "export: cloudflare"
            "$site_dir/ddphotos" --show-mounts export --cloudflare --export-site-id cloudflare
            exported=true
        fi
        if $DOIT; then
            echo "wrangler: deploy $project"
            "$site_dir/ddphotos" --non-interactive wrangler pages deploy --project-name "$project" export/cloudflare
        else
            echo "wrangler: skipping upload to $project (--doit not set)"
        fi
    done
}

# Deploy or verify: Surge
# Usage: _run_surge <site> <surge_domain>
_run_surge() {
    local site="$1" surge_domain="$2"
    local site_dir="$DEPLOY_DIR/$site"
    if $VERIFY; then
        echo; echo "=== Validating $site @ surge: https://${surge_domain} ==="
        "$SCRIPT_DIR/test-photos-server.sh" --remote "https://${surge_domain}" --surge
        return
    fi
    echo "export: surge"
    "$site_dir/ddphotos" --show-mounts export --copy --export-site-id surge
    if $DOIT; then
        echo "surge: deploy $surge_domain"
        "$site_dir/ddphotos" --non-interactive surge --domain "$surge_domain" export/surge
    else
        echo "surge: skipping upload to $surge_domain (--doit not set)"
    fi
}

##
## Deploy
##

if $DO_INIT; then
    step "Site: init"
    $VERIFY || _setup_site "init" "https://ddphotos-test.donohoe.info"
    $DO_S3         && _run_s3         "init" "ddphotos-test-donohoe-info" "${DDPHOTOS_TEST_CF_ID:-}" "https://ddphotos-test.donohoe.info"
    $DO_CLOUDFLARE && _run_cloudflare "init" "ddphotos-init"
    $DO_SURGE      && _run_surge      "init" "ddphotos-init.surge.sh"
fi

if $DO_SAMPLE; then
    step "Site: sample"
    $VERIFY || _setup_site "sample" ""
    $DO_S3         && _run_s3         "sample" "ddphotos-donohoe-info" "${DDPHOTOS_CF_ID:-}" "https://ddphotos.donohoe.info"
    $DO_CLOUDFLARE && _run_cloudflare "sample" "ddphotos-sample" "my-unique-site"
    $DO_SURGE      && _run_surge      "sample" "ddphotos-sample.surge.sh"
fi

echo
echo "Done."
