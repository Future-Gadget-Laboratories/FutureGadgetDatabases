#!/usr/bin/env bash
#
# Fail if the staged cockroach-oss binary looks like a CCL build.
# This does not replace the Bazel disallowed-imports test; it checks the
# artifact that will actually be packaged.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: verify-oss-binary.sh BINARY" >&2
  exit 2
fi

bin=$1
expect_go=${FGDB_EXPECT_GO:-go1.21.12}

if [[ ! -f "$bin" ]]; then
  echo "verify-oss-binary: missing ${bin}" >&2
  exit 1
fi

file_out=$(file -b "$bin")
echo "file: ${file_out}"
case "$file_out" in
  *"ELF 64-bit"*"x86-64"*) ;;
  *)
    echo "verify-oss-binary: expected a 64-bit x86-64 ELF" >&2
    exit 1
    ;;
esac

if ! command -v nm >/dev/null 2>&1 || ! command -v strings >/dev/null 2>&1; then
  echo "verify-oss-binary: nm and strings are required (binutils)" >&2
  exit 1
fi

echo "Scanning symbols and strings for CCL import paths"
if nm -a "$bin" | grep -E 'cockroach/pkg/ccl/|cockroach/pkg/ui/distccl'; then
  echo "verify-oss-binary: CCL symbols are present in ${bin}" >&2
  exit 1
fi
if strings -a "$bin" | grep -F 'github.com/cockroachdb/cockroach/pkg/ccl/'; then
  echo "verify-oss-binary: pkg/ccl import path is present in ${bin}" >&2
  exit 1
fi
if strings -a "$bin" | grep -F 'github.com/cockroachdb/cockroach/pkg/ui/distccl'; then
  echo "verify-oss-binary: pkg/ui/distccl import path is present in ${bin}" >&2
  exit 1
fi

echo "Running: ${bin} version"
version_out=$("$bin" version)
printf '%s\n' "$version_out"

dist_line=$(printf '%s\n' "$version_out" | grep 'Distribution:' || true)
case "$dist_line" in
  *"OSS"*) ;;
  *)
    echo "verify-oss-binary: Distribution line must contain OSS (got: ${dist_line:-<missing>})" >&2
    exit 1
    ;;
esac
case "$dist_line" in
  *"CCL"*)
    echo "verify-oss-binary: Distribution line contains CCL" >&2
    exit 1
    ;;
esac

type_line=$(printf '%s\n' "$version_out" | grep 'Build Type:' || true)
case "$type_line" in
  *"release"*) ;;
  *)
    echo "verify-oss-binary: Build Type must be release (got: ${type_line:-<missing>})" >&2
    exit 1
    ;;
esac

go_line=$(printf '%s\n' "$version_out" | grep 'Go Version:' || true)
case "$go_line" in
  *"${expect_go}"*) ;;
  *)
    echo "verify-oss-binary: Go Version must contain ${expect_go} (got: ${go_line:-<missing>})" >&2
    exit 1
    ;;
esac

echo "CCL-free checks passed"
