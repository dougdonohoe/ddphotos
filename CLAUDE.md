# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
See the [docs/](docs/) directory for full developer documentation (architecture, data flow, env vars,
Makefile targets, CLI flags, etc.).

## Directory structure sync requirement

If the `albums/` or `build/` directory structure changes, keep these in sync:

- `bin/deploy-photos.sh` — rsync and S3 logic
- `web/setup-htdocs.sh` — sets up the Apache htdocs directory
- `bin/gen-deploy-tree.py` — generates the directory tree image used in docs
- `## Syncing Logic` section in `docs/DEPLOY.md`

## Type sync requirement

The Go structs in `pkg/photogen/json.go` (`AlbumIndex`, `AlbumSummary`, `PhotoIndex`, `PhotoSrcIndex`)
define the JSON schema consumed by the frontend. Their TypeScript counterparts live in
`web/src/lib/types.ts`. **When changing a JSON field in either place, update the other.**

## Node/npm version sync requirement

`web/.nvmrc` (Node major) and `web/.npm-version` (exact npm version) are the single sources of
truth. Everything else reads them: the Makefile, `bin/docker-push.sh`, `bin/node-init.sh`, and the
three `setup-node` steps in `.github/workflows/ci.yml`. **Do not hardcode either version anywhere
else.**

`bin/node-init.sh` is the shell-side counterpart to the Makefile's `NODE_INIT`: the bash scripts
that run npm/npx (`bin/run-tests.sh`, `bin/docker-test.sh`, `bin/deploy-photos.sh`) source it
rather than each doing their own nvm setup. Both it and `NODE_INIT` use the node on PATH only
when its **major version matches** `web/.nvmrc` — testing for mere presence lets a distro node at
the wrong major (Ubuntu's apt `nodejs`) shadow the repo's. **Keep the two in sync.**

`docker/Dockerfile` takes both as **required** build args (`NODE_VERSION`, `NPM_VERSION`) with no
defaults, so it cannot carry a stale copy of either version. It is built only via `make
docker-build` and `bin/docker-push.sh`, which read the two files and pass the values in; a bare
`docker build -f docker/Dockerfile .` fails by design. **Do not give those ARGs default values.**

One exception, which must be updated by hand: the `engines.node` field in `web/package.json`.
Paired with `engine-strict=true` in `web/.npmrc`, it makes `npm install`/`npm ci` hard-fail on the
wrong Node, so a machine whose `nvm` default has drifted cannot silently install with it.
**When bumping `web/.nvmrc`, bump `engines.node` to match.**

## Commands

```bash
make build test vet              # Go build, unit tests, static analysis
make sample-build                # build static site with sample data
make web-sanity-test             # Playwright e2e tests: Apache, no-passwords + all-passwords (quick comprehensive web check)
make web-playwright-test-apache  # Playwright e2e tests, Apache, no-passwords only
make docker-test                 # Test 'ddphotos' docker commands
```

System dependency required: `brew install vips pkg-config`

## Testing Practices

- **Reproducing frontend bugs**: write a failing Playwright test that demonstrates the bug before fixing it
- **New UI features**: add a Playwright test covering the new behavior — tests live in `web/tests/`
- After any UI changes, run `make web-sanity-test` (Apache, no-passwords + all-passwords) as the standard web check
- Full coverage: `make web-playwright-test-all` (all password/CSS variants, dev + apache + nginx)
