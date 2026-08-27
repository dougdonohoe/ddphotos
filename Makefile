# Run recipes under bash. Make defaults to /bin/sh, which is dash on Debian/Ubuntu but
# bash on macOS. nvm.sh locates itself via BASH_SOURCE; under dash that is unset, so it
# guesses NVM_DIR=/bin and every nvm target fails with a cryptic error. Pinning bash keeps
# Linux and macOS on the same shell.
SHELL := /bin/bash

# Migration check: album data moved from web/albums/ to albums/ in the decouple refactor.
ifneq ($(wildcard web/albums),)
$(warning MIGRATION REQUIRED: web/albums/ exists but album data now lives in albums/ )
$(warning run: 'mv web/albums albums')
$(error ERROR)
endif

# Migration check: web/static/albums symlink is no longer used after the decouple refactor.
ifneq ($(wildcard web/static/albums),)
$(warning MIGRATION REQUIRED: web/static/albums symlink is no longer used)
$(warning run: 'rm web/static/albums')
$(error ERROR)
endif

# Migration check: web/static/sitemap.xml is no longer generated into web/static/ after the decouple refactor.
ifneq ($(wildcard web/static/sitemap.xml),)
$(warning MIGRATION REQUIRED: web/static/sitemap.xml is stale and should be removed)
$(warning run: 'rm web/static/sitemap.xml')
$(error ERROR)
endif

# Albums directory and site ID — defaults read from config/defaults.env.
# ?= means env vars and command-line assignments take precedence over the file defaults.
# Override on the command line, e.g.: make sample-build DDPHOTOS_SITE_ID=sample-css
DDPHOTOS_ALBUMS_DIR ?= $(shell sed -n 's/^DDPHOTOS_ALBUMS_DIR=//p' config/defaults.env)
DDPHOTOS_SITE_ID    ?= $(shell sed -n 's/^DDPHOTOS_SITE_ID=//p' config/defaults.env)
override DDPHOTOS_ALBUMS_DIR := $(abspath $(patsubst ~/%,$(HOME)/%,$(DDPHOTOS_ALBUMS_DIR)))

# nvm/Node.js initialization:
# - NVM_INIT always sources nvm.sh (nvm is a shell function, not a binary, so Make's subshell
#   never has it). NVM_SH is derived from NVM_DIR if set (e.g. Homebrew install), else ~/.nvm.
#   Override NVM_SH if your nvm lives elsewhere and NVM_DIR is not set.
# - If a 'node' on PATH already matches the exact version in web/.nvmrc (system install,
#   volta, fnm, etc.), NODE_INIT is empty and that node is used directly. Otherwise, nvm is
#   sourced from NVM_SH and switched to the version in web/.nvmrc (sourcing alone activates
#   nvm's default alias, which is not necessarily the version this repo wants). Matching on
#   the version, not mere presence, keeps a distro node at the wrong one (Ubuntu's apt
#   'nodejs' is commonly pulled in as a dependency) from shadowing the repo's Node.
NVM_SH ?= $(or $(NVM_DIR),$(HOME)/.nvm)/nvm.sh
NVM_INIT := . "$(NVM_SH)" &&
NODE_WANTED := $(shell cat web/.nvmrc)
NODE_FOUND := $(shell node -v 2>/dev/null | sed 's/^v//')
ifneq ($(NODE_FOUND),$(NODE_WANTED))
NODE_INIT := . "$(NVM_SH)" && nvm use --silent $(NODE_WANTED) &&
endif

# 1st item is default, so 'make' with no arguments shows help
.PHONY: help
## help: show this help message
help:
	@printf "Usage:\n\n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

# All Go source lives in cmd/ and pkg/. Scoping to those, rather than ./..., keeps Go out
# of web/node_modules — npm packages may vendor Go source (flatted ships a Go port), and
# since it has no go.mod of its own, ./... treats it as part of this module and builds it.
GO_PKGS := ./cmd/... ./pkg/...

