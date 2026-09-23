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

Three variables tell the dev server, build, and Docker container where album data lives:

| Variable              | Default  | Description                                                                                                                                                |
|-----------------------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `DDPHOTOS_ALBUMS_DIR` | `albums` | Path to the root albums directory (absolute or repo-root-relative)                                                                                         |
| `DDPHOTOS_SITE_ID`    | `sample` | Site ID — selects `<DDPHOTOS_ALBUMS_DIR>/<DDPHOTOS_SITE_ID>` as the active site. Also used to choose active build under `build`                            |
| `DDPHOTOS_SYNC_DIR`   | `sync`   | Root for albums synced from a photo manager; each lands in `<DDPHOTOS_SYNC_DIR>/<site-id>/<provider>/<slug>/`. Only read when an album has a `sync:` block |

Defaults are defined in `config/defaults.env` and are automatically picked up by the `Makefile`, `vite.config.ts`,
`photogen` and various other scripts.

Override them on the command line as needed:

```bash
# Use a different site ID
DDPHOTOS_SITE_ID=prod make web-npm-run-dev

# Albums directory outside the repo
DDPHOTOS_ALBUMS_DIR=~/photos/albums DDPHOTOS_SITE_ID=mySite make web-npm-build

# Sync downloads outside the repo (photogen only; -sync-dir does the same)
DDPHOTOS_SYNC_DIR=~/photos/sync go run ./cmd/photogen -sync-only
```

These variables are consumed by:

- `cmd/photogen` — writes processed photos and JSON to `<DDPHOTOS_ALBUMS_DIR>/<site-id>/` (site ID comes from the albums config YAML, not `DDPHOTOS_SITE_ID`), and downloads synced albums into `<DDPHOTOS_SYNC_DIR>/<site-id>/<provider>/<slug>/` to build them from there; nothing under the sync directory is deployed (see [Syncing](PHOTOGEN.md#syncing))
- `web/vite.config.ts` — dev server middleware serves `/albums/**` from `<DDPHOTOS_ALBUMS_DIR>/<DDPHOTOS_SITE_ID>/`
- `web/svelte.config.js` — build output goes to `build/<DDPHOTOS_SITE_ID>/`; album slugs are read for pre-rendered entries
- `web/src/hooks.server.ts` — intercepts fetch calls to `/albums/**` during `npm run build`
- `web/setup-htdocs.sh` — symlinks `build/<DDPHOTOS_SITE_ID>/` into the web server document root at container startup (called by both `apache-entrypoint.sh` and `nginx-entrypoint.sh`)
- `bin/deploy-photos.sh` — drives `npm run build`, Docker deployment, and S3/rsync sync
- `bin/search-cover.sh` — locates album data when searching for cover images
- `bin/run-tests.sh` — sets both when starting the dev server, building, and running Docker test containers

## Immich Sync Variables

Read only by `photogen`, and only when an album has a `sync:` block whose provider is
`immich`. They normally live in `immich.env` beside `albums.yaml` rather than the
environment; see [Immich credentials](CONFIGURATION.md#immich-credentials-immichenv) for that
file, where the key comes from, and which wins.

| Variable              | Description                                                                                                                                                                                                                                    |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `IMMICH_API_KEY`      | Immich API key. Needs `asset.read`, `asset.download` and `album.read`                                                                                                                                                                          |
| `IMMICH_INSTANCE_URL` | Instance URL, e.g. `http://localhost:2283`. Accepted with or without a trailing `/api`                                                                                                                                                         |
| `DDPHOTOS_IN_DOCKER`  | Set to `1` by the Docker image, and not something to set by hand. It is what tells `photogen` to reach the host rather than the container when `IMMICH_INSTANCE_URL` names `localhost` — see [Docker](DOCKER.md#syncing-from-immich-in-docker) |
