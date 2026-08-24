#!/usr/bin/env bash
echo "--------------------------------------------------------------"
echo " "
echo "Singbox-launcher MacOS Installer Script (0.3)"
echo "Project url: https://github.com/Leadaxe/singbox-launcher/"
echo " "
echo "--------------------------------------------------------------"
echo " "
set -euo pipefail

REPO="Leadaxe/singbox-launcher"

# Parse arguments: first arg can be version or target, second arg is target (only if first is version)
ARG1="${1:-}"
ARG2="${2:-}"

# If first argument is "auto" or "catalina", treat it as target (ignore second arg)
if [[ "$ARG1" == "auto" || "$ARG1" == "catalina" ]]; then
  TARGET="$(echo "$ARG1" | tr '[:upper:]' '[:lower:]')"
  VERSION=""
else
  # First argument is version (or empty for auto-detect), second is target
  VERSION="$ARG1"
  TARGET="$(echo "${ARG2:-auto}" | tr '[:upper:]' '[:lower:]')"
  # Validate target
  if [[ "$TARGET" != "auto" && "$TARGET" != "catalina" ]]; then
    echo "Unknown target: $TARGET"
    echo "Usage: $0 [version] [auto|catalina]"
    echo "  If version is not specified, latest version will be used automatically"
    exit 1
  fi
fi

if [[ -z "$VERSION" ]]; then
  echo "Detecting latest version..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [[ -z "$VERSION" ]]; then
    echo "Error: Could not detect latest version. Please specify version manually."
    echo "Usage: $0 [version] [auto|catalina]"
    exit 1
  fi
  echo "Latest version: ${VERSION#v}"
fi

if [[ ! "$VERSION" =~ ^v ]]; then
  VERSION="v${VERSION}"
fi

ASSET_DEFAULT="singbox-launcher-${VERSION}-macos.zip"
ASSET_CATALINA="singbox-launcher-${VERSION}-macos-catalina.zip"

INSTALL_DIR="/Applications"
APP_NAME="singbox-launcher.app"
BIN_REL="Contents/MacOS/singbox-launcher"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1"; exit 1; }; }
need curl; need unzip; need xattr; need chmod; need open; need mktemp; need find; need sw_vers; need grep; need sed

# Check write permissions to /Applications
if [[ ! -w "$INSTALL_DIR" ]]; then
  echo "Error: No write permission to $INSTALL_DIR"
  echo "You may need to run this script with sudo, or install to ${HOME}/Applications/ instead."
  echo ""
  echo "To install with sudo:"
  echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install-macos.sh | sudo bash"
  echo ""
  echo "Or modify INSTALL_DIR in the script to use ${HOME}/Applications/"
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

if [[ "$TARGET" == "catalina" || "$IS_CATALINA" == "true" ]]; then
  URL_CATALINA="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_CATALINA}"
  if curl -fsI "$URL_CATALINA" >/dev/null 2>&1; then
    ASSET="$ASSET_CATALINA"
    URL="$URL_CATALINA"
  else
    echo "Catalina asset not found, using universal build"
    ASSET="$ASSET_DEFAULT"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_DEFAULT}"
  fi
else
  ASSET="$ASSET_DEFAULT"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_DEFAULT}"
fi

if [[ "$ASSET" == "$ASSET_CATALINA" ]]; then
  echo "Note: Catalina build is best-effort compatibility."
  echo "If the app does not start, please update macOS or use the universal build." 
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$INSTALL_DIR"

echo "Downloading ${ASSET}..."
if ! curl -fL "$URL" -o "$tmp/$ASSET"; then
  echo "Error: Failed to download ${ASSET}"
  echo "The version ${VERSION} may not exist or the asset is not available."
  echo "Please check available releases at: https://github.com/${REPO}/releases"
  exit 1
fi

echo "Unpacking..."
unzip -q "$tmp/$ASSET" -d "$tmp/unpacked"

