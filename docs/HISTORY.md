# Photo Album Project - Session Summary

For current project documentation, see [`README.md`](../README.md).

This file is a historical log of work done with ChatGPT and Claude Code.

## Original Prompt

This project started with ChatGPT and this prompt:

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

### 1. WebP Image Format
- Changed from JPEG to WebP output for smaller file sizes
- Updated `resize.go` to use `ExportWebp()` instead of `ExportJpeg()`
- Added `WebPFileName()` helper to convert extensions
- Updated JSON paths to use `.webp`

### 2. Removed Unused Thumbnail Size
- Removed `SizeThumb` (200px) - was never used
- Now only generates two sizes: `grid` (600px) and `full` (1600px)

### 3. Static Site Deployment Setup
- Installed `@sveltejs/adapter-static`
- Updated `svelte.config.js` for static export
- Created `src/routes/+layout.ts` with `prerender = true`
- Build outputs to `photos/build/`

### 4. Production Readiness Fixes
- **Error handling**: Added try/catch to JSON fetch calls in `+page.ts` files
- **Error page**: Created `src/routes/+error.svelte` with styled 404/error display
- **Meta description**: Added to `app.html`
- **Image fallback**: Added `background: var(--bg-secondary)` to images for loading/error states

### 5. Sitemap Generation
- Added `WriteSitemap()` function to `json.go`
- Added `-site-url` flag (defaults to `https://photos.example.com`)
- Generates `sitemap.xml` alongside `albums.json` when `-index` flag is used

### 6. Mobile Title Size
- Added media query to reduce h1 font-size to 1.8rem on screens ≤480px

### 7. Copyright Footer
- Added footer to `+layout.svelte` with dynamic year
- Text: "Copyright © 2001-{currentYear}. Doug and Cindy Donohoe."

### 8. URL Handling
- Created redirect from `/albums` and `/albums/` to `/`
- Created `static/404.html` for Apache custom error page

### 9. Concurrent Image Resizing
- Added `pkg/photogen/resize_worker.go` implementing `pool.Worker[resizeWork]` for concurrent resizing
- `ResizePhotos()` now uses the worker pool instead of sequential processing
- Auto-detects worker count: `NumCPU() / 2` (min 2), configurable via `-workers` flag
- Added `Workers` field to `Config`
- *(Superseded by §30 — pool replaced with simpler goroutine implementation)*

### 10. CLI Improvements
- Added `-album` flag to filter processing to specific albums (comma-separated slugs)
- Added `-workers` flag to control concurrent resize workers
- Added EXIF date warning: logs count of photos missing dates per album

### 11. Higher Resolution Favicon
- Generated multi-size favicon from 512x512 source PNG
- `favicon.ico` (16/32/48px), `favicon-32.png`, `favicon-192.png` (Android), `apple-touch-icon.png` (iOS)
- Updated `app.html` with proper icon link tags

### 12. Apache URL Routing
- Added `.htaccess` with `DirectorySlash Off`, trailing-slash redirect (301), and `.html` file rewrite
- Set `trailingSlash: 'ignore'` in `+layout.ts` to let Apache handle URL normalization
- `/albums` serves redirect page, unknown paths fall back to SPA shell

### 13. Gallery UX Improvements
- Increased `max-width` from 1200px/1400px to 2000px on album list and gallery pages
- ESC key on album page navigates back to album list (with guard to not conflict with PhotoSwipe ESC)
- Back-to-top button arrow centering: responsive `padding-bottom` (2px mobile, 5px desktop)

### 14. Deploy Script Enhancements
- Added `--no-photogen` flag to skip image generation
- Added `--checksum` to rsync to skip unchanged files (Vite resets timestamps)
- Added post-deploy Apache test (`bin/test-photos-apache.sh`)

### 15. Apache Verification Script
- Created `bin/test-photos-apache.sh` to validate URL routing, redirects, 404s, and static assets
- Accounts for CloudFront SSL termination (redirect locations use `http://`)
- Runs automatically after deploy

