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

### Key files

- `pkg/photogen/` — album loading, EXIF extraction (govips + goexif), WebP resize (concurrent workers), JSON writing
- `web/src/routes/albums/[slug]/[[index]]/+page.svelte` — album page with justified grid, PhotoSwipe lightbox, URL-based photo permalinks, back-nav history management
- `web/src/routes/+page.svelte` / `+page.ts` — home page reading `albums.json`
- `web/static/.htaccess` — Apache URL routing (trailing slash redirect, HTML extension rewriting, photo permalink rewrite, SPA fallback)

## Commands

```bash
make build test vet          # Go build, unit tests, static analysis
make sample-build            # build static site with sample data
make web-playwright-test-apache  # Playwright e2e tests (Docker)
```

System dependency required: `brew install vips pkg-config`
