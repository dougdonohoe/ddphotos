# Testing

**Note:** This page is for contributors developing DD Photos, not for users building their own sites with it.

There are four ways of testing DD Photos:

1. **Manual testing** in a browser, against the Vite dev server or a local static build (via Docker)
2. **Vitest unit tests** for the plain TypeScript helpers in `web/src/lib`, with no browser involved
3. **Playwright e2e tests** that drive a headless Chromium browser to verify UI behavior
4. **Apache routing tests** using `curl` to verify `.htaccess` URL routing, redirects, and 404 handling

All four are discussed below.

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

## Automated Tests - Vitest Unit Tests

Logic that is a pure function of its inputs does not need a browser, a build or an album
to test. Those live in `web/src/lib` as plain TypeScript and are covered by Vitest, which
runs in under a second:

```bash
make web-unit-test
```

Tests sit beside the code they cover as `*.test.ts` (e.g. `src/lib/counts.test.ts` for
`src/lib/counts.ts`) and are table-driven where the code is a set of rules. `web/tests/`
stays Playwright-only: `vitest.config.ts` narrows its `include` to `src/**/*.test.ts` so the
two suites cannot pick up each other's files.

Use this for wording, formatting and calculation rules; use Playwright when the thing being
verified is what the page actually renders. The "n photos · n videos" meta line has both: the
rules are unit-tested, and `video.spec.ts` checks that the album header and the home page card
really show them.

For a watch loop while working on a helper, run `npm run test:unit:watch` in `web/`.

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

| File                  | What it tests                                                                                                                                                   |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `smoke.spec.ts`       | Home page album listing, album page metadata, grid renders, Open Graph tags                                                                                     |
| `captions.spec.ts`    | Lightbox caption rendering: grid click, permalink direct load, prev/next nav                                                                                    |
| `url.spec.ts`         | URL updates on photo open/navigate/close; permalink URL preserved on load                                                                                       |
| `navigation.spec.ts`  | Cross-album client-side navigation shows correct photos, title, description                                                                                     |
| `back-nav.spec.ts`    | Browser back button behavior: closes lightbox, restores URL, handles reload                                                                                     |
| `back-to-top.spec.ts` | Back-to-top button visibility and scroll behavior                                                                                                               |
| `privacy.spec.ts`     | Privacy page content, back link, scroll restoration on return to home                                                                                           |
| `password.spec.ts`    | Site/album prompts, wrong/correct passwords, remember on reload, hints, logout button, `?clear`                                                                 |
| `css.spec.ts`         | Custom CSS `<link>` injection, `--text-color-2nd` override, album card border-radius                                                                            |
| `video.spec.ts`       | Play badge, lightbox `<video>`, space to play/pause, pause on swipe/close, caption hidden while playing, MP4 MIME + ranges, photo/video counts in the meta line |

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

| Variable                    | Set by                          | Effect                                         |
|-----------------------------|---------------------------------|------------------------------------------------|
| `PLAYWRIGHT_PASSWORDS_FILE` | `bin/run-tests.sh`              | Path to passwords file; enables password tests |
| `PLAYWRIGHT_CUSTOM_CSS`     | `bin/run-tests.sh`              | Set to `true`; enables CSS tests               |
| `PLAYWRIGHT_SKIP_VIDEO`     | `bin/run-tests.sh --skip-video` | Skips `video.spec.ts`; see below               |

Use `bin/run-tests.sh` or `bin/test-all.sh` to run tests across all variants automatically.
`bin/test-all.sh` runs four variants: no passwords, `passwords-all.yaml`, `config-extras`
(`passwords-keyonly.yaml`, which declares no effective password, plus
`sample/config/custom-example.css` and `sample/config/customization-album-nav.yaml`), and
`passwords-uganda.yaml`.

#### Shared site IDs

