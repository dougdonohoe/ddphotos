# Web Server Configuration

DD Photos uses extensionless URLs (`/albums/patagonia`), photo permalink paths
(`/albums/patagonia/15`), and an SPA fallback for unknown routes. Web servers don't handle
any of these by default — each server needs URL rewriting rules to map these paths to the
correct `.html` files. The configurations below provide those rules for Apache, nginx,
CloudFront, Cloudflare Pages, and Surge.

## Local Testing with Python

The `export` command creates `export/<site-id>/` — a directory of symlinks combining
build output and album data into a single tree that any static file server can read
without the routing configuration required by Apache or nginx.

**Docker mode** (after `ddphotos build`):

```bash
ddphotos export
python3 -m http.server 8000 --directory export/my-photos
```

**Developer mode** (after `make sample-build`):

```bash
make sample-export
python3 -m http.server 8000 --directory export/sample
```

**Limitation:** Python's built-in server has no URL-rewriting capability, so extensionless
album URLs (`/albums/patagonia`) and photo permalinks (`/albums/patagonia/15`) won't resolve when entered directly.
The home page and any directly-typed `.html` URLs will load correctly. Use `ddphotos serve`
or `make web-docker-run-apache` for full routing.

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

- **Cache headers** — JSON files get `Cache-Control: no-cache` (content can change in-place);
  WebP files get `Cache-Control: max-age=31536000, immutable` (UUID filenames, never change)
- **`DirectorySlash Off`** - Prevents Apache from auto-appending trailing slashes to directories
- **Trailing slash redirect** - 301 redirects URLs with trailing slashes to their clean version
  (e.g., `/albums/patagonia/` -> `/albums/patagonia`)
- **HTML rewrite** - Serves `.html` files without the extension
  (e.g., `/albums/patagonia` serves `patagonia.html`)
- **Photo permalink rewrite** - Serves album HTML for photo permalink URLs
  (e.g., `/albums/patagonia/15` serves `patagonia.html`; JS reads the path and opens the lightbox)
- **SPA fallback** - Unknown root-level paths fall back to `index.html` for client-side routing

## nginx

Unlike Apache, nginx needs no per-directory config file — all routing rules live in
`web/nginx.conf`, which is baked into the Docker image. `web/nginx-entrypoint.sh`
symlinks the active build into the document root at container startup (same role as
`web/apache-entrypoint.sh`).

### nginx.conf

- **Cache headers** — JSON files get `Cache-Control: no-cache`; WebP files get `Cache-Control: max-age=31536000, immutable`
- **Trailing slash redirect** — 301 redirects URLs with trailing slashes to their clean version
  (e.g., `/albums/patagonia/` → `/albums/patagonia`)
- **Photo permalink rewrite** — Serves album HTML for photo permalink URLs
  (e.g., `/albums/patagonia/15` serves `patagonia.html`; JS reads the path and opens the lightbox)
- **HTML rewrite** — Serves `.html` files without the extension
  (e.g., `/albums/patagonia` serves `patagonia.html`)
- **SPA fallback** — Unknown root-level paths fall back to `index.html`; deeper unknown paths return 404

## CloudFront Function

When serving from S3+CloudFront, a CloudFront Function replaces the web server routing rules.
It runs at the **viewer-request** stage, rewriting URLs before S3 is ever contacted — no
round-trip cost.

For a SvelteKit `adapter-static` site like DD Photos, a function is **required** to handle:

- **URL routing** — extensionless paths like `/albums/patagonia` map to `patagonia.html`; the root
  `/` maps to `index.html`; unknown root-level paths fall back to `index.html` (SPA fallback)
- **Photo permalinks** — `/albums/slug/42` maps to `/albums/slug.html` so the album page can open
  the lightbox to photo 42 via the URL hash
- **Domain redirects** — apex-to-www (`example.com` → `www.example.com`) and any other domain consolidation

Here is a minimal function for a SvelteKit-based photo site (see also the [Cloudflare Pages Worker](#cloudflare-pages-worker) below, which handles the same routing for Cloudflare deployments):

```javascript
function handler(event) {
    var request = event.request;
    var uri = request.uri;

    // Root
    if (uri === '/') {
        request.uri = '/index.html';
        return request;
    }

    // Photo permalink: /albums/slug/42 → /albums/slug.html
    var photoPermalink = uri.match(/^\/albums\/([^\/]+)\/\d+$/);
    if (photoPermalink) {
        request.uri = '/albums/' + photoPermalink[1] + '.html';
        return request;
    }

    // Extensionless paths
    if (!uri.includes('.')) {
        if (uri.indexOf('/', 1) === -1) {
            // Root-level single-segment (/about, /unknown-page) → SPA fallback
            request.uri = '/index.html';
        } else {
            // Deeper path (/albums/slug) → pre-rendered .html page
            request.uri = uri + '.html';
        }
        return request;
    }

    return request;
}
```

## Cloudflare Pages Worker

When deploying to [Cloudflare Pages↗](https://pages.cloudflare.com), a `_worker.js` in the
export root handles URL routing — the equivalent of the CloudFront Function above.

The worker handles three cases not covered natively by Cloudflare Pages:

- **Photo permalinks** — `/albums/slug/42` → serves `/albums/slug.html` via `env.ASSETS.fetch()`,
  keeping the URL unchanged so the JS can read the photo index and open the lightbox
- **Photo permalink trailing slash** — `/albums/slug/42/` → 308 redirect to `/albums/slug/42`
- **Root-level SPA fallback** — unknown single-segment paths (e.g. `/nope`) → serves `index.html`
  so the client-side router can handle 404 display

All other routing — extensionless album URLs, static assets, `404.html` — is handled natively
by Cloudflare Pages.

`ddphotos export --cloudflare` (or `export.sh --cloudflare`) copies
[docker/cloudflare-worker.js](../docker/cloudflare-worker.js)
into the export root as `_worker.js` automatically.

To verify a Cloudflare Pages deployment with `bin/test-photos-server.sh`, pass `--cloudflare`
so the script expects 308 (not 301) for trailing slash redirects and HTTPS in Location headers:

```bash
bin/test-photos-server.sh --remote https://your-site.pages.dev --cloudflare
```

## Surge

[Surge↗](https://surge.sh) uses a `200.html` SPA fallback for all unmatched paths. There is
no server-side routing, so some behaviors differ from Apache/nginx/Cloudflare Pages:

- **Bad album slugs return 200** — `/albums/doesnotexist` returns the SPA shell (200) instead
  of the custom 404 page. The SvelteKit router handles the 404 display client-side.
- **No photo permalink trailing slash redirect** — `/albums/slug/1/` is handled by the SPA
  router rather than a server-side redirect; the page still loads correctly.

To verify a Surge deployment with `bin/test-photos-server.sh`, pass `--surge` so the script
expects HTTPS in Location headers and treats the above behaviors as expected rather than failures:

```bash
bin/test-photos-server.sh --remote https://your-site.surge.sh --surge
```
