#!/usr/bin/env bash
# Refresh one pin in checksums.txt.
#
# The downloaded file is never trusted by itself. The Actions Runner hash
# has to match the linux-x64 value in that version's GitHub release notes.
# The Bazelisk hash has to match the published bazelisk-linux-amd64.sha256
# file. checksums.txt stays mode 0644.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MAP=${ROOT}/checksums.txt
# shellcheck source=checksum-notes.sh
# shellcheck disable=SC1091
source "${ROOT}/checksum-notes.sh"

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 2 ]] || die "usage: update-checksums.sh COMPONENT VERSION"
component=$1
version=$2
case "$component" in
  actions-runner)
    platform=linux-x64
    filename="actions-runner-linux-x64-${version}.tar.gz"
    url="https://github.com/actions/runner/releases/download/v${version}/${filename}"
    ;;
  bazelisk)
    platform=linux-amd64
    filename=bazelisk-linux-amd64
    url="https://github.com/bazelbuild/bazelisk/releases/download/v${version}/${filename}"
    ;;
  *)
    die "unsupported component: ${component}"
    ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
artifact=${tmp}/${filename}
curl -fsSL "$url" -o "$artifact" || die "could not download ${url}"
sha=$(sha256sum "$artifact" | awk '{print $1}')

if [[ "$component" == actions-runner ]]; then
  notes_json=$(curl -fsSL "https://api.github.com/repos/actions/runner/releases/tags/v${version}") ||
    die "could not download the Actions Runner release notes for ${version}"
  body=$(printf '%s' "$notes_json" | jq -r '.body') ||
    die "could not parse the Actions Runner release notes for ${version}"
  [[ -n "$body" && "$body" != null ]] ||
    die "the Actions Runner release notes for ${version} are empty"
  published=$(runner_linux_x64_sha "$body" "$filename") ||
    die "the release notes for ${version} do not contain a linux-x64 checksum between BEGIN SHA and END SHA"
else
  sidecar=$(curl -fsSL "${url}.sha256") ||
    die "could not download the published Bazelisk checksum for ${version}. Refusing to trust the binary download by itself."
  published=$(bazelisk_sidecar_sha "$sidecar") ||
    die "the published Bazelisk checksum for ${version} is not a sha256 hash. Refusing to trust the binary download by itself."
fi
[[ "$published" == "$sha" ]] ||
  die "download hash ${sha} does not match publisher hash ${published}"

tmp_map=$(mktemp)
awk -v c="$component" -v v="$version" -v p="$platform" -v h="$sha" -v f="$filename" '
  $1 == c && $2 == v && $3 == p { next }
  { print }
  END { printf "%s %s %s %s %s\n", c, v, p, h, f }
' "$MAP" >"$tmp_map"
chmod 0644 "$tmp_map"
cat "$tmp_map" >"$MAP"
rm -f "$tmp_map"
chmod 0644 "$MAP"
printf 'updated %s %s %s %s\n' "$component" "$version" "$platform" "$sha"
