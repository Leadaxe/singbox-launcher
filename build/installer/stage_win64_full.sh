#!/usr/bin/env bash
# Stage the Windows "full" set (SPEC 140 §7): exe + pinned core + wintun +
# wizard template + Mesa3D in mesa3d/. One script for both consumers, so the
# win64-full.zip and the installer never diverge:
#   - release job (ubuntu): stages, adds portable.txt, zips win64-full.zip;
#   - build-windows-installer job (windows, Git Bash): stages, feeds ISCC.
# The staged folder never contains portable.txt: in Program Files the marker
# would turn on portable mode in a folder the launcher cannot write to.
#
# Versions come from the source constants of the same revision, so the set
# matches what the launcher itself requires.
#
# Usage: stage_win64_full.sh <singbox-launcher.exe> <app-version> <out-dir>
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <singbox-launcher.exe> <app-version> <out-dir>" >&2
  exit 2
fi
WIN_EXE="$1"
VERSION="$2"
OUT_DIR="$3"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [ ! -s "$WIN_EXE" ]; then
  echo "::error::stage_win64_full: $WIN_EXE not found or empty" >&2
  exit 1
fi
if ! command -v unzip >/dev/null 2>&1; then
  echo "::error::stage_win64_full: unzip is not available" >&2
  exit 1
fi

CORE_VER=$(sed -n 's/.*RequiredCoreVersion = "\([^"]*\)".*/\1/p' "$REPO_ROOT/internal/constants/constants.go" | head -1)
WINTUN_VER=$(sed -n 's/.*WinTunVersion = "\([^"]*\)".*/\1/p' "$REPO_ROOT/core/wintun_downloader.go" | head -1)
MESA_TAG=$(sed -n 's/.*mesaReleaseTag *= *"\([^"]*\)".*/\1/p' "$REPO_ROOT/internal/platform/glprobe_windows.go" | head -1)
MESA_ASSET=$(sed -n 's/.*mesaAssetName *= *"\([^"]*\)".*/\1/p' "$REPO_ROOT/internal/platform/glprobe_windows.go" | head -1)
echo "core=$CORE_VER wintun=$WINTUN_VER mesa=$MESA_TAG/$MESA_ASSET"
if [ -z "$CORE_VER" ] || [ -z "$WINTUN_VER" ] || [ -z "$MESA_TAG" ] || [ -z "$MESA_ASSET" ]; then
  echo "::error::win64-full: cannot resolve pinned versions from source constants" >&2
  exit 1
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/bin" "$OUT_DIR/mesa3d"
cp "$WIN_EXE" "$OUT_DIR/singbox-launcher.exe"
DL_TMP="$(mktemp -d)"
trap 'rm -rf "$DL_TMP"' EXIT
curl -fsSL --retry 3 -o "$DL_TMP/core.zip" \
  "https://github.com/Leadaxe/sing-box-lx/releases/download/v${CORE_VER}/sing-box-${CORE_VER}-windows-amd64.zip"
unzip -q -j "$DL_TMP/core.zip" '*/sing-box.exe' -d "$OUT_DIR/bin"
# libcronet.dll accompanies the naive outbound; the core archive may not carry it.
unzip -q -j "$DL_TMP/core.zip" '*/libcronet.dll' -d "$OUT_DIR/bin" || true
unzip -q -j "$REPO_ROOT/assets/wintun-${WINTUN_VER}.zip" "wintun/bin/amd64/wintun.dll" -d "$OUT_DIR/bin"
cp "$REPO_ROOT/bin/wizard_template.json" "$OUT_DIR/bin/wizard_template.json"
printf '%s\n' "$VERSION" > "$OUT_DIR/bin/wizard_template.version"
curl -fsSL --retry 3 -o "$DL_TMP/mesa.zip" \
  "https://github.com/Leadaxe/singbox-launcher/releases/download/${MESA_TAG}/${MESA_ASSET}"
unzip -q -j "$DL_TMP/mesa.zip" '*.dll' -d "$OUT_DIR/mesa3d"

echo "✓ Staged win64-full in $OUT_DIR"
