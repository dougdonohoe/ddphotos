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
