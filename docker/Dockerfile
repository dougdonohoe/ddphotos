# syntax=docker/dockerfile:1
# check=skip=InvalidDefaultArgInFrom
# (NODE_VERSION is intentionally defaultless — see below — which trips that lint check.
#  The two directives above must stay adjacent: a comment ends parser-directive parsing.)

# Required build args, deliberately without defaults: web/.nvmrc and web/.npm-version are the
# single source of truth, and a stale default here would silently build a mis-versioned image.
# Use `make docker-build` (or bin/docker-push.sh), which reads both files and passes them in.
ARG NODE_VERSION
ARG NPM_VERSION

# ── Stage 1: Build photogen (Go + libvips) ────────────────────────────────────
FROM golang:1.25-bookworm AS go-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd/
COPY pkg pkg/
RUN CGO_ENABLED=1 go build -o /photogen ./cmd/photogen/photogen.go
RUN CGO_ENABLED=1 go build -o /decode ./cmd/decode/decode.go

# ── Stage 2: Pre-install Node dependencies ────────────────────────────────────
FROM node:${NODE_VERSION}-bookworm-slim AS node-builder

# Global ARGs are not visible inside a stage without re-declaring them
ARG NPM_VERSION

WORKDIR /app/web
COPY web/package.json web/package-lock.json ./

