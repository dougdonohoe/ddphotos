# Photo Album Project - Session Summary

For current project documentation, see [`README.md`](../README.md).

This file is a historical log of work done with ChatGPT and Claude Code.

## Original Prompt

This project started with ChatGPT and this prompt (reproduced as written, typos and all):

_I want to build a web-based app for browsing photos, similar to Apple Photos
shared albums, but faster (it takes 15-20 seconds to load albums with 100+ photos).
I have photos in folders, so am thinking I'd have a Go-based program that takes as
input one or more folders and outputs static HTML + JavaScript.   Each folder
corresponds to a trip.  E.g., Galapagos or "Camino de Santiago"._

_The script should generate a menu page which lists each album and a preview picture
from each album (something that eventually should be configurable).  When clicking
into an album, you first see all the photos in a nice grid like Apple Photos
does - mixing horizontal vs vertical orientation._

_When clicking on a photo, it opens up a full screen view.  Arrowing left/right or
up/down on a keyboard moves through the photos in the album. On mobile, swipe
left/right works to move as well as tapping on left/right side of photo.  Photos
use up as much room as possible and respect orientation of a mobile device._

_Those are the basic requirements (I'll add more details later).  My first question
is which javascript framework should I use?  I think this is a single-page-app
(but with URL munging to allow bookmarking).  I don't think I need a server API - just
pre-generating smaller versions of images for the all-photos view.  Loading of
images should probably be done on-demand (e.g., when I scroll down, it fetches
the next images that will be in view ... this should improve perceived performance)._

_I don't want code generation from you yet, let's talk about architecture first.
I'm an expert in Go, am familiar with CSS and HTML and basic JavaScript, but don't
know the must current web frameworks, especially those that are mobile friendly._

## ChatGPT Spec

The [ChatGPT chat](https://chatgpt.com/share/69aedbec-375c-8010-b1da-2b39d78f6e6b)
resulted in the [PHOTOS.md](PHOTOS.md) spec.  An intial attempt at generating
code using OpenAI Codex was abandoned and the rest of the work, documented below,
was done with Claude Code.

## Claude Code Sessions

### 1. 03/08/2026 - WebP Image Format
- Changed from JPEG to WebP output for smaller file sizes
- Updated `resize.go` to use `ExportWebp()` instead of `ExportJpeg()`
- Added `WebPFileName()` helper to convert extensions
- Updated JSON paths to use `.webp`

### 2. 03/08/2026 - Removed Unused Thumbnail Size
- Removed `SizeThumb` (200px) - was never used
- Now only generates two sizes: `grid` (600px) and `full` (1600px)

### 3. 03/08/2026 - Static Site Deployment Setup
- Installed `@sveltejs/adapter-static`
- Updated `svelte.config.js` for static export
- Created `src/routes/+layout.ts` with `prerender = true`
- Build outputs to `photos/build/`

### 4. 03/08/2026 - Production Readiness Fixes
- **Error handling**: Added try/catch to JSON fetch calls in `+page.ts` files
- **Error page**: Created `src/routes/+error.svelte` with styled 404/error display
- **Meta description**: Added to `app.html`
- **Image fallback**: Added `background: var(--bg-secondary)` to images for loading/error states

### 5. 03/08/2026 - Sitemap Generation
- Added `WriteSitemap()` function to `json.go`
- Added `-site-url` flag (defaults to `https://photos.example.com`)
- Generates `sitemap.xml` alongside `albums.json` when `-index` flag is used

### 6. 03/08/2026 - Mobile Title Size
- Added media query to reduce h1 font-size to 1.8rem on screens ≤480px

### 7. 03/08/2026 - Copyright Footer
- Added footer to `+layout.svelte` with dynamic year
- Text: "Copyright © 2001-{currentYear}. Doug and Cindy Donohoe."

### 8. 03/08/2026 - URL Handling
- Created redirect from `/albums` and `/albums/` to `/`
- Created `static/404.html` for Apache custom error page

### 9. 03/08/2026 - Concurrent Image Resizing
- Added `pkg/photogen/resize_worker.go` implementing `pool.Worker[resizeWork]` for concurrent resizing
- `ResizePhotos()` now uses the worker pool instead of sequential processing
- Auto-detects worker count: `NumCPU() / 2` (min 2), configurable via `-workers` flag
- Added `Workers` field to `Config`
- *(Superseded by §30 — pool replaced with simpler goroutine implementation)*

### 10. 03/08/2026 - CLI Improvements
- Added `-album` flag to filter processing to specific albums (comma-separated slugs)
- Added `-workers` flag to control concurrent resize workers
- Added EXIF date warning: logs count of photos missing dates per album

### 11. 03/08/2026 - Higher Resolution Favicon
- Generated multi-size favicon from 512x512 source PNG
- `favicon.ico` (16/32/48px), `favicon-32.png`, `favicon-192.png` (Android), `apple-touch-icon.png` (iOS)
- Updated `app.html` with proper icon link tags

### 12. 03/08/2026 - Apache URL Routing
- Added `.htaccess` with `DirectorySlash Off`, trailing-slash redirect (301), and `.html` file rewrite
- Set `trailingSlash: 'ignore'` in `+layout.ts` to let Apache handle URL normalization
- `/albums` serves redirect page, unknown paths fall back to SPA shell

### 13. 03/08/2026 - Gallery UX Improvements
- Increased `max-width` from 1200px/1400px to 2000px on album list and gallery pages
- ESC key on album page navigates back to album list (with guard to not conflict with PhotoSwipe ESC)
- Back-to-top button arrow centering: responsive `padding-bottom` (2px mobile, 5px desktop)

### 14. 03/08/2026 - Deploy Script Enhancements
- Added `--no-photogen` flag to skip image generation
- Added `--checksum` to rsync to skip unchanged files (Vite resets timestamps)
- Added post-deploy Apache test (`bin/test-photos-apache.sh`)

### 15. 03/08/2026 - Apache Verification Script
- Created `bin/test-photos-apache.sh` to validate URL routing, redirects, 404s, and static assets
- Accounts for CloudFront SSL termination (redirect locations use `http://`)
- Runs automatically after deploy

### 16. 03/08/2026 - WebP Metadata
- Added comment documenting that all metadata is stripped from WebP output (smaller files, no GPS leak)
- Photo metadata preserved in JSON index files

### 17. 03/08/2026 - Photo Descriptions via `photogen.txt`
- Added `photogen.txt` support: optional file in the album source directory, one line per photo: `filename_without_extension Description text`
- Blank lines and `#` comments ignored
- Added `Description string` field to `Photo` and `PhotoIndex` (JSON: `omitempty`)
- Added `ManualSortOrder bool` to `AlbumConfig`: when true, photo order follows `photogen.txt` sequence
- Unmentioned photos warned about, sorted by date, and appended at the end
- Unknown IDs in `photogen.txt` produce a warning and are skipped
- `Photo.String()` logging now appends description if present
- Added `TestLoadPhotoDescriptions` and `TestReorderByDescriptionFile` in `album_test.go`
- Updated `testdata/index.json` to include a description on one photo; added assertions in `json_test.go`

### 18. 03/08/2026 - Photo Matching Tool (`cmd/photomatch`)
- Created `cmd/photomatch/photomatch.go` to match old website photos to new curated exports
- Uses perceptual hashing (`github.com/corona10/goimagehash`, pHash) with configurable Hamming distance threshold
- **Direct matching**: compares old site thumbnails (`pictures/picture-N.jpg`) against new album JPGs
- **Bridge matching** via `-originals` flag: for unmatched old photos, finds the matching original (e.g., `IMG_1777.JPG`), then checks if that original exists in the new set
- Extracts captions from old site HTML (`large-N.html`) via regex on `<li>Caption:...</li>`
- Generates `photogen.txt` with matched captions sorted by new photo filename
- Hash cache at `/tmp/photomatch-cache.json` (keyed on path + mod time + size) for fast re-runs
- Generates `/tmp/show-missing.sh` script using iTerm's `imgcat` to visually review unmatched photos, showing the original (full-res) image and the original filename for Lightroom lookup
- Dry-run by default, `-doit` to write `photogen.txt`
- Used to generate `photogen.txt` for Nepal and Peru albums

### 19. 03/08/2026 - Caption Display in Frontend
- Descriptions used as `alt` text on grid images and in PhotoSwipe lightbox (fallback to filename)
- **Grid hover caption** (desktop): `position: absolute` overlay at photo bottom with gradient, fades in on `:hover`
- **Grid always-on caption** (mobile): `@media (hover: none)` overrides opacity to always show
- **Lightbox caption**: injected into each of PhotoSwipe's 3 `itemHolders` elements (`pswp.mainScroll.itemHolders`) so captions swipe physically with their photo
  - Position computed mathematically: `scale = min(viewW/w, viewH/h)`, `bottom = (viewH - h*scale) / 2`
  - Caption elements queried via `holder.el.querySelector('.pswp-caption')` at update time — a parallel index array breaks after the first swipe because PhotoSwipe rotates the `itemHolders` array in place
  - `change` handler defers via `requestAnimationFrame` so PhotoSwipe finishes assigning slide holders before captions are updated
  - `slide.index` used to look up data from local `photoswipeItems` array; `slide.data` may not be populated when `change` fires

### 20. 03/08/2026 - Open Graph Tags & Social Sharing
- Added Open Graph meta tags to home page and album pages: `og:title`, `og:description`, `og:type`, `og:url`, `og:image`, `og:site_name`, `twitter:card`
- Home page uses first album's cover image; album pages use the first photo's grid thumbnail
- Description format: `"N photos from the 'Album Name' Donohoe photo album"`

### 21. 03/08/2026 - Album Date Span on Album Page
- Album page header now shows date span (e.g., "47 photos · Jan – Mar 2005")
- `albums/[slug]/+page.ts` fetches `albums.json` in parallel with `index.json` and plucks `dateSpan` for the matching slug — no changes to JSON generation needed

### 22. 03/08/2026 - Photo Permalinks (`/albums/slug/N`)
- Replaced hash-based approach (`#photo-15`) with clean path-based URLs (`/albums/antarctica/15`)
- Route restructured from `albums/[slug]/+page.svelte` to `albums/[slug]/[[index]]/+page.svelte` (SvelteKit optional param)
- `+page.ts` extracts `params.index`, converts to 0-based `photoIndex`, passes to component
- `openLightbox()` sets URL via `history.replaceState` on path (not hash) — no `hashchange` events fire, eliminating the infinite-loop risk that plagued the hash approach
- On close, URL is restored to `/albums/slug`
- On mount, if `data.photoIndex` is set, lightbox opens immediately (no animation — `showAnimationDuration: 0` — for instant feel on direct load)
- `.htaccess` updated with new rewrite rule: `albums/slug/N` → `albums/slug.html`
- `paths.relative: false` added to `svelte.config.js` so SvelteKit generates absolute asset paths (`/_app/...` instead of `../_app/...`) — required because album HTML is now served at a deeper path depth

### 23. 03/08/2026 - Docker Apache Test Environment
- Added `photos/Dockerfile` based on `httpd:2.4`; `sed` enables `mod_rewrite`, sets `AllowOverride All`, points `DocumentRoot` at `htdocs/build/`, adds `ServerName localhost`
- Volume mounts `photos/` (not `photos/build/`) so npm rebuilds — which delete and recreate `build/` — don't break the container's bind mount inode
- New Makefile targets: `photos-docker-build`, `photos-docker-run`, `photos-docker-stop`, `photos-docker-test`
- `bin/test-photos-apache.sh` updated with `--local [port]` flag: switches `BASE`/`REDIRECT_BASE` to `http://localhost:PORT`, uses `antarctica` as test album
- Added `check_body` helper to test script; new test verifies photo permalink HTML contains absolute `/_app/immutable` asset paths (would have caught the `paths.relative` bug)
- Added photo permalink routing tests (200 for `/albums/antarctica/1` and `/10`, 301 for trailing slash)

### 24. 03/08/2026 - Copy-Link Button in Lightbox
- Injected a chain-link SVG button into PhotoSwipe's `.pswp__top-bar` DOM after `pswp.init()`, just left of the close button — avoids `uiRegister`/`registerElement` which caused scroll-lock bugs in earlier attempts
- Copies `window.location.href` (always current due to `replaceState` calls) to clipboard via `navigator.clipboard.writeText()`
- On success, icon swaps to a green checkmark for 1.5 seconds then reverts — the standard GitHub/Notion feedback pattern
- Silent no-op if clipboard API is unavailable (old browser or denied permission)

### 25. 03/08/2026 - Album Descriptions
- Added `Description string` to `AlbumConfig` (Go) and `AlbumSummary` JSON (`omitempty`)
- `GetAlbumSummary()` propagates description into the JSON output
- Single source of truth: `albumDescriptions map[string]string` in `photogen.go`, keyed by slug; `applyDescriptions()` populates both `defaultAlbums()` and `defaultAlbumsLaptop()` after config is built — eliminates duplication between the two config functions
- All 21 production albums have real descriptions; `tbd` constant used as placeholder for any new albums added in the future
- `og:description` on album pages now uses the album description when available, falling back to the generated `"N photos from the 'Album Name'..."` string
- Frontend (`+page.ts`) passes `description` through alongside `dateSpan`
- **Home page cards**: description on its own line below the title; styled slightly brighter (`--text-color-2nd`, `opacity: 0.8`) and larger (`0.95rem`) than the muted meta line to create visual hierarchy; meta line (`N photos · dateSpan`) pinned to card bottom via `margin-top: auto` in a flex column layout, right-aligned and italic
- **Album detail page**: single meta line formatted as `Description\u00A0|\u00A0N photos · dateSpan`
- Updated `testdata/albums.json` with a description on the `way` entry; added assertions in `TestLoadAlbumSummaries` for both present and absent descriptions

### 26. 03/08/2026 - Canonical URL Tags
- Added `<link rel="canonical">` to `<svelte:head>` on both the home page and album pages
- Album page canonical always points to `/albums/slug` (not `/albums/slug/N`) so photo permalinks don't fragment search engine ranking signals

### 27. 03/08/2026 - SvelteKit Client-Side Navigation & Lightbox Caption Fixes

**Cross-album navigation bug (stale photos after browser back):**
- Root cause: `imageSrcs`/`imageLoaded` were initialized in `onMount`, which only runs once. SvelteKit reuses the same component instance when navigating between albums (same route pattern `[slug]/[[index]]`), so `data` updated but `imageSrcs` kept the previous album's image paths — title showed new album, photos showed old album.
- Fix: moved initialization into a `$effect` that re-runs whenever `data.album.photos` or `data.slug` changes. Pending slow-mode `setTimeout` handles are canceled in the effect's cleanup function.
- Pitfall: `imageSrcs[i] = src` inside `$effect` reads `imageSrcs` (to get the proxy), which Svelte tracks as a dependency → infinite update loop. Fix: build the full array in one assignment (`imageSrcs = photos.map(...)`) so no read of `imageSrcs` occurs inside the effect.
- `slowMode` moved from `onMount` to inline `$state(browser && ...)` so it's set before the effect's first run.

**`history.replaceState` conflict with SvelteKit router:**
- Using `history.replaceState` directly caused SvelteKit to intercept it as a navigation, re-run `load()`, return a new `data` object with a new `data.album.photos` array reference, and re-trigger the `$effect` mid-lightbox-open — resetting image state and disrupting caption timing.
- Fix: switched all three `history.replaceState` calls to `replaceState` from `$app/navigation`, which updates the URL shallowly without re-running `load()`.
- The initial `replaceState` after `pswp.init()` is also guarded with `if (animate)` — for URL-based opens (`animate=false`), the URL is already correct and the router may not be initialized yet during hydration.

**Lightbox caption not showing on first open:**
- Two separate `openingAnimationEnd` handlers exist in `openLightbox()`: the fullscreen handler is registered *before* `pswp.init()`, but the caption `updateAll` handler is registered *after* `pswp.init()`.
- When `animate=false` (`showAnimationDuration: 0`), PhotoSwipe fires `openingAnimationEnd` synchronously *inside* `pswp.init()` — before the caption handler is ever registered, so it never fires.
- For `animate=true`, `holder.slide` may not be assigned when `openingAnimationEnd` fires (same timing issue as `change`, which already used `requestAnimationFrame`).
- Fix: added `requestAnimationFrame(updateAll)` unconditionally at the end of the caption setup block. For `animate=false` this is the only trigger; for `animate=true` it runs alongside `openingAnimationEnd` (redundant but harmless).

### 28. 03/08/2026 - Playwright E2E Tests
- Added `@playwright/test` as a dev dependency in `photos/package.json`
- `photos/playwright.config.ts`: `baseURL` defaults to `http://localhost:8080`, overridable via `PLAYWRIGHT_BASE_URL` env var — no `webServer` config since tests run against the existing Docker Apache container
- New Makefile targets: `photos-playwright-install` (one-time `npm install && npx playwright install chromium`), `photos-playwright-test` (starts Docker on port **8081** to avoid conflict with deploy script's port 8080, runs tests, stops Docker, preserves exit code).
- `bin/deploy-photos.sh` runs Playwright tests at two points: locally against the Docker container before rsync, and against production (`PLAYWRIGHT_BASE_URL=https://photos.example.com`) after CloudFront cache invalidation.
- All test albums reference the dev/prod overlap set (`antarctica`, `honeymoon`, `uganda`) so tests run correctly in both environments.
- **`tests/captions.spec.ts`** (3 tests): grid click open (`animate=true`), permalink direct load (`animate=false`), prev/next navigation. Uses `locator('.pswp-caption', { hasText })` to avoid Playwright strict-mode errors — PhotoSwipe keeps 3 caption elements in the DOM simultaneously, one per holder.
- **`tests/url.spec.ts`** (4 tests): opening a photo updates URL to `/albums/slug/N`; navigating prev/next advances the URL; closing lightbox restores URL to `/albums/slug`; direct permalink URL is preserved on load.
- **`tests/navigation.spec.ts`** (3 tests): cross-album navigation (click back → click new album card) shows correct title/description; lightbox URL reflects the new album; chaining through three albums maintains correct state. Directly exercises the `$effect` stale-imageSrcs bug.
- **`tests/smoke.spec.ts`** (5 tests): home page lists known overlap albums; album cards show descriptions; album page renders title, description, and photo count; grid photos are visible; Open Graph tags are correct.

### 29. 03/08/2026 - YAML-Based Album Configuration

First phase of open-sourcing: replaced all hardcoded album data in Go source with a runtime-loaded YAML config file.

**New `pkg/photogen/albums_config.go`:**
- `AlbumsFile` struct: `settings` block (site_url, output_dir, descriptions filename), `bases` map (named source paths), `albums` list
- `LoadAlbumsFile(path)` — YAML parse + structural validation (required fields, base references); does not check disk paths
- `AlbumsFile.ToAlbumConfigs(configDir)` — resolves paths (fail-fast if source doesn't exist), loads descriptions, returns `[]*AlbumConfig`
- `LoadAlbumDescriptions(path)` — reads `slug<whitespace>description` file into `map[string]string`
- `LoadAlbumConfigs(configDir, filename)` — top-level convenience helper returning `([]*AlbumConfig, *AlbumsSettings, error)`
- 14 tests covering parse errors, missing fields, unknown base references, path resolution, and end-to-end loading

**`cmd/photogen/photogen.go`** — removed `defaultAlbums()`, `defaultAlbumsLaptop()`, `albumDescriptions`, `-laptop` flag; added `--config-dir` (default `config`) and `--albums` (default `albums.yaml`); CLI flags `--site-url` and `--out` override YAML settings when provided; added `exit.HandleSignal()` + `exit.ExitRequested()` loop check for clean CTRL-C handling.

**Config files** (in `dd-tbd/config/`, personal — not open-sourced):
- `albums.yaml` — 21-album production config
- `albums-laptop.yaml` — 6-album laptop subset (different base paths, same descriptions file)
- `descriptions.txt` — all album descriptions, moved from Go source

**`config/albums.example.yaml`** — documented example for open-source users.

**Key gotcha:** `OutputRoot` should be `photos/static` (not `photos/static/albums`) — the code appends `albums/<slug>/` internally in `OutputPath()` and `WriteAlbumsIndex()`.

### 30. 03/08/2026 - Simplified Concurrent Resizing & Worker Count Encapsulation

Replaced the generic `pkg/pool` worker pool with a simpler, purpose-built implementation directly in `resize_worker.go`. The pool was designed for dynamic work generation; since all resize work items are known upfront, a buffered channel suffices.

**New `ResizePhotos()` pattern:**
- Build all `resizeWork` items upfront, push into a buffered channel, close it
- Spin up `N` goroutines that drain the channel via `range`; each checks `exit.ExitRequested()` for clean CTRL-C handling
- `sync.WaitGroup` + `sync.Once` for completion and first-error capture
- ~40 lines vs ~370 lines across the old `pool.go` + `resize_worker.go`

**`Config.Workers()` method** (`pkg/photogen/config.go`):
- Encapsulates the "NumWorkers > 0 → use as-is, else NumCPU/2 min 2" logic
- Renamed field `Workers` → `NumWorkers` to avoid collision with method name
- `runtime` import moved from `resize_worker.go` to `config.go`
- 3 tests added to `config_test.go`

**Dependency removed:** `pkg/pool` no longer imported by `pkg/photogen`. Only `pkg/exit` remains as an internal dependency.

### 31. 03/08/2026 - Rename `photos/` → `web/`

Renamed the SvelteKit app directory from `photos/` to `web/` in preparation for extracting the project into a standalone open-source repo (where a `photos/` subdirectory inside a repo named `photos` would be confusing).

**Files changed:**
- `git mv photos web` — all git-tracked files moved atomically
- `Makefile` — all `photos-*` targets renamed to `web-*`; `npm-run-dev` renamed to `web-npm-run-dev`; added `web-install` target for `npm install`; all `cd photos` / `photos/` path references updated to `web/`
- `bin/deploy-photos.sh` — `cd photos` → `cd web`
- `cmd/mcp_photos/mcp_photos.go` — default `-data` flag updated
- `cmd/mcp_photos/README.md` — flag table and `.mcp.json` example updated
- `config/albums.yaml`, `config/albums-laptop.yaml`, `config/albums.example.yaml` — `output_dir: photos/static` → `web/static`
- `pkg/photogen/testdata/albums.yaml` — same `output_dir` fix
- `pkg/photogen/albums_config_test.go` — assertion string updated
- `README.md`, `CLAUDE.md`, `web/README.md`, `docs/photogen-plan.md`, `docs/open-source-plan.md` — all path/target references updated

**Key gotcha:** `node_modules` is not tracked by git, so after cloning or deleting the directory, `make web-install` is required before `make web-npm-build`.

### 32. 03/08/2026 - Externalize Hardcoded Site Values (Phase 2 open-source prep)

Removed all personal/infrastructure-specific values from the web source. Everything now lives in `config/site.env` (personal, alongside `albums.yaml`) with `config/site.example.env` as the committed template.

**`config/site.env` / `config/site.example.env`** — new files holding `VITE_*` vars (consumed by Vite and Svelte) and deploy/test vars (`CLOUDFRONT_ID`, `RSYNC_DEST`, `TEST_ALBUM_*`, consumed by `bin/` scripts). Example file renamed from `site.env.example` → `site.example.env` to keep `.env` extension consistent (matches `albums.example.yaml` naming pattern). Multi-word values must be double-quoted for bash `source` compatibility.

**`web/vite.config.ts`** — added `loadSiteEnv()` called before `defineConfig`. It reads `config/site.env` relative to `import.meta.url` (reliable regardless of CWD), exits with a clear error if missing, parses `key=value` lines (skipping blanks and `#` comments), strips surrounding quotes, and injects `VITE_*` keys into `process.env` only if not already set. Vite picks them up automatically as `import.meta.env.VITE_*` in both dev and build — no `envDir` or symlinks needed.

**`web/src/app.d.ts`** — added `ImportMetaEnv` and `ImportMeta` interfaces for TypeScript awareness of the five `VITE_*` vars.

**Svelte files updated** (hardcoded strings → `import.meta.env.VITE_*`):
- `app.html` — `%VITE_SITE_DESCRIPTION%` HTML substitution
- `+page.svelte` (home) — local consts `siteName`/`siteUrl`/`siteDesc` used in OG tags, title, h1
- `+layout.svelte` — footer uses `VITE_COPYRIGHT_YEAR` and `VITE_COPYRIGHT_OWNER`
- `+error.svelte` — title uses `VITE_SITE_NAME`
- `albums/[slug]/[[index]]/+page.svelte` — OG tags use `VITE_SITE_URL` and `VITE_SITE_NAME`; fallback `og:description` drops "Donohoe" → generic `photos from the '...' album`

**`web/static/404.html`** — title changed from `404 - Donohoe Photo Albums` to `404 - Not Found` (static file Vite doesn't process).

**`bin/deploy-photos.sh`** — sources `config/site.env` after `cd "$SDIR/.."`, then uses `$RSYNC_DEST`, `$CLOUDFRONT_ID`, and `$VITE_SITE_URL` in place of hardcoded values.

**`bin/test-photos-apache.sh`** — sources `config/site.env` (resolves path via `BASH_SOURCE`), uses `$VITE_SITE_URL` for `BASE`/`REDIRECT_BASE`, `$TEST_ALBUM_LOCAL/PROD/HYPHEN` for album slugs, and checks for `"404 - Not Found"` in the 404 body.

### 33. 03/08/2026 - Repo Extraction to Standalone `photos` Repo

Extracted the photos project from `dd-tbd` into its own public repo at `github.com/dougdonohoe/ddphotos`.

**Files moved** (copied to `~/work/photos`, then deleted from `dd-tbd`):
- `pkg/photogen/` — core library
- `pkg/exit/` — copied (not removed from `dd-tbd`; still used there)
- `cmd/photogen/` — CLI entrypoint
- `web/` — SvelteKit app
- `bin/deploy-photos.sh`, `bin/test-photos-apache.sh`
- `config/albums.example.yaml`, `config/descriptions.example.txt`, `config/site.example.env`
- `docs/photogen-plan.md`, `docs/open-source-plan.md`

**`go.mod`** — new module path `github.com/dougdonohoe/ddphotos`; import paths in `cmd/photogen/photogen.go` and `pkg/photogen/resize_worker.go` updated from `dd-tbd/pkg/...` → `github.com/dougdonohoe/ddphotos/pkg/...`.

**`.gitignore`** — ignores `config/site.env`, `config/albums.yaml`, `config/albums-laptop.yaml`, `config/descriptions.txt`, `web/build/`, `web/node_modules/`, `web/.svelte-kit/`, `web/static/albums/`.

**Node setup:**
- `web/.nvmrc` — added, specifying Node 22 (required by `@eslint/compat@2.0.2`)
- `Makefile` — added `web-nvm-install` target (`nvm install` from `web/`); renamed `web-install` → `web-npm-install`; `nvm install` only in setup targets, not in run/build/test targets

**`SITE_ENV` support** — `vite.config.ts` now accepts a `SITE_ENV` env var pointing to an external `site.env` (e.g. in a separate config repo), falling back to `../config/site.env`. Threaded through the `Makefile` with `SITE_ENV ?= config/site.env` so users can override: `make web-npm-run-dev SITE_ENV=~/work/my-config/site.env`.

**Tilde expansion fix** — `$(abspath ~/path)` in Make prepends CWD instead of expanding `~`. Fixed with `override SITE_ENV := $(abspath $(patsubst ~/%,$(HOME)/%,$(SITE_ENV)))`. The `override` directive is required because command-line variables have the highest precedence and silently ignore normal Makefile assignments.

**Other cleanup:**
- `web-playwright-test` → `web-playwright-test-apache` (pairs clearly with `web-playwright-test-dev`)
- `web/README.md` deleted — all content lives in root `README.md`
- `README.md` updated: added Prerequisites (Go/vips/nvm/node), Config Repo Pattern section, fixed Makefile targets table, made slow-loading URLs generic, removed personal references
- `dd-tbd/Makefile` stripped of all `web-*` targets; `dd-tbd/README.md` removed photo webapp references
- Annotated git tag `mcp_photos` added to `dd-tbd` to preserve the MCP server code location before it was removed from both repos

**Key gotcha:** Values with spaces (e.g. `VITE_SITE_NAME="Donohoe Photo Albums"`) must be double-quoted in `site.env` or bash `source` treats the remainder as a command. The Vite parser strips surrounding quotes so values arrive clean in `process.env`.

### 34. 03/08/2026 - Sample Albums, Multi-Site Output, and Phase 5 Setup

Phase 5 of the open-source effort: committed sample photos, built supporting tooling, and redesigned the output path layout to support multiple named sites cleanly.

#### `settings.id` and `SiteOutputPath()`

Added `id` field to the `settings` block in `albums.yaml`. The ID drives the output directory: `{output_dir}/albums/{id}/`. All photogen-generated files (per-album images, `albums.json`, `sitemap.xml`) now land inside this directory.

**`pkg/photogen/albums_config.go`** — added `ID string \`yaml:"id"\`` to `AlbumsSettings`.

**`pkg/photogen/config.go`**:
- Added `SiteID string` to `Config`
- Added `SiteOutputPath(parts ...string) string` method: `{OutputRoot}/albums/{SiteID}[/parts...]`
- Updated `Validate()` to require `SiteID` with format check: `^[a-z0-9][a-z0-9-]*$` (lowercase, digits, hyphens only)
- 3 new test cases in `config_test.go`: missing SiteID, invalid format, and `TestConfigSiteOutputPath`

**`pkg/photogen/album.go`** — `OutputPath()` now delegates to `cfg.SiteOutputPath()` instead of constructing the path manually.

**`pkg/photogen/json.go`** — `WriteAlbumsIndex` and `WriteSitemap` now take `siteDir` (= `cfg.SiteOutputPath()`) instead of `outputRoot`. Sitemap moved from `{output_dir}/sitemap.xml` → `{output_dir}/albums/{id}/sitemap.xml`, making it accessible via the symlink at `/albums/sitemap.xml`.

**`cmd/photogen/photogen.go`** — added `SiteID: settings.ID` to Config construction; updated call sites to use `cfg.SiteOutputPath()`.

#### Symlink-Based Multi-Site Switching

Photogen now writes to `web/albums/{id}/` (outside `web/static/`), which prevents Vite from double-copying the output during build. The SvelteKit static adapter follows `web/static/albums` as a symlink into `web/albums/{id}/`.

Switching sites:
```bash
make use-sample   # ln -sfn ../albums/sample web/static/albums
make use-prod     # ln -sfn ../albums/prod web/static/albums
```

rsync deploys `web/build/` (resolved real files), so the symlink is transparent to production.

**.gitignore** — added `web/static/albums` (the symlink itself) and `web/albums/` (all generated output dirs). Also added `.claude` to gitignore.

#### `resolvePath` Bug Fix

Relative `base` paths in `albums.yaml` (e.g. `base: sample/source`) were being joined with `configDir` instead of CWD, producing wrong paths like `sample/config/sample/source/slug`. Fixed in `AlbumsFile.resolvePath()`: relative bases now resolve from `os.Getwd()`. Absolute bases and absolute sources are unchanged. Source-without-base still resolves from configDir. Added test case `"relative base resolves to CWD"` in `albums_config_test.go`.

#### Sample Albums

Committed source `.jpg` files for three sample albums: `antarctica`, `uganda`, `the-way`. Photos contributed by Doug and Cindy Donohoe under CC BY-NC 4.0. Each album includes a `photogen.txt` with captions and sort order.

**`sample/config/albums.yaml`**:
- `settings.id: sample`
- `output_dir: web`
- `base: sample` pointing to `sample/source` (relative, resolves from CWD)

**`sample/README.md`** — CC BY-NC 4.0 license details.

Images compressed with `mogrify -quality 75` before commit (~119 MB total; no LFS needed for this size).

#### `cmd/copysample` Tool

Created `cmd/copysample/main.go` to copy selected photos from personal albums into `sample/source/`. It reads a selection file (`slug:1,2,7` — 1-based permalink indices), uses `photogen.LoadAlbumConfigs` + `NewAlbumProcessor.LoadPhotos()` to reproduce the exact photogen sort order, and copies the corresponding source JPGs.

Key flags: `-config-dir`, `-albums`, `-selection`, `-dest`, `-doit` (dry-run by default, matching photogen convention). Writes a filtered `photogen.txt` if descriptions exist for selected photos.

#### Makefile Targets

```bash
use-sample          # symlink web/static/albums → ../albums/sample
use-prod            # symlink web/static/albums → ../albums/prod
sample-photogen     # run photogen using sample config
sample-build        # use-sample + web-npm-build with sample/config/site.env
sample-npm-run-dev  # use-sample + web-npm-run-dev with sample/config/site.env
sample-test-apache  # run test-photos-apache.sh against Docker on port 8082
```

Doug's private targets (`doug-photogen-laptop`, `doug-build-prod`, etc.) added in a separate section of the Makefile as usage examples.

#### Playwright Test Updates

Updated tests to work against both sample and prod sites:

- **`navigation.spec.ts`** — made fully dynamic: reads first 3 album names and hrefs from `.album-card h2` at runtime. Works without hardcoding album names.
- **`smoke.spec.ts`** — limited overlap checks to `antarctica` and `uganda` (present in both sample and prod). Removed `honeymoon`. Removed `Great Wall` description check; kept `bottom of the world` (Antarctica, stable in both).
- **`captions.spec.ts`** — `PHOTO_N` changed from `14` → `1` (icebergs_12 is first in photogen.txt order for sample).

**Key gotcha:** `web/albums/` must be outside `web/static/` or Vite copies both the symlink target directory and the symlinked alias into `web/build/`, resulting in duplicate albums output.

### 35. 03/08/2026 - Footer: Repo Link and Build Timestamp

Added a second footer line: `Built with dougdonohoe/photos on March 5th, 2026 at 11:26 AM`.

**`web/vite.config.ts`** — injects `process.env.VITE_BUILD_TIME = new Date().toISOString()` immediately after `loadSiteEnv()`. Captured at Vite startup so the timestamp reflects when the build began.

**`web/src/app.d.ts`** — added `VITE_BUILD_TIME: string` to `ImportMetaEnv`.

**`web/src/routes/+layout.svelte`**:
- `ordinal(n)` helper: maps a day number to its ordinal string (`1st`, `2nd`, `3rd`, `4th`…)
- `formatBuildTime(iso)`: formats the ISO timestamp as `"Month Dth, YYYY at H:MM AM/PM"` using `Intl`-based locale formatting
- Footer now renders two `<div>`s: the existing copyright line, and the new build line with `margin-top: 0.35rem` for visual separation
- Link color in dark mode overridden to `#5a8ec0` (darker than the default powder-blue `#88b4e7`); light mode uses the standard `--link-color`

**`web/tests/captions.spec.ts`** — decoupled from specific caption text: replaced the hardcoded `CAPTION = 'Iceberg, right ahead!'` assertion with an `expectCaptionVisible()` helper that matches any non-empty `.pswp-caption` element. Tests now pass against both sample and prod sites regardless of photo sort order.

### 36. 03/08/2026 - npm audit fix

Ran `npm audit fix` in `web/`. Fixed 3 of 7 vulnerabilities (upgraded `svelte`, `rollup`, `minimatch`, `ajv`, `devalue`). 4 low severity vulnerabilities remain, all `cookie`-related inside `@sveltejs/kit` — SSR-only attack surface with no relevance to a statically generated site. The fix would require `--force`, which would downgrade `@sveltejs/kit` to `0.0.30` (a breaking ancient version). Already on the latest kit release; waiting on an upstream fix.

### 37. 03/08/2026 - Screenshot Script and SCREENSHOTS.md

#### Playwright Screenshot Script

Created `web/scripts/screenshots.mjs` — a standalone Node.js script (no extra dependencies) that uses the existing `@playwright/test` Chromium installation to capture 5 screenshots of a running site:

- `home-dark.png` / `home-light.png` — home page in each theme
- `album-dark.png` / `album-light.png` — album grid in each theme
- `lightbox-dark.png` — PhotoSwipe lightbox with a photo open

Key implementation notes:
- **Theme**: uses `page.addInitScript(fn, arg)` (two-argument form) so the theme value is serialized separately by Playwright and available in the page context. Closure variables do NOT survive `.toString()` serialization — the single-argument form silently passed `undefined`.
- **Transparent images**: injected `transition: none !important` via `addStyleTag` before navigation, so grid images snap to full opacity the moment the `loaded` class is added rather than fading in over 0.4s mid-screenshot.
- Album slug auto-detected from the home page if `--album` not provided.
- Added `make web-screenshots` target; output directory (`web/screenshots/`) is git-ignored.

#### SCREENSHOTS.md

Created `SCREENSHOTS.md` at the repo root displaying all 5 screenshots from `images/screenshots/`. Linked from `README.md` alongside the existing composite `images/screenshots.png`.

### 38. 03/08/2026 - GitHub Actions CI

Added `.github/workflows/ci.yml` — runs on every PR to `main`. Steps:

- `apt-get install libvips-dev pkg-config` (no custom Docker image needed)
- `actions/setup-go` (version from `go.mod`) and `actions/setup-node` (version from `web/.nvmrc`) with built-in caching
- `make build test vet` — Go build, unit tests, and static analysis
- `make use-sample` + `make sample-photogen` — generate sample album data
- `make web-docker-build` + `make sample-build` — build Docker image and static site
- `make web-playwright-install` + `npx playwright install-deps chromium` — Chromium and system libs (e.g. `libatk-bridge`)
- `make sample-test-apache` — Apache URL routing tests
- `make web-playwright-test-apache` — Playwright e2e tests

Validated via `act` (local GitHub Actions runner) during development, which surfaced an
interesting quirk: `act` runs the workflow in a Docker container, but `docker run -v` steps
inside the workflow use the **host** Docker daemon and mount from the **host** filesystem — not
from within the `act` container. Two copies of the repo exist simultaneously: one inside `act`
(where builds happen) and one on the host (what inner Docker mounts). Running `make sample-build`
on the host before `act` ensures the host copy is up to date for the Apache/Playwright steps.
Documented in `README-DEV.md` under `## CI (GitHub Actions)` (Inception joke included).

### 39. 03/08/2026 - README-DEV.md Review and robots.txt Route

#### README-DEV.md Review

Holistic review of `README.md` and `README-DEV.md` for grammar, accuracy, and consistency with code:

- Fixed photogen `-resize` flag help text: removed non-existent `thumb` size (only `grid` and `full` exist)
- Fixed em-dashes in `README-DEV.md` (lines 23, 455) and `Makefile` (line 1)
- Removed misleading `(deploy only)` label from `TEST_ALBUM_*` env var descriptions
- Rewrote confusing Testing section intro as a clear numbered list of three testing approaches

#### Dynamic robots.txt via SvelteKit Route

Replaced `web/static/robots.txt` (static, always disallow) with a SvelteKit pre-rendered route at
`web/src/routes/robots.txt/+server.ts`. Controlled by `VITE_ALLOW_CRAWLING` in `site.env`:

- `false` (default): `User-agent: *\nDisallow: /`
- `true`: `User-agent: *\nAllow: /\nSitemap: {VITE_SITE_URL}/albums/sitemap.xml`

Added `VITE_ALLOW_CRAWLING` to `app.d.ts`, `config/site.example.env`, and the README-DEV.md env table.
The static `web/static/robots.txt` was deleted.

### 40. 03/11/2026 - Back-to-Top Button on Album List Page + Shared Component Refactor

Added a back-to-top arrow button to the album list (home) page on mobile, where 21 albums make the page long enough to warrant it. Took the opportunity to extract the existing button from the album grid page into a shared `BackToTop.svelte` component.

**`web/src/lib/components/BackToTop.svelte`** — new component encapsulating scroll listener, state, and styles. Accepts a `mobileOnly` prop (default `false`); when true, CSS hides the button at `min-width: 769px` via `@media`.

**`web/src/routes/+page.svelte`** (home) — removed inline scroll logic; uses `<BackToTop mobileOnly={true} />`.

**`web/src/routes/albums/[slug]/[[index]]/+page.svelte`** (album grid) — removed `showBackToTop` state, `scrollToTop()` function, scroll listener from `onMount`, inline button, and `.back-to-top` CSS block; uses `<BackToTop />` (visible on all screen sizes).

**`web/tests/back-to-top.spec.ts`** — 4 new Playwright tests:
- Album page: button appears after scrolling on desktop and mobile
- Home page: button appears after scrolling on mobile; button is hidden on desktop

Key testing challenges:
- `window.scrollTo` queues the scroll event asynchronously; paired with `window.dispatchEvent(new Event('scroll'))` in the same `evaluate` call to fire it immediately
- Body `min-height` injected inline (via `document.body.style.minHeight`) with a synchronous `getBoundingClientRect()` reflow before scrolling — ensures the page is scrollable even with sparse sample data
- Album page tests wait for `.gallery.layout-ready` before scrolling (set by the album page's `onMount`, which runs after `BackToTop`'s `onMount` in Svelte's bottom-up mount order — reliable hydration signal)

### 41. 03/13/2026 - UI Tweaks and Frontend Refactoring

#### Album List Page Layout

Reworked the home page (`+page.svelte`) header to better balance the title and theme toggle:

- Moved `<h1>` out of `<main>` into its own `<header>` element (sibling of `<main>`), giving independent control over title vs. content area padding
- Removed `max-width`/`margin: auto` centering from the header so the title sits left-aligned with the page edge at any viewport width, matching the visual feel of the toggle button
- Bumped h1 font size to `2.4rem` (desktop) and `1.7rem` (mobile)
- Adjusted `main` top padding so the album grid sits comfortably below the title

#### Theme Toggle Refactor

Extracted the theme toggle button from `+layout.svelte` into a shared `ThemeToggle.svelte` component (`web/src/lib/components/ThemeToggle.svelte`). The layout now renders it via `<div class="theme-toggle-wrap">` with `position: absolute; top: 0.7rem; right: 1rem`, preserving the original behavior where the button stays in the same viewport position across page navigations.

Note: An alternative approach was tried where the button was moved into each page's header (in-flow), which gave better alignment with content edges but caused visible positional jumps during navigation. Reverted to absolute positioning for the smoother UX. `position: fixed` was also considered and rejected (tried early in the project).

#### OpenGraph Component

Extracted the near-identical `<svelte:head>` OpenGraph/Twitter meta tag blocks from both pages into a shared `OpenGraph.svelte` component (`web/src/lib/components/OpenGraph.svelte`). Takes four props: `title`, `description`, `url`, `image`. The constant tags (`og:type`, `og:site_name`, `twitter:card`) are hardcoded in the component.

#### Go Code Refactoring

Three duplication fixes in `pkg/photogen/`:

**`util.go`** (new file) — two shared helpers:
- `loadJSON[T any](path string)` — generic JSON file reader; eliminates identical read+unmarshal boilerplate in `LoadAlbumSummaries` and `LoadAlbumIndex`
- `scanLines(path string, fn func(line string))` — opens a file, skips blank lines and `#` comments, calls `fn` for each remaining line; eliminates duplicated scanner boilerplate in `loadPhotoDescriptions` and `LoadAlbumDescriptions`

**`album.go`** — added `coverPhoto() *Photo` method: returns the configured cover photo (searching by filename) or the first photo as fallback. Used by both `WriteCoverJPEG` and `GetAlbumSummary`, replacing duplicated cover-selection logic (the old code even had a comment noting the duplication). Removed `bufio` import.

**`albums_config.go`** — `LoadAlbumDescriptions` now uses `scanLines`. Removed `bufio` import.
- Home page tests use `toPass` with retries to handle the dev-server parallel execution race where the scroll listener may not yet be registered

### 42. 03/24/2026 - TypeScript Types, Code Quality, and IntelliJ Warning Fixes

#### TypeScript Types for JSON Data

Added `web/src/lib/types.ts` with interfaces mirroring the Go JSON structs in `pkg/photogen/json.go`:
- `PhotoSrc` (mirrors `PhotoSrcIndex`)
- `Photo` (mirrors `PhotoIndex`)
- `AlbumIndex`
- `AlbumSummary`

Updated `web/src/routes/+page.ts` and `web/src/routes/albums/[slug]/[[index]]/+page.ts` to use these types on `.json()` responses, replacing `any` casts. Removed `(p: any)` and `(photo: any)` annotations from the album page svelte (now inferred from typed data).

Added `// IMPORTANT: Keep in sync` comment near Go structs and a matching note at the top of `types.ts`. Added a "Type sync requirement" section to `CLAUDE.md`.

#### Type-Check Gating the Build

Added `npm run check` (which runs `svelte-check`) as a prefix to the `build` script in `web/package.json`:
```
"build": "npm run check && vite build"
```
This means type errors abort the build locally and in GitHub Actions CI (which runs `make sample-build`).

#### `@types/justified-layout`

Installed `@types/justified-layout` (`npm install --save-dev`) to resolve the IntelliJ "Could not find a declaration file" warning on `import justifiedLayout from 'justified-layout'`.

#### `bufio.Writer` for Sitemap

`WriteSitemap` in `json.go` was calling `file.WriteString` without checking errors. Switched to `bufio.NewWriter` wrapping the file, with a single `w.Flush()` error check at the end. This is the idiomatic Go pattern: `bufio.Writer` makes write errors sticky so they surface on `Flush()`.

#### IntelliJ False-Positive Suppressions

- `web/src/routes/albums/[slug]/[[index]]/+page.svelte` — added explanatory comment before `<!--suppress CssUnusedSymbol -->` at top of file (`:global(.pswp)` targets PhotoSwipe-injected DOM, invisible to static analysis)
- `web/src/app.d.ts` — added `// noinspection JSUnusedLocalSymbols` and explanatory comment before the `ImportMeta` augmentation (IntelliJ doesn't recognize the Vite module augmentation pattern)
- `web/scripts/screenshots.mjs` — added `// noinspection JSUnresolvedVariable` comment on `window.__svelte` access (Svelte runtime global, not in any type definition)

### 43. 03/28/2026 - Fix Unresolved `meta name="description"` Placeholder

The `app.html` shell used Vite's `%VITE_SITE_DESCRIPTION%` env substitution syntax for the global `<meta name="description">` tag. This only works when the variable is defined in a `.env` file at build time, but site config lives in the private config repo and is not in a `.env` file -- so Vite never substituted it, leaving the literal placeholder string in every built page.

Fix: removed the broken tag from `app.html` and added `<meta name="description" content={description}>` to `OpenGraph.svelte`, which already has `description` as a prop and injects it via `<svelte:head>`. Since every page uses `OpenGraph`, the description is now correctly set on all pages.

### 44. 03/31/2026 - V2 Auth: Go Backend — Encryption, Cover Logic, SiteID

Added encryption support to `photogen` for protecting album data at rest.

**`pkg/photogen/encrypt.go`** — new file with `EncryptConfig` (loaded from `passwords.txt`), `EncryptJSON`, `HasPerAlbumPassword(slug)`, and PBKDF2-SHA256 + AES-256-GCM encryption matching the browser's `SubtleCrypto` API.

**`pkg/photogen/json.go`**:
- `SiteConfig` struct gains `SiteID string \`json:"siteId"\`` — written to `config.json` so the frontend can scope localStorage keys to the current build
- `WriteConfigJSON()` populates the new field
- `GetAlbumSummary()` cover logic fix: cover URL is only included in the summary when the album is unencrypted, *or* when a site-wide password protects it but the album has no dedicated per-album password. Albums with their own password get no cover in the public index (the cover is only revealed after the user unlocks that album). Implemented via `HasPerAlbumPassword(slug)` check.
- `WriteAlbumsJSON()` / `WriteAlbumIndex()` — encrypt their output blobs when an `EncryptConfig` is present; per-album and site-wide encryption paths both handled.

**`pkg/photogen/json_test.go`** — updated `TestWriteConfigJSON` to assert `siteId` field in output; updated `TestGetAlbumSummary` mixed encryption case to assert cover is empty for the per-album-password album and non-empty for site-password-only albums.

**`cmd/photogen/photogen.go`** — added `-passwords` flag pointing to a passwords file; `-encrypt` flag to opt in; `-clean` flag to delete files from a previous encrypted build before regenerating.

**`sample/config/`** — added `passwords-all.txt` (site + Uganda album passwords) and `passwords-uganda.txt` (Uganda-only) for testing both encryption modes.

**Makefile** — added `sample-photogen-pw-all`, `sample-photogen-pw-uganda`, `use-sample-pw-all`, `use-sample-pw-uganda` targets.

### 45. 03/31/2026 - V2 Auth: Frontend — Password Dialogs, Cover Flash, localStorage Scoping

Wired the encrypted album/site data up to the SvelteKit frontend, then iterated through several flash and scoping bugs.

#### Password Prompts

**`web/src/lib/components/PasswordPrompt.svelte`** — new component: lock-icon card with password input, shake animation on wrong password, `autofocus` (suppressed with `<!-- svelte-ignore a11y_autofocus -->` comment since it is intentional for an explicit dialog).

**`web/src/routes/+page.svelte`** — site-encrypted home page: tries stored site password then any stored album password on mount; shows `PasswordPrompt` in a `position: fixed` full-screen overlay if neither works; fades in album grid after decryption; shows site title only once decrypted (`{#if !data.encryptedBlob || albums}`).

**`web/src/routes/albums/[slug]/[[index]]/+page.svelte`** — per-album encrypted page: same pattern; `handleUnlock` uses `decryptedAlbum.cover ?? decryptedAlbum.photos[0]?.src.grid` to cache the correct cover URL.

#### Cover Flash Fix (inline `<head>` script)

The core problem matched the existing light/dark theme flash: SSR bakes a placeholder into the static HTML, and JS only runs after first paint. Solution: same pattern as the theme fix — a synchronous inline `<head>` script sets CSS custom properties from localStorage before the browser lays out the body.

**`web/src/app.html`**:
- Moved `%sveltekit.head%` *before* the inline script so the `<meta name="ddp-site-id">` tag (injected by `+layout.svelte`) is in the DOM when the script runs
- Inline script reads `siteId` from the meta tag (not localStorage — avoids stale value on first load after build switch), then sets `--ddp-cover-{slug}` and `--ddp-icon-vis-{slug}` CSS custom properties on `<html>` for any cached cover URLs

**`web/src/routes/+page.svelte`** — placeholder div references `var(--ddp-cover-{slug}, none)` as `background-image` default, so the cover appears from the very first paint. After Svelte hydrates, `albumCovers` state (populated synchronously via `untrack(() => data.albums)` in `$state` initializer) takes over with an explicit `url(...)`.

Lock icon visibility trick: `--lock-vis` CSS variable set on the placeholder parent (`hidden` when a cover is cached, else inherits from `--ddp-icon-vis-{slug}` which the inline script sets). SVG uses `visibility: var(--lock-vis, visible)` — lock is always in the SSR HTML for layout but is CSS-hidden on first paint when a cover will show instead.

`coversLoaded` state (false during SSR) gates the non-encrypted placeholder icon (mountain SVG) so it isn't baked into the static HTML.

#### localStorage Key Scoping

All keys are now scoped to `siteId` to prevent 404s when switching between dev builds (which use different HMAC keys, producing different filenames):
- `ddp_cover_{siteId}_{slug}` — cover URL cache
- `ddp_album_{siteId}_{slug}` — per-album password
- `ddp_site_{siteId}` — site-wide password

**`web/src/lib/crypto.ts`** — `siteKey(siteId)`, `albumKey(siteId, slug)`, `coverKey(siteId, slug)` replace the old unscoped constants. `syncSiteId(siteId)` called on mount: detects a siteId change and clears all stale `ddp_cover_*`, `ddp_album_*`, `ddp_site_*` entries. `tryStoredAlbumPasswords(encryptedBlob, siteId)` scans the `ddp_album_{siteId}_*` prefix.

### 46. 03/31/2026 - V2 Auth: Playwright Testing System for Password Variants

Built a complete Playwright testing infrastructure that covers all three password variants (no-password, pw-all, pw-uganda) against both dev and Apache modes.

#### Shell Scripts

**`bin/run-tests.sh`** — runs one password variant against dev, Apache, or both. Handles: photogen + symlink setup, Vite dev server on port 5174 (`--clearScreen false` to preserve output), static build + Docker/Apache on port 8083, passing `PLAYWRIGHT_PASSWORDS_FILE` env var to Playwright. Key fix: `|| return 1` on build and docker commands inside `run_apache()` because bash suppresses `set -e` inside functions called via `||`.

**`bin/test-all.sh`** — loops over all three variants, passes `--mode` through. Both scripts have `--help/-?` and `trap 'exit 130' INT TERM` for clean Ctrl-C handling.

#### Playwright Helpers (`web/tests/helpers.ts`)

- `loadPasswords()` — parses `PLAYWRIGHT_PASSWORDS_FILE` (skips `#` comments and `_key_` entries)
- `unlockSite` / `unlockAlbum` — fill password, submit, wait for content. `unlockAlbum` waits for `.gallery.layout-ready` (set in onMount before decryption) not just `.gallery`
- `unlockSiteIfNeeded` / `unlockAlbumIfNeeded` — use `Promise.race()` between content appearing and `.fullscreen-overlay` appearing, both with 15s timeouts. Required because `PasswordPrompt` is gated on `{#if browser}` so it is not in the SSR HTML; `isVisible()` returns false before hydration

#### New `web/tests/password.spec.ts`

12 tests covering: site/album prompt visible, wrong password rejected, correct unlock, password remembered on reload, cover caching after unlock, `?clear` behavior, autofocus, title capitalization, lock icon before unlock. Tests skip when the current variant doesn't apply.

#### Adapted Existing Tests

All existing specs (`smoke`, `navigation`, `url`, `captions`, `back-nav`, `back-to-top`) gained `unlockSiteIfNeeded` / `unlockAlbumIfNeeded` calls. Permalink tests (`url.spec.ts:46`, `captions.spec.ts:32`) unlock at the base album URL first to store the password in localStorage, then navigate to the permalink — `handleUnlock` (form submit) does not call `openLightbox`; only `tryDecryptAlbum` (called from `onMount` with stored password) does.

#### Build Fix: Encrypted Builds and Prerendering

**Problem:** When all albums are encrypted the SvelteKit crawler never discovers album page links (hidden behind the site password prompt), so `prerender.entries` didn't include album routes, and the build failed with `handleUnseenRoutes` error.

**Fix (`web/svelte.config.js`):** Added `albumEntries()` which reads album slugs from `static/albums/` at build time using `readdirSync` and injects them into `prerender.entries`. This ensures album pages are always prerendered — in encrypted builds they render with the correct page skeleton (title, meta tags) and show the password prompt after JS hydration, rather than falling back to `index.html` which would show the *site* password prompt and confuse album-level test helpers.

#### Config and CI

**`web/playwright.config.ts`** — added `expect: { timeout: 10_000 }` (default 5s is too tight for `{#if browser}` components in Apache mode), `timeout: 15_000` per-test, `reporter: 'list'`.

**`.github/workflows/ci.yml`** — simplified from 10 steps to 5: Go build/test/vet, Playwright install, Apache routing tests (`make sample-photogen sample-build web-docker-build sample-test-apache`), and `bin/test-all.sh --mode apache` covering all three password variants. Apache-only (not `--mode both`) because CI should test what gets deployed, and dev mode can hide production issues.

**`web/src/routes/+layout.svelte`** — injects `<meta name="ddp-site-id" content={siteId}>` so the inline script can read the current build's siteId without touching localStorage.

### 47. 03/31/2026 - V2 Auth: Password Hints + YAML Passwords Format

#### YAML Passwords File

Migrated passwords files from a custom `key:value` text format to YAML. The old format (`_key_:`, `_all_:`, `slug:`) was fragile — adding per-entry metadata required ad-hoc suffixes (`slug!hint:`) that both parsers had to skip explicitly.

**New format (`sample/config/passwords-all.yaml`):**
```yaml
key: ddphotos-sample-key-all
site:
  password: allgood
  hint: What say you now?
albums:
  uganda:
    password: gorilla
    hint: A big brown ape
```

`_key_` → `key`, `_all_` → `site:` (nested map), per-album entries nested under `albums:`. Hints are optional on both `site` and any album entry.

**`pkg/photogen/encrypt.go`** — replaced `scanLines` text parser with `gopkg.in/yaml.v3` unmarshal. `EncryptConfig` gains `SiteHint string` and `AlbumHints map[string]string`. Validate error messages updated from `_key_`/`_all_` to `key`/`site`. Internal `passwordsFile` / `passwordEntry` structs handle YAML unmarshalling.

**`pkg/photogen/json.go`** — `SiteConfig` gains `SiteHint string` and `AlbumHints map[string]string` (both `omitempty`). `WriteConfigJSON` populates them from `EncryptConfig` when present.

Sample files renamed: `passwords-all.txt` → `passwords-all.yaml`, `passwords-uganda.txt` → `passwords-uganda.yaml`. Makefile, `bin/run-tests.sh`, `bin/test-all.sh`, and `README-DEV.md` all updated.

#### Hint Display in Password Dialogs

Hints flow from the passwords YAML → `config.json` (plaintext, always unencrypted) → frontend. No fallback: album hint is independent of site hint.

**`web/src/lib/types.ts`** — `SiteConfig` gains `siteHint?: string` and `albumHints?: Record<string, string>`.

**`web/src/routes/+page.ts`** — exposes `siteHint` from `siteConfig`.

**`web/src/routes/albums/[slug]/[[index]]/+page.ts`** — exposes `albumHint` via `config.albumHints?.[params.slug]` (no fallback to site hint).

**`web/src/lib/components/PasswordPrompt.svelte`** — new `hint?: string` prop. When set, renders `<p class="hint">Hint: <i>{hint}</i></p>` between the input and the Unlock button. Styled with `var(--text-muted)` at 0.85rem.

#### Test Infrastructure

**`web/tests/helpers.ts`** — `loadPasswords()` replaced with `js-yaml` YAML parser. `Passwords` interface gains `allHint: string | null` and `albumHints: Record<string, string>` so tests can check hint text without hardcoding it.

**`web/tests/password.spec.ts`** — two new tests: `site password dialog shows hint` (skips if no site hint configured) and `album password dialog shows hint` (skips if no hint for `firstAlbumSlug`). Both check `.card .hint` visibility and content.

**`web/tests/back-nav.spec.ts`** — fixed `back button after closing lightbox navigates to previous page`: added `unlockAlbumIfNeeded` after clicking the album card. In the pw-all variant, Antarctica is also site-encrypted; `waitForHydration` doesn't wait for auto-decryption to complete (gallery not in DOM yet), so the `.photo` click timed out.

### 48. 03/31/2026 - Recursive Album Support (`recurse: true`)

Added `recurse: true` to album entries in `albums.yaml` to collect photos from subdirectories. 

#### Design

Output is flattened: photos from subfolders get a sanitized prefix derived from their relative path to avoid filename collisions. The prefix strips everything except lowercase letters and digits, joining path segments with `_`:

```
Craig's/img001.jpg           → ID craigs_img001,        file craigs_img001.jpg
Ski 2007/Alan's/photo.jpg    → ID ski2007_alans_photo,   file ski2007_alans_photo.jpg
```

**Sort order (no `photogen.txt`):** photos at each level date-sorted, subdirectories alphabetical. This is the expected common case.

**Per-subfolder `photogen.txt`:** each subfolder can have its own `photogen.txt` for captions and local sort order. Entries use bare filenames (no prefix); photogen prefixes them during merge. With `manual_sort_order: true`, a `photogen.txt` at any level can reference subfolder names as placeholders that expand inline — enabling arbitrary interleaving of photos and subfolder groups across the whole album.

#### Implementation

**`pkg/photogen/albums_config.go`** — `AlbumEntry` gains `Recurse bool` (`yaml:"recurse"`). `AlbumConfig` gains the same field; passed through in `ToAlbumConfigs`.

**`pkg/photogen/album.go`**:
- `loadPhotoDescriptions` updated to strip image extensions from entries, so both `img001.jpg` and `img001` work in `photogen.txt`. Subfolder entries (no image extension) pass through unchanged.
- `sanitizePathSegment(s string) string` — strips non-alphanumeric characters, lowercases.
- `sanitizePrefix(relDir string) string` — splits relative path by `/`, sanitizes each segment, joins with `_`. Returns `""` for root.
- `LoadPhotos` branches on `AlbumConfig.Recurse` to call `loadPhotosRecursive`.
- `collectPhotosRecursive(dir, relDir string)` — core recursive function. Reads a directory, assigns prefixed `ID` and `FileName` to subfolder photos, applies captions from per-folder `photogen.txt`, then either expands `photogen.txt` order (when `ManualSortOrder`) or date-sorts local photos and recurses subdirs alphabetically.
- `expandManualOrder(...)` — processes `photogen.txt` entries: photo entries resolved by base ID, subfolder entries recursed inline. Unlisted photos and subfolders appended at end with warnings.

**`config/albums.example.yaml`** — added `recurse: false` to the full example and a new "Recursive" example section documenting the feature and subfolder `photogen.txt` behavior.

**`README.md`** — added backend features bullet for recursive album support.

**`README-DEV.md`** — added "Recursive Albums" subsection under Photo Descriptions covering default sort, per-subfolder `photogen.txt`, inter-folder ordering via placeholder entries, and the cover photo prefix requirement.

### 49. 04/01/2026 - Config Improvements + Minor Additions

#### `settings.passwords` in albums YAML

Previously encryption required passing `-encrypt <path>` on every `photogen` invocation. Now the passwords file can be declared once in `albums.yaml` under `settings.passwords` (filename relative to the config dir), eliminating the need to pass it on the command line for routine runs.

**`pkg/photogen/albums_config.go`** — `AlbumsSettings` gains `Passwords string` (`yaml:"passwords"`). New `(s *AlbumsSettings) LoadEncryptConfig(configDir string) (*EncryptConfig, error)` method resolves the filename relative to configDir and delegates to the existing `LoadEncryptConfig`.

**`cmd/photogen/photogen.go`** — `-encrypt` flag renamed to `-passwords` (updated description: "overrides `settings.passwords`"). Load logic: if `-passwords` is provided use it directly; otherwise if `settings.Passwords` is set, join it to `configDir`. Both paths call `LoadEncryptConfig`.

**`config/albums.example.yaml`** — added commented-out `passwords: passwords.yaml` entry under `settings:` with explanation and security note.

**`Makefile`** — updated `sample-photogen-pw-all` and `sample-photogen-pw-uganda` targets from `-encrypt` to `-passwords`.

**`README-DEV.md`** — updated CLI flags table (`-encrypt` → `-passwords`); updated Passwords File section to explain the YAML setting and CLI override relationship.

#### `Config.Summary()` method

**`pkg/photogen/config.go`** — new `Summary() string` method on `Config`. Prints a multi-line block of fields not already shown in the info line (mode, limit, site ID): `output`, `resize`, `index`, `force`, `clean`, `workers`, `site_url`, and `encrypt` (shows `none`, `key only`, `site`, `N album(s)`, or combinations, plus the passwords file path).

**`cmd/photogen/photogen.go`** — `fmt.Println(cfg.Summary())` added immediately after the existing info line.

#### `--no-playwright` flag for `deploy-photos.sh`

**`bin/deploy-photos.sh`** — added `--no-playwright` flag (`SKIP_PLAYWRIGHT=false`). When set, both the local Docker/Apache Playwright run and the post-deploy production Playwright run are skipped with a log message. The Apache routing tests (`test-photos-apache.sh`) are unaffected.

**`README-DEV.md`** — updated deploy steps list (step 4 and 8 note the skip flag) and added `--no-playwright` to the usage examples.

### 50. 04/02/2026 - Hero Image, Logout Button, and Custom CSS

#### Hero Image

Added an optional full-width banner image to the home page, configured in `albums.yaml`.

**`pkg/photogen/albums_config.go`** — `AlbumsSettings` gains `Hero *HeroEntry` and `CustomCSS string` (`yaml:"css"`). New `HeroEntry` struct has `Image`, `Base`, and `Crop` fields. `ToAlbumConfigs` resolves the hero image path (same base-map logic as album source paths) and CSS path into new `HeroImagePath` / `CustomCSSPath` fields on `AlbumsSettings`. `validate()` checks the hero base reference.

**`pkg/photogen/config.go`** — new `HeroConfig` struct (`ImagePath`, `Crop`). `Config` gains `Hero *HeroConfig` and `CustomCSS string`. `Summary()` reports both.

**`pkg/photogen/resize.go`** — new `ResizeHeroJPEG` function. Cover-scales the source image so both dimensions meet the target (1600×250px), then hard-crops: horizontally always centered; vertically anchored by `crop` param (`top`, `center`, `bottom`). Outputs JPEG/85, strips metadata.

**`pkg/photogen/json.go`** — `SiteConfig` gains `HeroImage string`, `CustomCSS string`, and `Encrypted bool` (all `omitempty`). `WriteConfigJSON` populates them from `Config.Hero`, `Config.CustomCSS`, and `Config.Encrypt`. Two new site-level write methods: `WriteHeroJPEG()` (called when `-resize`) and `WriteCSSFile()` (called when `-index`); both call `TrackFile` for `--clean` tracking.

**`cmd/photogen/photogen.go`** — wires `HeroImagePath`/`CustomCSSPath` from `AlbumsSettings` into `Config.Hero`/`Config.CustomCSS`. Calls `cfg.WriteHeroJPEG()` after the album loop when `cfg.Resize`, and `cfg.WriteCSSFile()` in the index block.

**`sample/config/albums.yaml`** — added `hero: image: theway/2024-The-Way-14.jpg, base: sample, crop: center`.

#### Custom CSS

`WriteCSSFile` copies the configured `.css` file to `{siteOutput}/custom.css`. The frontend injects it site-wide as a `<link rel="stylesheet">` after built-in styles, making any rules inside it effective cascade overrides. Redefining CSS custom properties is the recommended approach.

#### Frontend Refactor: `+layout.ts`

`config.json` loading was moved from `+page.ts` into a new `+layout.ts` load function, returning `{ siteConfig }` to all pages. `+page.ts` now calls `parent()` to get `siteConfig` instead of fetching `config.json` itself, eliminating the duplicate fetch. The `SiteConfig` TypeScript interface gains `encrypted`, `heroImage`, and `customCss` fields to match the updated Go struct.

#### Top Controls (Theme Toggle + Logout)

**`web/src/routes/+layout.svelte`**:
- The `.theme-toggle-wrap` container was renamed to `.top-controls` and converted to a flex row.
- A logout button (door-and-arrow SVG icon) is rendered next to the theme toggle when `siteConfig.encrypted` is true.
- When a hero image is present (`hasHero`), the `.over-hero` class adds `background: rgba(0,0,0,0.4)` to the pill so both buttons remain legible over any photo.
- The logout function mirrors the `?clear` logic: clears all `ddp_*` localStorage keys and calls `window.location.replace('/')`.
- `<link rel="stylesheet">` for `customCss` is injected in `<svelte:head>` when configured.
- The `<meta name="ddp-site-id">` tag now reads from `data.siteConfig.siteId` (layout data) rather than `page.data.siteId`.

**`web/src/routes/+page.svelte`**:
- Hero section added above the album grid. The entire block (hero or plain header) is gated on `!data.encryptedBlob || albums` so nothing flashes before the password prompt on encrypted sites.
- The hero renders a full-width image with a bottom gradient overlay and the site name in white text.
- `ogImage` derivation prefers `siteConfig.heroImage` over the first album's `coverJpeg`.

#### Tests

**`web/tests/smoke.spec.ts`** — `og:image` regex loosened from `/\/cover\.jpg$/` to `/\.jpg$/`: the test intent was always "must be a JPEG, not WebP"; `hero.jpg` satisfies this just as well as `cover.jpg`.

**`web/tests/password.spec.ts`** — two new tests in a "Logout button" section: `logout button is visible when encryption is configured` (works across pw-all and pw-uganda variants) and `logout button clears site password and shows prompt again` (pw-all only).

### 51. 04/02/2026 - `--hero-only` flag for `photogen`

**`cmd/photogen/photogen.go`** — added `-hero-only` boolean flag. When set, photogen regenerates the hero image and exits immediately, skipping all album processing, JSON/index generation, sitemap, CSS copying, and clean. It forces `Config.Force = true` so the existing `hero.jpg` is always overwritten. Exits with an error if no hero is configured in `albums.yaml`.

**`README-DEV.md`** — added `-hero-only` to the CLI flags table; added a usage note and example command under the Hero Image section.

### 52. 04/05/2026 - Decouple Album Data from Web Directory (branch: `decouple`)

The symlink-based approach for serving album data (`web/static/albums` → `web/albums/<site-id>`) was replaced with two environment variables (`DDPHOTOS_ALBUMS_DIR`, `DDPHOTOS_SITE_ID`) and a new per-site build output structure (`build/<site-id>/`). This eliminates the need for manual symlink management and allows multiple site builds to coexist.

#### Core Architecture Change

**Before:** `photogen` wrote output to `web/albums/<site-id>/`; a `web/static/albums` symlink pointed at the active site; the SvelteKit build consumed files via that symlink; Docker mounted `web/` with DocumentRoot at `web/build/`.

**After:** `photogen` writes output to `albums/<site-id>/` at the repo root (set via `output_dir: .` in `albums.yaml`). Two env vars — `DDPHOTOS_ALBUMS_DIR` (default: `albums`) and `DDPHOTOS_SITE_ID` (default: `sample`) — select the active site. The SvelteKit build outputs to `build/<site-id>/`. Docker mounts `build/` and `albums/<site-id>/` separately, with `entrypoint.sh` creating all symlinks inside the container.

#### Files Changed

**`config/defaults.env`** (new) — single source of truth for default values of `DDPHOTOS_ALBUMS_DIR` and `DDPHOTOS_SITE_ID`; read by both the Makefile and `vite.config.ts`.

**`web/svelte.config.js`** — adapter output dir is now dynamic: `pages`/`assets` set to `../build/${siteId}` so each site ID gets its own build directory at the repo root.

**`web/vite.config.ts`** — added `loadDefaultsEnv()` to read `config/defaults.env` as a lower-priority fallback; added `resolveAlbumsDir()` combining `DDPHOTOS_ALBUMS_DIR` + `DDPHOTOS_SITE_ID`; replaced the static-reload plugin with `albums-dev-server` plugin that serves `/albums/**` from the resolved albums dir via middleware and watches it for live reload.

**`web/src/hooks.server.ts`** (new) — build-time hook; `handleFetch` intercepts `fetch()` calls to `/albums/**` during `npm run build` and reads the files directly from disk, eliminating the need for any symlink at build time.

**`web/svelte.config.js`** — added `handleHttpError` to `prerender` config to silently ignore 404s on `/albums/**` paths (images and other assets are not pre-rendered, only served at runtime).

**`web/Dockerfile`** — removed DocumentRoot `sed` lines (no longer needed); kept `mod_rewrite`, `AllowOverride All`, `FollowSymLinks`, `ServerName`; updated to use `entrypoint.sh` for container startup.

**`web/entrypoint.sh`** (rewritten) — populates `/usr/local/apache2/htdocs` with symlinks at container startup: (1) uses `find` (not glob, so `.htaccess` is included) to symlink everything from `/build/<site-id>/` except `albums/`; (2) creates `htdocs/albums/` as a real directory; (3) symlinks `*.html` files from `/build/<site-id>/albums/` (pre-rendered album pages); (4) symlinks everything from `/albums/` (photogen output: image dirs, JSON). All symlinks live inside the container — nothing dangling is left on the host.

**`web/static/albums/README.md`** — removed; was only needed as a Docker mountpoint placeholder, which is no longer required.

**`web/.gitignore`** — removed `/build` entry (build output no longer lives in `web/`).

**`.gitignore`** — added `/build` (new output location at repo root).

**`Makefile`** — removed all `use-sample*`, `use-prod`, `use-sample-css`, `use-sample-demo` symlink targets; added migration check (`$(warning)` + `$(error)`) if `web/albums/` still exists; `DDPHOTOS_ALBUMS_DIR`/`DDPHOTOS_SITE_ID` read from `config/defaults.env` via `sed` (using `?=` to allow env var override); all npm build/dev targets pass the two env vars; `web-docker-run` updated to mount `$(PWD)/build:/build:ro` and pass `-e DDPHOTOS_SITE_ID`.

**`sample/config/albums.yaml`** and **`infra/photos/*/albums.yaml`** — removed `output_dir` entirely (now driven by `DDPHOTOS_ALBUMS_DIR`).

**`bin/run-tests.sh`** — removed symlink step; Docker run updated to mount `$(pwd)/build:/build:ro` and pass `-e DDPHOTOS_SITE_ID`; removed `mkdir -p web/build/albums`.

**`bin/deploy-photos.sh`** — added `--site-env` flag (separate from `--config-dir`) for specifying `site.env` location independently; added `--dry-run` flag that passes `--dry-run` to both rsync calls and skips CloudFront invalidation and post-deploy tests; added early guards for `RSYNC_HOST`, `RSYNC_DEST`, `CLOUDFRONT_ID`, `VITE_SITE_URL` (prevents rsync `--delete` targeting wrong path if a var is unset); enforces trailing `/` on `RSYNC_DEST`; stored `REPO_ROOT` before `cd web`; Docker run updated to new mount strategy; removed `find build/albums -type l -delete` (no longer needed); rsync source changed from `build/` to `$REPO_ROOT/build/$DDPHOTOS_SITE_ID/`.

**`infra/Makefile`** — added `DDPHOTOS_ALBUMS`; removed all `ddphotos-use-*` targets; all deploy targets pass `DDPHOTOS_ALBUMS_DIR` and `DDPHOTOS_SITE_ID` inline.

**`bin/search_cover.sh`** — updated stale `web/albums` path to `albums`.

**`README.md`** and **`README-DEV.md`** — updated throughout: removed symlink-based workflow, updated output paths (`web/albums/` → `albums/`, `web/build` → `build/<site-id>`), removed `use-*` targets from Makefile table, added "Album Location Variables" section documenting `DDPHOTOS_ALBUMS_DIR`/`DDPHOTOS_SITE_ID`, added `bin/search_cover.sh` section, removed Python static server section (no longer viable), updated Docker/Apache description.

#### Eliminating `settings.output_dir` from `albums.yaml`

`output_dir` was removed entirely from the YAML schema. `photogen` now reads its output directory from `DDPHOTOS_ALBUMS_DIR` (env var), falling back to `config/defaults.env`, with the `-out` CLI flag as an explicit override.

**`pkg/photogen/config.go`** — `SiteOutputPath()` simplified: no longer inserts an `albums/` component. `OutputRoot` is now the albums dir itself (e.g. `albums/`), so `SiteOutputPath()` = `OutputRoot/SiteID`.

**`pkg/photogen/albums_config.go`** — removed `OutputDir` field from `AlbumsSettings`.

**`cmd/photogen/photogen.go`** — added `repoRoot` var embedded at build time via `-ldflags "-X main.repoRoot=$(PWD)"`; `loadDefaultsEnv()` tries the compile-time repo root first, then falls back to cwd-relative `config/defaults.env` (so `go run ./cmd/photogen` still works from the repo root); fails fast with a clear error if `DDPHOTOS_ALBUMS_DIR` is still unresolved after all attempts.

**`Makefile`** — `build` target updated to pass `-ldflags "-X main.repoRoot=$(PWD)"`.

**Tests** (`config_test.go`, `albums_config_test.go`, `json_test.go`, `testdata/albums.yaml`) — updated to match new path structure; `OutputDir` references removed.

**`config/albums.example.yaml`** — removed `output_dir` setting; updated comments to reference `DDPHOTOS_ALBUMS_DIR`.

#### Key Design Decisions

- **No Docker restart on rebuild**: `build/` (parent) is mounted rather than `build/<site-id>/` (the actual output dir), so npm rebuilds that delete and recreate `build/<site-id>/` don't break the bind mount's inode binding.
- **Dangling symlinks eliminated**: all symlinks live inside the container's writable layer; the host filesystem is never written to by Docker.
- **Rsync safety**: two-pass rsync strategy — pass 1 syncs app files + pre-rendered HTML (excludes album image subdirs); pass 2 syncs album data (excludes `*.html` so pre-rendered pages aren't deleted). Early variable guards prevent `--delete` from targeting an empty or wrong remote path.
- **Migration**: Makefile detects if `web/albums/` still exists (old layout) and aborts with instructions to run `mv web/albums albums/`. Git cleanly replaces the old `web/static/albums` symlink with the new `web/static/albums/` directory on branch checkout.

### 53. 04/06/2026 - Go Refactoring, EXIF Improvements, and Date Semantics

#### Consolidate `LoadPhotos` / `loadPhotosRecursive`

`LoadPhotos()` previously had two entirely separate code paths: ~50 lines of inline photo-loading logic for the non-recursive case, and a separate `loadPhotosRecursive()` entry point that delegated to `collectPhotosRecursive()`. The non-recursive case was a base case of recursive loading (single directory, no subdirectory walk).

Refactoring: deleted `loadPhotosRecursive()` and the inline non-recursive code. Added a `recurse bool` parameter to `collectPhotosRecursive()` and `expandManualOrder()`. `LoadPhotos()` now calls `collectPhotosRecursive(..., ap.AlbumConfig.Recurse)` for both cases; when `recurse=false` the subdirectory walk is simply skipped. Removed now-unused `"path"` import (old code used `path.Join`; everything now uses `filepath.Join`). Tests updated; new sub-case added: `recurse=false: subfolders ignored`.

#### `SourcePath` Always Set, Relative from Base Directory

`SourcePath` was previously only populated for subfolder photos in recursive albums (comment said "recursive albums only, subfolder photos only"). `bin/search_cover.sh` reads `sourcePath` from JSON to locate the original source file, making it useful for all photos.

Changed: `SourcePath` is now always set, and is relative from the album's *source base directory* rather than from the album root itself. `LoadPhotos()` prefixes every photo's `SourcePath` with `filepath.Base(ap.AlbumConfig.Path)` after collection, so `sourcePath` includes the album folder name. For example, a photo `Craig's/img001.jpg` in an album sourced from `2008 - Big Sky/` gets `sourcePath: "2008 - Big Sky/Craig's/img001.jpg"`. For a root-level photo `foo.jpg` in that same album the result is `"2008 - Big Sky/foo.jpg"`. Removed `omitempty` from the JSON tag and TypeScript type since the field is now always present. Updated comments in `album.go`, `json.go`, `types.ts`, and `README-DEV.md`.

#### `vips.Startup` Error Handling

`init()` in `exif.go` called `vips.Startup(nil)` without checking the error. Fixed: now panics with `fmt.Errorf("vips startup failed: %w", err)` if startup fails. Passing an `error` to `panic` is idiomatic Go (preserves the error type and supports `%w` wrapping, unlike `fmt.Sprintf` which produces a plain string).

#### `exit.go` Comment Style

All exported function comments updated to proper Go doc comment style: each begins with the function name, uses complete sentences, and ends with a period.

#### `vite.config.ts` Improvements

- `loadSiteEnv()` now falls back to `sample/config/site.env` (with a warning) when `config/site.env` is absent and `$SITE_ENV` is not set. Fixes IntelliJ "Can't analyze vite.config.ts" error, which occurred because the config file hard-exited during static analysis when the default path was missing.
- Extracted a shared `loadEnvFile(path)` helper used by both `loadSiteEnv()` and `loadDefaultsEnv()`, eliminating 12 lines of duplicated env-file parsing logic.
- Added doc comments to all three functions.

#### Undated Photos Sort to End

The `sort.Slice` comparator `photos[i].DateTaken.Before(photos[j].DateTaken)` caused photos with no EXIF date (zero `time.Time`) to sort to the front, since `zero.Before(anyRealDate)` is true. Fixed with a new `sortByDate(photos []*Photo)` helper using `sort.SliceStable`: dated photos first (ascending), undated photos at the end in original scan order. Applied at all four sort call sites in `album.go`. New `TestSortByDate` unit tests cover all three cases; `TestCollectPhotosRecursive` gained an `undated photos sort to end in scan order` sub-case using a new `testdata/no-exif.jpg` fixture (minimal 10x10 JPEG with no EXIF).

#### `computeDateSpan` Bug Fix

`computeDateSpan()` used `ap.Photos[0].DateTaken` and `ap.Photos[len-1].DateTaken` as first/last dates. With undated photos now sorted to the end, `last` would be a zero time for any album containing undated photos, producing an incorrect or empty date span. Fixed: now scans `ap.Photos` to find the first and last non-zero `DateTaken`, ignoring undated photos entirely.

#### `Photo.date` Renamed to `Photo.datetime` + Full Timestamp

The `date` field in `index.json` (and the corresponding `Date string` Go struct field and `date: string` TypeScript type) stored only a date string (`"2003-03-09"`), discarding the time component. Since the field was unused in the frontend, renamed it to `datetime` / `DateTime` / `datetime` across Go, JSON, and TypeScript, and changed the format to RFC3339 (`time.RFC3339`), e.g. `"2003-03-09T12:00:30Z"`.

EXIF timestamps carry no timezone; `parseExifDateTime` now uses `time.ParseInLocation(..., time.UTC)` to make the UTC treatment explicit (previously relied on `time.Parse`'s implicit UTC default). Updated fixture `testdata/index.json` and test assertion in `json_test.go`.

### 54. 04/07/2026 - Home Page Scroll Restoration and HTTPS Dev Server

#### Home Page Scroll Restoration

When navigating from the home page to an album and pressing the back button (or "← Albums"), the home page always rendered from the top, losing the user's scroll position.

**Root cause**: SvelteKit's navigation cycle calls `afterNavigate` before the home page's album grid has finished rendering. At that point `document.scrollingElement.scrollHeight` equals `window.innerHeight` (the page is not yet scrollable), so `window.scrollTo(0, y)` silently clamps to 0. This is because the album data loads asynchronously: `afterNavigate` fires, then a `$derived` reactive variable `albums` is populated, then Svelte renders the 29-card grid — only *then* is the page tall enough to scroll.

**Why `await tick()` doesn't help**: SvelteKit's `commit_promise` calls `svelte.settled?.()` but has a known TODO comment that this doesn't reliably wait for full DOM layout. Even after `tick()`, the albums grid is absent.

**Solution**: A `$effect` that watches `albums` (the `$derived` value). When `albums` becomes non-null, Svelte has finished rendering the grid; a `requestAnimationFrame` then waits for the browser to compute layout before calling `scrollTo`.

Key pieces of the implementation in `web/src/routes/+page.svelte`:

- **Module-level `savedScrollY`** (`<script module>`): persists across home-page component remounts (navigating away and back reuses the same module instance).
- **`beforeNavigate`**: saves `window.scrollY` into `savedScrollY`.
- **`onMount`**: if `savedScrollY > 0`, calls `disableScrollHandling()` (prevents SvelteKit from resetting scroll to 0) and sets `document.documentElement.style.visibility = 'hidden'` (prevents a flash at position 0). A 2-second failsafe timeout always unhides.
- **`afterNavigate`**: if returning from an album page (`from?.url.pathname.startsWith('/albums/')`), sets `pendingScroll = savedScrollY` to trigger the `$effect`; otherwise clears `visibility` immediately.
- **`$effect`**: when `albums && pendingScroll > 0`, calls `requestAnimationFrame` to unhide and `scrollTo(0, y)` after layout.
- **`untrack()`**: used inside `$effect` to reset `pendingScroll` without re-triggering the effect.

The "← Albums" link uses SvelteKit's `goto()` (type `'goto'`), not a browser back button (type `'popstate'`), so the check uses `from?.url.pathname` rather than navigation type.

#### HTTPS Dev Server for Mobile Testing

Password-protected albums failed when accessing the dev server via LAN IP (e.g. `http://192.168.7.92:5173/`) on mobile. The error was silent — the correct password was rejected.

**Root cause**: The Web Crypto API (`crypto.subtle`) requires a [secure context](https://developer.mozilla.org/en-US/docs/Web/Security/Secure_Contexts). `localhost` qualifies, but LAN IPs over plain HTTP do not. `crypto.subtle` is `undefined` in that context, causing decryption to fail silently.

**Solution**: Added a `VITE_HTTPS=1` environment variable that gates loading `@vitejs/plugin-basic-ssl`, which generates a self-signed certificate for the Vite dev server.

`web/vite.config.ts` uses a top-level `await` (valid in ES module Vite configs) to conditionally import the plugin:

```ts
const httpsPlugin = process.env.VITE_HTTPS
    ? [(await import('@vitejs/plugin-basic-ssl')).default()]
    : [];

export default defineConfig({
    server: {
        host: true,
        https: !!process.env.VITE_HTTPS
    },
    plugins: [...httpsPlugin, sveltekit(), ...]
});
```

A dedicated `web-npm-run-dev-https` Makefile recipe was added (without `--open`, since mobile testing requires the LAN IP, not `localhost`):

```makefile
web-npm-run-dev-https:
    $(NODE_INIT) cd web && VITE_HTTPS=1 SITE_ENV=$(SITE_ENV) ... npm run dev
```

`README-DEV.md` documents the feature under the "LAN Access" subsection: the secure context requirement, how to start HTTPS mode (`VITE_HTTPS=1 make web-npm-run-dev-https`), and the browser self-signed cert warning users must accept.

### 55. 04/07/2026 - Configurable Default Theme and `?clear` Improvement

#### Configurable Default Theme

Added a `default_theme` setting to `albums.yaml` that controls the site's initial color theme for first-time visitors (before they manually toggle). Accepts `"light"` or `"dark"`; defaults to `"dark"` when omitted.

**Go backend**: `AlbumsSettings` gains a `DefaultTheme` field (validated to `"light"`, `"dark"`, or empty). It flows through `Config.DefaultTheme` and is written to `config.json` as `defaultTheme` (direct assignment; `omitempty` suppresses it when empty).

**Flash prevention**: The existing inline script in `app.html` already runs before first paint to set `data-theme` on `<html>`. Since `%sveltekit.head%` is positioned before that script, the prerendered meta tag emitted by `+layout.svelte` (`<meta name="ddp-default-theme">`) is in the DOM when the script runs. The script reads it as the fallback instead of hardcoding `'dark'`.

**Theme store**: `theme.ts` previously hardcoded `'dark'` as the fallback when localStorage has no stored value. It now reads `document.documentElement.getAttribute('data-theme')` — the value already applied by the inline script — so the Svelte store initialises to match what's on screen. This prevents a flash on first visit when the configured default is `'light'`.

**`?clear` improvement**: The `?clear` URL parameter (developer/testing tool that wipes stored passwords) now also removes the `'theme'` key from localStorage. The `logout()` function in `+layout.svelte` is intentionally left unchanged — it clears auth state only.

### 56. 04/08/2026 - nginx Docker POC

Added nginx as an alternative to Apache for serving the static site, mirroring the existing Docker setup exactly. Also took the opportunity to clean up Dockerfile naming conventions.

#### Dockerfile Renaming

Renamed existing files to use a consistent `server-name.*` convention that JetBrains IDEs recognize via the `.dockerfile` extension:

- `web/Dockerfile` → `web/apache.dockerfile`
- `web/entrypoint.sh` → `web/apache-entrypoint.sh`
- `bin/test-photos-apache.sh` → `bin/test-photos-server.sh` (server-agnostic routing tests)

#### Shared Entrypoint Logic

Extracted the common symlink setup logic from both entrypoints into a shared helper:

- **`web/setup-htdocs.sh`** — takes `<htdocs-dir>` as an argument; symlinks everything from `/build/<site-id>/` into the document root (using `find` to include dotfiles like `.htaccess`), then populates `<htdocs>/albums/` with symlinks to pre-rendered HTML pages and photogen output.
- **`web/apache-entrypoint.sh`** — sets `HTDOCS=/usr/local/apache2/htdocs`, calls `setup-htdocs.sh`, execs `httpd-foreground`.
- **`web/nginx-entrypoint.sh`** — sets `HTDOCS=/usr/share/nginx/html`, calls `setup-htdocs.sh`, execs `nginx -g 'daemon off;'`.

#### nginx Config

**`web/nginx.conf`** replicates the `.htaccess` URL routing rules:

- **Trailing slash removal** — `rewrite ^(.+)/$ $1 permanent` (301)
- **Photo permalink** — `location ~ ^/albums/([^/]+)/\d+$` serves `/albums/$1.html` via `try_files`
- **HTML extension mapping** — `try_files $uri $uri.html @spa_fallback` serves `/albums/slug` from `albums/slug.html`
- **SPA fallback** — `location @spa_fallback` serves `index.html` for root-level single-segment paths; returns 404 for nested paths (e.g. `/albums/doesnotexist`)

#### nginx Dockerfile

**`web/nginx.dockerfile`** uses `nginx:alpine` as the base image, replaces `/etc/nginx/conf.d/default.conf` with the custom routing config, and copies both `setup-htdocs.sh` and `nginx-entrypoint.sh` into the image.

#### `bin/docker-check.sh` Generalization

Added `--server apache|nginx` flag (default: `apache`). All existing calls remain unchanged. Per-server differences:

- Image tag: `photos-apache` vs `photos-nginx`
- Dockerfile: `web/apache.dockerfile` vs `web/nginx.dockerfile`
- Hash inputs: the relevant dockerfile + entrypoint(s) + `setup-htdocs.sh` (+ `nginx.conf` for nginx)

#### Test Infrastructure

- **`bin/run-tests.sh`** — added `--mode nginx` (port 8084) and `--mode all` (dev + apache + nginx). `both` retains its original meaning (dev + apache).
- **`bin/test-all.sh`** — default mode changed from `both` to `all`, so CI-like runs now cover all three servers.
- **Makefile** — added `web-docker-build-nginx`, `web-docker-run-nginx`, `web-playwright-test-nginx`, `sample-test-nginx`.

### 57. 04/09/2026 - Lightbox Grid Scroll Tracking

When navigating photos in the PhotoSwipe lightbox, closing it always returned the user to wherever the grid happened to be scrolled — typically the photo they originally clicked on, not the one they last viewed. The goal: when the lightbox closes, the underlying grid should be scrolled so the current photo is vertically centered on screen.

#### Why This Was Tricky

Three compounding problems, each hiding the next:

**1. Two separate scroll resets, not one.**
`history.go(-1)` (called on close to pop the pushState photo-URL entry) triggers SvelteKit's popstate handler, which resets scroll synchronously within ~2 frames to the position saved when `pushState` was called. A double-`requestAnimationFrame` handles that. But SvelteKit *also* re-runs the album page's load function (a network fetch) and restores scroll again when it completes — ~300–500ms later. This second reset was invisible in early debugging because probes only went to 200ms.

**2. `afterNavigate` doesn't fire for shallow pushState pops.**
SvelteKit's `pushState` creates a "shallow" history entry. When popped via `history.go(-1)`, SvelteKit processes it without firing full navigation lifecycle hooks — so `afterNavigate` (the idiomatic SvelteKit override point) never fires. This burned two rounds of investigation before console logging confirmed it.

**3. Live scrolling during the lightbox caused mobile glitches.**
An early iteration also scrolled the grid *live* while navigating (in the `change` event handler), so the close animation could animate back to a visible thumbnail. On desktop this was invisible under the opaque overlay. On iOS Safari, calling `scrollTo` while a `position: fixed` element is on screen causes the fixed element to momentarily mis-render — a visible flash. Removing the live scroll and relying purely on the on-close guard fixed both platforms.

#### Solution

On the PhotoSwipe `change` event, compute and store the target scroll position (`pendingScrollY`) that would vertically center the current photo:

```js
const galleryTop = container.getBoundingClientRect().top + window.scrollY;
const photoCenterY = galleryTop + box.top + box.height / 2;
pendingScrollY = photoCenterY - window.innerHeight / 2;
```

On `close`, capture the target and start a `requestAnimationFrame` guard loop that runs every frame for 700ms, re-applying the target whenever SvelteKit resets it:

```js
const deadline = performance.now() + 700;
const guard = () => {
    if (Math.abs(window.scrollY - target) > 1) {
        window.scrollTo({ top: target, behavior: 'instant' });
    }
    if (performance.now() < deadline) requestAnimationFrame(guard);
};
requestAnimationFrame(guard);
```

Any individual flash caused by a SvelteKit reset is at most one frame (~16ms) — imperceptible at 60fps. The guard covers both the immediate synchronous reset and the async one at ~400ms. All changes are in `web/src/routes/albums/[slug]/[[index]]/+page.svelte`.

### 58. 04/10/2026 - Lightbox Grid Focus Tracking

A natural follow-on to session 57. When the lightbox closes, the grid now scrolls to the last photo viewed — but keyboard focus was returning to whatever the browser chose, not that photo. The goal: focus the grid button for the last-viewed photo when the lightbox closes, so keyboard users can immediately interact with it.

#### Why This Was Tricky

Three separate things fight for focus on close, in sequence:

**1. PhotoSwipe's built-in focus restoration.**
When a modal closes, accessibility conventions say focus should return to the element that opened it. PhotoSwipe does this: on close it synchronously focuses the trigger button (the photo that was clicked to open the lightbox). This is the right behavior for the common case, but wrong when the user has navigated to a different photo.

**2. SvelteKit resets focus during navigation.**
Just as SvelteKit resets scroll after `history.go(-1)`, it also resets focus — to the body or a landmark element. This happens asynchronously at ~300–500ms, the same window as the scroll reset.

**3. Decoupling focus from the 700ms scroll guard.**
An early version called `focus()` at the *end* of the scroll guard loop (after 700ms). That correctly beat SvelteKit's reset, but meant the PhotoSwipe-restored focus sat on the wrong button for the full 700ms — clearly visible. Moving focus to a single `requestAnimationFrame` fixed the flash but then SvelteKit's async reset won at ~400ms.

#### Solution

Track the last-viewed photo index (`pendingFocusIndex`) alongside `pendingScrollY` — initialized when `openLightbox` is called and updated on every `change` event. On close, fold focus into the same guard loop as scroll:

```js
const focusBtn = container.querySelectorAll('.photo')[focusIdx];

const guard = () => {
    if (target !== null && Math.abs(window.scrollY - target) > 1) {
        window.scrollTo({ top: target, behavior: 'instant' });
    }
    if (focusBtn && document.activeElement !== focusBtn) {
        focusBtn.focus({ preventScroll: true });
    }
    if (performance.now() < deadline) requestAnimationFrame(guard);
};
```

The `document.activeElement !== focusBtn` check avoids redundant focus events — `focus()` is only called when something has actually stolen focus. `preventScroll: true` ensures the focus call doesn't fight the scroll guard. The guard fires on the first frame (beating PhotoSwipe's restoration) and continues through the 700ms window (beating SvelteKit's async reset).

Why not configure SvelteKit to skip scroll/focus restoration instead? SvelteKit's `noscroll` and `keepfocus` options apply to `goto()` calls, not to popstate navigations from `history.go(-1)`. And replacing `history.go(-1)` with `goto({ replaceState: true })` would leave a duplicate `/albums/slug` entry in the history stack — the user would need an extra back press to leave the album. Clean history is worth the guard loop.

### 59. 04/17/2026 - Home Page HTML Fields and Footer Redesign

#### New `albums.yaml` Settings: `site_title_html`, `site_subtitle_html`, `site_overview_html`

Added three optional site-level settings to `albums.yaml`:

- **`site_title_html`** — HTML for the home page title (in the hero overlay or page header). Falls back to `site_name` when omitted. Allows links, emphasis, or any inline HTML.
- **`site_subtitle_html`** — HTML rendered below the title in a smaller font.
- **`site_overview_html`** — HTML rendered above the album cards, in a slightly larger font than album descriptions, using the primary text color.

The pipeline change touched the usual four places: `AlbumsSettings` (YAML parsing in `albums_config.go`), `Config` (runtime build config in `config.go`), `SiteConfig` (JSON output in `json.go` / `WriteConfigJSON`), and the TypeScript `SiteConfig` interface in `web/src/lib/types.ts`.

Frontend changes in `+page.svelte`:
- Both hero and non-hero title paths use `{@html siteTitleHtml}` with fallback to `siteName`.
- Subtitle rendered beneath the title in both paths.
- Overview `<div>` rendered above the `.albums` grid.
- All three use `:global()` CSS selectors to reach `<a>` tags injected via `{@html}` (Svelte's scoped CSS cannot see dynamically injected elements).
- Links in the hero title: `color: inherit` (white), 1px underline offset 3px, slight opacity on hover.
- Links in the overview: `color: var(--link-color)` (theme-aware blue), no underline at rest, underline on hover — matching the footer link convention.

`sample/config/albums.yaml` updated with example values pointing to the DD Photos GitHub repo; `config/albums.example.yaml` and `README-DEV.md` updated to document the new fields.

#### Footer Redesign

Changed the footer "Built … with DD Photos" line to:

```
Built with joy by DD Photos on {date}  ⓘ
```

- **"DD Photos"** is now an `<a>` linking directly to `https://github.com/dougdonohoe/ddphotos` (opening in a new tab).
- **ⓘ** is a 16px Feather-style info SVG button that opens the existing About dialog.
- The info icon uses the same blue as the "DD Photos" link (`#5a8ec0` dark / `--link-color` light), with opacity fade on hover.
- `margin-left: 0.5rem` on the icon button separates it visually from the date.
- Space bar now activates album cards (keyboard accessibility): `onkeydown` handler on each `<a>` card calls `e.preventDefault()` + `e.currentTarget.click()` when Space is pressed.

### 60. 04/19/2026 - Encrypt Custom HTML Fields via `html.json` / `html.enc.json`

#### Motivation

The three home page HTML fields (`site_title_html`, `site_subtitle_html`, `site_overview_html`) can contain private information — links to private spreadsheets, email addresses, internal notes. Previously they were written into `config.json`, which is always served publicly. This change moves them out of `config.json` into a dedicated file that is encrypted when a site password is configured.

#### New File: `html.json` / `html.enc.json`

Follows the exact same pattern as `albums.json` / `albums.enc.json`:

- **No site password**: `html.json` (plaintext JSON) is written; `config.json` includes `"htmlFile": "html.json"`.
- **Site password**: `html.enc.json` (AES-256-GCM encrypted, same key derivation as albums) is written; `config.json` includes `"htmlFile": "html.enc.json"`.
- **No HTML fields configured**: neither file is written; `htmlFile` is omitted from `config.json`.

`photogen` writes the file in `WriteHTMLFile()` (called alongside `WriteConfigJSON()` in the index phase). The three HTML fields were removed from `SiteConfig` / `config.json`; the new `SiteHTMLContent` Go struct holds them.

#### Go changes

- `pkg/photogen/json.go`: removed `SiteTitleHTML`, `SiteSubtitleHTML`, `SiteOverviewHTML` from `SiteConfig`; added `HTMLFile string`; added `SiteHTMLContent` struct; added `WriteHTMLFile()` method.
- `cmd/photogen/photogen.go`: added `cfg.WriteHTMLFile()` call in the index phase.
- `pkg/photogen/json_test.go`: updated `TestWriteConfigJSON` for the new `htmlFile` behavior; added `TestWriteHTMLFile` covering no-op, plaintext, encrypted, and stale-file-removal cases.

#### Frontend changes

- `web/src/lib/types.ts`: removed HTML fields from `SiteConfig`; added `htmlFile?`; added `SiteHtmlContent` interface.
- `web/src/routes/+page.ts`: fetches `html.json` or `html.enc.json` when `htmlFile` is set; returns `siteHtml` (plaintext) or `encryptedHtmlBlob` (raw text for later decryption).
- `web/src/routes/+page.svelte`:
  - Added `decryptedSiteHtml` state; derives `effectiveSiteHtml`, `siteTitleHtml`, `siteSubtitleHtml`, `siteOverviewHtml` from decrypted or static content.
  - All three decryption paths (`handleUnlock`, stored site password, stored album password fallback) use `Promise.all` to decrypt `albums.enc.json` and `html.enc.json` in parallel, then assign `decryptedSiteHtml` before `decryptedAlbums` in a single synchronous block. Svelte 5 batches both state assignments into one DOM commit, preventing any flash of missing HTML content over the hero image.

#### Documentation

`README-DEV.md` updated: passwords section explains the new `html.json`/`html.enc.json` files; site settings table annotates the three HTML fields; frontend behavior section describes the parallel decryption; decode table adds `html.enc.json`.

### 61. 04/22/2026 - rsync Deploy Testing via Docker

#### Motivation

The production deploy path (rsync to EC2) was previously untested in CI. After migrating to S3, the EC2 server was decommissioned, leaving no way to test the rsync path. This session adds a self-contained Docker-based test that exercises the full photogen → build → rsync → verify cycle without a live server.

#### Approach

A new `photos-apache-ssh` Docker image (`httpd:2.4` + `openssh-server` + `rsync`) starts with an empty document root. `bin/rsync-test.sh` builds a temporary config dir with `site_url` patched to `http://localhost:8083`, then calls `deploy-photos.sh --no-pre-deploy-tests` with `RSYNC_RSH` set to route rsync through the container's SSH port (2222). After the rsync, `deploy-photos.sh` runs its normal post-deploy flow — server routing tests and Playwright — against the rsynced container. No volume mounts; rsync fills the server from scratch, proving the deploy commands work correctly.

A committed ed25519 key pair in `web/testdata/` (no passphrase) is baked into the image as `authorized_keys`. `RSYNC_RSH` points rsync at the private key and custom port, so `~/.ssh` is never touched. The private key is intentionally committed: it only unlocks a local throwaway container.

#### New files

- `web/apache-ssh.dockerfile` — `httpd:2.4` + `openssh-server` + `rsync`; bakes in test public key for root login; same Apache config as `apache.dockerfile`
- `web/apache-ssh-entrypoint.sh` — starts `sshd` then hands off to `httpd-foreground`
- `web/testdata/rsync-test-key` / `rsync-test-key.pub` — committed test-only SSH key pair
- `bin/rsync-test.sh` — orchestrates the full test: build image if missing, create temp config (patch `site_url`, write `site.env`), start container, export `RSYNC_RSH`, run `deploy-photos.sh`, clean up on exit

#### Changes to `bin/deploy-photos.sh`

- `--no-pre-deploy-tests` flag: skips `_pre_deploy` (the volume-mount Docker dance and pre-deploy server/Playwright tests) without affecting post-deploy tests. Needed because the rsync test supplies its own already-running container.
- `sleep 5` in `_post_deploy` made conditional on `CLOUDFRONT_ID` being set — eliminates a pointless 5-second wait when there is no CloudFront cache to invalidate.

#### Makefile and CI

- `web-docker-build-apache-ssh` target builds the new image.
- `sample-rsync-test` target is a one-liner calling `bin/rsync-test.sh` (previously a large inline recipe).
- `README-DEV.md` Makefile table updated with both new targets.
- CI gains a final step: `make sample-rsync-test`.

### 62. 04/23/2026 - S3 Deploy Testing via MinIO

#### Motivation

The S3 deploy path (`deploy-photos.sh --s3`) had no automated test. CloudFront Functions (which handle URL routing in production) cannot be replicated locally, but the `aws s3 sync` logic — file placement, Cache-Control headers, and the `--exclude "albums/*"` boundary — can be verified against a local S3-compatible server.

#### Approach

`bin/s3-test.sh` starts a [MinIO](https://min.io/) container (~130MB, S3-compatible), creates a test bucket, builds a temporary config dir with `site_url` patched to `http://localhost:9000`, and runs `deploy-photos.sh --s3 --no-server-test --no-playwright`. Post-deploy server and Playwright tests are skipped because MinIO serves the S3 API only, not HTTP. The script then runs 21 assertions covering:

- **Pass 1** — build files present at the bucket root; pre-rendered album HTML re-included via `--include "albums/*.html"`
- **Pass 2a** — album metadata (`albums.json`, `sitemap.xml`, `index.json`, `cover.jpg`) present with `Cache-Control: no-cache`; `.html` files not deleted by Pass 2a `--delete`
- **Pass 2b** — WebP images present with `Cache-Control: max-age=31536000,immutable`
- **Boundary** — a sentinel `.webp` uploaded directly to `albums/` survives Pass 1 `--delete` (protected by `--exclude "albums/*"`), then is removed by Pass 2b `--delete`

MinIO was chosen over LocalStack: LocalStack is ~1.1GB and hit a Docker 500 error locally; MinIO is ~130MB, starts in seconds, and supports the full S3 metadata API (including `CacheControl` on `HeadObject`).

#### New files

- `bin/s3-test.sh` — orchestrates the full test: start MinIO, create bucket, build temp config, run deploy, assert file placement and Cache-Control headers, clean up on exit

#### Makefile and CI

- `sample-s3-test` target calls `bin/s3-test.sh`.
- `README-DEV.md` Makefile table and deploy testing snippet updated to document the new target.
- CI gains a final step: `make sample-s3-test`.

### 63. 04/27/2026 - Full Docker Workflow

#### Motivation

Running ddphotos locally requires Go, libvips, pkg-config, and Node — a non-trivial setup, especially on Linux or for non-developers. This session adds a complete Docker-based workflow so anyone can run the full pipeline (photo processing, dev server, static build, and deploy) with only Docker installed.

#### Docker Image (`docker/Dockerfile`)

Multi-stage build:

- **Stage 1** (Go builder): compiles `photogen` with CGO against `libvips-dev`
- **Stage 2** (Node builder): runs `npm ci` to pre-install `node_modules`
- **Stage 3** (runtime): Debian slim with libvips, Node 22, Apache, rsync, openssh-client, and AWS CLI v2

Three values are baked in at build time via `ARG` + `RUN echo`:
- `/docker/VERSION` — short version tag (e.g., `dev` or `v1.2.0`)
- `/docker/GIT_DESCRIBE` — full `git describe --tags --long --dirty --always` output
- `/docker/IMAGE` — full image reference (e.g., `dougdonohoe/ddphotos:dev`); also patched into the wrapper script via `RUN sed -i`

#### Wrapper Script (`docker/ddphotos`)

A single `ddphotos` bash script that users install locally and invoke instead of `docker run` directly. It handles all volume mounts, port bindings, and environment variable forwarding. Key design decisions:

- Pre-command flags parsed before the command name: `--dir`, `--config-dir`, `--site-id`, `--site-env`
- `DDPHOTOS_ALBUMS_DIR` (the `/ddphotos` mount) defaults to the script's own directory, so the script works whether it lives inside the album directory or somewhere on `$PATH`
- `SCRIPT_DIR` is always mounted separately as `/ddphotos-script-dir` so the container can verify and upgrade the script regardless of what `DDPHOTOS_ALBUMS_DIR` points to
- External `--config-dir` paths are mounted as `/ddphotos-config` when they fall outside `DDPHOTOS_ALBUMS_DIR`
- `build_mount_args` parses `bases:` from `albums.yaml` and emits `-v` flags for external photo directories, resolving `~` and skipping paths already covered by the main mount
- Pre-flight validation checks that `albums.yaml` exists before launching Docker for any command that needs it, with a helpful error message guiding PATH-installed users to pass `--dir`

#### Entrypoint and Commands (`docker/entrypoint.sh`, `docker/do-*.sh`)

`entrypoint.sh` routes to the appropriate `do-*.sh` script. It also enforces a version check on every invocation (except `init`, `upgrade`, and `version`): if the mounted local `ddphotos` script differs from `/docker/ddphotos`, the user is told to run `upgrade`.

Commands:

- **`init`** — copies the wrapper script and config scaffold into the mounted directory; `--script-only` installs just the script for PATH-based use
- **`photogen`** — runs `photogen` with `cd /ddphotos` so relative base paths in `albums.yaml` resolve correctly; respects `DDPHOTOS_CONFIG_DIR`
- **`run`** — Vite dev server on port 5173; uses `set -m` bash job control so Ctrl-C goes to the shell only, killing the npm process group cleanly without printing npm's error output
- **`build`** — `npm run build`; symlinks `/ddphotos/build` for the adapter-static output
- **`serve`** — Apache on port 8000 (configurable via `$SERVE_PORT`)
- **`deploy`** — mounts `~/.ssh` and `~/.aws`, forwards AWS env vars, enforces a three-level staleness check: config files must be older than photogen output, which must be older than the build
- **`upgrade`** — atomically replaces the local script using a temp file + `/bin/mv -f` to avoid partial writes
- **`version`** — prints script path, image tag, albums dir, config dir, and site ID locally without launching Docker; `--image` additionally runs `docker run ... version` to pull `VERSION` and `GIT_DESCRIBE` from inside the image, indented under the Image line

#### Init Scaffolding (`docker/init/`)

- `albums.yaml` — starter config with three example albums: a main album (two bundled sample photos), a password-protected `secret` album, and an empty album
- `custom.css` — example CSS file with a light/dark `.accent` class, referenced in the starter `site_overview_html`
- `passwords.yaml` — example password file with hints
- `descriptions.txt` — placeholder album descriptions

#### Docker Hub Publishing (`bin/docker-push.sh`)

Builds and pushes a multi-arch image (`linux/amd64` + `linux/arm64`) via `docker buildx`:
- `:dev` tag for dirty or untagged builds; `:vX.Y.Z` + `:latest` for tagged releases
- `--doit` flag skips the interactive confirmation prompt
- Captures `git describe` and passes it as `DDPHOTOS_GIT_DESCRIBE` so the image carries its source provenance
- `make docker-build` for local single-arch builds; `make docker-push` wraps the script

#### About Dialog Version Info

The web UI's About dialog gains a Docker-aware **Image** row:

- In Docker builds, `do-build.sh` and `do-run.sh` export `VITE_GIT_DESCRIBE` (from `/docker/GIT_DESCRIBE`) and `VITE_DOCKER_IMAGE` (from `/docker/IMAGE`) before running npm
- `VITE_DOCKER_IMAGE` is only set in Docker builds; in local dev it is empty and the Image row is hidden
- In local dev builds, `vite.config.ts` continues to derive `VITE_GIT_DESCRIBE` from `git describe` as before

#### Documentation

- `README.md` — new **Docker Quick Start** section inserted before Motivation: eight-line quick start with link to the full reference
- `docs/DOCKER.md` — new complete Docker reference covering quick start, all commands with example output, pre-command flags table, directory layout, and version check/upgrade behavior
- `README-DEV.md` — `docker-build` and `docker-push` added to the Makefile targets table

### 64. 04/28/2026 - Documentation Reorganization

#### Motivation

`README.md` and `README-DEV.md` had grown into monolithic documents covering everything from quick start to deployment internals. With the addition of Docker mode as the preferred workflow, the two-mode distinction (Docker vs. Developer) needed to be explicit throughout the docs. This session splits the content into focused topic files, establishes consistent Docker/Developer framing, and cleans up accumulated stale references.

#### Structure

`README-DEV.md` deleted. Content distributed into 10 focused docs under `docs/`:

- **`CONFIGURATION.md`** (new) — full config reference for `albums.yaml`, `descriptions.txt`, and `site.env`. Key addition: "How Config Reaches the Frontend" section explaining that `photogen` acts as a conduit — the browser never reads `albums.yaml` directly; instead `photogen` writes `config.json`, `html.json`, `albums.json`, per-album `index.json`, `sitemap.xml`, `hero.jpg`, and `custom.css`. Encryption details (AES-256-GCM, PBKDF2-SHA256, Web Crypto API, HMAC-SHA256 WebP filename obfuscation, localStorage SiteID scoping) consolidated here from their former home in PHOTOGEN.md.
- **`INSTALL.md`** — renamed "Non-Docker Notes" → "Non-Docker Setup"; Configuration section removed and replaced with a link to CONFIGURATION.md; "Developer Tools on PATH" section added.
- **`PHOTOGEN.md`** — all `go run cmd/photogen/photogen.go` replaced with `bin/photogen`; output location table added (Docker vs. Developer defaults); recursive album sort behavior corrected (global date sort, not per-group).
- **`DEPLOY.md`** — "Deploying" section split into Docker mode (simple, prescriptive, auto-detects S3 from `site.env`, validates photogen/build freshness, no Playwright) and Developer mode (full pipeline with all flags). CloudFront Function moved to `DEPLOYMENT-SERVERS.md`.
- **`DEPLOYMENT-SERVERS.md`** — intro paragraph explaining why URL rewriting is needed; CloudFront Function added as a third peer section alongside Apache and nginx.
- **`TESTING.md`**, **`DEV.md`**, **`MAKEFILE.md`** — contributor-only note added.
- `README.md` doc index reordered: user-facing docs first, contributor docs (Development, Testing, Makefile) last. `ENV.md` Site Identity section removed (duplicated CONFIGURATION.md).

#### `bin/photogen` and `bin/decode`

Two new shell wrapper scripts committed to `bin/`. Each uses `REPO_ROOT` detection and `exec go run ...` so they work from any directory without a build step. Docs now show `bin/photogen` and `bin/decode` consistently for developer mode.

#### `TEST_ALBUM_*` Elimination

`TEST_ALBUM_LOCAL`, `TEST_ALBUM_PROD`, and `TEST_ALBUM_HYPHEN` removed from `site.env`. `bin/test-photos-server.sh` now fetches `albums.json` dynamically after establishing `BASE`, parses the first album slug via `jq` (falling back to `python3`), and skips album-specific tests gracefully if the fetch fails (encrypted site or server down). The two `TEST_ALBUM_HYPHEN` test blocks dropped entirely. `bin/rsync-test.sh`, `sample/config/site.env`, `config/site.example.env`, `docs/ENV.md`, and `docs/CONFIGURATION.md` all cleaned up.

#### `docker/init/description.txt` → `descriptions.txt`

The init scaffolding file was misnamed. Renamed the file; updated `docker/do-init.sh` and `docker/init/albums.yaml` to match.

#### Other Cleanup

- `site.env` examples (rsync and S3+CloudFront) added to `CONFIGURATION.md` and `config/site.example.env`
- `DEPLOY.md` personal-blog tone removed ("my server", "my origin server")
- `ENV.md` Site Identity section removed — it duplicated CONFIGURATION.md and predated the days when settings were passed via environment variables

### 65. 04/29/2026 - Export Command and Static Deployment

#### Motivation

S3 and rsync cover server-based deployments, but there was no path for users who want a simple static hosting service (e.g., Surge.sh, Netlify Drop, GitHub Pages). Adding an `export` command fills that gap and also gives a clean way to produce a self-contained local test directory without running a web server.

#### `export` Command

New `export` entrypoint (Docker mode: `./ddphotos export`; Developer mode: `bin/export.sh`) produces `export/<site-id>/` by calling `web/setup-htdocs.sh` to wire together the `build/` and `albums/` directories.

- Default output uses symlinks — fast and correct for local serving with `python3 -m http.server`
- `--copy` flag resolves all symlinks to real files via `/bin/cp -rL` — required for services that don't follow symlinks (Surge.sh, most upload-based hosts)
- Docker implementation in `docker/do-export.sh`; Developer implementation in `bin/export.sh`
- Both validate that `albums/<site-id>` and `build/<site-id>` exist before proceeding, with actionable error messages
- `make export` and `make sample-export` targets added to `Makefile`

#### Static Deployment Documentation (`docs/DEPLOY.md`)

New **Static Deployment** section documents the full workflow for Docker and Developer modes, with a dedicated **Surge** subsection covering the two-command deploy and a known limitation: image permalink URLs (e.g., `albums/antarctica/5`) 404 on Surge because it doesn't support URL rewriting.

#### SVG Deploy Tree Diagram

`docs/deploy-tree.svg` replaces inline ASCII art in `DEPLOY.md`. Generated by `bin/gen-deploy-tree.py` (Python, uses the `svgwrite` library via `requirements.txt`) and committed to the repo. `make gen-deploy-tree` regenerates it. The diagram shows the two-source structure (build output + album data) flowing into the four deployment targets (Apache/nginx, S3, Surge, Python).

#### About Dialog Wrapping Fix (Mobile)

On narrow mobile screens, the Docker image name (e.g., `dougdonohoe/ddphotos:v1.12.0`) was wrapping mid-word in the About dialog due to `word-break: break-all` on `.modal-body dd`.

- Changed to `word-break: break-word` — breaks only at natural word boundaries (`/`, `:`)
- Added `@media (max-width: 480px)` rule reducing `.modal-body` font size from `1.15rem` to `0.95rem`, giving enough width to keep the image name on one line
- Added `VITE_DOCKER_IMAGE` dev-server usage to `docs/DEV.md` so developers can test the dialog with a realistic image name: `VITE_DOCKER_IMAGE='dougdonohoe/ddphotos:v1.12.0' make sample-npm-run-dev`
