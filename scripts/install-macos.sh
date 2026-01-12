#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-0.6.2}"
TARGET_RAW="${2:-auto}"
REPO="Leadaxe/singbox-launcher"
ASSET_DEFAULT="singbox-launcher-${VERSION}-macos.zip"
ASSET_CATALINA="singbox-launcher-${VERSION}-macos-catalina.zip"

INSTALL_DIR="${HOME}/Applications/Singbox-Launcher"
APP_NAME="singbox-launcher.app"
BIN_REL="Contents/MacOS/singbox-launcher"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1"; exit 1; }; }
need curl; need unzip; need xattr; need chmod; need open; need mktemp; need find; need sw_vers

# Normalize target
TARGET="$(echo "$TARGET_RAW" | tr '[:upper:]' '[:lower:]')"
if [[ "$TARGET" != "auto" && "$TARGET" != "catalina" ]]; then
  echo "Unknown target: $TARGET_RAW"
  echo "Usage: $0 [version] [auto|catalina]"
  exit 1
fi

# Detect macOS version when target is auto
IS_CATALINA=false
if [[ "$TARGET" == "auto" ]]; then
  OSVER="$(sw_vers -productVersion)"
  MAJOR_MINOR="$(echo "$OSVER" | awk -F. '{print $1"."$2}')"
  if [[ "$MAJOR_MINOR" == "10.15" ]]; then
    IS_CATALINA=true
  fi
fi

# Choose asset URL (prefer catalina when requested/detected and available)
ASSET="$ASSET_DEFAULT"
if [[ "$TARGET" == "catalina" || "$IS_CATALINA" == "true" ]]; then
  URL_CATALINA="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_CATALINA}"
  if curl -fsI "$URL_CATALINA" >/dev/null 2>&1; then
    ASSET="$ASSET_CATALINA"
    URL="$URL_CATALINA"
  else
    echo "Catalina-targeted asset not found for ${VERSION}, falling back to universal macOS asset"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_DEFAULT}"
  fi
else
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_DEFAULT}"
fi

echo "Selected asset: $ASSET"
echo "Download URL: $URL"

# Inform user about Catalina compatibility when appropriate
if [[ "$ASSET" == "$ASSET_CATALINA" ]]; then
  echo "Note: Catalina build is best-effort compatibility."
  echo "If the app does not start, please update macOS or use the universal build." 
fi

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
config_path="${target}/Contents/MacOS/bin/config.json"
config_backup="${tmp}/config.json.backup"

# Backup config if exists
if [[ -d "$target" ]]; then
  echo "Removing existing installation..."
  if [[ -f "$config_path" ]]; then
    echo "Backing up config.json..."
    cp "$config_path" "$config_backup"
  fi
  rm -rf "$target"
fi

echo "Installing..."
cp -R "$app_path" "$target"

# Restore config if it was backed up
if [[ -f "$config_backup" ]]; then
  echo "Restoring config.json..."
  mkdir -p "$(dirname "$config_path")"
  cp "$config_backup" "$config_path"
  echo "Config restored successfully"
fi

echo "Fixing macOS attributes and permissions..."
xattr -cr "$target" || true

# Verify launcher binary exists before setting executable permissions
if [[ ! -f "$target/$BIN_REL" ]]; then
  echo "Error: launcher binary not found at $BIN_REL"
  exit 1
fi

chmod +x "$target/$BIN_REL"

echo "Installed: $target"
echo "Opening Finder..."
open "$INSTALL_DIR"