# Strip debug symbols from node binary before copying to runtime image
RUN apt-get update && apt-get install -y --no-install-recommends binutils \
    && strip /usr/local/bin/node \
    && rm -rf /var/lib/apt/lists/*

# Pin npm to the version in web/.npm-version so local dev, CI and Docker agree.
# (Historical note: npm <= 10.9.7 could not self-upgrade past npm 11.12.0, since it
# deleted promise-retry out from under the running process. Fixed in npm 10.9.8 /
# arborist 8.0.5, see https://github.com/npm/cli/pull/9152. Builds must pull a
# current base image.)
RUN : "${NPM_VERSION:?required build arg — run 'make docker-build', or pass the value in web/.npm-version}" \
    && npm install -g npm@${NPM_VERSION} \
    && npm ci

# ── Stage 3: Runtime image ────────────────────────────────────────────────────
FROM debian:bookworm-slim

# Deliberately no ffmpeg: the Debian package pulls in 200 packages and 448MB (SDL2, X11
# and Wayland, via ffplay's libavdevice dependency), which every photo-only user would pay
# for, and shipping its GPL libx264 would make this image a redistributor of it. Video
# support instead downloads a static build on first use, into a mounted volume --
# see docker/do-install-ffmpeg.sh. xz-utils is here because that tarball is .tar.xz.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips \
    libheif1 \
    ca-certificates \
    curl \
    xz-utils \
    apache2 \
    rsync \
    openssh-client \
    python3 \
    awscli \
    && rm -rf /var/lib/apt/lists/*

# Node binary from node-builder (avoids NodeSource apt install)
COPY --from=node-builder /usr/local/bin/node /usr/local/bin/node
COPY --from=node-builder /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm

# Use symlinks so __dirname-relative paths in npm-cli.js / npx-cli.js resolve correctly
RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# Apache: enable mod_rewrite, replace default site with ddphotos config
RUN a2enmod rewrite headers && a2dissite 000-default 2>/dev/null || true \
    && echo "ServerName localhost" >> /etc/apache2/apache2.conf
COPY docker/apache.conf /etc/apache2/sites-available/ddphotos.conf
RUN a2ensite ddphotos

# photogen and decode binaries
COPY --from=go-builder /photogen /usr/local/bin/photogen
COPY --from=go-builder /decode /usr/local/bin/decode

# Web app source + pre-installed node_modules
COPY web/ /app/web/
COPY --from=node-builder /app/web/node_modules /app/web/node_modules

# Docker scripts
COPY docker/entrypoint.sh docker/do-init.sh docker/do-photogen.sh \
     docker/do-build.sh docker/do-serve.sh docker/do-run.sh docker/do-export.sh docker/do-deploy.sh \
     docker/do-decode.sh docker/do-search-cover.sh docker/do-wrangler.sh docker/do-surge.sh \
     docker/do-install-ffmpeg.sh \
     docker/ddphotos docker/ddphotos.cmd docker/cloudflare-worker.js \
     web/setup-htdocs.sh bin/deploy-photos.sh bin/test-photos-server.sh /docker/
RUN chmod +x /docker/*.sh

# search-cover.sh lives under /app/bin so do-search-cover.sh can call it
COPY bin/search-cover.sh /app/bin/search-cover.sh
RUN chmod +x /app/bin/search-cover.sh

# The starter site, assembled here exactly as it should land on the user's disk, so
# 'ddphotos init' is a single recursive copy of this tree (see docker/do-init.sh).
# docker/init/ supplies config/ and sample-photos/README.md; the photos come from the
# same sample albums the demo site uses.
COPY docker/init/ /docker/init/
COPY docker/hero.jpg /docker/init/sample-photos/hero.jpg
COPY sample/source/theway/2024-The-Way-14.jpg /docker/init/sample-photos/vacation/
COPY sample/source/theway/2024-The-Way-20.jpg /docker/init/sample-photos/vacation/
COPY sample/source/theway/2024-The-Way-24.jpg /docker/init/sample-photos/vacation/
COPY sample/source/theway/2024-The-Way-21.jpg /docker/init/sample-photos/secret/
RUN echo "2024-The-Way-21 You found the secret photo!" > /docker/init/sample-photos/secret/photogen.txt \
    && mkdir -p /docker/init/sample-photos/empty

# Legacy container-internal sample paths. Sites scaffolded by older images still name
# these in albums.yaml (source: /ddphotos-init, hero: /ddphotos-hero/hero.webp), and both
# docker/ddphotos and ddphotos-app still special-case the /ddphotos prefix for them.
# Keep until such sites are assumed gone.
#
# The three album directories are symlinks rather than copies: photogen reaches a source
# dir with os.ReadDir, which follows them, so the old names resolve to the same files
# without duplicating ~17MB into this layer. The tests fail the build (rather than a much
# later photogen run) if the starter tree is ever moved or renamed.
RUN ln -s /docker/init/sample-photos/vacation /ddphotos-init \
    && ln -s /docker/init/sample-photos/secret /ddphotos-secret \
    && ln -s /docker/init/sample-photos/empty  /ddphotos-empty \
    && test -f /ddphotos-init/2024-The-Way-14.jpg \
    && test -f /ddphotos-secret/photogen.txt \
    && test -d /ddphotos-empty

# The legacy hero is its own file, not a symlink: the starter site now ships hero.jpg
# (ddphotos-app previews the hero, and Java ImageIO cannot decode WebP), so there is no
# longer a hero.webp in the starter tree to point at.
COPY docker/hero.webp /ddphotos-hero/hero.webp

RUN mkdir -p /htdocs

ARG DDPHOTOS_VERSION=dev
RUN echo "$DDPHOTOS_VERSION" > /docker/VERSION

ARG DDPHOTOS_GIT_DESCRIBE=unknown
RUN echo "$DDPHOTOS_GIT_DESCRIBE" > /docker/GIT_DESCRIBE
ENV VITE_GIT_DESCRIBE=$DDPHOTOS_GIT_DESCRIBE

ARG DDPHOTOS_IMAGE=ddphotos
RUN echo "$DDPHOTOS_IMAGE" > /docker/IMAGE \
    && sed -i "s|^IMAGE=\"ddphotos\"|IMAGE=\"$DDPHOTOS_IMAGE\"|" /docker/ddphotos
ENV VITE_DOCKER_IMAGE=$DDPHOTOS_IMAGE

ENV DDPHOTOS_ALBUMS_DIR=/ddphotos/albums \
    VITE_GIT_BRANCH="" \
    VITE_GIT_REPO_SLUG=dougdonohoe/ddphotos \
    VITE_GIT_REPO_URL=https://github.com/dougdonohoe/ddphotos

EXPOSE 80

ENTRYPOINT ["/docker/entrypoint.sh"]
