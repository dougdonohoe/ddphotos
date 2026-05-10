# Development Notes

**Note:** This page is for contributors developing DD Photos, not for users building their own sites with it.

## SvelteKit

SvelteKit is two things bundled together:

- **Svelte** — a UI component framework (a React alternative). Components are written in `.svelte`
  files combining HTML, CSS, and JavaScript. Unlike React, Svelte compiles components to vanilla
  JavaScript at build time with no virtual DOM and no runtime library shipped to the browser.
- **Kit** — the application framework built on top of Svelte, analogous to Next.js for React.
  It adds file-based routing (via `src/routes/`), data loading (`+page.ts`), adapters for different
  deployment targets, and the build pipeline via Vite.

What SvelteKit specifically does for this project:

- **Routing** — `src/routes/albums/[slug]/[[index]]/` becomes `/albums/antarctica/1` automatically
- **Data loading** — `+page.ts` fetches `albums.json` and `index.json` before the page renders
- **Component reactivity** — lightbox state, theme toggle, image loading effects
- **Build pipeline** — Vite bundles everything; `adapter-static` pre-renders all routes to `.html` files.
  For encrypted builds, the SvelteKit crawler cannot discover album links hidden behind the password
  prompt, so `svelte.config.js` uses an `albumEntries()` function to read album slugs from
  `web/static/albums/` at build time and inject them into `prerender.entries` directly.
- **Client-side navigation** — clicking between albums swaps content without a full page reload

The site is a hybrid of static and dynamic rendering:

- **Static**: the HTML shell (nav, footer, page structure) is pre-built at deploy time and served
  as plain files — no server generates pages on request
- **Dynamic**: `albums.json` and `index.json` are fetched by JavaScript in the browser after load;
  the photo grid, lightbox, and navigation are all rendered client-side from that JSON data

The JSON files themselves are static files, but their content is rendered in the browser. This
pattern is called CSR (Client-Side Rendering) with a static shell — the shell is pre-built, but
the content is rendered by JavaScript in the browser rather than on a server.

## LAN Access

When running the dev server (`make web-npm-run-dev`), you should see a Vite message listing 
the URLs where the site is accessible, typically http://localhost:5173 and any local network 
IPs (useful for testing on your phone or tablet).

Another way to get the LAN IP is as follows (helpful if running Apache, which doesn't print
out IPs):

```bash
# macOS
ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null

# Linux
hostname -I | awk '{print $1}'
```

## HTTPS on the Dev Server

