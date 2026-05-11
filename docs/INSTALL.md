# Developer Setup

The following are aimed at developers who want to work directly from this repo,
instead of using the [Docker](DOCKER.md)-based `ddphotos` tool.

## Prerequisites

DD Photos uses Go, Node.js, `libvips`, so they must be installed and configured first.

**NOTE**: The following setup instructions are Mac-centric (via [Homebrew↗](https://docs.brew.sh/Installation)). Linux should work with 
equivalent package manager commands (`apt`, `yum`). Windows users should use WSL2.

```bash
# Install Go, vips library and pkg-config dependency (for photogen)
brew install go vips pkg-config

# In root of this repo, fetch Go libraries
go mod download
```

The website is a Node.js app. Install [nvm↗](https://github.com/nvm-sh/nvm#installing-and-updating)
first if you don't already have it.

```bash
# Install Node and dependencies (for the web app):
make web-nvm-install  # installs the Node version specified in web/.nvmrc
make web-npm-install  # install npm dependencies

# Optional: Install playwright dependencies if running e2e tests
make web-playwright-install  # installs Playwright + Chromium for e2e tests
```

You may also want to install [Docker↗](https://www.docker.com/get-started/) if
you don't have it, as it is required for testing site behavior using Apache or nginx.

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
