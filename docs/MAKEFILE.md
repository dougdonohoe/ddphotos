# Makefile Targets

**Note:** This page is for contributors developing DD Photos, not for users building their own sites with it.

Common tasks are available via `make` from the repo root.

**NOTE**: Most targets use `$DDPHOTOS_SITE_ID` to choose which site to operate on.  This defaults to `sample`,
as defined in `config/defaults.env`.

| Target                        | Description                                                                                   |
|-------------------------------|-----------------------------------------------------------------------------------------------|
| `help`                        | Show all available make targets (default when running `make`)                                 |
| `build`                       | Compile all Go binaries                                                                       |
| `test`                        | Run Go unit tests                                                                             |
| `mod-tidy`                    | Run `go mod tidy` to clean up imports                                                         |
| `clean-cache`                 | Run `go clean -cache` (useful after a vips library upgrade)                                   |
| `vet`                         | Run `go vet` static analysis                                                                  |
| `web-nvm-install`             | Install the Node version specified in `web/.nvmrc`                                            |
| `web-npm-install`             | Install npm dependencies in `web/`                                                            |
| `web-npm-run-dev`             | Start Vite dev server and open browser                                                        |
| `web-npm-run-dev-https`       | Start Vite dev server over HTTPS (required for `crypto.subtle` on mobile/LAN)                 |
| `web-npm-build`               | Build the static site into `build/$DDPHOTOS_SITE_ID/`                                         |
| `web-docker-build-apache`     | Build the `photos-apache` Docker image                                                        |
| `web-docker-build-nginx`      | Build the `photos-nginx` Docker image                                                         |
| `web-docker-build-apache-ssh` | Build the `photos-apache-ssh` Docker image (Apache + SSH, used for rsync testing)             |
| `web-docker-run-apache`       | Run Apache on port 8080 (mounts `build/` and `albums/$DDPHOTOS_SITE_ID/`)                     |
| `web-docker-run-nginx`        | Run nginx on port 8080 (mounts `build/` and `albums/$DDPHOTOS_SITE_ID/`)                      |
| `web-docker-stop`             | Stop the container running on port 8080                                                       |
| `web-docker-test`             | Run `bin/test-photos-server.sh` against `localhost:8080`                                      |
| `web-playwright-install`      | One-time setup: install `@playwright/test` and Chromium binary                                |
| `web-playwright-test-apache`  | Run Playwright e2e tests (starts Docker/Apache on port 8083, runs, stops)                     |
| `web-playwright-test-nginx`   | Run Playwright e2e tests (starts Docker/nginx on port 8084, runs, stops)                      |
| `web-playwright-test-dev`     | Run Playwright e2e tests (against Vite dev server)                                            |
| `web-playwright-test-all`     | Run `bin/test-all.sh` across all password/CSS variants                                        |
| `web-sanity-test`             | Quick sanity check: Apache, no-passwords + all-passwords (companion to `make build test vet`) |
| `sample-photogen`             | Run photogen using `sample/config/albums.yaml`                                                |
| `sample-photogen-pw-all`      | Run photogen using sample config, all albums password-protected                               |
| `sample-photogen-pw-uganda`   | Run photogen using sample config, Uganda album password-protected                             |
| `sample-photogen-css`         | Run photogen using sample config with custom CSS injected                                     |
| `sample-photogen-demo`        | Run photogen using sample config with custom CSS and all albums password-protected            |
| `sample-demo`                 | One-step demo: photogen (CSS + passwords) and run dev server                                  |
| `sample-build`                | Build the static site using sample config                                                     |
| `sample-npm-run-dev`          | Run the Vite dev server using sample config                                                   |
| `sample-npm-run-dev-css`      | Run the Vite dev server using sample config with custom CSS                                   |
| `sample-test-apache`          | Run routing tests against Docker/Apache on port 8082                                          |
| `sample-test-nginx`           | Run routing tests against Docker/nginx on port 8082                                           |
| `sample-rsync-test`           | Test the rsync deploy path: photogen, build, rsync into a fresh Docker container, verify      |
| `sample-s3-test`              | Test the S3 deploy path against MinIO: verifies file placement and Cache-Control headers      |
| `web-screenshots`             | Capture screenshots (requires a running server on port 8080)                                  |
| `docker-build`                | Build the `ddphotos` Docker image locally (single-arch)                                       |
| `docker-push`                 | Build multi-arch image and push to Docker Hub (`bin/docker-push.sh`)                          |
