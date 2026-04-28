# Non-Docker Notes

The following are aimed at developers who want to work directly from this repo,
instead of using the [Docker](DOCKER.md)-based `ddphotos` tool.

## Prerequisites

The following setup instructions are Mac-centric (via [Homebrew](https://docs.brew.sh/Installation)). Linux should work with 
equivalent package manager commands (`apt`, `yum`). Windows users should use WSL2.

```bash
# Install Go, vips library and pkg-config dependency (for photogen)
brew install go vips pkg-config

# In root of this repo, fetch Go libraries
go mod download
```

The website is a Node.js app. Install
[nvm](https://github.com/nvm-sh/nvm#installing-and-updating) first if
you don't already have it.

```bash
# Install Node and dependencies (for the web app):
make web-nvm-install  # installs the Node version specified in web/.nvmrc
make web-npm-install  # install npm dependencies

# Optional: Install playwright dependencies if running e2e tests
make web-playwright-install  # installs Playwright + Chromium for e2e tests
```

You may also want to install [Docker](https://www.docker.com/get-started/) if
you don't have it, as it is required for testing site behavior using Apache or nginx.

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
make sample-demo
```

This photogen's the sample site with all albums password-protected and a custom CSS
override applied, then launches the dev server. The password for the sample site is
`allgood`; the Uganda album password is `gorilla`; the Antarctica password is
`penguin`.  The CSS changes the font color
to cyan and rounds the album card corners a bit more.

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
examples in `config`.  The sections below provide details about how everything
works.

## Configuration

There are three primary configuration files involved in creating a site:

* [`albums.yaml`](../config/albums.example.yaml) - **Required** - Defines your list of albums, an id for the site (useful if
  you have multiple sites), and the locations of your photos.
* [`descriptions.txt`](../config/descriptions.example.txt) - **Optional** - The description of the album that you see. This
  is in a separate file to allow sharing of albums across sites (useful in development), 
  and also enables localization in the future.
* [`site.env`](../config/site.example.env) - **Optional** - Environment variables for deployment and testing.

The `config` directory contains an example of each file which serves as its
detailed documentation.  The `sample/config` files are a working 
example that drives our sample photo album seen at [ddphotos.donohoe.info](https://ddphotos.donohoe.info).

The `config` directory is the default assumed by many commands, so feel free to put 
your config files there. Just copy the examples and edit them:

```bash
cp config/albums.example.yaml config/albums.yaml
cp config/descriptions.example.txt config/descriptions.txt
cp config/site.example.env config/site.env
```

**NOTE**: The `settings.id` value in `albums.yaml` is referred to as `<site-id>` below.
Make sure you change it to something that reflects your actual site, like `vacations` or 
`memories`.

Another option is to get yourself started is to edit the config files for the sample app.  
Or create your own config directory and use the `--config-dir` option.

## Commands

The `Makefile` is a good reference for the various DD Photos commands 
(you used them to run the sample site). Assuming you put your config files 
in `config`, these commands are useful:

### Resize and Index

```bash
# Dry run of indexing and resizing
go run cmd/photogen/photogen.go -resize -index -clean

# Do it for real
go run cmd/photogen/photogen.go -resize -index -clean -doit
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
the files needed to run the site.

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

These should pass against your site assuming you setup the `TEST_*`
variables in `site.env` to match your site.

## Developer Information

For complete details about `photogen`, the SvelteKit site, testing, 
deployment and other technical information, see the [docs/](.) directory.
