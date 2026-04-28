# Web Server Configuration

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