### 16. WebP Metadata
- Added comment documenting that all metadata is stripped from WebP output (smaller files, no GPS leak)
- Photo metadata preserved in JSON index files

### 17. Photo Descriptions via `photogen.txt`
- Added `photogen.txt` support: optional file in the album source directory, one line per photo: `filename_without_extension Description text`
- Blank lines and `#` comments ignored
- Added `Description string` field to `Photo` and `PhotoIndex` (JSON: `omitempty`)
- Added `ManualSortOrder bool` to `AlbumConfig`: when true, photo order follows `photogen.txt` sequence
- Unmentioned photos warned about, sorted by date, and appended at the end
- Unknown IDs in `photogen.txt` produce a warning and are skipped
- `Photo.String()` logging now appends description if present
- Added `TestLoadPhotoDescriptions` and `TestReorderByDescriptionFile` in `album_test.go`
- Updated `testdata/index.json` to include a description on one photo; added assertions in `json_test.go`

### 18. Photo Matching Tool (`cmd/photomatch`)
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

### 19. Caption Display in Frontend
- Descriptions used as `alt` text on grid images and in PhotoSwipe lightbox (fallback to filename)
- **Grid hover caption** (desktop): `position: absolute` overlay at photo bottom with gradient, fades in on `:hover`
- **Grid always-on caption** (mobile): `@media (hover: none)` overrides opacity to always show
- **Lightbox caption**: injected into each of PhotoSwipe's 3 `itemHolders` elements (`pswp.mainScroll.itemHolders`) so captions swipe physically with their photo
  - Position computed mathematically: `scale = min(viewW/w, viewH/h)`, `bottom = (viewH - h*scale) / 2`
  - Caption elements queried via `holder.el.querySelector('.pswp-caption')` at update time — a parallel index array breaks after the first swipe because PhotoSwipe rotates the `itemHolders` array in place
  - `change` handler defers via `requestAnimationFrame` so PhotoSwipe finishes assigning slide holders before captions are updated
  - `slide.index` used to look up data from local `photoswipeItems` array; `slide.data` may not be populated when `change` fires

### 20. Open Graph Tags & Social Sharing
- Added Open Graph meta tags to home page and album pages: `og:title`, `og:description`, `og:type`, `og:url`, `og:image`, `og:site_name`, `twitter:card`
- Home page uses first album's cover image; album pages use the first photo's grid thumbnail
- Description format: `"N photos from the 'Album Name' Donohoe photo album"`

### 21. Album Date Span on Album Page
- Album page header now shows date span (e.g., "47 photos · Jan – Mar 2005")
- `albums/[slug]/+page.ts` fetches `albums.json` in parallel with `index.json` and plucks `dateSpan` for the matching slug — no changes to JSON generation needed

### 22. Photo Permalinks (`/albums/slug/N`)
- Replaced hash-based approach (`#photo-15`) with clean path-based URLs (`/albums/antarctica/15`)
- Route restructured from `albums/[slug]/+page.svelte` to `albums/[slug]/[[index]]/+page.svelte` (SvelteKit optional param)
- `+page.ts` extracts `params.index`, converts to 0-based `photoIndex`, passes to component
- `openLightbox()` sets URL via `history.replaceState` on path (not hash) — no `hashchange` events fire, eliminating the infinite-loop risk that plagued the hash approach
- On close, URL is restored to `/albums/slug`
- On mount, if `data.photoIndex` is set, lightbox opens immediately (no animation — `showAnimationDuration: 0` — for instant feel on direct load)
- `.htaccess` updated with new rewrite rule: `albums/slug/N` → `albums/slug.html`
- `paths.relative: false` added to `svelte.config.js` so SvelteKit generates absolute asset paths (`/_app/...` instead of `../_app/...`) — required because album HTML is now served at a deeper path depth

