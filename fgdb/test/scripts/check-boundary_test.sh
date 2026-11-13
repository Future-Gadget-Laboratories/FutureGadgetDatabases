#!/usr/bin/env bash
# The boundary scanner must reject Starlark names other than *.bazel.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
script=${root}/fgdb/test/scripts/check-boundary.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Split so this script is not itself a dependency match.
dep='//pkg/'"ccl"

expect_fail() {
  local dir=$1
  local label=$2
  if FGDB_BOUNDARY_ROOT="$dir" bash "$script" >"${tmp}/out" 2>"${tmp}/err"; then
    echo "${label} was accepted" >&2
    exit 1
  fi
  grep -q 'Bazel dependency' "${tmp}/err"
}

mkdir -p "${tmp}/clean/nested"
printf 'package clean\n' > "${tmp}/clean/nested/a.go"
printf 'genrule(name = "ok")\n' > "${tmp}/clean/BUILD"
printf 'def ok():\n  pass\n' > "${tmp}/clean/nested/macros.bzl"
FGDB_BOUNDARY_ROOT="${tmp}/clean" bash "$script" > "${tmp}/clean.out"
grep -q 'passed' "${tmp}/clean.out"

mkdir -p "${tmp}/build"
printf 'deps = ["%s"]\n' "$dep" > "${tmp}/build/BUILD"
expect_fail "${tmp}/build" "BUILD"

mkdir -p "${tmp}/bzl"
printf 'x = "%s"\n' "$dep" > "${tmp}/bzl/macros.bzl"
expect_fail "${tmp}/bzl" ".bzl"

mkdir -p "${tmp}/workspace"
printf 'local_repository(name = "bad", path = "%s")\n' "$dep" > "${tmp}/workspace/WORKSPACE"
expect_fail "${tmp}/workspace" "WORKSPACE"

bash "$script" > "${tmp}/repo.out"
grep -q 'passed' "${tmp}/repo.out"
echo "check-boundary tests passed"
