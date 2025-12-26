#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-0.6.2}"
REPO="Leadaxe/singbox-launcher"
ASSET="singbox-launcher-${VERSION}-macos.zip"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

INSTALL_DIR="${HOME}/Applications/Singbox-Launcher"
APP_NAME="singbox-launcher.app"
BIN_REL="Contents/MacOS/singbox-launcher"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1"; exit 1; }; }
need curl; need unzip; need xattr; need chmod; need open; need mktemp; need find

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$INSTALL_DIR"

echo "Downloading ${ASSET}..."
curl -fL "$URL" -o "$tmp/$ASSET"

echo "Unpacking..."
unzip -q "$tmp/$ASSET" -d "$tmp/unpacked"

app_path="$(find "$tmp/unpacked" -maxdepth 3 -name "$APP_NAME" -type d | head -n 1)"
if [[ -z "${app_path}" ]]; then
  echo "Error: ${APP_NAME} not found in archive"
  find "$tmp/unpacked" -maxdepth 3 -name "*.app" -type d -print || true
  exit 1
fi

target="${INSTALL_DIR}/${APP_NAME}"
rm -rf "$target"
cp -R "$app_path" "$target"

echo "Fixing macOS attributes and permissions..."
xattr -cr "$target" || true
chmod +x "$target/$BIN_REL"

echo "Launching..."
open "$target"

echo "Installed: $target"

