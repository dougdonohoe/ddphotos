# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
See the [docs/](docs) directory for full developer documentation (architecture, data flow, env vars,
Makefile targets, CLI flags, etc.).

## Directory structure sync requirement

If the `albums/` or `build/` directory structure changes, keep these in sync
(album dirs currently hold `grid/`, `full/`, `video/`, `cover.jpg` and `index.json`):

- `bin/deploy-photos.sh` — rsync and S3 logic
- `web/setup-htdocs.sh` — sets up the Apache htdocs directory
- `bin/gen-deploy-tree.py` — generates the directory tree image used in docs
- `## Syncing Logic` section in `docs/DEPLOY.md`

There is a third root-level directory, `sync/` (`DDPHOTOS_SYNC_DIR`), holding albums
photogen downloads from an upstream photo manager:
`sync/{site-id}/{provider}/{slug}/`, each with `metadata.yaml`, `photogen.txt` and the
media. **It is a source, not output.** Nothing under it is deployed, so the four files
above need no change when it moves — but `pkg/photogen/sync.go` (`SyncAlbumPath`),
`config/defaults.env`, `docker/Dockerfile`'s `ENV` block and the layout in
`docs/CONFIGURATION.md` all name the shape and do.

## Type sync requirement

The Go structs in `pkg/photogen/json.go` (`AlbumIndex`, `AlbumSummary`, `PhotoIndex`, `PhotoSrcIndex`)
define the JSON schema consumed by the frontend. Their TypeScript counterparts live in
`web/src/lib/types.ts`. **When changing a JSON field in either place, update the other.**

## Media type sync requirement

`allowedPhotoExtensions` and `allowedVideoExtensions` (`pkg/photogen/album.go`,
`pkg/photogen/video.go`) are consumed through `IsPhotoFile` / `IsVideoFile` / `IsMediaFile`
in three semantically different places, and they are **not** interchangeable:

- the source scan and `photogen.txt` caption keys accept **both** (`IsMediaFile`)
- hero validation accepts **photos only** (`albums_config.go`) — a hero is a libvips crop
- serving needs a MIME entry per extension in **both** `web/vite.config.ts` (dev, which
  also needs HTTP Range for video) and `web/src/hooks.server.ts` (prerender), plus a cache
  rule in `web/static/.htaccess` and `web/nginx.conf`

`~/work/ddphotos-app` has its own `IMAGE_EXTENSION` regex in `PathValidation.java` that
gates its photo chooser and caption editor. It does not yet know about video, so a `.mov`
is invisible there even though photogen publishes it.

## URL routing sync requirement

The same routing rules are implemented once per hosting target, and a change to one is almost
always a change to all four:

| File                            | Target                                                            |
|---------------------------------|-------------------------------------------------------------------|
| `web/static/.htaccess`          | Apache (rsynced with every deploy)                                |
| `web/nginx.conf`                | nginx (baked into the image; never deployed by the scripts)       |
| `docker/cloudflare-worker.js`   | Cloudflare Pages (`_worker.js`, shipped by `export --cloudflare`) |
| `docker/cloudfront-function.js` | S3 + CloudFront (viewer-request stage)                            |

The rules: extensionless path to pre-rendered `.html`, `/albums/slug/N` photo permalinks to
`/albums/slug.html`, trailing-slash redirects, and `404.html` for unknown paths. **Adding or
changing a route means editing all four files, then teaching `bin/test-photos-server.sh` about
it** — it has `--cloudflare`, `--s3` and `--surge` modes for the platforms that differ.

`docker/cloudfront-function.js` is written for CloudFront's runtime, which has no module system;
`bin/s3-edge-proxy.js` loads it through `vm` so the file stays deployable verbatim. **Do not add
`module.exports` or `import`/`export` to it, and keep its redirect `Location` relative** — the S3
test serves over `http://localhost`, where a hardcoded `https://` breaks.

See `docs/DEPLOYMENT-SERVERS.md` for what each target's config does.

## Node/npm version sync requirement

`web/.nvmrc` and `web/.npm-version` hold **exact** versions and are the single sources of truth.
The Makefile, `bin/docker-push.sh`, `bin/node-init.sh` and the three `setup-node` steps in
`.github/workflows/ci.yml` all read them. **Do not hardcode either version anywhere else, and do
not relax the pin to a major-only tag** — a floating `node:24` once shipped a regression with no
commit to bisect (see `copyDirRecursive` in `web/vite.config.ts`). Bumping Node is a deliberate
edit to `web/.nvmrc` that CI then tests.

Four rules follow from that:

- `docker/Dockerfile` takes `NODE_VERSION` and `NPM_VERSION` (and `GO_VERSION`, the major.minor
  of go.mod's `go` directive) as **required** build args.
  **Never give them defaults** — a bare `docker build -f docker/Dockerfile .` fails by design.
- `bin/node-init.sh` is the shell-side counterpart to the Makefile's `NODE_INIT`. **Keep the two
  in sync, and keep both comparing the full `node -v` output** — matching on the major alone lets
  a distro node at the wrong version shadow the repo's.
- `web/package.json`'s `engines.node` is a major range, updated by hand. **Bump it when
  `web/.nvmrc` crosses into a new major.**
- `bin/check-versions.sh` reads the base image variant off the Dockerfile's `FROM` line, so **the
  Dockerfile stays the only place naming the base image.**

Why the pins are exact, how `engine-strict` enforces them, and how the nightly drift check covers
what Dependabot cannot: `docs/INSTALL.md` and `docs/TESTING.md`.

## Commands

```bash
make build test vet              # Go build, unit tests, static analysis
make sample-build                # build static site with sample data
make web-unit-test               # Vitest unit tests for TypeScript helpers in web/src/lib
make web-sanity-test             # Playwright e2e tests: Apache, no-passwords + all-passwords (quick comprehensive web check)
make web-playwright-test-apache  # Playwright e2e tests, Apache, no-passwords only
make docker-test                 # Test 'ddphotos' docker commands
```

System dependency required: `brew install vips pkg-config`

## Testing Practices

- **Pure TypeScript helpers** (`web/src/lib/*.ts`): unit-test them with Vitest as `src/lib/<name>.test.ts`
  rather than reaching for a browser; `make web-unit-test` runs them, and `web-sanity-test` runs them first
- **Reproducing frontend bugs**: write a failing Playwright test that demonstrates the bug before fixing it
- **New UI features**: add a Playwright test covering the new behavior — tests live in `web/tests/`
- After any UI changes, run `make web-sanity-test` (Apache, no-passwords + all-passwords) as the standard web check
- Full coverage: `make web-playwright-test-all` (all password/CSS variants, dev + apache + nginx)
