# syntax=docker/dockerfile:1

ARG NODE_VERSION=22

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

WORKDIR /app/web
COPY web/package.json web/package-lock.json ./

# Strip debug symbols from node binary before copying to runtime image
RUN apt-get update && apt-get install -y --no-install-recommends binutils \
    && strip /usr/local/bin/node \
    && rm -rf /var/lib/apt/lists/*

# npm 10.9.7 (bundled with node:22) can't self-upgrade due to missing promise-retry;
# install 11.13.0 directly from the registry tarball instead.
RUN node -e "\
const https=require('https'),fs=require('fs');\
function dl(u,cb){https.get(u,r=>{if(r.statusCode>=300&&r.statusCode<400)return dl(r.headers.location,cb);const f=fs.createWriteStream('/tmp/npm.tgz');r.pipe(f);f.on('finish',cb);});}\
dl('https://registry.npmjs.org/npm/-/npm-11.13.0.tgz',()=>process.exit(0));" \
    && tar -xz -C /tmp -f /tmp/npm.tgz \
    && /bin/rm -rf /usr/local/lib/node_modules/npm \
    && mv /tmp/package /usr/local/lib/node_modules/npm \
    && npm ci

# ── Stage 3: Runtime image ────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips \
    ca-certificates \
    curl \
    apache2 \
    rsync \
    openssh-client \
    python3 \
    awscli \
    && rm -rf /var/lib/apt/lists/*

# Node binary from node-builder (avoids NodeSource apt install)
COPY --from=node-builder /usr/local/bin/node /usr/local/bin/node
COPY --from=node-builder /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm

# Use a symlink so __dirname-relative paths in npm-cli.js resolve correctly
RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm

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
     docker/do-decode.sh docker/ddphotos docker/cloudflare-worker.js \
     web/setup-htdocs.sh bin/deploy-photos.sh bin/test-photos-server.sh /docker/
RUN chmod +x /docker/*.sh

# Init templates and placeholder album directory
COPY docker/init/ /docker/init/
COPY docker/hero.webp /docker/hero.webp
COPY sample/source/theway/2024-The-Way-14.jpg /ddphotos-init/
COPY sample/source/theway/2024-The-Way-20.jpg /ddphotos-init/
COPY sample/source/theway/2024-The-Way-24.jpg /ddphotos-init/
COPY sample/source/theway/2024-The-Way-21.jpg /ddphotos-secret/
RUN echo "2024-The-Way-21 You found the secret photo!" > /ddphotos-secret/photogen.txt
RUN mkdir -p /ddphotos-empty
RUN mkdir -p /htdocs

ARG DDPHOTOS_VERSION=dev
RUN echo "$DDPHOTOS_VERSION" > /docker/VERSION

ARG DDPHOTOS_GIT_DESCRIBE=unknown
RUN echo "$DDPHOTOS_GIT_DESCRIBE" > /docker/GIT_DESCRIBE

ARG DDPHOTOS_IMAGE=ddphotos
RUN echo "$DDPHOTOS_IMAGE" > /docker/IMAGE \
    && sed -i "s|^IMAGE=\"ddphotos\"|IMAGE=\"$DDPHOTOS_IMAGE\"|" /docker/ddphotos

EXPOSE 80

ENTRYPOINT ["/docker/entrypoint.sh"]