.PHONY: build
## build: run `go build`
build:
	go build -ldflags "-X main.repoRoot=$(PWD)" $(GO_PKGS)

.PHONY: test
## test: run `go test` (with the race detector)
# -race catches data races in the concurrent resize and metadata workers. It needs cgo,
# which is already required by govips, and roughly doubles the runtime.
#
# -cover is deliberately NOT here: it needs the `covdata` tool, which a toolchain fetched
# via GOTOOLCHAIN auto-download does not supply, so `go test -cover` fails with
# `go: no such tool "covdata"` on any package that has no test files (cmd/decode,
# cmd/photogen). That bites whenever the installed Go is older than the `go` directive in
# go.mod — e.g. Ubuntu 24.04, whose apt golang-go is 1.22 and downloads 1.25 on demand.
# Use `make test-cover` for coverage; it needs a real Go install (see docs/INSTALL.md).
test:
	go test -v -race $(GO_PKGS)

.PHONY: test-cover
## test-cover: run `go test` with coverage (needs a full Go install, not a downloaded toolchain)
test-cover:
	go test -v -race -cover $(GO_PKGS)

.PHONY: vet
## vet: run `go vet`
vet:
	go vet $(GO_PKGS)

.PHONY: mod-tidy
## mod-tidy: run `go mod tidy` (clean up imports)
mod-tidy:
	go mod tidy

.PHONY: clean-cache
## clean-cache: run `go clean -cache` (useful after vips library upgrade)
clean-cache:
	go clean -cache

.PHONY: web-nvm-install
## web-nvm-install: install the Node version in web/.nvmrc and the npm in web/.npm-version
web-nvm-install:
	@test -f "$(NVM_SH)" || { echo "nvm not found at $(NVM_SH). Install it from https://github.com/nvm-sh/nvm#installing-and-updating"; exit 1; }
	$(NVM_INIT) cd web && nvm install && npm install -g npm@$$(cat .npm-version)

.PHONY: web-npm-install
## web-npm-install: install npm dependencies in web/
web-npm-install:
	$(NODE_INIT) cd web && npm install

.PHONY: web-npm-audit
## web-npm-audit: run npm audit in web
web-npm-audit:
	$(NODE_INIT) cd web && npm audit

.PHONY: web-npm-audit-fix
## web-npm-audit-fix: run npm audit fix in web
web-npm-audit-fix:
	$(NODE_INIT) cd web && npm audit fix

.PHONY: web-lint
## web-lint: check formatting (prettier) and lint (eslint) in web/
web-lint:
	$(NODE_INIT) cd web && npm run lint

.PHONY: web-format
## web-format: reformat web/ sources with prettier
web-format:
	$(NODE_INIT) cd web && npm run format

.PHONY: web-unit-test
## web-unit-test: run Vitest unit tests for the TypeScript helpers in web/src/lib
web-unit-test:
	$(NODE_INIT) cd web && npm run test:unit

.PHONY: web-playwright-install
## web-playwright-install: install Playwright and browser binaries (one-time setup)
web-playwright-install:
	$(NODE_INIT) cd web && npx playwright install chromium

.PHONY: web-npm-run-dev
## web-npm-run-dev: run npm dev server in web, opening a browser window to the site
web-npm-run-dev:
	$(NODE_INIT) cd web && DDPHOTOS_ALBUMS_DIR=$(DDPHOTOS_ALBUMS_DIR) DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) npm run dev -- --open

.PHONY: web-npm-run-dev-https
## web-npm-run-dev-https: run npm dev server over HTTPS (for mobile testing via LAN IP — crypto.subtle requires a secure context)
web-npm-run-dev-https:
	$(NODE_INIT) cd web && VITE_HTTPS=1 DDPHOTOS_ALBUMS_DIR=$(DDPHOTOS_ALBUMS_DIR) DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) npm run dev

.PHONY: web-npm-build
## web-npm-build: build web app
web-npm-build:
	$(NODE_INIT) cd web && DDPHOTOS_ALBUMS_DIR=$(DDPHOTOS_ALBUMS_DIR) DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) npm run build