Three of those four variants generate into the same albums directory, `albums/sample`. Only
`passwords-all.yaml` gets its own (`albums/sample-pw-all`). This is the single biggest saving
in CI: a directory that already holds every WebP and MP4 leaves photogen with nothing to
resize and no video to transcode, so a variant costs a few seconds instead of ~40, and the
sample videos are transcoded twice per CI job instead of six times.

It works because media bytes do not depend on encryption, CSS or customization. What
encryption changes is output *filenames*: `Config.PhotoOutputName` HMACs the name only for
albums that actually have a password, so variants encrypting the same set of albums produce
identically named, byte-identical media.

| Variant                  | the-way | uganda | antarctica | Site ID         |
|--------------------------|---------|--------|------------|-----------------|
| no passwords             | plain   | plain  | plain      | `sample`        |
| `config-extras`          | plain   | plain  | plain      | `sample`        |
| `passwords-uganda.yaml`  | plain   | HMAC   | plain      | `sample`        |
| `passwords-all.yaml`     | HMAC    | HMAC   | HMAC       | `sample-pw-all` |

Sharing stays correct because photogen runs with `-clean` on every variant, which normalizes
the directory to that variant: everything it writes is registered with `TrackFile`, and
`CleanOutputDir` deletes whatever the previous variant left behind, top-level files
(`custom.css`, `albums.enc.json`) included. Two consequences worth knowing:

- **The static build is rebuilt for every variant.** `handleFetch` in `web/src/hooks.server.ts`
  reads `/albums/*.json` off disk during pre-rendering and bakes it into the HTML, so
  `build/sample` belongs to one variant even though the site ID is shared.
- **`bin/test-all.sh` leaves `albums/sample` and `build/sample` holding the last variant it
  ran.** Anything that assumes the plain sample site, such as `make sample-rsync-test` and
  `make sample-s3-test`, has to run before it. `.github/workflows/ci.yml` orders its steps
  that way. Run `make sample-photogen sample-build` to get back to the plain site.

A direct `bin/run-tests.sh` invocation still derives a self-describing site ID
(`sample-css`, `sample-pw-uganda`, and so on), which is more useful when inspecting the
output of one variant. Pass `--site-id` to override it.

#### Why `config-extras` is one variant and not three

`passwords-keyonly`, `custom-css` and `album-nav` used to be three separate runs. Diffing
their generated sites against the plain one shows why they no longer are. Each differs from
plain by a single `config.json` field and nothing else, while the two encrypted variants are
genuinely different sites:

| Variant             | Entries differing from the plain site                      |
|---------------------|------------------------------------------------------------|
| `passwords-keyonly` | 1: `config.json` gains `"keyId"`                           |
| `custom-css`        | 2: `config.json` gains `"customCss"`, plus `custom.css`    |
| `album-nav`         | 1: `config.json` gains `"albumNav"`                        |
| `passwords-uganda`  | 89: Uganda HMAC'd, no `cover.jpg`, `albums.json` differs   |
| `passwords-all`     | 278: all three albums HMAC'd, `albums.enc.json`, no covers |

The three fields are orthogonal, so one run with all three set covers what the three runs
did. Measured: the union of the three was 69 tests, the combined run is 67, and the only two
it drops are the negative assertions ("CSS link is NOT present when not configured",
"default back link when `album_nav` is not configured"), which still run in every other
variant. What it gives up is "css alone" and "album-nav alone": plain covers
neither-configured and `config-extras` covers both-configured, so only the exactly-one
combinations are untested. Split it back out if that ever matters.

#### Skipping `video.spec.ts` where it is redundant

`video.spec.ts` is the most expensive spec in the suite (~14-17s of a ~34s run). A
transcoded MP4 depends only on its source and the encode settings: encryption changes the
output *filename*, never the bytes, and CSS and customization touch neither. So every
unencrypted variant publishes byte-identical video.

