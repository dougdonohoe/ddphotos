# Environment Variables

## Deploy Variables (`site.env`)

The `site.env` file holds variables used by `bin/deploy-photos.sh` — nothing that affects the built site itself.

| Variable        | Description                                                                         |
|-----------------|-------------------------------------------------------------------------------------|
| `CLOUDFRONT_ID` | CloudFront distribution ID; if set, cache is invalidated after deploy via `aws` CLI |
| `S3_BUCKET`     | S3 bucket name for deployment (S3 mode only; requires `--s3`)                       |
| `RSYNC_HOST`    | Rsync target host, e.g. `user@your-server.example.com` (rsync mode only)            |
| `RSYNC_DEST`    | Rsync destination path on the server (rsync mode only)                              |

See [site.env](CONFIGURATION.md#siteenv) in the Configuration docs for rsync and S3 examples.

## Album Location Variables (development)

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
- `cmd/photogen` — writes processed photos and JSON to `<DDPHOTOS_ALBUMS_DIR>/<site-id>/` (site ID comes from the albums config YAML, not `DDPHOTOS_SITE_ID`)
- `web/vite.config.ts` — dev server middleware serves `/albums/**` from `<DDPHOTOS_ALBUMS_DIR>/<DDPHOTOS_SITE_ID>/`
- `web/svelte.config.js` — build output goes to `build/<DDPHOTOS_SITE_ID>/`; album slugs are read for pre-rendered entries
- `web/src/hooks.server.ts` — intercepts fetch calls to `/albums/**` during `npm run build`
- `web/setup-htdocs.sh` — symlinks `build/<DDPHOTOS_SITE_ID>/` into the web server document root at container startup (called by both `apache-entrypoint.sh` and `nginx-entrypoint.sh`)
- `bin/deploy-photos.sh` — drives `npm run build`, Docker deployment, and S3/rsync sync
- `bin/search_cover.sh` — locates album data when searching for cover images
- `bin/run-tests.sh` — sets both when starting the dev server, building, and running Docker test containers
