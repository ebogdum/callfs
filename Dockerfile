# Web console build stage
FROM node:22-alpine AS web-builder

WORKDIR /build/web

COPY web/package.json web/package-lock.json ./

RUN npm ci

COPY web/ ./

RUN npm run build

# Build stage
FROM golang:1.26-alpine@sha256:27f2d3b83cfc5803c8078f8e730af0f7c431bf067afaec8f60a36fb763187cba AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Install git for fetching dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Copy the built web console into the embed directory
COPY --from=web-builder /build/web/build ./web/build

# Build the binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags ui \
    -ldflags="-w -s -extldflags \"-static\" -X github.com/ebogdum/callfs/internal/buildinfo.Version=${VERSION} -X github.com/ebogdum/callfs/internal/buildinfo.Commit=${COMMIT} -X github.com/ebogdum/callfs/internal/buildinfo.Date=${BUILD_DATE}" \
    -o callfs ./cmd

# Data directory owned by the runtime user
RUN mkdir -p /var/lib/callfs

# Final stage
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /build/callfs /callfs

# Local backend data directory
COPY --from=builder --chown=65532:65532 /var/lib/callfs /var/lib/callfs

# Expose the default port
EXPOSE 8443

# Run as non-root
USER 65532:65532

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/callfs", "healthcheck"]

# Run the binary
ENTRYPOINT ["/callfs"]
