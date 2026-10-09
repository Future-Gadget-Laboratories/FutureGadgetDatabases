#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MAP=${ROOT}/checksums.txt

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
curl -fsSL "$url" -o "$artifact"
sha=$(sha256sum "$artifact" | awk '{print $1}')

if [[ "$component" == actions-runner ]]; then
  notes=$(curl -fsSL "https://api.github.com/repos/actions/runner/releases/tags/v${version}")
  published=$(printf '%s' "$notes" | jq -r '.body' | tr -d '\r' | awk -v file="$filename" '
    index($0, file) { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9a-fA-F]{64}$/) { print tolower($i); exit } }')
  [[ "$published" == "$sha" ]] || die "download hash ${sha} does not match publisher hash ${published:-<missing>}"
fi

tmp_map=$(mktemp)
awk -v c="$component" -v v="$version" -v p="$platform" -v f="$filename" -v h="$sha" '
  $1 == c && $2 == v && $3 == p { next }
  { print }
  END { printf "%s %s %s %s %s\n", c, v, p, h, f }
' "$MAP" >"$tmp_map"
mv "$tmp_map" "$MAP"
printf 'updated %s %s %s %s\n' "$component" "$version" "$platform" "$sha"