`bin/test-all.sh` therefore passes `--skip-video` (which sets `PLAYWRIGHT_SKIP_VIDEO`) on
`config-extras` and `passwords-uganda`, leaving the no-passwords variant to cover it.
`passwords-all` skips the spec on its own, because a fully locked site publishes no
discoverable video. Unset is the default, so a direct `npx playwright test` still runs
everything.

#### The `@deploy` tag

`bin/rsync-test.sh` used to re-run all 87 tests against the deployed container, ~38s to
prove nothing the other variants had not already proven. It now passes
`--playwright-smoke` to `bin/deploy-photos.sh`, which narrows both Playwright runs to
`--grep @deploy`: the tests that fail when the *deployed tree* is wrong rather than when the
app is, covering what `bin/test-photos-server.sh` cannot, since that only curls for status
codes.

The tag is off by default. A real deploy still runs the whole suite against the live site.

One of the tagged tests is new. `album page renders photos in the grid` renders from
`index.json` alone, so a tile is visible whether its WebP exists, which meant no test
in the suite failed when album media was missing. `album page grid images actually load`
polls `naturalWidth`, and is the one assertion that does. Verified by deleting every
Antarctica grid WebP from a served site: the old test still passed, the new one failed.

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
bin/run-tests.sh --css sample/config/custom-example.css --mode dev

# Run album-nav variant against dev server
bin/run-tests.sh --customization sample/config/customization-album-nav.yaml --mode dev

# Run a single test file against dev server (useful for debugging a specific test)
bin/run-tests.sh --mode dev --test tests/privacy.spec.ts
```

### Sanity Check

A good sanity check runs the unit tests, then verifies against Apache (which requires a
build), testing both password and no-password sites.  It's quicker than running all 4
variants against dev, Apache and nginx:

```bash
make web-sanity-test
```

The `bin/deploy-photos.sh` script runs Playwright automatically: locally before rsync,
and against production after CloudFront cache invalidation.

## Testing Deployment

The two deploy paths can be validated locally without touching a real server:

```bash
# rsync path — rsyncs into a local Docker container; runs server routing tests and Playwright
make sample-rsync-test        # into Apache (photos-apache-ssh)
make sample-rsync-test-nginx  # into nginx  (photos-nginx-ssh)

