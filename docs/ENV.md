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

Read only by `photogen`, and only when an album in `albums.yaml` has a `sync:` block whose
provider is `immich`. A site with no such album never looks for them.

| Variable              | Description                                                                                                       |
|-----------------------|-------------------------------------------------------------------------------------------------------------------|
| `IMMICH_API_KEY`      | Immich API key (Account Settings → API Keys). Needs `asset.read`, `asset.download` and `album.read`, nothing more |
| `IMMICH_INSTANCE_URL` | Instance URL, e.g. `http://localhost:2283`. Accepted with or without a trailing `/api`                            |

Both normally live in `immich.env` beside `albums.yaml`, so secrets stay out of the YAML:

```bash
# config/immich.env
IMMICH_API_KEY=your-api-key
IMMICH_INSTANCE_URL=http://localhost:2283
```

**A value set in the environment wins over the file**, the same precedence
`DDPHOTOS_ALBUMS_DIR` has, so a CI run needs no secrets file on disk. The file is optional
when both variables are exported. `config/immich.env` and `config/immich-*.env` are
gitignored. The API key is never logged, never printed in an error, and never written to
`metadata.yaml`.

See [Syncing an Album](CONFIGURATION.md#syncing-an-album-from-a-photo-manager) for the
`sync:` block itself.

## Container Variables

| Variable             | Description                                                                                      |
|----------------------|--------------------------------------------------------------------------------------------------|
| `DDPHOTOS_IN_DOCKER` | Set to `1` by `docker/Dockerfile`. Its presence is the whole signal; the value is never read     |

`photogen` has no other way to tell it is containerized, and it needs to know for one reason:
inside the container, `localhost` is the container. When `DDPHOTOS_IN_DOCKER` is set and
`IMMICH_INSTANCE_URL` names a loopback address (`localhost`, `127.0.0.1`, `::1`, `0.0.0.0`),
the host is rewritten to `host.docker.internal` and the run says so. A hostname, LAN IP or
public URL passes through untouched. See [Docker](DOCKER.md#syncing-from-immich-in-docker).

**Do not set it by hand.** Setting it outside a container makes `photogen` rewrite a
`localhost` URL to a name that does not resolve there.