.PHONY: web-docker-build-apache
## web-docker-build-apache: build the photos Apache Docker image
web-docker-build-apache:
	bin/docker-check.sh --force

.PHONY: web-docker-build-nginx
## web-docker-build-nginx: build the photos nginx Docker image
web-docker-build-nginx:
	bin/docker-check.sh --server nginx --force

.PHONY: web-docker-build-apache-ssh
## web-docker-build-apache-ssh: build the Apache+SSH Docker image used for rsync testing
web-docker-build-apache-ssh:
	docker build --pull -t photos-apache-ssh -f web/apache-ssh.dockerfile web/

.PHONY: web-docker-build-nginx-ssh
## web-docker-build-nginx-ssh: build the nginx+SSH Docker image used for rsync testing
web-docker-build-nginx-ssh:
	docker build --pull -t photos-nginx-ssh -f web/nginx-ssh.dockerfile web/

.PHONY: _check-docker-schema-apache
_check-docker-schema-apache:
	bin/docker-check.sh --server apache

.PHONY: _check-docker-schema-nginx
_check-docker-schema-nginx:
	bin/docker-check.sh --server nginx

.PHONY: web-docker-run-apache
## web-docker-run-apache: run the photos Apache Docker container on port 8080
web-docker-run-apache: _check-docker-schema-apache
	docker run --rm -p 8080:80 \
		-e DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) \
		-v $(PWD)/build:/build:ro \
		-v $(DDPHOTOS_ALBUMS_DIR)/$(DDPHOTOS_SITE_ID):/albums:ro \
		photos-apache

.PHONY: web-docker-run-nginx
## web-docker-run-nginx: run the photos nginx Docker container on port 8080
web-docker-run-nginx: _check-docker-schema-nginx
	docker run --rm -p 8080:80 \
		-e DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) \
		-v $(PWD)/build:/build:ro \
		-v $(DDPHOTOS_ALBUMS_DIR)/$(DDPHOTOS_SITE_ID):/albums:ro \
		photos-nginx

.PHONY: web-docker-stop
## web-docker-stop: stop the running photos Apache Docker container
web-docker-stop:
	docker stop $$(docker ps -q --filter publish=8080) 2>/dev/null || true

.PHONY: web-docker-test
## web-docker-test: run server routing tests against the local Docker container
web-docker-test:
	bin/test-photos-server.sh --local 8080

.PHONY: web-playwright-test-apache
## web-playwright-test-apache: run Playwright e2e tests (no passwords) against Docker/Apache only
web-playwright-test-apache:
	bin/run-tests.sh --mode apache

.PHONY: web-playwright-test-nginx
## web-playwright-test-nginx: run Playwright e2e tests (no passwords) against Docker/nginx only
web-playwright-test-nginx:
	bin/run-tests.sh --mode nginx

.PHONY: web-playwright-test-dev
## web-playwright-test-dev: run Playwright e2e tests (no passwords) against Vite dev server only
web-playwright-test-dev:
	bin/run-tests.sh --mode dev

.PHONY: web-playwright-test-all
## web-playwright-test-all: run Playwright e2e tests against all password variants (dev + apache + nginx)
web-playwright-test-all:
	bin/test-all.sh

.PHONY: web-sanity-test
## web-sanity-test: quick sanity check — Vitest unit tests, then Playwright e2e against Apache, no-passwords + all-passwords variants
web-sanity-test:
	$(MAKE) web-unit-test # pure-function checks first: seconds, and no browser needed
	bin/run-tests.sh --mode apache
	$(MAKE) sample-test-apache # also test routing tests against sample, which was just built
	bin/run-tests.sh --mode apache --passwords sample/config/passwords-all.yaml

.PHONY: gen-deploy-tree
## gen-deploy-tree: regenerate docs/deploy-tree.svg (colored directory tree for DEPLOY.md)
gen-deploy-tree:
	.venv/bin/python3 bin/gen-deploy-tree.py

