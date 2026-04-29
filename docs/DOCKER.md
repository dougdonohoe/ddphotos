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

**Quick option — [Surge↗](https://surge.sh)** (free, one command, no server required):

```bash
./ddphotos export --copy
surge --domain my-unique-site.surge.sh export/my-photos
```

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
can use the script to photogen and run the [sample site](https://ddphotos.donohoe.info/):

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

### `run`

Starts a Vite dev server at `http://localhost:5173`. Live-reloads on template/CSS changes.

```bash
ddphotos run
```

### `build`

Builds the static site output into `build/`.

```bash
ddphotos build
```

### `serve`

Serves the built static site via Apache at `http://localhost:8000`. 
Good for testing the final output before deploying.

```bash
ddphotos serve
```

### `export`

Exports the built site into `export/<site-id>/` — a directory of relative symlinks
that any static file server can read. Useful for serving with `python3 -m http.server`
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

Use `--copy` to produce real files instead of symlinks — required for static hosting
services like [Surge↗](https://surge.sh):

```bash
ddphotos export --copy
```

To deploy to Surge (free, no server required):

```bash
ddphotos export --copy
surge --domain my-unique-site.surge.sh export/my-photos
```

### `deploy`

Syncs the built site and album data to a remote host via rsync or S3. 
Requires `config/site.env`.

```bash
ddphotos deploy
```

See [Deployment](DEPLOY.md) for full setup details.

### `upgrade`

Updates the local `ddphotos` wrapper script to match the current Docker image.

```bash
ddphotos upgrade
```

### `version`

Prints the script location, image tag, and config paths. Runs locally — no Docker required.

```bash
ddphotos version
```

```
Script:      /Users/anseladams/.local/bin/ddphotos
Image:       dougdonohoe/ddphotos:v1.2.0
Albums dir:  /Users/anseladams/my-ddphotos
Config dir:  /Users/anseladams/my-ddphotos/config
Site ID:     my-photos
```

Add `--image` to also query the image for its build details:

```bash
ddphotos version --image
```

```
Script:      /Users/anseladams/.local/bin/ddphotos
Image:       dougdonohoe/ddphotos:v1.2.0
               Version:  v1.2.0
               Git:      v1.2.0-0-gabcdef1
Albums dir:  /Users/anseladams/my-ddphotos
Config dir:  /Users/anseladams/my-ddphotos/config
Site ID:     my-photos
```

The `--dir`, `--config-dir`, and `--site-id` pre-command flags also work with `version`, making it useful for confirming which config a given invocation would use.

---

## Pre-command Flags

These flags go before the command name and apply to all commands that need them:

| Flag                  | Description                                                                                           |
|-----------------------|-------------------------------------------------------------------------------------------------------|
| `--dir <path>` | Directory containing your config and albums output (default: same directory as the `ddphotos` script) |
| `--config-dir <path>` | Path to a config directory other than `<albums-dir>/config`                                           |
| `--site-id <id>`      | Override the site ID (normally read from `config/albums.yaml`)                                        |
| `--site-env <path>`   | Path to a `site.env` file other than `<config-dir>/site.env`                                          |

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
    descriptions.txt ← per-album descriptions
    custom.css       ← optional CSS overrides
    passwords.yaml   ← optional password protection
    site.env         ← deploy credentials (not committed to git)
  albums/            ← photogen output (generated, not edited)
  build/             ← static site output (generated, not edited)
  export/            ← export output (generated, not edited)
```

---

## Version Check and Upgrade

Every command (except `init`, `upgrade`, and `version`) checks that your local `ddphotos` script matches the image. If they differ, you'll see:

```
Error: local ddphotos script does not match the image.
Run: ddphotos upgrade
```

Run `ddphotos upgrade` to update the script in place.
