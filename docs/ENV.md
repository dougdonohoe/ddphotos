# Environment Variables

## Site Identity (`albums.yaml`)

Site identity settings live in the `settings:` block of `albums.yaml` and are written
into either `config.json` or `html.json` by `photogen`. The frontend reads them at 
runtime via `fetch('/albums/[config|html].json')` — no build-time injection needed.

| Setting              | Required | Description                                                                                                                                              |
|----------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `site_name`          | yes      | Site title shown in the browser tab and OG tags                                                                                                          |
| `site_url`           | yes      | Canonical base URL (e.g. `https://photos.example.com`); used in sitemap and OG tags                                                                      |
| `site_description`   | yes      | Meta description and OG description for the home page                                                                                                    |
| `copyright_owner`    | yes      | Name shown in the footer copyright line                                                                                                                  |
| `copyright_year`     | yes      | Start year shown in the footer copyright line                                                                                                            |
| `allow_crawling`     | no       | Set to `true` to allow search engine crawling; adds `Sitemap:` to `robots.txt` (default: `false`)                                                        |
| `site_title_html`    | no       | HTML for the site title on the home page; falls back to `site_name` when omitted. Allows links, emphasis, etc. Written to `html.json` / `html.enc.json`. |
| `site_subtitle_html` | no       | HTML rendered below the site title in a smaller font. Written to `html.json` / `html.enc.json`.                                                          |
| `site_overview_html` | no       | HTML rendered above the album cards (slightly larger than album descriptions). Written to `html.json` / `html.enc.json`.                                 |

`photogen`'s `Config.Validate()` enforces all required fields before any files are written.

## Deploy and Test Variables (`site.env`)

The `site.env` file holds variables used only by deployment scripts and tests — nothing
that affects the built site itself.

| Variable            | Used by                     | Description                                                              |
|---------------------|-----------------------------|--------------------------------------------------------------------------|
| `CLOUDFRONT_ID`     | `bin/deploy-photos.sh`      | CloudFront distribution ID; if set, cache is invalidated after deploy    |
| `S3_BUCKET`         | `bin/deploy-photos.sh`      | S3 bucket name for deployment (S3 mode only; requires `--s3`)            |
| `RSYNC_HOST`        | `bin/deploy-photos.sh`      | Rsync target host, e.g. `user@your-server.example.com` (rsync mode only) |
| `RSYNC_DEST`        | `bin/deploy-photos.sh`      | Rsync destination path on the server (rsync mode only)                   |
| `TEST_ALBUM_LOCAL`  | `bin/test-photos-server.sh` | Album slug used for local server tests                                   |
| `TEST_ALBUM_PROD`   | `bin/test-photos-server.sh` | Album slug used for production tests                                     |
| `TEST_ALBUM_HYPHEN` | `bin/test-photos-server.sh` | Album slug with a hyphen (tests URL routing edge case)                   |

The `bin` scripts `source` this file directly.

## Album Location Variables

Two variables tell the dev server, build, and Docker container where to find album data:

| Variable              | Default  | Description                                                                                                                     |
|-----------------------|----------|---------------------------------------------------------------------------------------------------------------------------------|
| `DDPHOTOS_ALBUMS_DIR` | `albums` | Path to the root albums directory (absolute or repo-root-relative)                                                              |
| `DDPHOTOS_SITE_ID`    | `sample` | Site ID — selects `<DDPHOTOS_ALBUMS_DIR>/<DDPHOTOS_SITE_ID>` as the active site. Also used to choose active build under `build` |

Defaults are defined in `config/defaults.env` and are automatically picked up by the Makefile
and `vite.config.ts`. Override them on the command line as needed:

```bash
# Use a different site ID
DDPHOTOS_SITE_ID=prod make web-npm-run-dev

# Albums directory outside the repo
DDPHOTOS_ALBUMS_DIR=~/photos/albums DDPHOTOS_SITE_ID=mySite make web-npm-build
```

These variables are consumed by:
- `vite.config.ts` — dev server middleware serves `/albums/**` from `<DDPHOTOS_ALBUMS_DIR>/<DDPHOTOS_SITE_ID>/`
- `web/svelte.config.js` — build output goes to `build/<DDPHOTOS_SITE_ID>/`; album slugs are read for pre-rendered entries
- `web/hooks.server.ts` — intercepts fetch calls to `/albums/**` during `npm run build`
- `web/apache-entrypoint.sh` — symlinks `build/<DDPHOTOS_SITE_ID>/` into the Apache document root at container startup