Password-protected albums use the [Web Crypto API↗](https://developer.mozilla.org/en-US/docs/Web/API/Web_Crypto_API)
(`crypto.subtle`), which browsers only expose in [secure contexts↗](https://developer.mozilla.org/en-US/docs/Web/Security/Secure_Contexts).
`localhost` qualifies, but a LAN IP address (e.g. `192.168.x.x`) over plain HTTP does not —
the password prompt will appear but decryption will silently fail.

To serve the dev server over HTTPS, set `VITE_HTTPS=1`:

```bash
VITE_HTTPS=1 make web-npm-run-dev        # or your own site-specific recipe
```

This loads `@vitejs/plugin-basic-ssl`, which generates a self-signed certificate automatically.
Your browser (and mobile browser) will show a certificate warning — click through
**Advanced → Proceed** once and the site works normally, including password decryption.

## Simulating Slow Image Loading

Album pages fade images in as they load. On a fast local connection this is
imperceptible. To simulate slow loading and see the effect, append `?slow` to
any album URL:

```
http://localhost:5173/albums/your-album?slow
```

Each image's `src` is assigned after a random 500–2500ms delay, triggering a
real browser load cycle rather than just a visual trick. Works on production too:

```
https://photos.example.com/albums/your-album?slow
```

## Logging Dev Server Requests

To see each request to the dev server (useful in debugging) set `VITE_LOG_REQUESTS=1`:

```bash
VITE_LOG_REQUESTS=1 make sample-npm-run-dev
```

## Setting VITE Variables

To set values not normally set in dev server:

```bash
VITE_DOCKER_IMAGE='dougdonohoe/ddphotos:v1.12.0' make sample-npm-run-dev
```

## Debugging

To enable the `debug` library, where `debug()` calls are logged in the JavaScript
console, and also logged in the dev server, set `VITE_DEBUG=1`:

```bash
VITE_DEBUG=1 make sample-npm-run-dev
```

Usage examples:

```ts
import { debug } from '$lib/debug';

// Simple message
debug("I'm here")

// Pretty-print an object as JSON
const siteConfig: SiteConfig = await res.json();
debug('siteConfig', siteConfig);

// Avoid SvelteKit warnings about state
let { data } = $props();
$effect(() => { debug("In home page svelte, got $props()", data) });
```

## Python Setup

Two scripts use Python:

- `bin/generate-screenshot-composite.py` — generates `images/screenshots.png` (invoked by `make web-screenshots`); requires [Pillow↗](https://pillow.readthedocs.io/)
- `bin/gen-deploy-tree.py` — regenerates `docs/deploy-tree.svg` and `docs/deploy-tree.png` (the colored directory tree in `DEPLOY.md`); requires [rich↗](https://github.com/Textualize/rich) and `rsvg-convert` (from `librsvg`)

Set up a local virtualenv once using [uv↗](https://github.com/astral-sh/uv):

```bash
brew install uv librsvg  # if not already installed
uv venv .venv
uv pip install -r requirements.txt
```

The `.venv/` directory is git-ignored. The `make web-screenshots` and `make gen-deploy-tree`
targets call `.venv/bin/python3` directly, so no manual activation is needed.

## Docker Notes

To build the DD Photos Docker `ddphotos` image for testing locally:

```bash
make docker-build
```

To quickly test it out manually:

```bash
rm -rf /tmp/my-ddphotos && mkdir -p /tmp/my-ddphotos
docker run --rm -v /tmp/my-ddphotos:/ddphotos ddphotos init
/tmp/my-ddphotos/ddphotos photogen
/tmp/my-ddphotos/ddphotos run
```

Visit [http://localhost:5173/](http://localhost:5173/).

To install `ddphotos` in `~/.local/bin`:

```bash
make ddphotos-install-dev # from local docker ddphotos (having run make docker-build)
make ddphotos-install-prod # from prod docker dougdonohoe/ddphotos:latest
```

To run test suite:

```bash
make docker-test
```

See [Testing Docker](TESTING.md#testing-docker) for information on the test suite.

### Docker Push

Pushing a multi-arch image to Docker Hub is automated via the GitHub Actions workflow
`.github/workflows/docker-release.yml`, which triggers whenever a version tag (e.g. `v1.2.3`)
is pushed. It calls `bin/docker-push.sh --doit` and publishes both `:vX.Y.Z` and `:latest` tags.

You can also push manually:

```bash
make docker-push
```

#### Required GitHub Actions Secrets

The workflow requires two repository secrets. Run these from inside the repo directory
(secrets are repo-specific):

```bash
gh secret set DOCKERHUB_USERNAME --body "dougdonohoe"
gh secret set DOCKERHUB_TOKEN   # prompts for value — keeps token out of shell history
```

To generate the `DOCKERHUB_TOKEN`:

1. Log in to [hub.docker.com](https://hub.docker.com)
2. Account Settings → Personal access tokens → Generate new token
3. Set access to **Read, Write** (Write is required to push images; Admin is not needed)
4. Copy the token and paste it when `gh secret set DOCKERHUB_TOKEN` prompts you

### Docker Build Errors

After doing many local builds, you may suddenly experience build failures and see messages like this:

```bash
W: GPG error: http://deb.debian.org/debian bookworm InRelease: At least one invalid signature was encountered.
E: The repository 'http://deb.debian.org/debian bookworm InRelease' is not signed.

error: failed to solve: failed to create temp dir: mkdir /tmp/xyz: no space left on device
```

This means Docker is running out of disk space. To reclaim space, run:

```bash
docker buildx prune # less aggressive
docker system prune # get all the bytes possible
```

The amount of space is set in _Docker Desktop → Settings → Resources → Advanced → Disk usage limit_.

## Static Site Examples

Use `bin/deploy-sample-sites.sh --doit` to deploy `init` and `sample` sites to Cloudflare and surge.

* Surge sites
  * Init: [ddphotos-init.surge.sh↗](https://ddphotos-init.surge.sh) (was [ddphotos-test-docker.surge.sh↗](https://ddphotos-test-docker.surge.sh/), now redirects)
  * Sample: [ddphotos-sample.surge.sh↗](https://ddphotos-sample.surge.sh) (was [ddphotos-test-sample.surge.sh↗](https://ddphotos-test-sample.surge.sh/), now redirects)
* Cloudflare sites:
  * Init: [ddphotos-init.pages.dev↗](https://ddphotos-init.pages.dev)
  * Sample: [ddphotos-sample.pages.dev↗](https://ddphotos-sample.pages.dev)
  * Sample alternate: [my-unique-site.pages.dev↗](https://my-unique-site.pages.dev/)

Use `deploy-sample-sites.sh --verify` to run smoke tests against these sites

### CI Setup

The `deploy-sample-sites.yml` workflow is triggered when the `docker-release.yml` workflow succeeds.
It runs `deploy-sample-sites.sh --doit` and then `--verify`.  The following secrets are needed
for each deployment type:

#### Cloudflare (wrangler)

 * `CLOUDFLARE_API_TOKEN` — create at [dash.cloudflare.com](https://dash.cloudflare.com) 
   * _My Profile > API Tokens > + Create Token_ 
   * **Create Custom Token** with **Account > Cloudflare Pages > Edit**
 * `CLOUDFLARE_ACCOUNT_ID` — visible in the Cloudflare dashboard sidebar

#### Surge

 * `SURGE_LOGIN` — your `surge` email
 * `SURGE_TOKEN` — run `surge token` locally to print it

#### S3 (AWS)

S3 deployment uses GitHub Actions OIDC — no long-lived credentials are stored. The IAM role
is defined in a private infra repo.

 * `DDPHOTOS_DEPLOY_ROLE_ARN` — IAM role ARN; get from tofu output (see below)
 * `DDPHOTOS_CF_ID` — CloudFront distribution ID for `ddphotos.donohoe.info`
 * `DDPHOTOS_TEST_CF_ID` — CloudFront distribution ID for `ddphotos-test.donohoe.info`

#### Secrets

How to set GitHub secrets:

```bash
gh secret set CLOUDFLARE_ACCOUNT_ID --body "your-account-id"
gh secret set CLOUDFLARE_API_TOKEN # paste token
gh secret set SURGE_LOGIN --body "your@email.com"
gh secret set SURGE_TOKEN # paste token
gh secret set DDPHOTOS_DEPLOY_ROLE_ARN --body $(tofu output ddphotos_ci_deploy_role_arn)
gh secret set DDPHOTOS_CF_ID --body $(tofu output ddphotos_donohoe_cloudfront_id)
gh secret set DDPHOTOS_TEST_CF_ID --body $(tofu output ddphotos_test_donohoe_cloudfront_id)
gh secret list
```

Manual Trigger

```bash
gh workflow run deploy-sample-sites.yml # main branch
gh workflow run deploy-sample-sites.yml --ref [branch-name] # on specified branch
```

## Project History

Much of this project was built with Claude Code. See [HISTORY.md](history/HISTORY.md)
for a detailed session log.
