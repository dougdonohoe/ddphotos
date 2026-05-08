# Testing

**Note:** This page is for contributors developing DD Photos, not for users building their own sites with it.

There are three ways of testing DD Photos:

1. **Manual testing** in a browser, against the Vite dev server or a local static build (via Docker)
2. **Playwright e2e tests** that drive a headless Chromium browser to verify UI behavior
3. **Apache routing tests** using `curl` to verify `.htaccess` URL routing, redirects, and 404 handling

All three are discussed below.

## Manual Testing - Dev

As seen in the [README](../README.md), development is primarily done via
the Vite server. This is the easiest, as it automatically reloads when
any of the SvelteKit files change or even when `photogen` is re-run.

```bash
# Sample site
make sample-npm-run-dev

# Named site
DDPHOTOS_SITE_ID=<site-id> make web-npm-run-dev
```

You should see a `VITE` message and a browser window should
open at [localhost:5173](http://localhost:5173/).

## Manual Testing - Build

As seen in the [README](../README.md), the site has a build step:

```bash
# Sample site
make sample-build

# Uses default site (specified in config/defaults.env)
make web-npm-build

# Uses named site
DDPHOTOS_SITE_ID=<site-id> make web-npm-build
```

Once the site is built, you can serve it via Docker (Apache/nginx).

## Manual Testing - Build Served via Docker

The Docker environment mirrors one possible production setup and applies URL routing
locally. The `build/` directory is mounted in the container (not `build/<site-id>/`)
so that npm rebuilds (which delete and recreate `build/<site-id>/`) don't break the
container's bind mount. Apache and nginx are both supported.

```bash
# One-time: build the Docker image(s)
make web-docker-build-apache # Apache
make web-docker-build-nginx  # nginx

# Start on port 8080 (runs in foreground; Ctrl-C to stop)
# Site rebuilds do not require a restart
make web-docker-run-apache # Apache
make web-docker-run-nginx  # nginx

# Uses named site
DDPHOTOS_SITE_ID=<site-id> make web-docker-run-apache
```

You should be able to see the site at [localhost:8080](http://localhost:8080).

## Automated Tests - Docker via Curl

If Docker is running, `make web-docker-test` runs 
`bin/test-photos-server.sh --local 8080`, which tests URL routing, redirects, 
404 handling, photo permalink URLs, static asset accessibility,
and verifies asset paths in HTML are absolute (required for photo permalink
pages to render correctly).

```bash
make web-docker-test
```

You can also run the `test-photos-server.sh` script directly, against production or locally:

```bash
bin/test-photos-server.sh --remote https://photos.example.com                # remote site
bin/test-photos-server.sh --remote https://your-site.pages.dev --cloudflare  # Cloudflare Pages
bin/test-photos-server.sh --remote https://your-site.surge.sh --surge        # Surge
bin/test-photos-server.sh --local                                            # local Docker on port 8080
bin/test-photos-server.sh --local 9090                                       # local Docker on custom port
```

The deployment script runs this script automatically after deploying.

## Automated Tests - Playwright E2E Tests

Playwright runs a real headless Chromium browser against a Docker container (Apache
or nginx), the dev server, or even a production server, testing JavaScript behavior
that static HTML checks can't cover - specifically lightbox caption rendering across
the different open paths.

```bash
# One-time setup (downloads ~100 MB Chromium binary)
make web-playwright-install

# starts a separate Docker/Apache on port 8083, runs no-passwords tests, stops Docker
make web-playwright-test-apache

# starts a separate Docker/nginx on port 8084, runs no-passwords tests, stops Docker
make web-playwright-test-nginx

# starts a separate dev server on port 5174, runs no-passwords tests, stops Docker
make web-playwright-test-dev
```

Tests are in `web/tests/` and cover:

| File                  | What it tests                                                                                   |
|-----------------------|-------------------------------------------------------------------------------------------------|
| `smoke.spec.ts`       | Home page album listing, album page metadata, grid renders, Open Graph tags                     |
| `captions.spec.ts`    | Lightbox caption rendering: grid click, permalink direct load, prev/next nav                    |
| `url.spec.ts`         | URL updates on photo open/navigate/close; permalink URL preserved on load                       |
| `navigation.spec.ts`  | Cross-album client-side navigation shows correct photos, title, description                     |
| `back-nav.spec.ts`    | Browser back button behavior: closes lightbox, restores URL, handles reload                     |
| `back-to-top.spec.ts` | Back-to-top button visibility and scroll behavior                                               |
| `privacy.spec.ts`     | Privacy page content, back link, scroll restoration on return to home                           |
| `password.spec.ts`    | Site/album prompts, wrong/correct passwords, remember on reload, hints, logout button, `?clear` |
| `css.spec.ts`         | Custom CSS `<link>` injection, `--text-color-2nd` override, album card border-radius            |

Navigation tests are fully dynamic - they read album names from the page at runtime and
work against any site without hardcoding album names.  Several tests require the presence of 
the `antarctica` album in the sample website, and are skipped if that album is missing.  Other
tests require a minimum number of albums and are skipped if the site doesn't comply.  In general,
the tests can be run against any site to verify base functionality.

The `baseURL` is set via `PLAYWRIGHT_BASE_URL`. `bin/run-tests.sh` sets it automatically
to the port for the selected mode (5174 for dev, 8083 for Apache, 8084 for nginx).
The `playwright.config.ts` default of `http://localhost:8080` is only used when running
Playwright directly (e.g. via `deploy-photos.sh`).

Password and CSS tests are gated by environment variables so they only run against
the appropriate site variant:

| Variable                       | Set by               | Effect                                         |
|--------------------------------|----------------------|------------------------------------------------|
| `PLAYWRIGHT_PASSWORDS_FILE`    | `bin/run-tests.sh`   | Path to passwords file; enables password tests |
| `PLAYWRIGHT_CUSTOM_CSS`        | `bin/run-tests.sh`   | Set to `true`; enables CSS tests               |

Use `bin/run-tests.sh` or `bin/test-all.sh` to run tests across all variants automatically.
`bin/test-all.sh` runs four variants: no passwords, `passwords-all.yaml`, `passwords-uganda.yaml`,
and `custom-css` (with `sample/config/custom.css` injected).

```bash
# Run all 4 variants against dev + Apache + nginx (default; recommended locally)
bin/test-all.sh

# Run all 4 variants against Apache only (mirrors CI)
bin/test-all.sh --mode apache

# Run all 4 variants against nginx only
bin/test-all.sh --mode nginx

# Run all 4 variants against dev server, Apache, and nginx
bin/test-all.sh --mode all

# Run a single variant against Apache (no password)
bin/run-tests.sh --mode apache

# Run a single variant against nginx (no password)
bin/run-tests.sh --mode nginx

# Run pw-all variant against Apache
bin/run-tests.sh --passwords sample/config/passwords-all.yaml --mode apache

# Run custom CSS variant against dev server
bin/run-tests.sh --css sample/config/custom.css --mode dev

# Run a single test file against dev server (useful for debugging a specific test)
bin/run-tests.sh --mode dev --test tests/privacy.spec.ts
```

### Sanity Check

A good sanity check verifies against Apache (which requires a build), and tests
both password and no-password sites.  It's quicker than running all 4 variants against
dev, Apache and nginx:

```bash
make web-sanity-test
```

The `bin/deploy-photos.sh` script runs Playwright automatically: locally before rsync,
and against production after CloudFront cache invalidation.

## Testing Deployment

The two deploy paths can be validated locally without touching a real server:

```bash
# rsync path — rsyncs into a local Docker container; runs server routing tests and Playwright
make sample-rsync-test

# S3 path — syncs against MinIO; verifies file placement and Cache-Control headers
# (post-deploy server and Playwright tests are skipped: MinIO serves S3 API only, not HTTP)
make sample-s3-test
```

## Testing Docker

The `bin/docker-test.sh` script exercises the full `ddphotos` Docker workflow end-to-end —
from `init` through `photogen`, `run`, `build`, `serve`, `export`, and `version` —
using the built-in sample photos that ship with the image.

```bash
make docker-test              # build the ddphotos image and run all tests
bin/docker-test.sh --no-build # skip image build (reuse existing ddphotos image)
```

The script runs the following steps in a fresh temp workspace:

1. Builds the `ddphotos` Docker image via `make docker-build`
2. Runs `init` and verifies the `ddphotos` script and config files are created
3. Runs `photogen` on the bundled sample photos and verifies album output
4. Runs `decode` on an encrypted album index and verifies the output, including files outside `DDPHOTOS_DIR` (via `--passwords` flag and embedded `pwFile` path)
5. Runs `search-cover` against the decoded album and verifies the cover file is found
6. Regression test: runs `decode` and `search-cover` with an external `--config-dir` (outside `DDPHOTOS_DIR`) to verify the config mount path is handled correctly
7. Starts the Vite dev server (`run`) and runs Playwright e2e tests against it
8. Runs `build` and verifies the static site output
9. Starts Apache (`serve`) and runs Playwright e2e tests + `bin/test-photos-server.sh` routing tests
10. Tests `export` (symlink mode), `export --copy` (all files resolved, no symlinks), and `export --cloudflare` (adds `_worker.js`)
11. Verifies `version` and `version --image` output — checks script path and image `Git:`/`Version:` fields
12. Runs `init --script-only` and verifies only the script is installed (no `config/` or `albums/`)

Playwright tests skip assertions that depend on sample-site-specific albums (e.g. `antarctica`) when
those albums are not present in the init site, so the full test suite runs cleanly against the
smaller built-in sample. The temp workspace is cleaned up automatically on exit.

## CI (GitHub Actions)

The workflow in [.github/workflows/ci.yml](../.github/workflows/ci.yml) runs on every push or pull 
request to `main`. See that file for all the tests it runs.  Tests are configured to run in parallel
to minimize CI run time.

The workflow in [.github/workflows/docker-release.yml](../.github/workflows/docker-release.yml)
runs when a new git version tag is created.  If this succeeds,
[.github/workflows/deploy-sample-sites.yml](../.github/workflows/deploy-sample-sites.yml) is
run to deploy latest code to the Cloudflare and Surge sample sites.

### Testing CI Locally with `act`

It is often helpful to run GitHub CI locally using [`act`↗](https://nektosact.com/).
It requires Docker. Before running, there is one key prerequisite and one important caveat to understand:

```bash
# Prerequisite: generate and build sample site before running `act`
make web-docker-build-apache web-docker-build-nginx sample-photogen sample-build

# Run act to simulate GitHub
act --reuse --pull=false -W .github/workflows/ci.yml
```

**Why Sample:** `act` runs the workflow inside a Docker container with a copy of your repo. However,
when the workflow invokes `docker run -v $(PWD)/web:...` (for Apache/Playwright tests), that
command goes to the **host** Docker daemon with **host** filesystem paths, effectively ignoring
whatever was built inside the `act` container. There are two versions of the repo in play: one
inside `act`'s container (where Go builds, photogen, and npm build run), and one on your host
(which the inner Docker mounts for Apache/Playwright). Generating the sample site first ensures
the host copy has up-to-date sample data and `web/build` for the inner Docker to serve.
(Think Inception: Docker within Docker, each with its own reality).

**Caveat:** `act` copies your working directory including git-ignored files, so photogen will
skip already-generated files rather than regenerating them from scratch. Real GitHub CI always
starts from a clean checkout.

For full end-to-end CI validation from a clean slate, push to GitHub. A draft PR triggers CI
without signaling the code is ready to merge:

```bash
git commit --allow-empty -m "ci: test GitHub Actions workflow"
gh pr create --draft --title "wip: testing CI" --body ""
```
