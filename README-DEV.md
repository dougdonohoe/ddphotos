# DD Photos Development Notes

## Introduction

This page covers the technical details of DD Photos for developers and those
who want to understand how the pieces fit together. Topics include the SvelteKit
frontend, environment configuration, all Makefile targets, `photogen` CLI flags
and output layout, photo descriptions and sort order, deployment, Apache routing,
and development tips.

## SvelteKit

SvelteKit is two things bundled together:

- **Svelte** — a UI component framework (a React alternative). Components are written in `.svelte`
  files combining HTML, CSS, and JavaScript. Unlike React, Svelte compiles components to vanilla
  JavaScript at build time with no virtual DOM and no runtime library shipped to the browser.
- **Kit** — the application framework built on top of Svelte, analogous to Next.js for React.
  It adds file-based routing (via `src/routes/`), data loading (`+page.ts`), adapters for different
  deployment targets, and the build pipeline via Vite.

What SvelteKit specifically does for this project:

- **Routing** — `src/routes/albums/[slug]/[[index]]/` becomes `/albums/antarctica/1` automatically
- **Data loading** — `+page.ts` fetches `albums.json` and `index.json` before the page renders
- **Component reactivity** — lightbox state, theme toggle, image loading effects
- **Build pipeline** — Vite bundles everything; `adapter-static` pre-renders all routes to `.html` files
- **Client-side navigation** — clicking between albums swaps content without a full page reload

The site is a hybrid of static and dynamic rendering:

- **Static**: the HTML shell (nav, footer, page structure) is pre-built at deploy time and served
  as plain files — no server generates pages on request
- **Dynamic**: `albums.json` and `index.json` are fetched by JavaScript in the browser after load;
  the photo grid, lightbox, and navigation are all rendered client-side from that JSON data

The JSON files themselves are static files, but their content is rendered in the browser. This
pattern is called CSR (Client-Side Rendering) with a static shell — the shell is pre-built, but
the content is rendered by JavaScript in the browser rather than on a server.

## LAN Access

When running the dev server (`make web-npm-run-dev`), you should see a Vite message listing 
the URLs where the site is accessible, typically http://localhost:5173 and any local network 
IPs (useful for testing on your phone or tablet).

Another way to get the LAN IP is as follows (helpful if running Apache, which doesn't print
out IPs):

```bash
# macOS
ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null

# Linux
hostname -I | awk '{print $1}'
```

## Simulating Slow Image Loading

Album pages fade images in as they load. On a fast local connection this is
imperceptible. To simulate slow loading and see the effect, append `?slow` to
any album URL:

```
http://localhost:5173/albums/your-album?slow
```

Each image's `src` is assigned after a random 500–2500ms delay, triggering a
real browser load cycle rather than just a visual trick. Works on production too:

```
https://photos.example.com/albums/your-album?slow
```

## Environment Variables

The `site.env` variables are:

| Variable                | Used by                     | Description                                                                             |
|-------------------------|-----------------------------|-----------------------------------------------------------------------------------------|
| `VITE_ALLOW_CRAWLING`   | Vite, Svelte                | Set to `true` to allow crawling and include `Sitemap:` in robots.txt (default: `false`) |
| `VITE_SITE_NAME`        | Vite, Svelte                | Site title shown in browser and OG tags                                                 |
| `VITE_SITE_URL`         | Vite, Svelte, bin/          | Canonical base URL (e.g. `https://photos.example.com`)                                  |
| `VITE_SITE_DESCRIPTION` | Vite, Svelte                | Meta description and OG description                                                     |
| `VITE_COPYRIGHT_OWNER`  | Vite, Svelte                | Footer copyright name                                                                   |
| `VITE_COPYRIGHT_YEAR`   | Vite, Svelte                | Footer copyright start year                                                             |
| `CLOUDFRONT_ID`         | `bin/deploy-photos.sh`      | CloudFront distribution ID for cache invalidation (deploy only)                         |
| `RSYNC_DEST`            | `bin/deploy-photos.sh`      | Rsync destination path on the server (deploy only)                                      |
| `TEST_ALBUM_LOCAL`      | `bin/test-photos-apache.sh` | Album slug used for local Apache tests                                                  |
| `TEST_ALBUM_PROD`       | `bin/test-photos-apache.sh` | Album slug used for production tests                                                    |
| `TEST_ALBUM_HYPHEN`     | `bin/test-photos-apache.sh` | Album slug with a hyphen (tests URL routing edge case)                                  |

