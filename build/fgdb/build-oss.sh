#!/usr/bin/env bash
#
# Build //pkg/cmd/cockroach-oss and //c-deps:libgeos.
#
# Upstream v23.2.15 release builds call bazel directly. See
# pkg/cmd/publish-provisional-artifacts and
# build/teamcity/cockroach/ci/builds/build_impl.sh. They do not use ./dev.
#
# ./dev is a workstation wrapper. On a clean checkout it exits with
# "please run `dev doctor`" until bin/.dev-status exists (doctor status
# version 8). `dev doctor` on Linux is interactive: it asks whether to
# append `build --config=dev` or `build --config=crosslinux` to
# .bazelrc.user, and other checks append more lines (lintonbuild,
# test --test_tmpdir). That would fight the file write-bazelrc-user.sh
# already wrote. --interactive=false does not skip those prompts; the
# Linux autofix refuses to run unless interactive is on.
#
# Bazel settings (channel stamp, crosslinuxbase, -c opt, nogo, cache,
# CPU, RAM) come from .bazelrc.user. This script only names the two
# targets. //pkg/ui/distoss is a dependency of cockroach-oss, so the UI
# is built by that target. There is no separate webpack or `dev ui` step.
#
# Staged paths match pkg/cmd/dev/build.go stageArtifacts and what
# verify-oss-binary.sh, package-oss-tarball.sh, and the workflow expect:
#   SRC/cockroach-oss
#   SRC/lib/libgeos.so
#   SRC/lib/libgeos_c.so
#
# Do not run as root. The shared disk cache must stay owned by the
# runner user.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: build-oss.sh SRC_DIR" >&2
  exit 2
fi

src=$1

if [[ "$(id -u)" -eq 0 ]]; then
  echo "build-oss.sh: refusing to run bazel as root" >&2
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

# ci's test config wants this absolute directory. The imports test
# overrides --test_tmpdir. Creating it keeps a test that does not
# override the flag from failing on a missing directory.
mkdir -p /artifacts/tmp 2>/dev/null || mkdir -p "${src}/artifacts/tmp"

echo "Building //pkg/cmd/cockroach-oss:cockroach-oss and //c-deps:libgeos"
echo "Not building //pkg/cmd/cockroach."
# Targets only. Config, stamp, and resources are build lines in
# .bazelrc.user. Do not append ./dev-style aliases ("oss", "geos"):
# those are dev's names, not Bazel labels.
bazel build //pkg/cmd/cockroach-oss:cockroach-oss //c-deps:libgeos

# .bazelrc sets --symlink_prefix=_bazel/, so this symlink is the
# bazel-bin of the build that just finished. `bazel info bazel-bin`
# does not apply `build` lines from the rc unless the same flags are
# repeated, and repeating them is easy to get wrong. The symlink is
# the configuration Bazel actually used.
bazel_bin="${src}/_bazel/bin"
oss_built="${bazel_bin}/pkg/cmd/cockroach-oss/cockroach-oss_/cockroach-oss"
if [[ ! -f "$oss_built" ]]; then
  echo "build-oss.sh: ${oss_built} is missing after the build" >&2
  echo "Expected the _bazel/bin symlink from --symlink_prefix=_bazel/." >&2
  exit 1
fi

# Prebuilt libgeos is fetched as @archived_cdep_libgeos_linux.
# dev copies it from execution_root/external/..., which points at the
# same tree as output_base/external/... output_base does not depend on
# the build configuration. The _bazel/<checkout-name> symlink is the
# execution root Bazel just wrote (--symlink_prefix=_bazel/).
# force_build_cdeps is not used; if that config is on, neither directory
# holds the library we ship.
output_base=$(bazel info output_base)
output_base=${output_base//[[:space:]]/}
checkout_name=$(basename "$src")
geos_candidates=(
  "${output_base}/external/archived_cdep_libgeos_linux/lib"
  "${src}/_bazel/${checkout_name}/external/archived_cdep_libgeos_linux/lib"
)
geos_dir=
for candidate in "${geos_candidates[@]}"; do
  if [[ -f "${candidate}/libgeos.so" && -f "${candidate}/libgeos_c.so" ]]; then
    geos_dir=$candidate
    break
  fi
done
if [[ -z "$geos_dir" ]]; then
  echo "build-oss.sh: prebuilt libgeos is missing" >&2
  echo "Looked in:" >&2
  printf '  %s\n' "${geos_candidates[@]}" >&2
  echo "This release uses the public archived c-deps. Do not enable --config=force_build_cdeps." >&2
  exit 1
fi

install -m 0755 "$oss_built" "${src}/cockroach-oss"
mkdir -p "${src}/lib"
# install follows symlinks and writes regular files, so a versioned
# libgeos.so.N symlink becomes the soname file the loader expects.
install -m 0644 "${geos_dir}/libgeos.so" "${src}/lib/libgeos.so"
install -m 0644 "${geos_dir}/libgeos_c.so" "${src}/lib/libgeos_c.so"

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

echo "Staged ${src}/cockroach-oss"
echo "Staged ${src}/lib/libgeos.so"
echo "Staged ${src}/lib/libgeos_c.so"
