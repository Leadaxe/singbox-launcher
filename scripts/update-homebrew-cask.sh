#!/usr/bin/env bash
# Refresh the Cask from the latest *stable* GitHub Release.
# Intended for GitHub Actions (gh, jq and python3 are available on the runner).
set -euo pipefail

REPOSITORY="${UPSTREAM_REPOSITORY:-Leadaxe/singbox-launcher}"
CASK_FILE="${1:-Casks/singbox-launcher.rb}"

for command in gh jq python3; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Missing required command: $command" >&2
    exit 1
  fi
done

if [[ ! -f "$CASK_FILE" ]]; then
  echo "Cask file not found: $CASK_FILE" >&2
  exit 1
fi

release_json="$(gh api "repos/${REPOSITORY}/releases/latest")"
tag="$(jq -er '.tag_name' <<< "$release_json")"

# The release workflow uses vX.Y.Z stable tags; ignore dev/prerelease tags.
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Latest stable release has unexpected tag '$tag'; not updating" >&2
  exit 1
fi

asset="singbox-launcher-${tag}-macos.zip"
asset_json="$(jq -cer --arg name "$asset" '.assets[] | select(.name == $name)' <<< "$release_json")" || {
  echo "No '$asset' in stable release $tag; not updating" >&2
  exit 1
}

# GitHub provides the SHA-256 digest for release assets. Older assets may lack
# this field, so retain checksums.txt as a compatibility fallback.
digest="$(jq -r '.digest // empty' <<< "$asset_json")"
if [[ "$digest" =~ ^sha256:[[:xdigit:]]{64}$ ]]; then
  checksum="${digest#sha256:}"
else
  temp_dir="$(mktemp -d)"
  trap 'rm -rf "$temp_dir"' EXIT
  gh release download "$tag" --repo "$REPOSITORY" --pattern checksums.txt --dir "$temp_dir"
  checksum="$(awk -v name="$asset" '$2 == name {print $1}' "$temp_dir/checksums.txt")"
fi

if [[ ! "$checksum" =~ ^[0-9a-f]{64}$ ]]; then
  echo "Invalid SHA-256 for '$asset': $checksum" >&2
  exit 1
fi

# Only the version and SHA may change. Reject unexpected cask layouts.
python3 - "$CASK_FILE" "${tag#v}" "$checksum" <<'PY'
from pathlib import Path
import re
import sys

path = Path(sys.argv[1])
version = sys.argv[2]
checksum = sys.argv[3]
original = path.read_text(encoding="utf-8")
existing_versions = re.findall(r'(?m)^  version "([0-9]+\.[0-9]+\.[0-9]+)"
    r'(?m)^(  version ")[^"\n]+(")$',
    lambda match: f"{match.group(1)}{version}{match.group(2)}",
    original,
)
updated, checksum_count = re.subn(
    r'(?m)^(  sha256 ")[0-9a-f]{64}(")$',
    lambda match: f"{match.group(1)}{checksum}{match.group(2)}",
    updated,
)
if version_count != 1 or checksum_count != 1:
    raise SystemExit("Expected exactly one version and sha256 stanza in Cask")
if original == updated:
    print(f"Cask already up to date at {version}")
else:
    path.write_text(updated, encoding="utf-8")
    print(f"Updated Cask to {version} ({checksum})")
PY
, original)
if len(existing_versions) != 1:
    raise SystemExit("Expected exactly one semantic version stanza in Cask")
if tuple(map(int, version.split("."))) < tuple(map(int, existing_versions[0].split("."))):
    print(f"Skipping older stable release {version}; current Cask is {existing_versions[0]}")
    sys.exit(0)
updated, version_count = re.subn(
    r'(?m)^(  version ")[^"\n]+(")$',
    lambda match: f"{match.group(1)}{version}{match.group(2)}",
    original,
)
updated, checksum_count = re.subn(
    r'(?m)^(  sha256 ")[0-9a-f]{64}(")$',
    lambda match: f"{match.group(1)}{checksum}{match.group(2)}",
    updated,
)
if version_count != 1 or checksum_count != 1:
    raise SystemExit("Expected exactly one version and sha256 stanza in Cask")
if original == updated:
    print(f"Cask already up to date at {version}")
else:
    path.write_text(updated, encoding="utf-8")
    print(f"Updated Cask to {version} ({checksum})")
PY