The last five variables are only needed if using the deployment script. For local development,
only the `VITE_*` vars are required.

In the web app, `vite.config.ts` reads `config/site.env` at startup and injects `VITE_*` keys into `process.env`
before Vite runs, so the values are available as `import.meta.env.VITE_*` in Svelte components.
Multi-word values must be quoted (e.g. `VITE_SITE_NAME="My Photo Albums"`).

The `bin` scripts `source` the file directly.

The `SITE_ENV` environment variable overrides which `site.env` file is loaded. This is useful
when your config lives outside the repo (e.g. in a private config repo):

```bash
SITE_ENV=~/work/my-config/site.env make web-npm-run-dev
```

## Makefile Targets

Common tasks are available via `make` from the repo root:

| Target                       | Description                                                        |
|------------------------------|--------------------------------------------------------------------|
| `help`                       | Show all available make targets (default when running `make`)      |
| `build`                      | Compile all Go binaries                                            |
| `test`                       | Run Go unit tests                                                  |
| `mod-tidy`                   | Run `go mod tidy` to clean up imports                              |
| `clean-cache`                | Run `go clean -cache` (useful after a vips library upgrade)        |
| `vet`                        | Run `go vet` static analysis                                       |
| `web-nvm-install`            | Install the Node version specified in `web/.nvmrc`                 |
| `web-npm-install`            | Install npm dependencies in `web/`                                 |
| `web-npm-run-dev`            | Start Vite dev server and open browser                             |
| `web-npm-build`              | Build the static site into `web/build/`                            |
| `web-docker-build`           | Build the `photos-apache` Docker image                             |
| `web-docker-run`             | Run Apache on port 8080 with `web/` mounted as document root       |
| `web-docker-stop`            | Stop the running `photos-apache` container                         |
| `web-docker-test`            | Run `bin/test-photos-apache.sh` against `localhost:8080`           |
| `web-playwright-install`     | One-time setup: install `@playwright/test` and Chromium binary     |
| `web-playwright-test-apache` | Run Playwright e2e tests (starts Docker on port 8081, runs, stops) |
| `web-playwright-test-dev`    | Run Playwright e2e tests (against Vite dev server)                 |
| `use-sample`                 | Symlink `web/static/albums` → `../albums/sample`                   |
| `use-sample-pw-all`          | Symlink `web/static/albums` → `../albums/sample-pw-all`            |
| `use-sample-pw-uganda`       | Symlink `web/static/albums` → `../albums/sample-pw-uganda`         |
| `use-sample-css`             | Symlink `web/static/albums` → `../albums/sample-css`               |
| `use-prod`                   | Symlink `web/static/albums` → `../albums/prod`                     |
| `sample-photogen`            | Run photogen using `sample/config/albums.yaml`                     |
| `sample-photogen-pw-all`     | Run photogen using sample config, all albums password-protected    |
| `sample-photogen-pw-uganda`  | Run photogen using sample config, Uganda album password-protected  |
| `sample-photogen-css`        | Run photogen using sample config with custom CSS injected          |
| `sample-build`               | Build the static site using sample config                          |
| `sample-npm-run-dev`         | Run the Vite dev server using sample config                        |
| `sample-test-apache`         | Run Apache routing tests against Docker on port 8082               |
| `web-screenshots`            | Capture screenshots (requires a running server on port 8080)       |

## Generating Photos (`photogen`)

To resize photos and generate the JSON indexes, run `photogen`. Albums are
defined in a YAML config file (default: `config/albums.yaml`). See
[config/albums.example.yaml](config/albums.example.yaml) for the format.  

