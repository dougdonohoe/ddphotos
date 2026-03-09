# PHOTOS.md — Static Photo Albums (Architecture Brief)

_**NOTE:**_ _This is the original architecture brief created by ChatGPT.
The actual DD Photos site is pretty close to the original vision.
Keeping this around as historical documentation._

## Goal

Build a **blazing-fast, static, mobile-friendly** photo browser that feels like Apple Photos shared albums—without the 15–20s lag. No server API. Everything prebuilt, cached, and lazy-loaded.

## High-Level Stack

* **Prebuild:** Go tool walks folders → emits images at multiple sizes + JSON indices. Uses `libvips`
* **Frontend:** **SvelteKit** with `adapter-static` (or Astro + Svelte islands if you want even less JS).
* **Lightbox:** **PhotoSwipe** (mature gestures, zoom, keyboard, A11y).
* **Hosting:** Any static host (Apache + CloudFront is perfect).

## Content Model (per Album)

* Input: each folder = one album (e.g., `albums/galapagos/`).
* Output:

    * `albums.json` → list of albums with `{ slug, title, count, cover, dateSpan }`.
    * `albums/<slug>/index.json` → ordered photos with precomputed dimensions & variants.
    * Images:

        * `thumb/` (menu cards, ~200–320px short side)
        * `grid/` (album grid, ~800–1200px long side)
        * `full/` (viewer, ~1600–2048px long side)
        * **Formats:** AVIF/WebP + JPEG fallback
        * **Names:** hashed (`IMG_1234.a1b2c3.avif`) for immutable caching
    * **Placeholders:** BlurHash or tiny LQIP + dominant color

> **Keep** original file names in JSON for traceability, but serve hashed assets for caching.

## Frontend Routes (bookmarkable)

* `/` → Albums menu (cards with cover thumb, title, count)
* `/a/[slug]` → Album grid (masonry/justified layout)
* `/a/[slug]/p/[i]` → Photo viewer (deep links; Back/Forward works)

## Performance Playbook

* **Zero API at runtime**: fetch static JSON only.
* **No CLS**: precompute `w/h` in JSON; use CSS `aspect-ratio`.
* **Lazy everything**: `loading="lazy"`, `decoding="async"`, IntersectionObserver to preload just-ahead.
* **Preload neighbors** in lightbox (±2).
* **Srcset/sizes** tuned for breakpoints; never ship “full” into the grid.
* **Optional SW** (Workbox): cache current album’s grid images + next/prev fulls for instant feel/offline.

## Image Size Tiers (baseline)

* **thumb**: 256px short side (menu)
* **grid**: 1024px long side (retina-friendly grid without bloat)
* **full**: 2048px long side (phones fill; desktop looks crisp)
* Encode **AVIF quality ~45–55**, **WebP ~75**, **JPEG ~75**; adjust after visual test.

## JSON Shapes (example)

* `albums.json` item:

  ```json
  { "slug":"galapagos","title":"Galápagos 2024","count":186,"cover":"albums/galapagos/cover.a1b2.avif","dateSpan":"2024-05" }
  ```
* `index.json` photo item:

  ```json
  {
    "id":"IMG_0001",
    "w":4032,"h":3024,"date":"2024-05-14",
    "caption":"Blue-footed booby",
    "src":{"thumb":".../thumb/IMG_0001.a1b2.avif","grid":".../grid/IMG_0001.b2c3.avif","full":".../full/IMG_0001.c3d4.avif"},
    "lqip":"data:image/jpeg;base64,...",
    "dominant":"#1f2a3c"
  }
  ```

## Grid Layout Options

* **Simplest:** CSS multi-column (fast to implement, good enough).
* **Premium:** Justified rows (Flickr-style). Heavier logic, best visual balance.
* Add virtualization (e.g., `svelte-virtual`) only if albums exceed ~1k photos.

## Input & Metadata

* Extract EXIF (date/orientation), store canonical orientation in JSON, rotate at build time.
* Optional sidecar YAML/JSON per album for **title/cover selection/order/captions** overrides.

## Caching & Headers

* **Images/JS/CSS:** `Cache-Control: public, max-age=31536000, immutable`
* **JSON indices:** `max-age=300` (or your tolerance); include content hash in filename if you want longer caches.
* Enable **HTTP/2, Brotli**, and **ETag**.

## Accessibility & UX

* PhotoSwipe covers focus trap, ARIA, keyboard (←/→/Esc).
* Respect device orientation; “contain” with neutral background.
* Visible **keyboard help** (`?`) and clear hit targets on mobile (tap left/right).

## Build Pipeline (Go)

In `cmd/photogen`, a `photogen.go` file is root.  Use existing or add new packages in
`pkg` where appropriate (`cmd` should be fairly light, as shown in existing code).

1. Initial list of albums hard-coded in `photogen.go` (on first pass, generate 3 examples in `/tmp`, I'll adjust)
2. Walk albums → collect files.
3. Read EXIF, compute canonical dimensions.
4. Generate **thumb/grid/full** variants (AVIF/WebP/JPEG) + hashes.  Use 'libvips' (see Dependencies)
5. Compute BlurHash/LQIP + dominant color.
6. Emit `index.json` per album + `albums.json`.
7. (Optional) Write a **manifest** for quick rebuilds (skip unchanged).

## Dependencies

Brew dependencies:

```bash
# Core
brew install vips

# Strongly recommended codecs for your use-case
brew install libheif libavif webp

# (Optional) extras you might ingest
brew install imagemagick exiftool ffmpeg
```

Make sure `pkg-config` can find the brew-installed libs:

# Apple Silicon (most likely for you)

```bash
export PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig"
export CPATH="/opt/homebrew/include:${CPATH}"
export LIBRARY_PATH="/opt/homebrew/lib:${LIBRARY_PATH}"
export DYLD_LIBRARY_PATH="/opt/homebrew/lib:${DYLD_LIBRARY_PATH}"
```

If you’re on Intel, swap /opt/homebrew → /usr/local.

## Roadmap

* v1.0: Generate images (Go tool)
* v1.1: Menu → Grid → Lightbox, lazy load, hashed assets, static deploy.
* v1.2: Justified grid, neighbor prefetch, SW caching.
* v1.3: Per-album config (covers/order/captions), favorites, keyboard help overlay.
* v1.4: Simple share links, optional password gate (static token page).

**North star:** tiny JS, prebuilt media, instant navigation. Keep the browser dumb and the build smart.
