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
	go build -ldflags "-X main.repoRoot=$(PWD)" ./...

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
	docker build -t photos-apache-ssh -f web/apache-ssh.dockerfile web/

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
## web-sanity-test: quick sanity check — Playwright e2e tests against Apache, no-passwords + all-passwords variants
web-sanity-test:
	bin/run-tests.sh --mode apache
	bin/run-tests.sh --mode apache --passwords sample/config/passwords-all.yaml

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

.PHONY: sample-photogen-css
## sample-photogen-css: run photogen using sample images with custom CSS injected
sample-photogen-css:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -css sample/config/custom.css -site-id sample-css -doit

.PHONY: sample-photogen-demo
## sample-photogen-demo: run photogen using sample images with custom CSS and all albums password-protected
sample-photogen-demo:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -clean -css sample/config/custom.css -passwords sample/config/passwords-all.yaml -site-id sample-demo -doit

.PHONY: sample-demo
## sample-demo: one-step demo with custom CSS + password protection — photogens and runs dev server
sample-demo: sample-photogen-demo
	DDPHOTOS_SITE_ID=sample-demo $(MAKE) web-npm-run-dev

.PHONY: sample-build
## sample-build: build web app using sample config
sample-build:
	$(MAKE) web-npm-build

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
	bin/test-photos-server.sh --config-dir sample/config --local 8082; \
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
	bin/test-photos-server.sh --config-dir sample/config --local 8082; \
	EXIT=$$?; docker stop sample-test-nginx 2>/dev/null || true; exit $$EXIT

.PHONY: sample-rsync-test
## sample-rsync-test: test deploy-photos.sh rsync path by rsyncing into a fresh Docker container (starts/stops automatically)
sample-rsync-test:
	bin/rsync-test.sh

.PHONY: sample-npm-run-dev
## sample-npm-run-dev: run npm dev server using sample config
sample-npm-run-dev:
	$(MAKE) web-npm-run-dev

.PHONY: sample-npm-run-dev-css
## sample-npm-run-dev-css: run npm dev server using sample config with custom CSS
sample-npm-run-dev-css:
	DDPHOTOS_SITE_ID=sample-css $(MAKE) web-npm-run-dev
