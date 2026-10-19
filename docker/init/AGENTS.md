# AGENTS.md

This directory is a photo site built with **DD Photos**, a static photo/video album site
generator. It was created by `ddphotos init`. This file tells AI coding agents how to work
in it.

- DD Photos: https://github.com/dougdonohoe/ddphotos
- DD Photos App (desktop app, recommended for editing this config): https://github.com/dougdonohoe/ddphotos-app

## Ground rules

- **Use DD Photos, don't change it.** Everything a site owner controls lives in `config/`.
  DD Photos itself runs from a Docker image through the `./ddphotos` wrapper. If something
  can only be fixed in DD Photos, say so, and suggest an issue at
  https://github.com/dougdonohoe/ddphotos/issues instead of patching around it.
- **Prefer the DD Photos App.** It is much easier and less error-prone than editing
  `config/` by hand: it validates everything as it is entered and shows the photos when
  picking a hero or cover. It edits all of `config/` except the advanced
  `customization.yaml` and `descriptions.txt`. Before making substantial config changes
  yourself, suggest the owner use the app instead. When you do edit by hand, change only
  what was asked and run `photogen` to check the result.
- **Never edit generated output.** `albums/`, `build/` and `export/` are rewritten by
  `photogen`, `build` and `export`.
- **Ask before deploying.** `deploy`, `wrangler` and `surge` publish the site.
- **Keep secrets private.** `config/site.env`, `config/passwords.yaml`, `config/immich.env`
  and `config/photoprism.env` can hold credentials and passwords. Don't print them. If this
  directory is a public git repo, make sure they are ignored before anything is committed
  (a private repo may be acceptable in some organizations).

## Layout

| Path                        | What it is                                                                       |
|-----------------------------|----------------------------------------------------------------------------------|
| `ddphotos`                  | Wrapper script; runs every command in Docker (`./ddphotos help`)                 |
| `config/albums.yaml`        | Site settings and album definitions                                              |
| `config/custom.css`         | Optional style overrides                                                         |
| `config/passwords.yaml`     | Optional album/site passwords                                                    |
| `config/site.env`           | Deploy settings (rsync, S3, CloudFront)                                          |
| `config/customization.yaml` | Optional, advanced: site chrome such as the album page's back link (`album_nav`) |
| `config/descriptions.txt`   | Optional, advanced: album descriptions, named by `settings.descriptions`         |
| `config/static/`            | Optional: files `build` copies to the site root (`llms.txt`, site verification)  |
| `sample-photos/`            | Starter photos; delete with the starter albums once real albums exist            |
| `sync/`                     | Albums downloaded from Immich or PhotoPrism; source, not output                  |
| `albums/`                   | `photogen` output (generated)                                                    |
| `build/`, `export/`         | Static site output (generated)                                                   |

Captions and sort order for an album go in a `photogen.txt` inside that album's source
folder. In `sync/`, `photogen.txt` is the only file to edit.

Put album descriptions in `albums.yaml` (`description:` on the album) unless there is a
good reason not to. An `albums.yaml` description takes precedence over a `descriptions.txt`
entry for the same album. `descriptions.txt` is only really useful when a script generates
the descriptions, or when it helps to see them all in one place.

## Commands

```bash
./ddphotos help       # every command and its options
./ddphotos photogen   # resize photos, transcode videos, write index files
./ddphotos run        # dev server at http://localhost:5173
./ddphotos build      # build the static site
./ddphotos serve      # serve the build via Apache at http://localhost:8000
./ddphotos export     # export for static hosting (--copy, --cloudflare)
./ddphotos deploy     # deploy via rsync or S3 (ask first)
```

After changing `config/` or source photos, run `photogen`, then `build`.

## Docs

Read the relevant page before guessing at a config option:

- Docker commands, layout and deploys: https://github.com/dougdonohoe/ddphotos/blob/main/docs/DOCKER.md
- `albums.yaml`, `customization.yaml`, passwords, syncing from Immich or PhotoPrism: https://github.com/dougdonohoe/ddphotos/blob/main/docs/CONFIGURATION.md
- Captions, `photogen.txt`, video, recursive albums: https://github.com/dougdonohoe/ddphotos/blob/main/docs/PHOTOGEN.md
- Environment variables (deployment, album location, Immich/PhotoPrism): https://github.com/dougdonohoe/ddphotos/blob/main/docs/ENV.md
- Restyling with `custom.css`: https://github.com/dougdonohoe/ddphotos/blob/main/docs/CUSTOM-CSS.md
- Deployment via rsync and S3+CloudFront: https://github.com/dougdonohoe/ddphotos/blob/main/docs/DEPLOY.md
- Web server routing rules (Apache, nginx, CloudFront, Cloudflare Pages): https://github.com/dougdonohoe/ddphotos/blob/main/docs/DEPLOYMENT-SERVERS.md
- Rebuilding a config directory from a deployed site: https://github.com/dougdonohoe/ddphotos/blob/main/docs/SCRAPE.md
