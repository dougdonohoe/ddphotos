# DD Photos - Static Photo Album Site Generator

[![CI](https://github.com/dougdonohoe/ddphotos/actions/workflows/ci.yml/badge.svg)](https://github.com/dougdonohoe/ddphotos/actions)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Demo](https://img.shields.io/badge/Demo-ddphotos.donohoe.info-blue)](https://ddphotos.donohoe.info)

## Docker Quick Start

The easiest way to run DD Photos is via [Docker↗](https://www.docker.com/get-started/) — no Go, 
Node, or `libvips` required.  Try the starter site:

```bash
mkdir ~/my-ddphotos
docker run --rm -v ~/my-ddphotos:/ddphotos dougdonohoe/ddphotos init
cd ~/my-ddphotos
./ddphotos photogen   # resize images and create index files
./ddphotos run        # run dev server at http://localhost:5173
./ddphotos build      # build static site
./ddphotos serve      # serve static site via Apache at http://localhost:8000
```

Then edit `config/albums.yaml` to define your own albums and repeat.

**Deploy in seconds** (free, no server required) via [Cloudflare Pages↗](https://pages.cloudflare.com) (unlimited bandwidth) or
[Surge↗](https://surge.sh):

```bash
./ddphotos export --cloudflare
wrangler pages deploy --project-name my-unique-site export/my-photos

./ddphotos export --copy
surge --domain my-unique-site.surge.sh export/my-photos
```

See the [Docker Mode](docs/DOCKER.md) page for full details including deploying via `rsync` or to S3.

## Motivation

I was dissatisfied with photo sharing sites, especially Apple's iCloud shared albums,
which typically take 20+ seconds to load.  Other sites for sharing have their own 
irritations like cumbersome UIs, advertising, hawking of photo paraphernalia and
social media distractions.

I just want to share my photos with friends and family.  I want it fast, easy, mobile
friendly, and distraction free. Focus on the photos. So I built DD Photos, and it is what 
is behind [photos.donohoe.info↗](https://photos.donohoe.info).
It's actually pretty good, wicked fast, and meets my needs.  Maybe it will meet yours
too, which is why I've open-sourced it.

**P.S.** _I wrote about building DD Photos in [this Medium article↗](https://medium.com/@DougDonohoe/3b48fdd1350c?source=friends_link&sk=4094f33198de93f5488da6539c9981ee)._

## Overview

A DD Photos site has a home page, with all of your albums and their description.
You can easily switch between dark and light themes.  Click/touch an album and 
you see a grid of all photos.  Click/touch a photo to see the full size version and
a caption, if it has one. You can easily swipe between photos (or use
arrow keys on a laptop).  It works great on mobile, tablet, and desktop.

Here's what it looks like on a big display (see [Screenshots](docs/SCREENSHOTS.md) for larger versions):

![screenshots.png](images/screenshots.png)

## How it Works

The idea is that you already use _something else_ to curate and filter your photos. Maybe it
is Adobe Lightroom Classic (my tool).  Or maybe it is Apple Photos or Google Photos.
It doesn't matter, but once you get a selection of photos that comprise an album,
you export the photos into a folder.  All the photos in a folder make up an album.
It's that simple.

You can create an optional `photogen.txt` file in each album folder to
define captions for each photo.  This file can also be used to define the
album's sort order, if order-by-date isn't sufficient.

With DD Photos, you define where your albums live in an `albums.yaml` file.
In a separate `descriptions.txt` you provide a short description of each album.

Once you have defined where your photos live, you run the `photogen` tool,
which resizes the photos for web viewing and generates index files that
the web app uses.

That's it.  You can now view your photo albums on your machine using the dev server.

Finally, there is a build step which creates a static site that can easily be
deployed to a machine that has a web server (like Apache or nginx), to AWS S3,
or to any static hosting service. No code runs on a server.  No database is needed.
It's just HTML, CSS, JavaScript and your (resized) photos.

## Key Features

Website features:

- Concise album cards with description, number of photos, date range and
  your choice of cover photo.
- An album's page has a nicely justified photo grid layout with PhotoSwipe lightbox that
  adjusts well to any screen size.
- Keyboard support: arrow keys navigate in lightbox, ESC key exits
  lightbox and returns to home page from album page.
- Optional per-photo descriptions via `photogen.txt`: used as image `alt` text, grid
  mouse-hover caption (desktop), always-visible caption (mobile), and lightbox caption.
- Each album has a human-readable URL (e.g., `/albums/antarctica`).
- Each photo has a shareable permalink (e.g., `/albums/patagonia/5`) accessible via a copy-to-clipboard button.
- Optional hero image: a full-width banner at the top of the home page, specified in
  `albums.yaml` with a configurable crop position (top/center/bottom).
- Optional `HTML` title, subtitle, and site overview.
- Optional password protection: encrypt individual albums or the entire site. Passwords
  are never stored server-side, decryption happens in-browser using the Web Crypto API.
  Optional hints can be shown in the password dialog. A logout button clears stored
  passwords on encrypted sites.
- Dark/light theme toggle.
- Custom CSS override: specify a CSS file in `albums.yaml` to restyle the site without
  modifying the source code.
- OpenGraph tags for rich link previews when sharing album or photo URLs on social media
  or messaging apps. The hero image (if configured) or an album cover JPEG is used as
  the preview image.
- Privacy page (`/privacy`) documents what is stored in browser local storage (theme
  preference, site ID, and passwords/covers on encrypted sites). Append `?clear` to any
  URL to wipe all stored data.

Backend features:

- Two efficient WebP image sizes created: `grid` (600px) and `full` (1600px).
- EXIF metadata extraction (dimensions, date) stored in JSON.
- All image metadata stripped from WebP output (smaller files, no GPS leak).
- Concurrent image resizing via goroutines (buffered channel, WaitGroup).
- Dry-run mode by default (use `-doit` to write files).
- Optionally use `photogen.txt` to override sort order (default is by capture date).
- Recursive album support: set `recurse: true` to collect photos from subdirectories, 
  with automatic filename prefixing to avoid collisions.
- WebP filenames for encrypted albums are HMAC-derived, preventing filename guessing
  even if the original source filename is known.

## Tech Details

The `photogen` Go program (`cmd/photogen/photogen.go`) resizes your photos to WebP
format and generates the JSON index files (`albums.json`, per-album `index.json`) 
that are consumed by the frontend.  It also generates a `sitemap.xml` file that
identifies each album.

The site (in `web`, a Node.js app) is built with SvelteKit and statically generated. 
The HTML shell and assets are pre-built files served directly by a web server, with photo data 
fetched client-side from the static JSON indexes generated by `photogen`.

There are many ways to deploy a static site like this. The quickest options are
[Cloudflare Pages↗](https://pages.cloudflare.com) or [Surge↗](https://surge.sh) —
one command and your site is live at a public URL, free.
For production, DD Photos provides two options out of the box: Apache via `rsync`, and
S3+CloudFront using `aws s3 sync`. See [Deployment](docs/DEPLOY.md) for setup details.

## Documentation

These documents are primarily meant for users of DD Photos:

| Document                                               | Description                                                                        |
|--------------------------------------------------------|------------------------------------------------------------------------------------|
| [Docker Mode](docs/DOCKER.md)                          | Docker workflow: init, photogen, run, build, serve, deploy, upgrade                |
| [Configuration](docs/CONFIGURATION.md)                 | `albums.yaml`, `descriptions.txt`, `site.env`, and how config reaches the frontend |
| [Photogen](docs/PHOTOGEN.md)                           | `photogen` CLI: flags, photo descriptions, recursive albums                        |
| [Deployment](docs/DEPLOY.md)                           | Deployment via rsync and S3+CloudFront                                             |
| [Web Server Configuration](docs/DEPLOYMENT-SERVERS.md) | Apache, nginx, CloudFront, and Cloudflare Pages routing rules                      |
| [Environment Variables](docs/ENV.md)                   | Deployment variables                                                               |

These documents are primarily meant for developers of DD Photos:

| Document                             | Description                                          |
|--------------------------------------|------------------------------------------------------|
| [Developer Setup](docs/INSTALL.md)   | Prerequisites, sample app, commands (developer mode) |
| [Development Notes](docs/DEV.md)     | SvelteKit details, LAN access, debugging             |
| [Testing](docs/TESTING.md)           | Manual testing, Playwright e2e tests, CI             |
| [Makefile Targets](docs/MAKEFILE.md) | All `make` targets                                   |
| [Environment Variables](docs/ENV.md) | Deploy and album location variables                  |

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE.txt) (AGPL v3).

If you'd like to use this project under different terms, contact doug [at] donohoe [dot] info.
