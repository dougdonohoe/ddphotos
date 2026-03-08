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
## web-playwright-test-apache: run Playwright e2e tests against local Docker container on port 8081 (starts/stops Docker automatically)
web-playwright-test-apache:
	@test -d web/build || { echo "Error: web/build not found. Run 'make web-npm-build' first."; exit 1; }
	docker run -d --rm --name photos-apache-playwright -p 8081:80 \
		-v $(PWD)/web:/usr/local/apache2/htdocs:ro photos-apache
	@echo "Waiting for Apache to be ready..."; \
	until curl -s -o /dev/null http://localhost:8081; do sleep 1; done
	$(NODE_INIT) cd web && PLAYWRIGHT_BASE_URL=http://localhost:8081 npx playwright test; \
	EXIT=$$?; docker stop photos-apache-playwright 2>/dev/null || true; exit $$EXIT

.PHONY: web-playwright-test-dev
## web-playwright-test-dev: run Playwright e2e tests against local dev server
web-playwright-test-dev:
	$(NODE_INIT) cd web && PLAYWRIGHT_BASE_URL=http://localhost:5173 npx playwright test

.PHONY: web-screenshots
## web-screenshots: capture screenshots (home dark/light, album dark/light, lightbox) — requires a running server on port 8080
web-screenshots:
	$(NODE_INIT) cd web && node scripts/screenshots.mjs --album antarctica --photo 4

.PHONY: use-sample
## use-sample: symlink web/static/albums -> ../albums/sample (web/albums/sample/)
use-sample:
	ln -sfn ../albums/sample web/static/albums

.PHONY: use-prod
## use-prod: symlink web/static/albums -> ../albums/prod (web/albums/prod/)
use-prod:
	ln -sfn ../albums/prod web/static/albums

.PHONY: sample-photogen
## sample-photogen: run photogen using sample images
sample-photogen:
	go run cmd/photogen/photogen.go -config-dir sample/config -resize -index -doit

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

###
### Doug's private make commands, which I use to publish my photos site, and
### are an example for other folks to learn from.
###

.PHONY: doug-use-laptop
## use-laptop: symlink web/static/albums -> ../albums/laptop (web/albums/laptop/)
doug-use-laptop:
	ln -sfn ../albums/laptop web/static/albums

.PHONY: doug-photogen-laptop
## doug-photogen-laptop: run photogen using laptop albums
doug-photogen-laptop:
	go run cmd/photogen/photogen.go -config-dir ../dd-go/config -albums albums-laptop.yaml -resize -index -doit

.PHONY: doug-npm-run-dev-laptop
## doug-npm-run-dev-laptop: run npm dev server using laptop config
doug-npm-run-dev-laptop: doug-use-laptop
	SITE_ENV=../dd-go/config/site.env $(MAKE) web-npm-run-dev

.PHONY: doug-build-laptop
## doug-build-laptop: build web app using laptop config
doug-build-laptop: doug-use-laptop
	SITE_ENV=../dd-go/config/site.env $(MAKE) web-npm-build

.PHONY: doug-use-prod
## use-prod: symlink web/static/albums -> ../albums/prod (web/albums/prod/)
doug-use-prod:
	ln -sfn ../albums/prod web/static/albums

.PHONY: doug-photogen-prod
## doug-photogen-prod: run photogen using prod albums
doug-photogen-prod:
	go run cmd/photogen/photogen.go -config-dir ../dd-go/config -albums albums.yaml -resize -index -doit

.PHONY: doug-npm-run-dev-prod
## doug-npm-run-dev-prod: run npm dev server using prod config
doug-npm-run-dev-prod: doug-use-prod
	SITE_ENV=../dd-go/config/site.env $(MAKE) web-npm-run-dev

.PHONY: doug-build-prod
## doug-build-prod: build web app using prod config
doug-build-prod: doug-use-prod
	SITE_ENV=../dd-go/config/site.env $(MAKE) web-npm-build

.PHONY: doug-deploy
## doug-deploy: deploy prod site
doug-deploy: doug-use-prod
	bin/deploy-photos.sh --config-dir ../dd-go/config

.PHONY: doug-deploy-no-photo-gen
## doug-deploy-no-photo-gen: deploy prod site
doug-deploy-no-photogen: doug-use-prod
	bin/deploy-photos.sh --config-dir ../dd-go/config --no-photogen

.PHONY: doug-test-prod
## doug-test-prod: run Playwright e2e tests against prod server
doug-test-prod:
	$(NODE_INIT) cd web && PLAYWRIGHT_BASE_URL=https://photos.donohoe.info npx playwright test
