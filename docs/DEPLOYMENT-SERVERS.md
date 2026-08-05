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
- **Unknown paths** - Return 404 (served as `404.html` via `ErrorDocument 404`)

## nginx

Unlike Apache, nginx needs no per-directory config file — all routing rules live in
`web/nginx.conf`. The Docker images bake it in; on a real server it is installed once by hand
(see [Installing nginx.conf on a server](#installing-nginxconf-on-a-server) below), because
nothing in the deploy path transfers it. `web/nginx-entrypoint.sh` symlinks the active build
into the document root at container startup (same role as `web/apache-entrypoint.sh`).

### nginx.conf

- **Cache headers** — JSON files get `Cache-Control: no-cache`; WebP files get `Cache-Control: max-age=31536000, immutable`
- **Trailing slash redirect** — 301 redirects URLs with trailing slashes to their clean version
  (e.g., `/albums/patagonia/` → `/albums/patagonia`)
- **Photo permalink rewrite** — Serves album HTML for photo permalink URLs
  (e.g., `/albums/patagonia/15` serves `patagonia.html`; JS reads the path and opens the lightbox)
- **HTML rewrite** — Serves `.html` files without the extension
  (e.g., `/albums/patagonia` serves `patagonia.html`)
- **Unknown paths** — Return 404 (served as `404.html` via `error_page 404`)

### Installing nginx.conf on a server

`web/nginx.conf` is a bare `server { ... }` block, so it goes wherever your nginx package's
`http` block already includes config from. The stock `/etc/nginx/nginx.conf` includes both of
these, so either location works:

| Distribution                     | Destination                                                                       |
|----------------------------------|-----------------------------------------------------------------------------------|
| Debian / Ubuntu                  | `/etc/nginx/sites-available/ddphotos`, symlinked into `/etc/nginx/sites-enabled/` |
| RHEL / Alma / Rocky, nginx image | `/etc/nginx/conf.d/ddphotos.conf`                                                 |

**Edit `root` before reloading.** The file ships with `root /usr/share/nginx/html;` — the
document root of the Docker images it was written for. On a real server it must match the
`RSYNC_DEST` you set in `config/site.env`, or nginx will serve a directory the deploy never
writes to. You will also want a `server_name`, and TLS if you are not terminating it upstream.

```bash
scp web/nginx.conf myserver:/etc/nginx/sites-available/ddphotos
ssh myserver
sudo sed -i 's|root /usr/share/nginx/html;|root /var/www/photos;|' /etc/nginx/sites-available/ddphotos
sudo ln -sf /etc/nginx/sites-available/ddphotos /etc/nginx/sites-enabled/ddphotos
sudo rm -f /etc/nginx/sites-enabled/default   # the stock site also claims :80
sudo nginx -t && sudo systemctl reload nginx
```

`nginx -t` validates the config before you reload; a reload with a broken config is refused and
leaves the running server untouched.

This is a one-time step, plus a repeat whenever `web/nginx.conf` changes — the deploy script
never touches it. See [Apache or nginx + rsync](DEPLOY.md#apache-or-nginx--rsync) for how this
differs from Apache, where `.htaccess` is rsynced with every deploy.

## CloudFront Function

When serving from S3+CloudFront, a CloudFront Function replaces the web server routing rules.
It runs at the **viewer-request** stage, rewriting URLs before S3 is ever contacted — no
round-trip cost.

For a SvelteKit `adapter-static` site like DD Photos, a function is **required** to handle:

- **URL routing** — extensionless paths like `/albums/patagonia` map to `patagonia.html`; the root
  `/` maps to `index.html`; other root-level paths like `/privacy` map to `privacy.html`; unknown
  paths produce a 403/404 from S3, caught by `custom_error_response` and served as `404.html`
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

    // Extensionless paths → pre-rendered .html page.
    // Unknown paths produce a 403/404 from S3, caught by custom_error_response → 404.html.
    if (!uri.includes('.')) {
        request.uri = uri + '.html';
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
- **Root-level extensionless paths** — single-segment paths like `/privacy` → serves `privacy.html`;
  truly unknown paths (e.g. `/nope`) produce a 404 from `ASSETS`, which Cloudflare Pages resolves to `404.html`

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
