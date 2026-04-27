# Docker

The easiest way to run ddphotos is via Docker — no Go, Node, or libvips installation required.

## Quick Start

### 1. Install the `ddphotos` wrapper script

The wrapper script handles all `docker run` invocations for you. Install it into a directory on your `PATH`:

```bash
# Into ~/.local/bin (common on Linux/Mac)
docker run --rm -v ~/.local/bin:/ddphotos dougdonohoe/ddphotos init --script-only

# Or into ~/bin
docker run --rm -v ~/bin:/ddphotos dougdonohoe/ddphotos init --script-only
```

Or initialize a dedicated working directory that contains both the script and your config:

```bash
mkdir ~/my-ddphotos
docker run --rm -v ~/my-ddphotos:/ddphotos dougdonohoe/ddphotos init
cd ~/my-ddphotos
```

### 2. Generate, run, build, and serve the example site

```bash
cd ~/my-ddphotos
./ddphotos photogen   # resize images and create index files
./ddphotos run        # dev server at http://localhost:5173
./ddphotos build      # build static site
./ddphotos serve      # serve static site via Apache at http://localhost:8080
```

### 3. Build your own site

1. Edit `config/albums.yaml` to define your albums (see [Configuration](../README.md#configuration))
2. Repeat: `photogen` → `run` / `build` → `serve`

### 4. Deploy

Configure `config/site.env` for rsync or S3, then:

```bash
./ddphotos deploy
```

See [Deployment](../README.md#deployment) for full setup details.

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

Resizes source photos to WebP and generates JSON index files. Must be run before `build`, `run`, or `deploy`.

```bash
./ddphotos photogen
```

### `run`

Starts a Vite dev server at `http://localhost:5173`. Live-reloads on template/CSS changes.

```bash
./ddphotos run
```

### `build`

Builds the static site output into `build/`.

```bash
./ddphotos build
```

### `serve`

Serves the built static site via Apache at `http://localhost:8080`. Good for testing the final output before deploying.

```bash
./ddphotos serve
```

### `deploy`

Syncs the built site and album data to a remote host via rsync or S3. Requires `config/site.env`.

```bash
./ddphotos deploy
```

### `upgrade`

Updates the local `ddphotos` wrapper script to match the current Docker image.

```bash
./ddphotos upgrade
```

---

## Pre-command Flags

These flags go before the command name and apply to all commands that need them:

| Flag | Description |
|---|---|
| `--albums-dir <path>` | Directory containing your config and albums output (default: same directory as the `ddphotos` script) |
| `--config-dir <path>` | Path to a config directory other than `<albums-dir>/config` |
| `--site-id <id>` | Override the site ID (normally read from `config/albums.yaml`) |
| `--site-env <path>` | Path to a `site.env` file other than `<config-dir>/site.env` |

Example — using a separate source repo as the albums dir:

```bash
./ddphotos --albums-dir ~/work/ddphotos --config-dir ~/work/ddphotos/sample/config photogen
./ddphotos --albums-dir ~/work/ddphotos --site-id sample build
```

---

## Directory Layout

After `init`, your ddphotos directory looks like this:

```
my-ddphotos/
  ddphotos          ← wrapper script
  config/
    albums.yaml     ← album definitions and site settings
    description.txt ← per-album descriptions
    custom.css      ← optional CSS overrides
    passwords.yaml  ← optional password protection
    site.env        ← deploy credentials (not committed to git)
  albums/           ← photogen output (generated, not edited)
  build/            ← static site output (generated, not edited)
```

---

## Version Check and Upgrade

Every command (except `init` and `upgrade`) checks that your local `ddphotos` script matches the image. If they differ, you'll see:

```
Error: local ddphotos script does not match the image.
Run: ./ddphotos upgrade
```

Run `./ddphotos upgrade` to update the script in place.