# S3 path — syncs against MinIO; verifies file placement and Cache-Control headers
# (post-deploy server and Playwright tests are skipped: MinIO serves S3 API only, not HTTP)
make sample-s3-test
```

The two rsync targets exercise a real difference, not just a swapped base image: Apache gets its
routing from the `.htaccess` that rsync transfers, while nginx gets it from the `nginx.conf` baked
into `photos-nginx-ssh`. Both start with an empty document root that rsync fills from scratch, so
they also verify the deploy against real files rather than the symlink tree that
`web/setup-htdocs.sh` builds for the other Docker test images.

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
2. Runs `init` and verifies the `ddphotos` script, the config files, and the host-writable
   `sample-photos/` starter photos are created
3. Runs `photogen` on the installed sample photos and verifies album output, including
   that the relative base `sample-base` resolves to `sample-photos/`
4. Back-compat: runs `photogen` against a config that still uses the pre-`sample-photos`
   container-internal paths (`/ddphotos-init` and friends) and verifies it still works
5. Runs `decode` on an encrypted album index and verifies the output, including files outside `DDPHOTOS_DIR` (via `--passwords` flag and embedded `pwFile` path)
6. Runs `search-cover` against the decoded album and verifies the cover file is found
7. Regression test: runs `decode` and `search-cover` with an external `--config-dir` (outside `DDPHOTOS_DIR`) to verify the config mount path is handled correctly
8. Starts the Vite dev server (`run`) and runs Playwright e2e tests against it
9. Runs `build` and verifies the static site output
10. Starts Apache (`serve`) and runs Playwright e2e tests + `bin/test-photos-server.sh` routing tests
11. Tests `export` (symlink mode), `export --copy` (all files resolved, no symlinks), and `export --cloudflare` (adds `_worker.js`)
12. Verifies `version` and `version --image` output — checks script path and image `Git:`/`Version:` fields
13. Runs `init --script-only` and verifies only the script is installed (no `config/`,
    `albums/` or `sample-photos/`)

Playwright tests skip assertions that depend on sample-site-specific albums (e.g. `antarctica`) when
those albums are not present in the init site, so the full test suite runs cleanly against the
smaller built-in sample. The temp workspace is cleaned up automatically on exit.

## CI (GitHub Actions)

The workflow in [.github/workflows/ci.yml](../.github/workflows/ci.yml) runs on every push or pull 
request to `main`. See that file for all the tests it runs.  Tests are configured to run in parallel
to minimize CI run time.

It also runs nightly at 05:17 UTC. A push or pull request run tests the code; the nightly run
tests everything the repo does *not* pin. Node, npm, `package-lock.json` and the Go toolchain are
pinned exactly, but the ffmpeg `latest` release, the `debian:bookworm-slim` and `golang:1.25-bookworm`
base images, the `ubuntu-latest` runner image, apt packages and Playwright's browser downloads all
move on their own. Those break on a calendar rather than on a commit, so without a scheduled run the
first person to find out is whoever opens the next PR. When a nightly run fails, the
`report-nightly-failure` job opens a GitHub issue labeled `nightly-ci` (or comments on the open one)
via [bin/ci-open-issue.sh](../bin/ci-open-issue.sh), because nobody is watching an overnight run and the
notification email is easy to miss. Push and pull request runs skip that job.

That split is also why two things are deliberately slower on the nightly than on a PR:

- **The ffmpeg and Playwright browser caches are not restored on `schedule` or
  `workflow_dispatch`.** Catching drift in the floating ffmpeg `latest` release and in
  Playwright's browser downloads is a stated reason the nightly exists, and a cache hit
  would hide exactly that. On a push or pull request the download is a pure cost, so it is
  cached there.
- **The Go race detector runs on the nightly only.** `-race` is ~67s of the ~102s that
  `make build test-cover vet` takes on a runner. The workflow passes `RACE=` on push and
  pull request and leaves the Makefile default on the schedule, so a race introduced in a
  PR surfaces that night as a `nightly-ci` issue rather than on the PR itself.

The workflow in [.github/workflows/version-drift.yml](../.github/workflows/version-drift.yml) runs
[bin/check-versions.sh](../bin/check-versions.sh) nightly at 05:37 UTC and opens a `version-drift` issue when the
versions in `web/.nvmrc` or `web/.npm-version` have fallen behind upstream. Pinning those exactly is
deliberate, and the cost of it is that nothing otherwise tells you a bugfix or security release
shipped; Dependabot cannot fill the gap (it has no `.nvmrc` ecosystem, and cannot see a version in
`FROM node:${NODE_VERSION}-bookworm-slim`). It never edits a file: bumping Node is a deliberate
change that CI then tests. Run the same check locally with `make check-versions`.

Both schedules only ever run on the default branch, so a cron edit on a branch does nothing until it
merges, and GitHub disables scheduled workflows on a public repo after 60 days of inactivity. Both
can be triggered by hand from the Actions tab (`workflow_dispatch`).

**Do not expect either at the stated minute.** Scheduled Actions are best-effort, and the delivery
lag is measured in hours rather than minutes: on 2026-08-29 the Version Drift slot arrived 1h34m
late and the CI slot 5h13m late. An occurrence is sometimes dropped outright, and a dropped run
leaves no trace anywhere, so a missing run and a run that has not been delivered yet look identical.
A newly added or edited cron also takes 15 to 60 minutes just to register, and every edit restarts
that clock, which makes bisecting a schedule by pushing repeatedly actively misleading. None of that
matters for a nightly report that only needs to run sometime, but do not reuse the pattern for
anything time-sensitive. Both times are plain UTC rather than a `timezone:` key, which avoids
reasoning about DST.

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
