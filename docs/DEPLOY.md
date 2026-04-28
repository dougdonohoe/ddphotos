# Deployment

DD Photos was originally built to serve my personal photo albums.  My first deployment
re-used an existing EC2 instance with Apache which served my other websites.  The
second (and current) deployment uses S3 as the backing store.  Both are described below.

## Syncing Logic

The web root is assembled from two independent sources:

| Source              | Contents                                                                         | Maps to             |
|---------------------|----------------------------------------------------------------------------------|---------------------|
| `build/<site-id>/`  | SvelteKit output: HTML shell, JS/CSS bundles, pre-rendered `albums/*.html` pages | web root `/`        |
| `albums/<site-id>/` | photogen output: WebP images, JSON indexes, hero images, `sitemap.xml`           | web root `/albums/` |

```
build/<site-id>/              albums/<site-id>/
  index.html                    albums.json
  albums.html                   config.json
  404.html                      hero.jpg
  robots.txt                    html.json
  *.png, *.ico                  sitemap.xml
  _app/                         antarctica/
  albums/                         cover.jpg
    antarctica.html               index.json
    hawaii.html                   full/
    albums.json  (ignored)          *.webp
    config.json  (ignored)        grid/
    html.json    (ignored)          *.webp
    antarctica/
      index.json (ignored)
         |                            |
         | Pass 1: sync -> /          | Pass 2: sync -> /albums/
         +----------------------------+
                       |
                       v
               Web root /
                 index.html    (build)
                 albums.html   (build)
                 404.html      (build)
                 robots.txt    (build)
                 *.png, *.ico  (build)
                 _app/         (build)
                 albums/
                   antarctica.html  (build)
                   hawaii.html      (build)
                   albums.json      (albums)
                   config.json      (albums)
                   hero.jpg         (albums)
                   html.json        (albums)
                   sitemap.xml      (albums)
                   antarctica/      (albums)
                     cover.jpg
                     index.json
                     full/
                       *.webp
                     grid/
                       *.webp
```

**NOTE**:  If passwords are on, you might see `albums.enc.json`, `html.enc.json`, or `index.enc.json` files.

SvelteKit copies album JSON into the build during pre-rendering, but those copies are marked
`(ignored)` — excluded by Pass 1 and replaced by the authoritative files from `albums/`.

Both sources contribute files under `/albums/` — `build/` provides the pre-rendered `.html` pages
and `albums/` provides images and JSON — so a two-pass sync is required to prevent each pass from
deleting the other's files:

- **Pass 1** (build → `/`): syncs app files; skips or protects existing `albums/` data so images
  and JSON are not deleted
- **Pass 2** (album data → `/albums/`): syncs images and JSON; skips `*.html` so pre-rendered
  album pages are not deleted

Both rsync and S3 implement this pattern, with minor differences:

|                      | rsync                                                                                                                                   | S3                                                                                                             |
|----------------------|-----------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| **Pass 1**           | `--filter='protect albums/**'` preserves album data on the server                                                                       | `--exclude "albums/*" --include "albums/*.html"` uploads only `.html` from `albums/`                           |
| **Pass 2**           | `--exclude=*.html` skips pre-rendered pages                                                                                             | Two sub-passes: one for JSON/XML/covers (`Cache-Control: no-cache`), one for WebP (`Cache-Control: immutable`) |
| **Change detection** | Pass 1 uses `--checksum` (Vite resets timestamps every build); Pass 2 uses size+time (photogen preserves timestamps on unchanged files) | Size+time only (no checksum option in `aws s3 sync`)                                                           |

The local Docker testing environment uses the same separation: `web/setup-htdocs.sh` symlinks
build output into `htdocs/` and album data into `htdocs/albums/` from separate bind mounts,
mirroring the two-source structure without transferring any files.

## Apache + rsync

In this scenario, traffic is handled by CloudFront, which filters
requests through a WAFv2 web ACL before forwarding clean traffic to an Apache
origin on any SSH-accessible server.

```mermaid
flowchart LR
    User -->|HTTPS| WAF["WAFv2 Web ACL"]
    WAF --> CF["CloudFront CDN"]
    CF -->|HTTP| Apache["Server / Apache"]
```

The WAF (Web Application Firewall) inspects every incoming request and blocks
suspicious or malicious traffic (things like bots or known bad IP addresses)
before it ever reaches my server.

The CDN (Content Delivery Network) caches content at edge locations around
the world so visitors get fast load times regardless of where they are,
and my origin server handles far less traffic.

The deployment script (described below) builds the static site and rsyncs it to
a server behind CloudFront. It is specific to my setup, but it is
parameterized via `site.env` so that others with a similar setup can re-use it.
It can also be extended or changed to suit your needs.

## S3 + CloudFront

An alternative is to serve the site entirely from S3 and CloudFront — no server
required. Site files live in a private S3 bucket; CloudFront serves them using a
signed-request mechanism called OAC (Origin Access Control).

```mermaid
flowchart LR
    User -->|HTTPS| CF["CloudFront CDN\n+ CloudFront Function"]
    CF -->|SigV4| S3["S3 Bucket (private)"]
```

### AWS Components

Several AWS components are needed to serve an S3-based site:

| Component                       | Purpose                                                                                                                               |
|---------------------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| **S3 bucket**                   | Stores all site files. Must be private — no public access block overrides.                                                            |
| **Origin Access Control (OAC)** | Lets CloudFront sign requests to S3 using SigV4. Required because the bucket is private.                                              |
| **S3 bucket policy**            | Grants the OAC principal `s3:GetObject` on the bucket. Without this, CloudFront gets a `403` even with OAC.                           |
| **ACM certificate**             | TLS certificate for your domain. Must be provisioned in `us-east-1` — CloudFront requires this regardless of where your bucket lives. |
| **CloudFront distribution**     | CDN that serves from S3 via OAC. Requires custom error responses (see below).                                                         |
| **CloudFront Function**         | Lightweight JavaScript function (viewer-request stage) that handles URL routing. See below.                                           |
| **DNS**                         | CNAME or alias record pointing your domain to the CloudFront distribution domain name.                                                |

**Custom error responses:** A private S3 bucket returns `403 Forbidden` (not `404`) for keys that
don't exist — returning `404` would confirm the key's absence and enable bucket enumeration.
Your CloudFront distribution must map both `403` and `404` to `/404.html` with a `404` response code,
or users will see a raw XML error from S3 instead of your custom 404 page.

### CloudFront Function

CloudFront Functions are lightweight JavaScript functions that run at the edge on every request.
Attaching one at the **viewer-request** stage lets you rewrite and redirect URLs before S3 is ever
contacted — no round-trip cost.

For a SvelteKit `adapter-static` site like DD Photos, a function is **required** to handle:

- **URL routing** — extensionless paths like `/albums/patagonia` map to `patagonia.html`; the root
  `/` maps to `index.html`; unknown root-level paths fall back to `index.html` (SPA fallback)
- **Photo permalinks** — `/albums/slug/42` maps to `/albums/slug.html` so the album page can open
  the lightbox to photo 42 via the URL hash
- **Domain redirects** — apex-to-www (`example.com` → `www.example.com`) and any other domain consolidation

The function effectively replicates the Apache `.htaccess` rules. 
Here is a minimal function for a SvelteKit-based photo site:

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

## Deploy Script

`bin/deploy-photos.sh` handles both S3 and rsync modes. Add `--s3` for S3 mode.

1. Runs `photogen` to resize images and generate JSON
2. Builds the static site via `npm run build` into `build/<site-id>/`
3. *(rsync mode only)* Starts Docker/Apache, runs `bin/test-photos-server.sh --local` to verify
   routing locally, runs Playwright tests against Docker/Apache, then stops the container
4. Deploys the site:
   - **S3**: two-pass `aws s3 sync` — pass 1 syncs the build output (excluding `albums/*` but
     re-including `albums/*.html`); pass 2 syncs album images and JSON (`--size-only`, excluding
     `*.html`). The two-pass approach keeps app files and photo data independent.
   - **rsync**: two-pass `rsync` — pass 1 uses `--checksum` (Vite resets timestamps on every build);
     pass 2 syncs album data independently.
5. Invalidates the CloudFront cache via `$CLOUDFRONT_ID` (skipped if not set)
6. Runs `bin/test-photos-server.sh` to verify the deployment against production
7. Runs Playwright tests against production (URL read from `config.json`)

The script uses `set -eo pipefail` — any failure aborts before deployment.

## Flags

| Flag                    | Description                                                                                                             |
|-------------------------|-------------------------------------------------------------------------------------------------------------------------|
| `--s3`                  | Deploy to S3 instead of rsync (requires `S3_BUCKET` in `site.env`; skips pre-deploy Docker/Apache and Playwright tests) |
| `--dry-run`             | Pass `--dry-run`/`--dryrun` to rsync or `aws s3 sync`; skips CloudFront invalidation and post-deploy tests              |
| `--no-photogen`         | Skip photo generation step                                                                                              |
| `--no-rsync`            | Skip deploy, CloudFront invalidation, and post-deploy tests (build + local test only)                                   |
| `--no-pre-deploy-tests` | Skip pre-deploy Docker/Apache test and Playwright (rsync mode only); post-deploy tests still run                        |
| `--no-server-test`      | Skip both the local and post-deploy server routing tests                                                                |
| `--no-playwright`       | Skip Playwright tests (both local and production)                                                                       |
| `--config-dir`          | Directory containing `albums.yaml`, `descriptions.txt`, and (by default) `site.env`                                     |
| `--site-env`            | Path to `site.env` — overrides `--config-dir/site.env` when the two live in different locations                         |

Examples:

```bash
# S3 mode
bin/deploy-photos.sh --s3                          # full S3 deploy
bin/deploy-photos.sh --s3 --dry-run                # preview what s3 sync would transfer, no changes made
bin/deploy-photos.sh --s3 --no-photogen            # skip photo generation

# rsync mode
bin/deploy-photos.sh                               # full deploy
bin/deploy-photos.sh --dry-run                     # preview what rsync would transfer, no changes made
bin/deploy-photos.sh --no-photogen                 # skip photo generation
bin/deploy-photos.sh --no-rsync                    # build + local test only (safe on a dev machine)
bin/deploy-photos.sh --no-photogen --no-rsync      # build + local test, skip both photogen and rsync
```

The deploy paths are validated by local test scripts:

```bash
# rsync path — rsyncs into a local Docker container; runs server routing tests and Playwright
make sample-rsync-test

# S3 path — syncs against MinIO; verifies file placement and Cache-Control headers
# (post-deploy server and Playwright tests are skipped: MinIO serves S3 API only, not HTTP)
make sample-s3-test
```