Album descriptions are in a TXT file (default: `config/descriptions.txt`).
See [config/descriptions.example.txt](config/descriptions.example.txt)
for the format.

#### Hero Image

An optional full-width banner image can be displayed at the top of the home page.
Add a `hero:` block under `settings:` in `albums.yaml`:

```yaml
settings:
  hero:
    image: my-banner.jpg   # filename; joined to 'base' if set, else relative to config dir
    base: drive            # optional — same base map as album entries
    crop: center           # top | center | bottom (default: center)
```

`photogen` hard-crops the source image to 1600×250px and writes it as `hero.jpg`
alongside `config.json` (generated when `-resize` is set). The hero is never
encrypted and takes priority as the `og:image` on the home page.

To regenerate the hero without reprocessing albums or rebuilding indexes, use
`-hero-only`. It always overwrites the existing `hero.jpg` regardless of `-force`:

```bash
go run cmd/photogen/photogen.go -hero-only -doit
```

#### Custom CSS

To override site styles, add a `css:` entry under `settings:`:

```yaml
settings:
  css: custom.css   # filename relative to this config dir
```

`photogen` copies the file to the site output as `custom.css` (generated when
`-index` is set). The frontend injects it site-wide as a `<link>` after the
built-in styles, so any rules inside it take effect as normal cascade overrides.
Redefining CSS custom properties (e.g. `--bg-color`, `--text-color-2nd`) is the
cleanest approach — no specificity battles needed.

```bash
go run cmd/photogen/photogen.go -resize -index -doit
```

To use a different albums file (e.g., a development subset):

```bash
go run cmd/photogen/photogen.go -albums albums-dev.yaml -resize -index -doit
```

### CLI Flags

