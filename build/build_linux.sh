#!/bin/bash

# Build script for Linux
#
# Required system packages (OpenGL + X11/GLFW for Fyne):
#   Debian/Ubuntu (apt):
#     build-essential pkg-config libgl1-mesa-dev libegl1-mesa-dev \
#     libxcursor-dev libxrandr-dev libxi-dev libxinerama-dev libxft-dev \
#     libxkbcommon-x11-dev libxxf86vm-dev libwayland-dev
#   Fedora/RHEL (dnf): mesa-libGL-devel mesa-libEGL-devel libXcursor-devel \
#     libXrandr-devel libXi-devel libXinerama-devel libXft-devel \
#     libxkbcommon-x11-devel libXxf86vm-devel libwayland-devel
#   openSUSE (zypper): gcc gcc-c++ make pkg-config Mesa-libGL-devel \
#     Mesa-libEGL-devel libXcursor-devel libXrandr-devel libXi-devel \
#     libXinerama-devel libXft-devel libxkbcommon-x11-devel \
#     libXxf86vm-devel wayland-devel
#   Or use Docker: from repo root:
#     docker build -f build/Dockerfile.linux --target export -o type=local,dest=. .

set -e

cd "$(dirname "$0")/.."

echo ""
echo "========================================"
echo "  Building Sing-Box Launcher (Linux)"
echo "========================================"
echo ""

# Check for build dependencies (CGO + GL/GLFW)
check_deps() {
    local missing=0
    if ! command -v go &>/dev/null; then
        echo "Missing: go (Go toolchain not found in PATH; install from https://go.dev/dl and check 'go version')"
        missing=1
    fi
    if ! command -v pkg-config &>/dev/null; then
        echo "Missing: pkg-config"
        missing=1
    fi
    if ! pkg-config --exists gl 2>/dev/null; then
        echo "Missing: OpenGL development files (e.g. libgl1-mesa-dev)"
        missing=1
    fi
    if [ ! -f /usr/include/X11/Xcursor/Xcursor.h ] 2>/dev/null && [ ! -f /usr/local/include/X11/Xcursor/Xcursor.h ] 2>/dev/null; then
        echo "Missing: X11 Xcursor headers (e.g. libxcursor-dev)"
        missing=1
    fi
    return $missing
}

if ! check_deps; then
    echo ""
    echo "--- Install build dependencies ---"
    echo "Debian/Ubuntu:"
    echo "  sudo apt-get update && sudo apt-get install -y \\"
    echo "    build-essential pkg-config libgl1-mesa-dev libegl1-mesa-dev \\"
    echo "    libxcursor-dev libxrandr-dev libxi-dev libxinerama-dev libxft-dev \\"
    echo "    libxkbcommon-x11-dev libxxf86vm-dev libwayland-dev"
    echo ""
    echo "Fedora/RHEL:"
    echo "  sudo dnf install -y \\"
    echo "    mesa-libGL-devel mesa-libEGL-devel libXcursor-devel \\"
    echo "    libXrandr-devel libXi-devel libXinerama-devel libXft-devel \\"
    echo "    libxkbcommon-x11-devel libXxf86vm-devel libwayland-devel"
    echo ""
    echo "openSUSE (Leap/Tumbleweed):"
    echo "  sudo zypper install -y \\"
    echo "    gcc gcc-c++ make pkg-config Mesa-libGL-devel Mesa-libEGL-devel \\"
    echo "    libXcursor-devel libXrandr-devel libXi-devel libXinerama-devel \\"
    echo "    libXft-devel libxkbcommon-x11-devel libXxf86vm-devel wayland-devel"
    echo ""
    echo "Or build in Docker (from repo root):"
    echo "  docker build -f build/Dockerfile.linux --target export -o type=local,dest=. ."
    echo ""
    exit 1
fi

echo "=== Tidying Go modules ==="
go mod tidy

echo ""
echo "=== Setting build environment ==="
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=amd64

# GLFW 3.4 enables X11 and Wayland by default. Its Go binding links the
# Wayland libraries without importing their pkg-config CFLAGS, while openSUSE
# exposes the headers through /usr/include/wayland. Use pkg-config when the
# complete native Wayland toolchain is present; otherwise build the X11
# backend, which also works in Wayland sessions through XWayland.
BUILD_TAG_ARGS=()
if pkg-config --exists egl wayland-client wayland-cursor wayland-egl xkbcommon 2>/dev/null; then
    WAYLAND_CFLAGS=$(pkg-config --cflags-only-I wayland-client wayland-cursor wayland-egl xkbcommon)
    export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }$WAYLAND_CFLAGS"
    echo "Wayland include flags: $WAYLAND_CFLAGS"
else
    BUILD_TAG_ARGS=(-tags x11)
    echo "Native Wayland development files are incomplete; building the X11 backend."
    echo "Install libegl1-mesa-dev (Debian/Ubuntu) or Mesa-libEGL-devel (openSUSE)"
    echo "together with the Wayland development packages to enable both backends."
fi

# Determine output filename
BASE_NAME="singbox-launcher"
EXTENSION=""
OUTPUT_FILENAME="${BASE_NAME}${EXTENSION}"
COUNTER=0

while [ -f "$OUTPUT_FILENAME" ]; do
    COUNTER=$((COUNTER + 1))
    OUTPUT_FILENAME="${BASE_NAME}-${COUNTER}${EXTENSION}"
done

echo "Using output file: $OUTPUT_FILENAME"

echo ""
echo "=== Getting version from git tag ==="
VERSION=$(git describe --tags --always --dirty --exclude='*-prerelease' 2>/dev/null || echo "0.4.1")
echo "Version: $VERSION"

# RequiredTemplateRef pinned to commit being built — see SPEC 046.
TEMPLATE_REF=$(git rev-parse HEAD 2>/dev/null || echo "")
if [ -z "$TEMPLATE_REF" ]; then
    echo "!!! Could not resolve git HEAD for RequiredTemplateRef; aborting !!!"
    exit 1
fi
echo "Template ref: $TEMPLATE_REF"

echo ""
echo "=== Starting Build ==="
go build "${BUILD_TAG_ARGS[@]}" -buildvcs=false -ldflags="-s -w -X singbox-launcher/internal/constants.AppVersion=$VERSION -X singbox-launcher/internal/constants.RequiredTemplateRef=$TEMPLATE_REF" -o "$OUTPUT_FILENAME"

if [ $? -eq 0 ]; then
    echo ""
    echo "========================================"
    echo "  Build completed successfully!"
    echo "  Output: $OUTPUT_FILENAME"
    echo "========================================"
else
    echo ""
    echo "!!! Build failed !!!"
    exit 1
fi
