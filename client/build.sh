#!/bin/bash

# Build script for outView Client

VERSION="1.0.0"
BUILD_DIR="build"
BINARY_NAME="outview-client"

echo "===================================="
echo "outView Client Build Script"
echo "===================================="
echo

# Clean build directory
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Get build timestamp
BUILD_DATE=$(date '+%Y-%m-%d %H:%M:%S')

LDFLAGS="-s -w -X main.Version=$VERSION -X main.BuildDate=$BUILD_DATE"

# Build for multiple platforms
PLATFORMS=(
    "windows/amd64/.exe"
    "windows/386/.exe"
    "linux/amd64/"
    "linux/arm64/"
    "darwin/amd64/"
    "darwin/arm64/"
)

for PLATFORM in "${PLATFORMS[@]}"; do
    IFS='/' read -r GOOS GOARCH EXT <<< "$PLATFORM"
    OUTPUT="${BUILD_DIR}/${BINARY_NAME}-${GOOS}-${GOARCH}${EXT}"

    echo "Building for $GOOS/$GOARCH..."
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/outview-client

    if [ $? -ne 0 ]; then
        echo "Build failed for $GOOS/$GOARCH!"
        exit 1
    fi

    echo "  - ${BINARY_NAME}-${GOOS}-${GOARCH}${EXT}"
done

echo
echo "===================================="
echo "Build completed!"
echo "Output directory: $BUILD_DIR"
echo "===================================="

ls -la "$BUILD_DIR"