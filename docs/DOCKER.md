# Docker Mode

After the [DD Photos desktop app↗](https://github.com/dougdonohoe/ddphotos-app/blob/main/README.md), the next easiest way to run `ddphotos` is via Docker — no complicated
developer tool installation is required.

## Quick Start

### 1. Install Docker

If you don't have Docker installed, click on the **Download Docker Desktop** button at the
[Docker Getting Started↗](https://www.docker.com/get-started/) page to download the appropriate
installer for your machine.  Install it and then start _Docker.app_ (Mac) or _Docker Desktop_ (Windows/Linux).

### 1w. Windows Setup

Windows users also need to install [Git For Windows↗](https://git-scm.com/download/win).  This is 
so that the internal `ddphotos` bash and linux-style commands work. **NOTE**: It is
perfectly **OK** to just accept all the default options (there are a lot of screens!).

To adapt these docs for Windows:

* Run commands in the `Terminal` app running `Windows PowerShell`
* In place of `~` use `$HOME` (`~` works for some things, but not always)

### 2. Initialize Scaffolding

Initialize a dedicated working directory that contains the `ddphotos` script, a starter
config, and the sample photos that config points at:

```bash
mkdir $HOME/my-ddphotos

# Mac/Linux
docker run --rm -v $HOME/my-ddphotos:/ddphotos dougdonohoe/ddphotos init

# Windows - also installs `ddphotos.cmd` wrapper
docker run --rm -v $HOME/my-ddphotos:/ddphotos dougdonohoe/ddphotos init --windows
```

This creates `config/` (`albums.yaml`, `custom.css`, `passwords.yaml`, `site.env`) and a
`sample-photos/` folder beside it, along with the empty `albums/`, `build/` and `export/`
output directories. See [Directory Layout](#directory-layout).

### 3. Generate, run, build, and serve the starter site

```bash
cd ~/my-ddphotos
./ddphotos photogen   # resize images and create index files
./ddphotos run        # run dev server at http://localhost:5173
./ddphotos build      # build static site
./ddphotos serve      # serve static site via Apache at http://localhost:8000
```

The starter site has three albums — `vacation`, a password-protected `secret` (the password
is `secret`), and an intentionally `empty` one. All three are ordinary folders inside
`sample-photos/`, reached from `config/albums.yaml` through the `sample-base` entry in its
`bases:` block:

```yaml
bases:
  sample-base: sample-photos   # relative to your ddphotos folder

albums:
  - slug: vacation
    name: Vacation
    base: sample-base
    source: vacation           # -> sample-photos/vacation
```

### 4. Build your own site

1. Edit `config/albums.yaml` to define your albums (see [Configuration](CONFIGURATION.md) for details)
2. Repeat: `photogen` → `run` / `build` → `serve`
3. Once your own albums are in place, delete `sample-photos/` along with the `sample-base`
   entry and the three starter albums

### 5. Deploy

**Quick option #1 — [Cloudflare Pages↗](https://pages.cloudflare.com)** - free, unlimited bandwidth (requires
a [Cloudflare account](https://dash.cloudflare.com/login); `wrangler` is bundled — no local install needed):

```bash
# One-time login (opens browser; credentials cached for future deploys)
./ddphotos wrangler login

# Export and deploy
./ddphotos export --cloudflare
./ddphotos wrangler pages deploy --project-name my-unique-site export/my-photos
```

The site will be at https://my-unique-site.pages.dev.

**Quick option #2 — [Surge↗](https://surge.sh)** - free, one command, no server required (`surge` is
bundled — no local install needed):

```bash
# Export and deploy (prompts for login on first run)
./ddphotos export --copy
./ddphotos surge --domain my-unique-site.surge.sh export/my-photos
```

The site will be at https://my-unique-site.surge.sh.

**Production option** — configure `config/site.env` for rsync or S3
(see [site.env](CONFIGURATION.md#siteenv)), then:

```bash
./ddphotos deploy
```

See [Deployment](DEPLOY.md) for full setup details.

### 6. Install the `ddphotos` wrapper script

Advanced users can install just the wrapper script into a directory on their `PATH`:

```bash
# Into ~/.local/bin (common on Linux/Mac)
docker run --rm -v ~/.local/bin:/ddphotos dougdonohoe/ddphotos init --script-only

# Or into ~/bin
docker run --rm -v ~/bin:/ddphotos dougdonohoe/ddphotos init --script-only
```

Once the script is on your `PATH`, you can create a starter site (config scaffold) in any
directory by passing `--dir`:

```bash
mkdir ~/my-ddphotos
ddphotos --dir ~/my-ddphotos init
```

This scaffolds `config/` (`albums.yaml`, `custom.css`, `passwords.yaml`, `site.env`) and the
`sample-photos/` folder into `~/my-ddphotos`, the same as the full `init` in
[Initialize Scaffolding](#2-initialize-scaffolding). Pass `--site-id` to set a custom site ID. From then on, 
use `--dir ~/my-ddphotos` (or `cd ~/my-ddphotos`) with the other commands.

If you have `ddphotos` on the path and the `ddphotos` repo checked out under `~/work`, you
can use the script to photogen and run the [sample site↗](https://ddphotos.donohoe.info/):

```bash
ddphotos --dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config photogen
ddphotos --dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config run
```
---

## `ddphotos`

Usage:

```text
ddphotos [options] [command] [args]
```

---

### Pre-Command Options

These flags go before the command name and apply to all commands that need them:

| Flag                | Description                                                                                                |
|---------------------|------------------------------------------------------------------------------------------------------------|
| `--dir DIR`         | Directory containing your `config` and `albums` dirs (default: same directory as the `ddphotos` script)    |
| `--config-dir DIR`  | Path to a config directory other than `<dir>/config`                                                       |
| `--site-id ID`      | Override the site ID (normally read from `config/albums.yaml`)                                             |
| `--site-env FILE`   | Path to a `site.env` file other than `<config-dir>/site.env`                                               |
| `--non-interactive` | Run `serve` and `run` without a TTY (no `-it` flag) — useful for scripted/CI contexts                      |
| `--show-mounts`     | Print the Docker volume mounts before running the command — useful for debugging mount issues              |
| `--dev`             | Use the locally-built `ddphotos` image instead of the pinned release tag — useful for testing local builds |

Example — using a separate source repo as the albums dir:

```bash
ddphotos --dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config photogen
ddphotos --dir ~/work/ddphotos --site-id sample build
```

---

## Commands

| Command                         | Description                                                       |
|---------------------------------|-------------------------------------------------------------------|
| [`init`](#init)                 | Create config scaffold and install the `ddphotos` wrapper script. |
| [`photogen`](#photogen)         | Resize photos to WebP and generate JSON index files.              |
| [`run`](#run)                   | Start a Vite dev server at http://localhost:5173.                 |
| [`build`](#build)               | Build the static site into `build/`.                              |
| [`serve`](#serve)               | Serve the built site via Apache at http://localhost:8000.         |
| [`export`](#export)             | Export the built site to `export/<site-id>/` for static hosting.  |
| [`deploy`](#deploy)             | Sync the built site to a remote host via rsync or S3.             |
| [`wrangler`](#wrangler)         | Run the bundled Cloudflare CLI (deploy to Cloudflare Pages).      |
| [`surge`](#surge)               | Run the bundled Surge CLI (deploy to Surge).                      |
| [`decode`](#decode)             | Decrypt an `.enc.json` file and print the plaintext JSON.         |
| [`search-cover`](#search-cover) | Find the original filename for a photo given its URL.             |
| [`upgrade`](#upgrade)           | Update the local `ddphotos` wrapper script to match the image.    |
| [`version`](#version)           | Print script location, image tag, and config paths.               |

### `init`

Creates the config scaffold and the `sample-photos/` starter photos, and installs the
`ddphotos` wrapper script.

```bash
# Full init (script + config)
docker run --rm -v ~/my-ddphotos:/ddphotos dougdonohoe/ddphotos init

# Set a custom site ID (written into config/albums.yaml; default: my-photos)
docker run --rm -v ~/my-ddphotos:/ddphotos dougdonohoe/ddphotos init --site-id my-site

# Script only (no config scaffold)
docker run --rm -v ~/.local/bin:/ddphotos dougdonohoe/ddphotos init --script-only

# Using an already-installed script (e.g. after --script-only) to scaffold a new site dir
ddphotos --dir ~/my-ddphotos init
```

| Flag            | Description                                                                           |
|-----------------|---------------------------------------------------------------------------------------|
| `--site-id ID`  | Site ID written into `config/albums.yaml` as `settings.id` (default: `my-photos`)     |
| `--script-only` | Install just the `ddphotos` wrapper script; skip config and sample photos             |
| `--windows`     | Also install the `ddphotos.cmd` Windows launcher (added automatically under Git Bash) |

If `ddphotos` is already on your `PATH` (for example, installed via `--script-only`), run
`ddphotos --dir <path> init` to scaffold a starter config into `<path>` without a raw
`docker run`. See [Install the `ddphotos` wrapper script](#6-install-the-ddphotos-wrapper-script).

### `photogen`

Resizes source photos to WebP and generates JSON index files. Must be run 
before `build`, `run`, or `deploy`.

```bash
ddphotos photogen
```

By default, this uses `-resize -index -clean -doit`.  To define your own
flags, use `--`:

```bash
ddphotos photogen -- -hero-only
```

See [photogen CLI Flags](PHOTOGEN.md#cli-flags) for all `photogen` flags.

Source videos (`.mov`, `.mp4`, `.m4v`) are transcoded to web-playable MP4 alongside the
photos. The first run that meets a video downloads ffmpeg into the `ddphotos-ffmpeg` Docker
volume, where it stays for later runs; a photo-only site never downloads anything. See
[Video](PHOTOGEN.md#video) and [`install-ffmpeg`](#install-ffmpeg).

### `install-ffmpeg`

Pre-downloads ffmpeg into the `ddphotos-ffmpeg` Docker volume. Optional: `photogen` does
this automatically the first time it encounters a video.

```bash
ddphotos install-ffmpeg
ddphotos install-ffmpeg --force   # reinstall over an existing copy
```

| Aspect       | Detail                                                                                  |
|--------------|-----------------------------------------------------------------------------------------|
| Cache        | Docker volume `ddphotos-ffmpeg`, mounted at `/opt/ddphotos/ffmpeg`                      |
| Download     | A pinned static build, verified against a recorded SHA-256, roughly 120 MB              |
| Image impact | None: ffmpeg is deliberately not baked into the image (see [Video](PHOTOGEN.md#ffmpeg)) |

To reclaim the space, `docker volume rm ddphotos-ffmpeg`. It is re-downloaded on the next
run that needs it.

### `run`

Starts a Vite dev server at http://localhost:5173. Live-reloads on template/CSS changes.

```bash
ddphotos run
```

### `build`

Builds the static site output into `build/<site-id>`.

```bash
ddphotos build
```

If your `config` directory has a
`static` sub-directory, any files in there are copied to the root
of the build.  This is useful for files like `humans.txt` or `llms.txt`.

### `serve`

Serves the built static site via Apache at http://localhost:8000. 
Good for testing the final output before deploying.

```bash
ddphotos serve
```

### `export`

Exports the built site into `export/<site-id>/` — a directory of relative symlinks or
real files that any static file server can read. Useful for serving with `python3 -m http.server`
or uploading to a static hosting service.

```bash
ddphotos export
```

Serve the exported directory with Python:

```bash
python3 -m http.server 8000 --directory export/my-photos
```

See [Local Testing with Python](DEPLOYMENT-SERVERS.md#local-testing-with-python) for notes
on limitations and usage.

Use `--copy` to produce real files instead of symlinks — required for services like
[Surge↗](https://surge.sh) that don't follow symlinks:

```bash
ddphotos export --copy
ddphotos surge --domain my-unique-site.surge.sh export/my-photos
```

See [`surge`](#surge) for credentials and other details.

Use `--cloudflare` for [Cloudflare Pages↗](https://pages.cloudflare.com) — adds a `_worker.js`
for photo permalink routing (symlinks are followed, so `--copy` is not needed):

```bash
ddphotos export --cloudflare
ddphotos wrangler pages deploy --project-name my-unique-site export/my-photos
```

See [Cloudflare Pages Worker](DEPLOYMENT-SERVERS.md#cloudflare-pages-worker) for how the routing
works, and [`wrangler`](#wrangler) for credentials and other details.

Use `--export-site-id` to write the export to a different subdirectory name instead of
`export/<site-id>/`:

```bash
ddphotos export --export-site-id my-alternate-name
```

### `deploy`

Syncs the built site and album data to a remote host via rsync or S3. 
Requires `config/site.env`.

```bash
ddphotos deploy
```

Extra flags pass through to the underlying deploy script. For example, to select a named
AWS profile from `~/.aws` instead of relying on `AWS_*` environment variables:

```bash
ddphotos deploy --aws-profile my-profile
```

See [Deployment](DEPLOY.md) for full setup details.

### `wrangler`

Runs [wrangler↗](https://developers.cloudflare.com/workers/wrangler/), the Cloudflare CLI, inside
the image via `npx` — no local Node or wrangler install needed. The working directory is `--dir`,
so paths like `export/<site-id>` resolve as you would expect. All arguments pass straight through.

See [Cloudflare Pages](DEPLOY.md#cloudflare-pages) for the deploy walkthrough.

```bash
ddphotos wrangler login    # one-time
ddphotos wrangler pages deploy --project-name my-unique-site export/my-photos
```

Docker-specific behavior:

| Aspect           | Detail                                                                                   |
|------------------|------------------------------------------------------------------------------------------|
| Credentials      | `wrangler login`, or set `CLOUDFLARE_API_TOKEN`                                          |
| `wrangler login` | Exposes port 8976 for the OAuth callback and prints the auth URL to open in your browser |
| Credential cache | Docker volume `ddphotos-wrangler-config`, so login persists across runs                  |
| First run        | Downloads wrangler into the `ddphotos-npm-cache` volume                                  |

`ddphotos wrangler pages deploy` checks the target directory first and refuses to deploy one that
is missing or has no `_worker.js`, since that means `export` was run without `--cloudflare` and
photo permalinks would 404.

### `surge`

Runs the [Surge↗](https://surge.sh) CLI inside the image via `npx` — no local install needed.
As with `wrangler`, the working directory is `--dir` and all arguments pass through.

See [Surge](DEPLOY.md#surge) for the deploy walkthrough.

```bash
ddphotos surge --domain my-unique-site.surge.sh export/my-photos
```

Docker-specific behavior:

| Aspect          | Detail                                                                           |
|-----------------|----------------------------------------------------------------------------------|
| Credentials     | Prompted on first run and stored in `~/.netrc`, mounted read-write from the host |
| Non-interactive | Set `SURGE_LOGIN` and `SURGE_TOKEN`                                              |
| First run       | Downloads surge into the `ddphotos-npm-cache` volume                             |

Surge does not follow symlinks, so the export must be made with `--copy`. `ddphotos surge` checks
for this and fails with a clear message rather than uploading a directory of broken links.
Bare subcommands (`ddphotos surge list`, `whoami`, `login`) skip the check.

### `decode`

Decrypts an `.enc.json` file produced by `photogen` and prints the plaintext JSON.
Useful for inspecting what an encrypted album or site index contains — for example,
to find a photo's original filename from its UUID so you can set it as a cover.

```bash
ddphotos decode albums/my-photos/secret/index.enc.json
ddphotos decode albums/my-photos/albums.enc.json
```

The passwords file path is embedded in every `.enc.json` by `photogen`, so no extra
flags are needed in normal use. If the embedded path is unreachable, pass it explicitly:

```bash
ddphotos decode --passwords config/passwords.yaml albums/my-photos/secret/index.enc.json
```

Paths are resolved relative to the `--dir` directory (default: the `ddphotos` script
location). Files outside that directory are mounted automatically.

### `search-cover`

Finds the original filename for a photo given its URL — useful for setting a cover image
in `albums.yaml`. Pass any photo URL from your site (full-size or grid thumbnail):

```bash
ddphotos search-cover https://my-site.example.com/albums/banff-2002/full/0918bedf-2f7d-dedc-9e89-b99ec5bb2752.webp
```

Output:

```
Searching...
  Album:  banff-2002
  Index:  /ddphotos/albums/my-photos/banff-2002/index.json
  Source: full/0918bedf-2f7d-dedc-9e89-b99ec5bb2752.webp

Found:
  id:         0918bedf-2f7d-dedc-9e89-b99ec5bb2752
  sourcePath: /photos/banff-2002/IMG_1234.jpg
  fileName:   IMG_1234.jpg

Use for cover:
  cover: IMG_1234.jpg
```

Use the `--site-id` flag to search a different site:

```bash
ddphotos --site-id other-site search-cover <url>
```

### `upgrade`

Updates the local `ddphotos` wrapper script to match the image. Run this when an update
notice appears to install the newer version, or to fix a script/image mismatch.

```bash
ddphotos upgrade
```

### `version`

Prints the script location, image tag, and config paths. Runs locally — no Docker required.

```bash
ddphotos version
```

```
Script:         /Users/anseladams/.local/bin/ddphotos
Image:          dougdonohoe/ddphotos:v1.2.0
DD Photos dir:  /Users/anseladams/my-ddphotos
Config dir:     /Users/anseladams/my-ddphotos/config
Site ID:        my-photos
State dir:      /Users/anseladams/.config/ddphotos
```

Add `--image` to also query the image for its build details:

```bash
ddphotos version --image
```

```
Script:         /Users/anseladams/.local/bin/ddphotos
Image:          dougdonohoe/ddphotos:v1.2.0
                  Version:  v1.2.0
                  Git:      v1.2.0-0-gabcdef1
DD Photos dir:  /Users/anseladams/my-ddphotos
Config dir:     /Users/anseladams/my-ddphotos/config
Site ID:        my-photos
State dir:      /Users/anseladams/.config/ddphotos
```

The `--dir`, `--config-dir`, and `--site-id` pre-command flags also work with `version`, making it useful for confirming which config a given invocation would use.

---

## Directory Layout

After `init`, your ddphotos directory looks like this:

```
my-ddphotos/
  ddphotos           ← wrapper script
  config/
    albums.yaml      ← album definitions, site settings, and descriptions
    custom.css       ← optional CSS overrides
    passwords.yaml   ← optional password protection
    site.env         ← deploy credentials
  sample-photos/     ← starter photos (delete once you add your own albums)
    README.md        ← what these are and how they are licensed
    hero.webp        ← the starter site's hero banner
    vacation/        ← the 'vacation' album's source photos
    secret/          ← the 'secret' album, plus a photogen.txt caption
    empty/           ← the 'empty' album (intentionally has no photos)
  albums/            ← photogen output (generated, not edited)
  build/             ← static site output (generated, not edited)
  export/            ← export output (generated, not edited)
```

`sample-photos/` is an ordinary folder on your machine, so you can browse, caption and
delete its photos like any other album source.

---

## Version Check and Upgrade

### Automatic update check

Each time you run `ddphotos`, it checks Docker Hub for a newer release — at most once per
day, and only when the script is pinned to a versioned release (not a `dev` build). State is
stored in `~/.config/ddphotos/` (created automatically on first run).

If a newer version is available, a notice prints to stderr on every subsequent run:

```
Update available: v1.3.0 - run 'ddphotos upgrade' to update
```

Run `ddphotos upgrade` to pull and install it. The notice is cleared automatically once
the installed version matches the latest.

### Script/image mismatch check

Every command (except `init`, `upgrade`, and `version`) also checks that your local
`ddphotos` script matches the running image. In normal use with a tagged release this
should never fire — the automatic update check keeps things in sync. It is primarily
relevant for `dev` builds, or in the unlikely event you manually edit the `ddphotos`
script. If they differ, a warning is printed and the command continues normally:

```
WARNING:  The local 'ddphotos' script does not match the image.
          Run: 'ddphotos upgrade' to fix this.
```

Run `ddphotos upgrade` to bring the script back in sync.
