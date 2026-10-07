#!/usr/bin/env bash
#
# Build //pkg/cmd/cockroach-oss and //c-deps:libgeos.
# Must be run as a non-root user (./dev refuses root).
# Reads Bazel settings from .bazelrc.user in SRC_DIR.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: build-oss.sh SRC_DIR" >&2
  exit 2
fi

src=$1

if [[ "$(id -u)" -eq 0 ]]; then
  echo "build-oss.sh: ./dev cannot be run as root" >&2
  exit 1
fi

if [[ ! -f "${src}/.bazelrc.user" ]]; then
  echo "build-oss.sh: ${src}/.bazelrc.user is missing; run write-bazelrc-user.sh first" >&2
  exit 1
fi

if command -v ccache >/dev/null 2>&1; then
  echo "warning: ccache is on PATH. This build does not use it." >&2
fi

cd "$src"
export DEV_NO_REMOTE_CACHE=1

# ci's test config wants this absolute directory. The imports test below
# overrides --test_tmpdir, but creating it keeps other ci test flags quiet.
mkdir -p /artifacts/tmp 2>/dev/null || mkdir -p "${src}/artifacts/tmp"

echo "Building //pkg/cmd/cockroach-oss and //c-deps:libgeos (not //pkg/cmd/cockroach)"
./dev build oss geos

if [[ ! -x "${src}/cockroach-oss" ]]; then
  echo "build-oss.sh: ${src}/cockroach-oss was not staged" >&2
  exit 1
fi
if [[ ! -f "${src}/lib/libgeos.so" || ! -f "${src}/lib/libgeos_c.so" ]]; then
  echo "build-oss.sh: libgeos.so and libgeos_c.so were not staged under ${src}/lib" >&2
  exit 1
fi

# A stray CCL binary from a previous workspace must not be what we ship.
# Packaging reads only cockroach-oss; remove the other name if present.
if [[ -e "${src}/cockroach" ]]; then
  echo "Removing staged ${src}/cockroach so it cannot be confused with cockroach-oss"
  rm -f "${src}/cockroach"
fi
