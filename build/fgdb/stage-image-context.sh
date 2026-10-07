#!/usr/bin/env bash
#
# Prepare a docker build context for build/deploy-oss/Dockerfile
# from a packaged OSS tarball plus the upstream entrypoint script.

set -euo pipefail

tarball=
source_dir=
out=

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tarball)
      tarball=$2
      shift 2
      ;;
    --source)
      source_dir=$2
      shift 2
      ;;
    --out)
      out=$2
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [[ -z "$tarball" || -z "$source_dir" || -z "$out" ]]; then
  echo "usage: stage-image-context.sh --tarball TGZ --source SRC_DIR --out DIR" >&2
  exit 2
fi

entrypoint="${source_dir}/build/deploy/cockroach.sh"
if [[ ! -f "$entrypoint" ]]; then
  echo "stage-image-context: missing ${entrypoint}" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tar -C "$tmp" -xzf "$tarball"

prefix=$(find "$tmp" -mindepth 1 -maxdepth 1 -type d -name 'cockroach-oss-*.linux-amd64' -print -quit)
if [[ -z "$prefix" ]]; then
  echo "stage-image-context: tarball has no cockroach-oss-*.linux-amd64 directory" >&2
  exit 1
fi

rm -rf "$out"
mkdir -p "$out"
install -m 0755 "${prefix}/cockroach" "${out}/cockroach"
install -m 0755 "$entrypoint" "${out}/cockroach.sh"
install -m 0644 "${prefix}/lib/libgeos.so" "${out}/libgeos.so"
install -m 0644 "${prefix}/lib/libgeos_c.so" "${out}/libgeos_c.so"
mkdir -p "${out}/licenses"
cp -a "${prefix}/licenses/." "${out}/licenses/"

echo "Staged image context at ${out}"