| Flag          | Default       | Description                                                                                    |
|---------------|---------------|------------------------------------------------------------------------------------------------|
| `-config-dir` | `config`      | Directory containing the albums YAML and descriptions files                                    |
| `-albums`     | `albums.yaml` | Albums YAML filename within `-config-dir`                                                      |
| `-doit`       | `false`       | Write files; without this, runs in dry-run mode                                                |
| `-resize`     | `false`       | Generate resized WebP image variants                                                           |
| `-index`      | `false`       | Generate JSON index files and sitemap.xml                                                      |
| `-out`        | *(from YAML)* | Output directory override (overrides `settings.output_dir`)                                    |
| `-limit N`    | `0` (all)     | Limit photos per album (useful during development)                                             |
| `-force`      | `false`       | Regenerate files even if they already exist                                                    |
| `-workers N`  | `0` (auto)    | Concurrent resize workers (auto = NumCPU/2, min 2)                                             |
| `-album`      | `""` (all)    | Comma-separated album slugs to process                                                         |
| `-site-url`   | *(from YAML)* | Sitemap base URL override (overrides `settings.site_url`)                                      |
| `-site-id`    | *(from YAML)* | Override `settings.id`; useful for generating multiple output sites from one config            |
| `-passwords`  | *(from YAML)* | Path to passwords file; overrides `settings.passwords` (see [Passwords File](#passwords-file)) |
| `-css`        | *(from YAML)* | Path to custom CSS file; overrides `settings.css` (see [Custom CSS](#custom-css))              |
| `-clean`      | `false`       | Remove stale files from processed album directories after a run                                |
| `-hero-only`  | `false`       | Regenerate the hero image only; skips all album processing and index/JSON generation           |

`settings.id` is required and determines the output directory name (e.g. `id: prod`
produces `web/albums/prod`). It must contain only lowercase letters, digits, and hyphens.
The `-site-id` flag overrides this, which is useful when generating an encrypted variant
alongside the standard output from the same config.

Output goes to `{output_dir}/albums/{id}` (configured via `settings.output_dir` and
`settings.id` in the YAML; git-ignored). Set `output_dir: web` - the code appends
`albums/{id}` automatically. Do not set it to `web/static` or Vite will double-copy
the generated files during build.

Use `make use-sample` or `make use-prod` to point `web/static/albums` at the desired
output via symlink before running the dev server or building.

### Photo Descriptions (`photogen.txt`)

To add per-photo descriptions, create a `photogen.txt` file in the album's
source photo directory. One line per photo:

```
filename_without_extension Description text here.
# blank lines and lines starting with # are ignored
```

Example:
```
Patagonia-042 First view of Torres del Paine at sunrise.
Patagonia-107 Crossing the John Gardner Pass in the wind.
```

Descriptions are stored in `index.json` and used as:

- `alt` text on grid and lightbox images
- Hover caption overlay in the grid (desktop)
- Always-visible caption in the grid (mobile)
- Caption overlaid on the photo in the lightbox

To also use the file for **sort order** (instead of EXIF date), set
`manual_sort_order: true` on the album entry in `albums.yaml`. Photos not
listed in `photogen.txt` are sorted by date and appended at the end.

### Recursive Albums (`recurse: true`)

Set `recurse: true` on an album entry to collect photos from all subdirectories.
The output is flattened: each photo's ID and filename get a sanitized prefix
derived from its subdirectory path, preventing name collisions.

```
Craig's/img001.jpg      → ID: craigs_img001,       file: craigs_img001.jpg
Ski 2007/Alan's/a.jpg   → ID: ski2007_alans_a,     file: ski2007_alans_a.jpg
```

**Default sort order** (no `photogen.txt`): photos at the root sorted by date, then
subdirectories processed alphabetically, each date-sorted. No configuration needed.

**Per-subfolder `photogen.txt`**: place a `photogen.txt` in any subfolder for captions
and (with `manual_sort_order: true`) local sort order within that folder. Entries use
the bare filename without prefix — photogen applies the prefix automatically.

**Controlling inter-folder order**: with `manual_sort_order: true`, a `photogen.txt`
at any level can reference subfolder names as placeholders. Subfolder entries expand
inline, so you can freely interleave root photos and subfolder groups:

```
# photogen.txt at album root
photo_a.jpg
Craig's
photo_b.jpg
Halstead
```

Subfolders not listed in `photogen.txt` are appended alphabetically at the end with
a warning. Photos not listed are date-sorted and appended at the end of their group
with a warning.

**Cover photo**: when `cover` is set on a recursive album, use the prefixed filename
(e.g. `cover: craigs_img001.jpg`). If omitted, the first collected photo is used.
The prefixed filename is in the `fileName` field of `index.json`. To find it from an
original filename, use `sourcePath` (see below) or grep the decoded index.

**`sourcePath` field**: subfolder photos include a `sourcePath` field in `index.json`
with their original relative path from the album root (e.g. `"Craig's/img001.jpg"`).
This makes it easy to find the prefixed `fileName` for a given original file:

```bash
# plain album
grep -B2 "IMG_0436" web/albums/my-site/my-album/index.json

# encrypted album
go run cmd/decode/decode.go web/albums/my-site/my-album/index.enc.json | grep -B2 "IMG_0436"
```

### Passwords File

Encryption is enabled by pointing photogen at a YAML passwords file, either via
`settings.passwords` in `albums.yaml` (filename relative to the config dir) or the
`-passwords` CLI flag (absolute or relative path; overrides `settings.passwords`).
Comments (lines starting with `#`) are ignored.

```yaml
key: hmac-secret

site:
  password: site-wide-password
  hint: Optional hint shown in the password dialog

albums:
  album-slug:
    password: per-album-password
    hint: Optional hint shown in the album password dialog
```

| Field                    | Description                                                                                                                                                       |
|--------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `key`                    | HMAC-SHA256 secret used to derive UUID-format WebP filenames for encrypted albums, preventing filename guessing (e.g. `IMG_3961.webp` becomes `3f8a1c2d-...webp`) |
| `site.password`          | Encrypts `albums.json` and all per-album `index.json` files site-wide                                                                                             |
| `site.hint`              | Optional hint shown in the site-wide password dialog (always visible, even before a password attempt)                                                             |
| `albums.<slug>.password` | Per-album password; encrypts only that album's `index.json`. Falls back to `site.password` if not set                                                             |
| `albums.<slug>.hint`     | Optional hint shown in that album's password dialog                                                                                                               |

Sample passwords files are in `sample/config/` — `passwords-all.yaml` (full site) and
`passwords-uganda.yaml` (single album). Both contain demo-only passwords and a prominent WARNING header.

**Do not commit real passwords.** Store production passwords outside the repo or in a
git-ignored directory (e.g. `.secrets/`).

## Decoding Encrypted Files (`decode`)

The `decode` tool decrypts `.enc.json` files produced by `photogen` and prints the
plaintext JSON. Useful for inspecting what photogen wrote without running the full
frontend.

```bash
go run cmd/decode/decode.go <path.enc.json>
go run cmd/decode/decode.go -passwords <pw-file> <path.enc.json>
```

`photogen` embeds the passwords file path in every `.enc.json` it writes, so in most
cases no flags are needed:

```bash
go run cmd/decode/decode.go web/albums/sample-pw-uganda/uganda/index.enc.json
go run cmd/decode/decode.go web/albums/sample-pw-all/albums.enc.json
```

If the passwords file has moved, or the file was generated without an embedded path,
pass `-passwords` explicitly:

```bash
go run cmd/decode/decode.go -passwords sample/config/passwords-uganda.yaml \
  web/albums/sample-pw-uganda/uganda/index.enc.json
```

The correct password is selected automatically from the filename:

| File              | Password used                                    |
|-------------------|--------------------------------------------------|
| `albums.enc.json` | Site-wide password (`_all_`)                     |
| `index.enc.json`  | Per-album password for the parent directory slug |

## Testing

There are three ways of testing the website:

1. **Manual testing** in a browser, against the Vite dev server or a local static build (via Python or Docker/Apache)
2. **Playwright e2e tests** that drive a headless Chromium browser to verify UI behavior
3. **Apache routing tests** using `curl` to verify `.htaccess` URL routing, redirects, and 404 handling

All three are discussed below.

### Manual Testing - Dev

As seen in the [README](README.md), development is primarily done via
the Vite server. This is the easiest, as it automatically reloads when
any of the SvelteKit files change or even when `photogen` is re-run.

```bash
# Sample site
make sample-npm-run-dev

# Uses current web/static/albums symlink
make web-npm-run-dev

# Uses custom site.env
SITE_ENV=private/config/site.env make web-npm-run-dev
```

You should see a `VITE` message and a browser window should
open at [localhost:5173](http://localhost:5173/).

### Manual Testing - Build

As seen in the [README](README.md), the site has a build step:

```bash
# Sample site
make sample-build

# Uses default config/site.env
make web-npm-build

# Uses custom site.env
ln -sfn ../albums/private web/static/albums
SITE_ENV=private/config/site.env make web-npm-build
```

Once the site is built (into `web/build`), you can serve
it via Python or Docker/Apache.

### Manual Testing - Build Served via Python

If you have Python installed, this will serve up the site:

```bash
python3 -m http.server 8000 --directory web/build
```

Note: Python's server doesn't apply `.htaccess` rules, so URL routing won't
match Apache. Use the Docker setup below for accurate Apache testing.

### Manual Testing - Build Served via Docker/Apache

The Docker/Apache environment mirrors one possible production setup and applies
`.htaccess` routing locally. The `web` directory is mounted in the container (not
`web/build`) so that npm rebuilds (which delete and recreate `build`)
don't break the container's bind mount.

```bash
# One-time: build the Docker image
make web-docker-build

# Start Apache on port 8080 (runs in foreground; Ctrl-C to stop) 
# Site rebuilds do not require a restart
make web-docker-run
```

You should be able to see the site at [localhost:8080](http://localhost:8080).

### Automated Tests - Docker/Apache via Curl

If Docker/Apache is running, `make web-docker-test` runs 
`bin/test-photos-apache.sh --local 8080`, which tests URL routing, redirects, 
404 handling, photo permalink URLs, static asset accessibility,
and verifies asset paths in HTML are absolute (required for photo permalink
pages to render correctly).

```bash
make web-docker-test
```

You can also run the script directly, against production or locally:

```bash
bin/test-photos-apache.sh               # production ($VITE_SITE_URL)
bin/test-photos-apache.sh --local       # local Docker on port 8080
bin/test-photos-apache.sh --local 9090  # local Docker on custom port
```

The deployment script runs this script automatically after deploying.

### Automated Tests - Playwright E2E Tests

Playwright runs a real headless Chromium browser against the Docker/Apache
container, the dev server, or even a production server, testing JavaScript behavior 
that static HTML checks can't cover - specifically lightbox caption rendering across 
the different open paths.

```bash
# One-time setup (downloads ~100 MB Chromium binary)
make web-playwright-install

# starts a separate Docker/Apache on port 8081, runs tests, stops Docker
make web-playwright-test-apache

# runs against dev server (which must be running)
make web-playwright-test-dev
```

Tests are in `web/tests/` and cover:

| File                  | What it tests                                                                       |
|-----------------------|-------------------------------------------------------------------------------------|
| `smoke.spec.ts`       | Home page album listing, album page metadata, grid renders, Open Graph tags         |
| `captions.spec.ts`    | Lightbox caption rendering: grid click, permalink direct load, prev/next nav        |
| `url.spec.ts`         | URL updates on photo open/navigate/close; permalink URL preserved on load           |
| `navigation.spec.ts`  | Cross-album client-side navigation shows correct photos, title, description         |
| `back-nav.spec.ts`    | Browser back button behavior: closes lightbox, restores URL, handles reload         |
| `back-to-top.spec.ts` | Back-to-top button visibility and scroll behavior                                   |
| `password.spec.ts`    | Site/album prompts, wrong/correct passwords, remember on reload, hints, logout button, `?clear` |
| `css.spec.ts`         | Custom CSS `<link>` injection, `--text-color-2nd` override, album card border-radius           |

Smoke and caption tests assume the presence of albums in the sample website (`antarctica`, `uganda`).
Navigation tests are fully dynamic - they read album names from the page at runtime and
work against any site without hardcoding album names.

The `baseURL` defaults to `http://localhost:8080` (used by `deploy-photos.sh`)
and can be overridden via `PLAYWRIGHT_BASE_URL` - the Makefile target passes
`http://localhost:8081` to avoid port conflicts.

Password and CSS tests are gated by environment variables so they only run against
the appropriate site variant:

| Variable                  | Set by                  | Effect                                          |
|---------------------------|-------------------------|-------------------------------------------------|
| `PLAYWRIGHT_PASSWORDS_FILE` | `bin/run-tests.sh`    | Path to passwords file; enables password tests  |
| `PLAYWRIGHT_CUSTOM_CSS`     | `bin/run-tests.sh`    | Set to `true`; enables CSS tests                |

Use `bin/run-tests.sh` or `bin/test-all.sh` to run tests across all variants automatically.
`bin/test-all.sh` runs four variants: no passwords, `passwords-all.yaml`, `passwords-uganda.yaml`,
and `custom-css` (with `sample/config/custom.css` injected).

The `bin/deploy-photos.sh` script runs Playwright automatically: locally before rsync,
and against production after CloudFront cache invalidation.

## Apache

If using Apache, the `VirtualHost` definition must specify the `ErrorDocument` and
allow use of `.htaccess` files (`AllowOverride All`):

```text
<VirtualHost *:80>
    ServerName photos.example.com
    DocumentRoot /my/www
    ErrorDocument 404 /404.html

    <Directory /my/www>
      AllowOverride All
    </Directory>
</VirtualHost>
```

### .htaccess

The `.htaccess` file (`web/static/.htaccess`) configures URL routing:

- **`DirectorySlash Off`** - Prevents Apache from auto-appending trailing slashes to directories
- **Trailing slash redirect** - 301 redirects URLs with trailing slashes to their clean version
  (e.g., `/albums/patagonia/` -> `/albums/patagonia`)
- **HTML rewrite** - Serves `.html` files without the extension
  (e.g., `/albums/patagonia` serves `patagonia.html`)
- **Photo permalink rewrite** - Serves album HTML for photo permalink URLs
  (e.g., `/albums/patagonia/15` serves `patagonia.html`; JS reads the path and opens the lightbox)
- **SPA fallback** - Unknown root-level paths fall back to `index.html` for client-side routing

## Deployment

DD Photos was originally built to serve my personal photo albums.  I happened
to have my own EC2 instance with Apache for my other websites, so it was easy
to add another one.

Traffic to [photos.donohoe.info](https://photos.donohoe.info) is handled by CloudFront, which filters 
requests through a WAFv2 web ACL before forwarding clean traffic to the Apache 
origin on EC2.

```mermaid
flowchart LR
    User -->|HTTPS| WAF["WAFv2 Web ACL"]
    WAF --> CF["CloudFront CDN"]
    CF -->|HTTP| Apache["EC2 / Apache"]
```

The WAF (Web Application Firewall) inspects every incoming request and blocks 
suspicious or malicious traffic (things like bots or known bad IP addresses)
before it ever reaches my server.

The CDN (Content Delivery Network) caches my content at edge locations around 
the world so visitors get fast load times regardless of where they are,
and my origin server handles far less traffic.

The deployment script (described below) builds the static site and rsyncs it to 
my EC2 instance behind CloudFront.  It is specific to my setup, but it is
parameterized via `site.env` so that others with a similar setup can re-use it.
It can also be extended or changed to suit your needs.

### Deploy Script

The included deployment script assumes the site is running from an EC2
server with `ssh` access and is using a CloudFront CDN.  It uses the `CLOUDFRONT_ID`,
`RSYNC_DEST` and `VITE_SITE_URL` variables from `site.env`.  It also
assumes `AWS_APACHE` is in the environment and specifies an accessible IP to your EC2 instance.

To deploy, I run `bin/deploy-photos.sh`, which:

1. Runs `photogen` to resize images and generate JSON (skip with `--no-photogen`)
2. Builds the static site via `npm run build` into `web/build`
3. Starts the Docker/Apache container if not already running, runs
   `bin/test-photos-apache.sh --local` to verify routing locally, then stops the container
4. Runs Playwright tests against Docker/Apache (skip with `--no-playwright`)
5. Rsync `web/build` to the `$RSYNC_DEST` directory on the EC2 server (`$AWS_APACHE`).
   It uses `--checksum` to reduce unnecessary re-copying since Vite resets timestamps.
6. Invalidates the CloudFront cache (`$CLOUDFRONT_ID`)
7. Runs `bin/test-photos-apache.sh` to verify the deployment against production
8. Runs Playwright tests against production (`$VITE_SITE_URL`) (skip with `--no-playwright`)

The script uses `set -eo pipefail` - any failure (including local tests) aborts before rsync.

```bash
bin/deploy-photos.sh                          # full deploy
bin/deploy-photos.sh --no-photogen            # skip photo generation
bin/deploy-photos.sh --no-rsync               # build + local test only, no deploy (safe on a dev machine)
bin/deploy-photos.sh --no-playwright          # skip Playwright tests (local and production)
bin/deploy-photos.sh --no-photogen --no-rsync # build + local test, skip both photogen and rsync
```

## CI (GitHub Actions)

The workflow in `.github/workflows/ci.yml` runs on every pull request to `main`. It:

1. Installs `libvips-dev` and `pkg-config` via `apt-get`
2. Sets up Go (version from `go.mod`) and Node (version from `web/.nvmrc`)
3. Runs `make build test vet`
4. Runs `make sample-photogen` to resize sample photos and generate JSON
5. Builds the Docker image and sample site
6. Runs Apache routing tests (`make sample-test-apache`)
7. Runs Playwright e2e tests (`make web-playwright-test-apache`)

### Testing CI Locally with `act`

It is often helpful to run GitHub CI locally using [`act`](https://nektosact.com/).
It requires Docker. Before running, there is one key prerequisite and one important caveat to understand:

```bash
# Prerequisite: generate and build sample site before running `act`
make web-docker-build sample-photogen sample-build

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