### 23. Docker Apache Test Environment
- Added `photos/Dockerfile` based on `httpd:2.4`; `sed` enables `mod_rewrite`, sets `AllowOverride All`, points `DocumentRoot` at `htdocs/build/`, adds `ServerName localhost`
- Volume mounts `photos/` (not `photos/build/`) so npm rebuilds — which delete and recreate `build/` — don't break the container's bind mount inode
- New Makefile targets: `photos-docker-build`, `photos-docker-run`, `photos-docker-stop`, `photos-docker-test`
- `bin/test-photos-apache.sh` updated with `--local [port]` flag: switches `BASE`/`REDIRECT_BASE` to `http://localhost:PORT`, uses `antarctica` as test album
- Added `check_body` helper to test script; new test verifies photo permalink HTML contains absolute `/_app/immutable` asset paths (would have caught the `paths.relative` bug)
- Added photo permalink routing tests (200 for `/albums/antarctica/1` and `/10`, 301 for trailing slash)

### 24. Copy-Link Button in Lightbox
- Injected a chain-link SVG button into PhotoSwipe's `.pswp__top-bar` DOM after `pswp.init()`, just left of the close button — avoids `uiRegister`/`registerElement` which caused scroll-lock bugs in earlier attempts
- Copies `window.location.href` (always current due to `replaceState` calls) to clipboard via `navigator.clipboard.writeText()`
- On success, icon swaps to a green checkmark for 1.5 seconds then reverts — the standard GitHub/Notion feedback pattern
- Silent no-op if clipboard API is unavailable (old browser or denied permission)

### 25. Album Descriptions
- Added `Description string` to `AlbumConfig` (Go) and `AlbumSummary` JSON (`omitempty`)
- `GetAlbumSummary()` propagates description into the JSON output
- Single source of truth: `albumDescriptions map[string]string` in `photogen.go`, keyed by slug; `applyDescriptions()` populates both `defaultAlbums()` and `defaultAlbumsLaptop()` after config is built — eliminates duplication between the two config functions
- All 21 production albums have real descriptions; `tbd` constant used as placeholder for any new albums added in the future
- `og:description` on album pages now uses the album description when available, falling back to the generated `"N photos from the 'Album Name'..."` string
- Frontend (`+page.ts`) passes `description` through alongside `dateSpan`
- **Home page cards**: description on its own line below the title; styled slightly brighter (`--text-color-2nd`, `opacity: 0.8`) and larger (`0.95rem`) than the muted meta line to create visual hierarchy; meta line (`N photos · dateSpan`) pinned to card bottom via `margin-top: auto` in a flex column layout, right-aligned and italic
- **Album detail page**: single meta line formatted as `Description\u00A0|\u00A0N photos · dateSpan`
- Updated `testdata/albums.json` with a description on the `way` entry; added assertions in `TestLoadAlbumSummaries` for both present and absent descriptions

### 26. Canonical URL Tags
- Added `<link rel="canonical">` to `<svelte:head>` on both the home page and album pages
- Album page canonical always points to `/albums/slug` (not `/albums/slug/N`) so photo permalinks don't fragment search engine ranking signals

### 27. SvelteKit Client-Side Navigation & Lightbox Caption Fixes

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