.PHONY: web-screenshots
## web-screenshots: capture screenshots and regenerate composite — requires a running server on port 8080
web-screenshots:
	# run `make sample-photogen sample-build web-docker-run-apache` to start docker/apache for this script
	$(NODE_INIT) cd web && node scripts/screenshots.mjs --album antarctica --photo 4
	.venv/bin/python3 bin/generate-screenshot-composite.py

.PHONY: sample-photogen
## sample-photogen: run photogen using sample images
sample-photogen:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -doit

.PHONY: sample-photogen-pw-all
## sample-photogen-pw-all: run photogen using sample images, all albums password-protected
sample-photogen-pw-all:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -passwords sample/config/passwords-all.yaml -site-id sample-pw-all -doit

.PHONY: sample-photogen-pw-uganda
## sample-photogen-pw-uganda: run photogen using sample images, uganda album password-protected
sample-photogen-pw-uganda:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -passwords sample/config/passwords-uganda.yaml -site-id sample-pw-uganda -doit

.PHONY: sample-photogen-pw-keyonly
## sample-photogen-pw-keyonly: run photogen using sample images, passwords file with no effective password
sample-photogen-pw-keyonly:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -passwords sample/config/passwords-keyonly.yaml -site-id sample-pw-keyonly -doit

.PHONY: sample-photogen-css
## sample-photogen-css: run photogen using sample images with custom CSS injected
sample-photogen-css:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -css sample/config/custom-example.css -site-id sample-css -doit

.PHONY: sample-photogen-demo-1
## sample-photogen-demo-1: run photogen using sample images with custom CSS and all albums password-protected
sample-photogen-demo-1:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -css sample/config/custom-example.css -passwords sample/config/passwords-all.yaml -site-id sample-demo-1 -doit

.PHONY: sample-photogen-demo-2
## sample-photogen-demo-2: run photogen using sample images with custom CSS and uganda album password-protected
sample-photogen-demo-2:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -css sample/config/custom-example.css -passwords sample/config/passwords-uganda.yaml -site-id sample-demo-2 -doit

.PHONY: sample-demo-1
## sample-demo-1: one-step demo with custom CSS + password protection — photogen's and runs dev server
sample-demo-1: sample-photogen-demo-1
	DDPHOTOS_SITE_ID=sample-demo-1 $(MAKE) web-npm-run-dev

.PHONY: sample-demo-2
## sample-demo-2: one-step demo with custom CSS + 1 album password protection — photogen's and runs dev server
sample-demo-2: sample-photogen-demo-2
	DDPHOTOS_SITE_ID=sample-demo-2 $(MAKE) web-npm-run-dev

.PHONY: sample-build
## sample-build: build web app using sample config
sample-build:
	$(MAKE) web-npm-build

.PHONY: sample-export
## sample-export: create export/<site-id>/ with symlinks for local serving
sample-export:
	@bin/export.sh

.PHONY: sample-test-apache
## sample-test-apache: run routing tests against local Apache Docker container on port 8082 (starts/stops Docker automatically)
sample-test-apache: _check-docker-schema-apache
	@test -d build/$(DDPHOTOS_SITE_ID) || { echo "Error: build/$(DDPHOTOS_SITE_ID) not found. Run 'make web-npm-build' first."; exit 1; }
	docker run -d --rm --name sample-test-apache -p 8082:80 \
		-e DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) \
		-v $(PWD)/build:/build:ro \
		-v $(DDPHOTOS_ALBUMS_DIR)/$(DDPHOTOS_SITE_ID):/albums:ro \
		photos-apache
	@echo "Waiting for Apache to be ready..."; \
	until curl -s -o /dev/null http://localhost:8082; do sleep 1; done
	bin/test-photos-server.sh --local 8082; \
	EXIT=$$?; docker stop sample-test-apache 2>/dev/null || true; exit $$EXIT

