# syntax=docker/dockerfile:1

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

# ── Stage 2: Pre-install Node dependencies ────────────────────────────────────
FROM node:22-bookworm-slim AS node-builder

WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci

# ── Stage 3: Runtime image ────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips \
    ca-certificates \
    curl \
    unzip \
    apache2 \
    rsync \
    openssh-client \
    && rm -rf /var/lib/apt/lists/*

# AWS CLI v2
RUN ARCH=$(uname -m) \
    && curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}.zip" -o /tmp/awscliv2.zip \
    && unzip -q /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install \
    && rm -rf /tmp/aws /tmp/awscliv2.zip

# Node 22 via NodeSource
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Apache: enable mod_rewrite, replace default site with ddphotos config
RUN a2enmod rewrite headers && a2dissite 000-default 2>/dev/null || true \
    && echo "ServerName localhost" >> /etc/apache2/apache2.conf
COPY docker/apache.conf /etc/apache2/sites-available/ddphotos.conf
RUN a2ensite ddphotos

# photogen binary
COPY --from=go-builder /photogen /usr/local/bin/photogen

# Web app source + pre-installed node_modules
COPY web/ /app/web/
COPY --from=node-builder /app/web/node_modules /app/web/node_modules

# Docker scripts
COPY docker/entrypoint.sh docker/do-init.sh docker/do-photogen.sh \
     docker/do-build.sh docker/do-serve.sh docker/do-run.sh docker/do-export.sh docker/do-deploy.sh \
     docker/ddphotos web/setup-htdocs.sh bin/deploy-photos.sh bin/test-photos-server.sh /docker/
RUN chmod +x /docker/*.sh

# Init templates and placeholder album directory
COPY docker/init/ /docker/init/
COPY docker/hero.webp /docker/hero.webp
COPY sample/source/theway/2024-The-Way-14.jpg /ddphotos-init/
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
