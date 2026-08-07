# Developer Setup

The following are aimed at developers who want to work directly from this repo,
instead of using the [Docker](DOCKER.md)-based `ddphotos` tool.

## Clone This Repo

Clone `ddphotos` using your preferred method:

```bash
# HTTPS
git clone https://github.com/dougdonohoe/ddphotos.git

# or SSH
git clone git@github.com:dougdonohoe/ddphotos.git

cd ddphotos
```

HTTPS needs no GitHub account or credentials to clone a public repo, so it is the
simplest option if you just want to build and run the site. SSH requires a GitHub
account and an SSH key.

## Prerequisites

DD Photos uses Go, Node.js, `libvips`, so they must be installed and configured first.
Instructions are given for macOS (via [Homebrew↗](https://docs.brew.sh/Installation)) and
Debian/Ubuntu (`apt`). Other distributions should work with equivalent package manager
commands (`dnf`, `pacman`). Windows users should use WSL2.

### macOS

```bash
# Install Go, vips library and pkg-config dependency (for photogen)
brew install go vips pkg-config
```

### Debian/Ubuntu

```bash
# Install Go, vips library and pkg-config dependency (for photogen)
sudo apt-get install golang-go libvips-dev pkg-config build-essential

# HEIC/HEIF decoding (iPhone photos)
sudo apt-get install libheif-plugin-libde265
```

Note the package is `libvips-dev`, not `vips` — the headers are needed to compile
`photogen`. `build-essential` supplies the C compiler that cgo requires, and is often
already installed.

### Both platforms

```bash
# In root of this repo, fetch Go libraries
go mod download
```

The website is a Node.js app. Install [nvm↗](https://github.com/nvm-sh/nvm#installing-and-updating)
first if you don't already have it.

```bash
# Install Node and dependencies (for the web app):
make web-nvm-install  # installs Node (web/.nvmrc) and npm (web/.npm-version)
make web-npm-install  # install npm dependencies

# Optional: Install playwright dependencies if running e2e tests
make web-playwright-install  # installs Playwright + Chromium for e2e tests
```

The `web/.nvmrc` (Node major version) and `web/.npm-version` (exact npm version) files are the
single sources of truth for the toolchain versions - the Makefile, Docker build and
CI all read them. `web/package.json` sets a matching `engines.node`, and with
`engine-strict=true` in `web/.npmrc` an `npm install` on the wrong Node version
fails fast rather than silently installing.

If your system already provides a `node` — Ubuntu's `nodejs` package is often pulled in as a
dependency of something else — the Makefile uses it only when its major version matches
`web/.nvmrc`, and otherwise falls back to nvm. A distro Node at the wrong version will not
shadow the repo's.

You may also want to install [Docker↗](https://www.docker.com/get-started/) if
you don't have it, as it is required for testing site behavior using Apache or nginx.
On Linux, install Docker Engine and add yourself to the `docker` group
([post-install steps↗](https://docs.docker.com/engine/install/linux-postinstall/)) so the
`make` targets can run it without `sudo`.

## Developer Tools on PATH

The repo's `bin/` directory contains developer wrapper scripts. Add it to your PATH
so you can run `bin/photogen` and `bin/decode` from anywhere in the repo:

```bash
# Add to your shell profile (~/.zshrc or ~/.bashrc)
export PATH="$PATH:/path/to/ddphotos/bin"
```

Or just use the `bin/` prefix when invoking from the repo root, which is what all
examples in these docs use.

## Sample App

Once you have the required software installed, you should be able to
build and view the sample site provided within this repo (in the `sample` dir).

```bash
# Resize photos and generate .json files
make sample-photogen

# Run dev server
make sample-npm-run-dev
```

You should see a `VITE` message and a browser window should
open at [localhost:5173](http://localhost:5173/).

To try a site with password protection and custom CSS together in one step:

```bash
make sample-demo-1
make sample-demo-2
```

Demo #1 `photogen`'s the sample site with all albums password-protected and a custom CSS
override applied, then launches the dev server. The password for the sample site is
`allgood`; the Uganda album password is `gorilla`; the Antarctica password is
`penguin`.  The CSS changes the font color to cyan and rounds the album card corners a bit more.

Demo #2 is the same, but the site has no password, just the Uganda album.

You can also build the static site and test it in Apache/nginx (requires Docker and
assumes `photogen` has been run).

```bash
# Build docker image (one time)
make web-docker-build-apache
make web-docker-build-nginx

# Build sample site
make sample-build

# Run it in Docker w/ Apache/nginx
make web-docker-run-apache
make web-docker-run-nginx  
```

You should be able to see the site at [localhost:8080](http://localhost:8080).

**Congratulations!**  Now that you've got the sample site working, you can
work on your own albums.  You can start first by adding to the sample config
in `sample/config/albums.yaml`.  Or you can start building your own using the
examples in `config`.  See [CONFIGURATION.md](CONFIGURATION.md) for the full
config reference.

## Commands

The `Makefile` is a good reference for the various DD Photos commands 
(you used them to run the sample site). Assuming you put your config files 
in `config`, these commands are useful:

### Resize and Index

```bash
# Dry run of indexing and resizing
bin/photogen -resize -index -clean

# Do it for real
bin/photogen -resize -index -clean -doit
```

**NOTE**: output goes to `albums/<site-id>` at the repo root by default. For example,
the sample site is in `albums/sample`.

### Run Site

Once `photogen` has been successfully run, you can run the
dev server.

```bash
DDPHOTOS_SITE_ID=<site-id> make web-npm-run-dev
```

### Build and Test with Docker

To test the build process:

```bash
DDPHOTOS_SITE_ID=<site-id> make web-npm-build
```

This deletes and recreates the `build/<site-id>` directory, which will have all
the files needed to run the site.  If your `config` directory has a 
`static` sub-directory, any files in there are copied to the root
of the build.  This is useful for files like `humans.txt` or `llms.txt`.

To run the built site using Docker, choose Apache or nginx:

```bash
DDPHOTOS_SITE_ID=<site-id> make web-docker-run-apache # Apache
DDPHOTOS_SITE_ID=<site-id> make web-docker-run-nginx  # nginx
```

You should be able to see the site at [localhost:8080](http://localhost:8080).

### Test Site

Assuming an Apache or nginx server is running, you can run the 
routing smoke tests:

```bash
make web-docker-test
```

These should pass against any non-password-protected site.

## Developer Information

For complete details about `photogen`, the SvelteKit site, testing, 
deployment and other technical information, see the [Documentation index](../README.md#documentation).