### 28. Playwright E2E Tests
- Added `@playwright/test` as a dev dependency in `photos/package.json`
- `photos/playwright.config.ts`: `baseURL` defaults to `http://localhost:8080`, overridable via `PLAYWRIGHT_BASE_URL` env var — no `webServer` config since tests run against the existing Docker Apache container
- New Makefile targets: `photos-playwright-install` (one-time `npm install && npx playwright install chromium`), `photos-playwright-test` (starts Docker on port **8081** to avoid conflict with deploy script's port 8080, runs tests, stops Docker, preserves exit code).
- `bin/deploy-photos.sh` runs Playwright tests at two points: locally against the Docker container before rsync, and against production (`PLAYWRIGHT_BASE_URL=https://photos.example.com`) after CloudFront cache invalidation.
- All test albums reference the dev/prod overlap set (`antarctica`, `honeymoon`, `uganda`) so tests run correctly in both environments.
- **`tests/captions.spec.ts`** (3 tests): grid click open (`animate=true`), permalink direct load (`animate=false`), prev/next navigation. Uses `locator('.pswp-caption', { hasText })` to avoid Playwright strict-mode errors — PhotoSwipe keeps 3 caption elements in the DOM simultaneously, one per holder.
- **`tests/url.spec.ts`** (4 tests): opening a photo updates URL to `/albums/slug/N`; navigating prev/next advances the URL; closing lightbox restores URL to `/albums/slug`; direct permalink URL is preserved on load.
- **`tests/navigation.spec.ts`** (3 tests): cross-album navigation (click back → click new album card) shows correct title/description; lightbox URL reflects the new album; chaining through three albums maintains correct state. Directly exercises the `$effect` stale-imageSrcs bug.
- **`tests/smoke.spec.ts`** (5 tests): home page lists known overlap albums; album cards show descriptions; album page renders title, description, and photo count; grid photos are visible; Open Graph tags are correct.

### 29. YAML-Based Album Configuration

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

### 30. Simplified Concurrent Resizing & Worker Count Encapsulation

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

### 31. Rename `photos/` → `web/`

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

### 32. Externalize Hardcoded Site Values (Phase 2 open-source prep)

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

### 33. Repo Extraction to Standalone `photos` Repo

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

### 34. Sample Albums, Multi-Site Output, and Phase 5 Setup

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

### 35. Footer: Repo Link and Build Timestamp

Added a second footer line: `Built with dougdonohoe/photos on March 5th, 2026 at 11:26 AM`.

**`web/vite.config.ts`** — injects `process.env.VITE_BUILD_TIME = new Date().toISOString()` immediately after `loadSiteEnv()`. Captured at Vite startup so the timestamp reflects when the build began.

**`web/src/app.d.ts`** — added `VITE_BUILD_TIME: string` to `ImportMetaEnv`.

**`web/src/routes/+layout.svelte`**:
- `ordinal(n)` helper: maps a day number to its ordinal string (`1st`, `2nd`, `3rd`, `4th`…)
- `formatBuildTime(iso)`: formats the ISO timestamp as `"Month Dth, YYYY at H:MM AM/PM"` using `Intl`-based locale formatting
- Footer now renders two `<div>`s: the existing copyright line, and the new build line with `margin-top: 0.35rem` for visual separation
- Link color in dark mode overridden to `#5a8ec0` (darker than the default powder-blue `#88b4e7`); light mode uses the standard `--link-color`

**`web/tests/captions.spec.ts`** — decoupled from specific caption text: replaced the hardcoded `CAPTION = 'Iceberg, right ahead!'` assertion with an `expectCaptionVisible()` helper that matches any non-empty `.pswp-caption` element. Tests now pass against both sample and prod sites regardless of photo sort order.

### 36. npm audit fix

Ran `npm audit fix` in `web/`. Fixed 3 of 7 vulnerabilities (upgraded `svelte`, `rollup`, `minimatch`, `ajv`, `devalue`). 4 low severity vulnerabilities remain, all `cookie`-related inside `@sveltejs/kit` — SSR-only attack surface with no relevance to a statically generated site. The fix would require `--force`, which would downgrade `@sveltejs/kit` to `0.0.30` (a breaking ancient version). Already on the latest kit release; waiting on an upstream fix.

### 37. Screenshot Script and SCREENSHOTS.md

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

### 38. GitHub Actions CI

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

### 39. README-DEV.md Review and robots.txt Route

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

### 40. Back-to-Top Button on Album List Page + Shared Component Refactor

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
- Home page tests use `toPass` with retries to handle the dev-server parallel execution race where the scroll listener may not yet be registered
