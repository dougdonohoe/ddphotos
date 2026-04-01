# Path to site.env - override if your config lives elsewhere, e.g.:
#   make web-npm-run-dev SITE_ENV=~/work/my-photos/config/site.env
SITE_ENV ?= config/site.env
override SITE_ENV := $(abspath $(patsubst ~/%,$(HOME)/%,$(SITE_ENV)))

# nvm/Node.js initialization:
# - NVM_INIT always sources nvm.sh (nvm is a shell function, not a binary, so Make's subshell
#   never has it). NVM_SH is derived from NVM_DIR if set (e.g. Homebrew install), else ~/.nvm.
#   Override NVM_SH if your nvm lives elsewhere and NVM_DIR is not set.
# - If 'node' is already on PATH (system install, volta, fnm, etc.),
#   NODE_INIT is empty and node is used directly. Otherwise, nvm is sourced from NVM_SH.
NVM_SH ?= $(or $(NVM_DIR),$(HOME)/.nvm)/nvm.sh
NVM_INIT := . "$(NVM_SH)" &&
NODE := $(shell command -v node 2>/dev/null)
ifndef NODE
NODE_INIT := . "$(NVM_SH)" &&
endif

# 1st item is default, so 'make' with no arguments shows help
.PHONY: help
## help: show this help message
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: build
## build: run `go build`
build:
	go build ./...

.PHONY: test
## test: run `go test`
test:
	go test -v -cover ./...

.PHONY: vet
## vet: run `go vet`
vet:
	go vet ./...

.PHONY: mod-tidy
## mod-tidy: run `go mod tidy` (clean up imports)
mod-tidy:
	go mod tidy

.PHONY: clean-cache
## clean-cache: run `go clean -cache` (useful after vips library upgrade)
clean-cache:
	go clean -cache

.PHONY: web-nvm-install
## web-nvm-install: install the Node version specified in web/.nvmrc
web-nvm-install:
	@test -f "$(NVM_SH)" || { echo "nvm not found at $(NVM_SH). Install it from https://github.com/nvm-sh/nvm#installing-and-updating"; exit 1; }
	$(NVM_INIT) cd web && nvm install

.PHONY: web-npm-install
## web-npm-install: install npm dependencies in web/
web-npm-install:
	$(NODE_INIT) cd web && npm install

.PHONY: web-playwright-install
## web-playwright-install: install Playwright and browser binaries (one-time setup)
web-playwright-install:
	$(NODE_INIT) cd web && npx playwright install chromium

.PHONY: web-npm-run-dev
## web-npm-run-dev: run npm dev server in web, opening a browser window to the site
web-npm-run-dev:
	$(NODE_INIT) cd web && SITE_ENV=$(abspath $(SITE_ENV)) npm run dev -- --open

.PHONY: web-npm-build
## web-npm-build: build web app
web-npm-build:
	$(NODE_INIT) cd web && SITE_ENV=$(abspath $(SITE_ENV)) npm run build

.PHONY: web-docker-build
## web-docker-build: build the photos Apache Docker image
web-docker-build:
	docker build -t photos-apache web/

.PHONY: web-docker-run
## web-docker-run: run the photos Apache Docker container on port 8080 (mount web/build as document root)
web-docker-run:
	docker run --rm -p 8080:80 -v $(PWD)/web:/usr/local/apache2/htdocs:ro photos-apache

.PHONY: web-docker-stop
## web-docker-stop: stop the running photos Apache Docker container
web-docker-stop:
	docker stop $$(docker ps -q --filter publish=8080) 2>/dev/null || true

.PHONY: web-docker-test
## web-docker-test: run Apache routing tests against the local Docker container
web-docker-test:
	bin/test-photos-apache.sh --local 8080

.PHONY: web-playwright-test-apache
## web-playwright-test-apache: run Playwright e2e tests (no passwords) against Docker/Apache only
web-playwright-test-apache:
	bin/run-tests.sh --mode apache

.PHONY: web-playwright-test-dev
## web-playwright-test-dev: run Playwright e2e tests (no passwords) against Vite dev server only
web-playwright-test-dev:
	bin/run-tests.sh --mode dev

.PHONY: web-playwright-test-all
## web-playwright-test-all: run Playwright e2e tests against all password variants (dev + apache)
web-playwright-test-all:
	bin/test-all.sh

.PHONY: web-screenshots
## web-screenshots: capture screenshots (home dark/light, album dark/light, lightbox) — requires a running server on port 8080
web-screenshots:
	$(NODE_INIT) cd web && node scripts/screenshots.mjs --album antarctica --photo 4

.PHONY: use-sample
## use-sample: symlink web/static/albums -> ../albums/sample (web/albums/sample/)
use-sample:
	ln -sfn ../albums/sample web/static/albums

.PHONY: use-sample-pw-all
## use-sample-pw-all: symlink web/static/albums -> ../albums/sample-pw-all
use-sample-pw-all:
	ln -sfn ../albums/sample-pw-all web/static/albums

.PHONY: use-sample-pw-uganda
## use-sample-pw-uganda: symlink web/static/albums -> ../albums/sample-pw-uganda
use-sample-pw-uganda:
	ln -sfn ../albums/sample-pw-uganda web/static/albums

.PHONY: use-prod
## use-prod: symlink web/static/albums -> ../albums/prod (web/albums/prod/)
use-prod:
	ln -sfn ../albums/prod web/static/albums

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

.PHONY: sample-build
## sample-build: build web app using sample config
sample-build: use-sample
	SITE_ENV=sample/config/site.env $(MAKE) web-npm-build

.PHONY: sample-test-apache
## sample-test-apache: run test-photos-apache.sh tests against local Docker container on port 8082 (starts/stops Docker automatically)
sample-test-apache:
	@test -d web/build || { echo "Error: web/build not found. Run 'make web-npm-build' first."; exit 1; }
	docker run -d --rm --name sample-test-apache -p 8082:80 \
		-v $(PWD)/web:/usr/local/apache2/htdocs:ro photos-apache
	@echo "Waiting for Apache to be ready..."; \
	until curl -s -o /dev/null http://localhost:8082; do sleep 1; done
	bin/test-photos-apache.sh --config-dir sample/config --local 8082; \
	EXIT=$$?; docker stop sample-test-apache 2>/dev/null || true; exit $$EXIT

.PHONY: sample-npm-run-dev
## sample-npm-run-dev: run npm dev server using sample config
sample-npm-run-dev: use-sample
	SITE_ENV=sample/config/site.env $(MAKE) web-npm-run-dev