# Locate .app bundle (handle Catalina naming differences and tolerantly fallback)
app_path=""
if [[ "$ASSET" == "$ASSET_CATALINA" ]]; then
  # Prefer explicit catalina-named app when downloading catalina asset
  app_path="$(find "$tmp/unpacked" -maxdepth 3 -name "singbox-launcher-macos-catalina.app" -type d | head -n 1 || true)"
  if [[ -z "$app_path" ]]; then
    app_path="$(find "$tmp/unpacked" -maxdepth 3 -name "$APP_NAME" -type d | head -n 1 || true)"
  fi
else
  app_path="$(find "$tmp/unpacked" -maxdepth 3 -name "$APP_NAME" -type d | head -n 1 || true)"
  if [[ -z "$app_path" ]]; then
    # Fallback: accept any .app in the archive
    app_path="$(find "$tmp/unpacked" -maxdepth 3 -name "*macos-catalina.app" -type d | head -n 1 || true)"
    if [[ -z "$app_path" ]]; then
      app_path="$(find "$tmp/unpacked" -maxdepth 3 -name "*.app" -type d | head -n 1 || true)"
    fi
  fi
fi

if [[ -z "${app_path}" ]]; then
  echo "Error: .app not found in archive"
  find "$tmp/unpacked" -maxdepth 3 -name "*.app" -type d -print || true
  exit 1
fi

echo "Found app in archive: $app_path"

target="${INSTALL_DIR}/${APP_NAME}"
old_macos_dir="${target}/Contents/MacOS"
data_backup="${tmp}/user_data_backup"

# Backup ALL user data before moving the old app to Trash.
#
# Раньше переносился только wizard_states/ — обновление скриптом теряло
# settings.json (hwid, режим движка ядра, debug API), bin/daemon/ (клиентский
# mTLS-сертификат: без него установленный демон перестаёт доверять лаунчеру),
# кэши подписок и rule-sets. Пользователь после апдейта получал «чистый»
# лаунчер и отвалившееся сопряжение с демоном.
#
# Теперь бэкапится весь bin/ + logs/, а после установки данные возвращаются
# поверх нового бандла БЕЗ затирания файлов, пришедших из архива
# (rsync --ignore-existing). Файлы, которые обязаны обновляться по версии
# (wizard_template.json, бинарь ядра), лаунчер сверяет и перекачивает сам —
# перенос старых копий безопасен.
if [[ -d "$target" ]]; then
  echo "Moving existing installation to Trash..."
  mkdir -p "$data_backup"
  if [[ -d "$old_macos_dir/bin" ]]; then
    echo "Backing up user data (settings, daemon pairing, caches, states)..."
    cp -R "$old_macos_dir/bin" "$data_backup/bin"
  fi
  if [[ -d "$old_macos_dir/logs" ]]; then
    cp -R "$old_macos_dir/logs" "$data_backup/logs"
  fi
  # Move to Trash using AppleScript
  osascript -e "tell application \"Finder\" to move POSIX file \"$target\" to trash" 2>/dev/null || {
    # Fallback if osascript fails
    echo "Warning: Could not move to Trash, removing directly..."
    rm -rf "$target"
  }
fi

echo "Installing..."
cp -R "$app_path" "$target"

# Restore user data (automatically, no questions). --ignore-existing: файлы
# из нового архива всегда важнее одноимённых старых.
if [[ -d "$data_backup/bin" || -d "$data_backup/logs" ]]; then
  echo "Restoring user data..."
  if command -v rsync >/dev/null 2>&1; then
    [[ -d "$data_backup/bin" ]] && rsync -a --ignore-existing "$data_backup/bin/" "$target/Contents/MacOS/bin/"
    [[ -d "$data_backup/logs" ]] && rsync -a --ignore-existing "$data_backup/logs/" "$target/Contents/MacOS/logs/"
  else
    # rsync есть на всех поддерживаемых macOS; fallback на случай урезанных
    # окружений — прежнее поведение (хотя бы wizard_states).
    if [[ -d "$data_backup/bin/wizard_states" && ! -d "$target/Contents/MacOS/bin/wizard_states" ]]; then
      mkdir -p "$target/Contents/MacOS/bin"
      cp -R "$data_backup/bin/wizard_states" "$target/Contents/MacOS/bin/wizard_states"
    fi
  fi
  echo "User data restored"
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

