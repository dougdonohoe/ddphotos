# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
See [README-DEV.md](README-DEV.md) for full developer documentation.

## Architecture

This is a two-component project:

1. **`photogen`** — A Go CLI (`cmd/photogen/photogen.go`) that processes source JPEGs: resizes them
   to WebP (grid 600px, full 1600px), strips EXIF metadata, and writes JSON index files (`albums.json`,
   per-album `index.json`, `sitemap.xml`). Core logic lives in `pkg/photogen/`.

2. **SvelteKit frontend** (`web/`) — A static site (`adapter-static`) that reads the JSON indexes
   and photo files from `web/static/albums/` (symlinked via `make use-sample` or `make use-prod`).
   Uses `justified-layout` for the photo grid and PhotoSwipe for the lightbox.

### Data flow

```
Source JPEGs → photogen → WebP files + JSON → web/static/albums/ (symlink) → npm run build → web/build/
```

### Config separation

Personal config (album definitions, descriptions, site values) lives in a separate private repo
pointed to via `--config-dir`. The committed `config/` directory only contains example files.
The `sample/` directory contains working sample data for out-of-the-box testing.

### Type sync requirement

The Go structs in `pkg/photogen/json.go` (`AlbumIndex`, `AlbumSummary`, `PhotoIndex`, `PhotoSrcIndex`)
define the JSON schema consumed by the frontend. Their TypeScript counterparts live in
`web/src/lib/types.ts`. **When changing a JSON field in either place, update the other.**

### Key files

- `pkg/photogen/` — album loading, EXIF extraction (govips + goexif), WebP resize (concurrent workers), JSON writing
- `web/src/routes/albums/[slug]/[[index]]/+page.svelte` — album page with justified grid, PhotoSwipe lightbox, URL-based photo permalinks, back-nav history management
- `web/src/routes/+page.svelte` / `+page.ts` — home page reading `albums.json`
- `web/static/.htaccess` — Apache URL routing (trailing slash redirect, HTML extension rewriting, photo permalink rewrite, SPA fallback)

## Commands

```bash
make build test vet          # Go build, unit tests, static analysis
make sample-build            # build static site with sample data
make web-sanity-test         # Playwright e2e tests: Apache, no-passwords + all-passwords (quick comprehensive web check)
make web-playwright-test-apache  # Playwright e2e tests, Apache, no-passwords only
```

System dependency required: `brew install vips pkg-config`

## Testing Practices

- **Reproducing frontend bugs**: write a failing Playwright test that demonstrates the bug before fixing it
- **New UI features**: add a Playwright test covering the new behavior — tests live in `web/tests/`
- After any UI changes, run `make web-sanity-test` (Apache, no-passwords + all-passwords) as the standard web check
- Full coverage: `make web-playwright-test-all` (all password/CSS variants, dev + apache + nginx)