.PHONY: sample-test-nginx
## sample-test-nginx: run routing tests against local nginx Docker container on port 8082 (starts/stops Docker automatically)
sample-test-nginx: _check-docker-schema-nginx
	@test -d build/$(DDPHOTOS_SITE_ID) || { echo "Error: build/$(DDPHOTOS_SITE_ID) not found. Run 'make web-npm-build' first."; exit 1; }
	docker run -d --rm --name sample-test-nginx -p 8082:80 \
		-e DDPHOTOS_SITE_ID=$(DDPHOTOS_SITE_ID) \
		-v $(PWD)/build:/build:ro \
		-v $(DDPHOTOS_ALBUMS_DIR)/$(DDPHOTOS_SITE_ID):/albums:ro \
		photos-nginx
	@echo "Waiting for nginx to be ready..."; \
	until curl -s -o /dev/null http://localhost:8082; do sleep 1; done
	bin/test-photos-server.sh --local 8082; \
	EXIT=$$?; docker stop sample-test-nginx 2>/dev/null || true; exit $$EXIT

.PHONY: sample-rsync-test
## sample-rsync-test: test deploy-photos.sh rsync path into a fresh Apache Docker container (starts/stops automatically)
sample-rsync-test:
	bin/rsync-test.sh

.PHONY: sample-rsync-test-nginx
## sample-rsync-test-nginx: test deploy-photos.sh rsync path into a fresh nginx Docker container (starts/stops automatically)
sample-rsync-test-nginx:
	bin/rsync-test.sh --server nginx

.PHONY: sample-s3-test
## sample-s3-test: test deploy-photos.sh S3 path against MinIO; verifies file placement and Cache-Control headers
sample-s3-test:
	bin/s3-test.sh

.PHONY: sample-npm-run-dev
## sample-npm-run-dev: run npm dev server using sample config
sample-npm-run-dev:
	$(MAKE) web-npm-run-dev

.PHONY: sample-npm-run-dev-css
## sample-npm-run-dev-css: run npm dev server using sample config with custom CSS
sample-npm-run-dev-css:
	DDPHOTOS_SITE_ID=sample-css $(MAKE) web-npm-run-dev

#
# ── ddphotos Docker image ─────────────────────────────────────────────────────
#
DDPHOTOS_IMAGE  ?= ddphotos

.PHONY: docker-build
## docker-build: build the ddphotos Docker image
docker-build:
	docker build --pull -t $(DDPHOTOS_IMAGE) \
		--build-arg NODE_VERSION=$$(cat web/.nvmrc) \
		--build-arg NPM_VERSION=$$(cat web/.npm-version) \
		--build-arg DDPHOTOS_GIT_DESCRIBE="$$(git describe --tags --long --dirty --always 2>/dev/null || echo unknown)" \
		-f docker/Dockerfile .

.PHONY: docker-push
## docker-push: build multi-arch image and push to Docker Hub (tag is dev or vX.Y.Z+latest)
docker-push:
	bin/docker-push.sh

.PHONY: docker-test
## docker-test: build the ddphotos Docker image and run end-to-end Docker workflow tests
docker-test:
	bin/docker-test.sh

.PHONY: ddphotos-install-dev
## ddphotos-install-dev: install ddphotos script from local dev image into ~/.local/bin
ddphotos-install-dev:
	docker run --rm -v ~/.local/bin:/ddphotos ddphotos init --script-only

.PHONY: ddphotos-install-prod
## ddphotos-install-prod: install ddphotos script from Docker Hub image into ~/.local/bin
ddphotos-install-prod:
	docker run --rm -v ~/.local/bin:/ddphotos dougdonohoe/ddphotos:latest init --script-only

.PHONY: ddphotos-patch
## ddphotos-patch: patch ~/.local/bin/ddphotos script from local docker/ dir, preserving IMAGE= value
ddphotos-patch:
	@_img=$$(grep '^IMAGE=' ~/.local/bin/ddphotos); \
	/bin/cp docker/ddphotos ~/.local/bin/ddphotos; \
	sed -i '' "s|^IMAGE=.*|$$_img|" ~/.local/bin/ddphotos
