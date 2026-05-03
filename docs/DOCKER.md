# Docker

The easiest way to run ddphotos is via Docker — no Go, Node, or libvips installation required.

## Quick Start

### 1. Initialize Scaffolding

Initialize a dedicated working directory that contains both the `ddphotos` script and
a starter config:

```bash
mkdir ~/my-ddphotos
docker run --rm -v ~/my-ddphotos:/ddphotos dougdonohoe/ddphotos init
```

### 2. Generate, run, build, and serve the starter site

```bash
cd ~/my-ddphotos
./ddphotos photogen   # resize images and create index files
./ddphotos run        # run dev server at http://localhost:5173
./ddphotos build      # build static site
./ddphotos serve      # serve static site via Apache at http://localhost:8000
./ddphotos export     # (optional) export for local serving without Apache
```

### 3. Build your own site

1. Edit `config/albums.yaml` to define your albums (see [Configuration](CONFIGURATION.md) for details)
2. Repeat: `photogen` → `run` / `build` → `serve`

### 4. Deploy

**Quick option — [Surge↗](https://surge.sh)** - free, one command, no server required (requires
[Node.js](http://nodejs.org/)):

```bash
# Install
npm install --global surge

# Export and deploy (and on 1st time, create account/login)
./ddphotos export --copy
surge --domain my-unique-site.surge.sh export/my-photos
```

The site will be at https://my-unique-site.surge.sh.

**Quick option — [Cloudflare Pages↗](https://pages.cloudflare.com)** - free, unlimited bandwidth (requires
[Node.js](http://nodejs.org/) and [Cloudflare account](https://dash.cloudflare.com/login)):

```bash
# Install and login
npm install -g wrangler --ignore-scripts
wrangler login

# Export and deploy
./ddphotos export --cloudflare
wrangler pages deploy --project-name my-unique-site export/my-photos
```

The site will be at https://my-unique-site.pages.dev.

**Production option** — configure `config/site.env` for rsync or S3
(see [site.env](CONFIGURATION.md#siteenv)), then:

```bash
./ddphotos deploy
```

See [Deployment](DEPLOY.md) for full setup details.

### 5. Install the `ddphotos` wrapper script

Advanced users can install just the wrapper script into a directory on their `PATH`:

```bash
# Into ~/.local/bin (common on Linux/Mac)
docker run --rm -v ~/.local/bin:/ddphotos dougdonohoe/ddphotos init --script-only

# Or into ~/bin
docker run --rm -v ~/bin:/ddphotos dougdonohoe/ddphotos init --script-only
```

If you have `ddphotos` on the path and the `ddphotos` repo checked out under `~/work`, you
can use the script to photogen and run the [sample site↗](https://ddphotos.donohoe.info/):

```bash
ddphotos --dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config photogen
ddphotos --dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config run
```

---

## Commands

### `init`

Creates the config scaffold and installs the `ddphotos` wrapper script.

```bash
# Full init (script + config)
docker run --rm -v ~/my-ddphotos:/ddphotos dougdonohoe/ddphotos init

# Script only (no config scaffold)
docker run --rm -v ~/.local/bin:/ddphotos dougdonohoe/ddphotos init --script-only
```

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

See [CLI Flags](PHOTOGEN.md#cli-flags) for all flags.

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

### `run`

Starts a Vite dev server at http://localhost:5173. Live-reloads on template/CSS changes.

```bash
ddphotos run
```

### `build`

Builds the static site output into `build/`.

```bash
ddphotos build
```

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
surge --domain my-unique-site.surge.sh export/my-photos
```

Use `--cloudflare` for [Cloudflare Pages↗](https://pages.cloudflare.com) — adds a `_worker.js`
for photo permalink routing (symlinks are followed, so `--copy` is not needed):

```bash
ddphotos export --cloudflare
wrangler pages deploy --project-name my-unique-site export/my-photos
```

See [Cloudflare Pages Worker](DEPLOYMENT-SERVERS.md#cloudflare-pages-worker) for details.

### `deploy`

Syncs the built site and album data to a remote host via rsync or S3. 
Requires `config/site.env`.

```bash
ddphotos deploy
```

See [Deployment](DEPLOY.md) for full setup details.

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

## Pre-command Flags

These flags go before the command name and apply to all commands that need them:

| Flag                  | Description                                                                                             |
|-----------------------|---------------------------------------------------------------------------------------------------------|
| `--dir <path>`        | Directory containing your `config` and `albums` dirs (default: same directory as the `ddphotos` script) |
| `--config-dir <path>` | Path to a config directory other than `<dir>/config`                                                    |
| `--site-id <id>`      | Override the site ID (normally read from `config/albums.yaml`)                                          |
| `--site-env <path>`   | Path to a `site.env` file other than `<config-dir>/site.env`                                            |
| `--non-interactive`   | Run `serve` and `run` without a TTY (no `-it` flag) — useful for scripted/CI contexts                   |

Example — using a separate source repo as the albums dir:

```bash
ddphotos --dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config photogen
ddphotos --dir ~/work/ddphotos --site-id sample build
```

---

## Directory Layout

After `init`, your ddphotos directory looks like this:

```
my-ddphotos/
  ddphotos           ← wrapper script
  config/
    albums.yaml      ← album definitions and site settings
    custom.css       ← optional CSS overrides
    descriptions.txt ← per-album descriptions
    passwords.yaml   ← optional password protection
    site.env         ← deploy credentials
  albums/            ← photogen output (generated, not edited)
  build/             ← static site output (generated, not edited)
  export/            ← export output (generated, not edited)
```

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
