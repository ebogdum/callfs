#!/bin/bash

# Define your application name and main package path
APP_NAME="callfs"
PACKAGE_PATH="./cmd" # Main package is in cmd/main.go

# Define your target platforms (OS/ARCH)
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "darwin/amd64"
    "darwin/arm64"
    "freebsd/amd64"
    "freebsd/arm64"
    "openbsd/amd64"
    "openbsd/arm64"
)

# Create a directory for the builds
BUILD_DIR="builds"
MODULE="github.com/ebogdum/callfs/v2"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X ${MODULE}/internal/buildinfo.Version=${VERSION} -X ${MODULE}/internal/buildinfo.Commit=${COMMIT} -X ${MODULE}/internal/buildinfo.Date=${DATE}"

if [ ! -f web/build/index.html ]; then
    echo "Building the web console..."
    if ! (cd web && npm ci && npm run build); then
        echo "Failed to build the web console."
        exit 1
    fi
fi
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

build_platform() {
    local platform="$1"
    local GOOS GOARCH OUTPUT_NAME
    IFS='/' read -r GOOS GOARCH <<< "$platform"

    OUTPUT_NAME="$BUILD_DIR/${APP_NAME}-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME+=".exe"
    fi

    echo "Building for $GOOS/$GOARCH..."
    if ! env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -tags ui -ldflags="$LDFLAGS" -o "$OUTPUT_NAME" "$PACKAGE_PATH"; then
        echo "An error occurred while building for $GOOS/$GOARCH."
        return 1
    fi
}

pids=()
for platform in "${PLATFORMS[@]}"
do
    build_platform "$platform" &
    pids+=("$!")
done

failed=0
for pid in "${pids[@]}"
do
    if ! wait "$pid"; then
        failed=1
    fi
done

if [ "$failed" -ne 0 ]; then
    echo "One or more builds failed."
    exit 1
fi

if ! (cd "$BUILD_DIR" && shasum -a 256 "${APP_NAME}"-* > SHA256SUMS); then
    echo "Failed to generate SHA256SUMS."
    exit 1
fi

echo "All builds completed."