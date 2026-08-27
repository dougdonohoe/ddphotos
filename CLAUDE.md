# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
See the [docs/](docs/) directory for full developer documentation (architecture, data flow, env vars,
Makefile targets, CLI flags, etc.).

## Directory structure sync requirement

If the `albums/` or `build/` directory structure changes, keep these in sync
(album dirs currently hold `grid/`, `full/`, `video/`, `cover.jpg` and `index.json`):

- `bin/deploy-photos.sh` — rsync and S3 logic
- `web/setup-htdocs.sh` — sets up the Apache htdocs directory
- `bin/gen-deploy-tree.py` — generates the directory tree image used in docs
- `## Syncing Logic` section in `docs/DEPLOY.md`

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

## Node/npm version sync requirement

`web/.nvmrc` and `web/.npm-version` hold **exact** versions and are the single sources of truth.
Everything else reads them: the Makefile, `bin/docker-push.sh`, `bin/node-init.sh`, and the three
`setup-node` steps in `.github/workflows/ci.yml`. **Do not hardcode either version anywhere else.**

Both must stay exact rather than major-only. `docker/Dockerfile` interpolates `NODE_VERSION` into
`FROM node:${NODE_VERSION}-bookworm-slim`, and a major-only tag like `node:24` is a moving pointer:
the same commit then builds different images depending on when you built, and an upstream
regression lands with no commit to bisect. That happened once already — Node 24.2.0 broke recursive
`fs.cpSync` onto Docker bind mounts and reached the image through the floating tag (see the comment
on `copyDirRecursive` in `web/vite.config.ts`). **Bumping Node is a deliberate edit to `web/.nvmrc`
that CI then tests.**

`bin/node-init.sh` is the shell-side counterpart to the Makefile's `NODE_INIT`: the bash scripts
that run npm/npx (`bin/run-tests.sh`, `bin/docker-test.sh`, `bin/deploy-photos.sh`) source it
rather than each doing their own nvm setup. Both it and `NODE_INIT` use the node on PATH only
when its **exact version matches** `web/.nvmrc` — testing for mere presence lets a distro node at
the wrong version (Ubuntu's apt `nodejs`) shadow the repo's. Both compare against the full `node
-v` output, so neither may go back to matching on the major alone. **Keep the two in sync.**

`docker/Dockerfile` takes both as **required** build args (`NODE_VERSION`, `NPM_VERSION`) with no
defaults, so it cannot carry a stale copy of either version. It is built only via `make
docker-build` and `bin/docker-push.sh`, which read the two files and pass the values in; a bare
`docker build -f docker/Dockerfile .` fails by design. **Do not give those ARGs default values.**

One exception, which must be updated by hand: the `engines.node` field in `web/package.json`.
Paired with `engine-strict=true` in `web/.npmrc`, it makes `npm install`/`npm ci` hard-fail on the
wrong Node, so a machine whose `nvm` default has drifted cannot silently install with it. It is
deliberately a major range (`24.x`), not the exact version, so routine patch bumps to `web/.nvmrc`
do not need a second edit. **When bumping `web/.nvmrc` to a new major, bump `engines.node` too.**

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
