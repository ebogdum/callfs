# Build stage
FROM golang:1.24-alpine AS builder

# Install git for fetching dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o callfs ./cmd

# Build the healthcheck binary
COPY tests/integration/healthcheck.go /tmp/healthcheck.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s' \
    -o callfs-healthcheck /tmp/healthcheck.go

# Final stage
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binaries
COPY --from=builder /build/callfs /callfs
COPY --from=builder /build/callfs-healthcheck /callfs-healthcheck

# Expose the default port
EXPOSE 8443

# Health checks should be configured externally against /health.
HEALTHCHECK NONE

# Run the binary
ENTRYPOINT ["/callfs"]
