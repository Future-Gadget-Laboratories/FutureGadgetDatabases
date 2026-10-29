#!/usr/bin/env bash
#
# Fail if the staged cockroach-oss binary looks like a CCL build.
# This does not replace the Bazel disallowed-imports test; it checks the
# artifact that will actually be packaged.
#
# FGDB_STRICT_CCL is ON unless set explicitly to 0. Strict mode also
# rejects CCL protobuf/import references in the binary and OSS UI bundle.
# UI_BUNDLE defaults to the companion _bazel/bin OSS bundle under the
# binary directory; pass it explicitly when verifying a relocated binary.
# FGDB_STRICT_CCL=0 skips these additional checks, retaining the original
# binary checks. build-oss.sh checks the configured OSS dependency graph.

set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: verify-oss-binary.sh BINARY [UI_BUNDLE]" >&2
  exit 2
fi

bin=$1
expect_go=${FGDB_EXPECT_GO:-go1.21.12}
ui_bundle=${2:-$(dirname "$bin")/_bazel/bin/pkg/ui/workspaces/db-console/db-console-oss/assets/bundle.js}

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
if ! nm_output=$(nm -a "$bin"); then
  echo "verify-oss-binary: nm failed for ${bin}" >&2
  exit 1
fi
if grep -E 'cockroach/pkg/ccl/|cockroach/pkg/ui/distccl' <<<"$nm_output"; then
  echo "verify-oss-binary: CCL symbols are present in ${bin}" >&2
  exit 1
fi
if ! strings_output=$(strings -a "$bin"); then
  echo "verify-oss-binary: strings failed for ${bin}" >&2
  exit 1
fi
if grep -F 'github.com/cockroachdb/cockroach/pkg/ccl/' <<<"$strings_output"; then
  echo "verify-oss-binary: pkg/ccl import path is present in ${bin}" >&2
  exit 1
fi
if grep -F 'github.com/cockroachdb/cockroach/pkg/ui/distccl' <<<"$strings_output"; then
  echo "verify-oss-binary: pkg/ui/distccl import path is present in ${bin}" >&2
  exit 1
fi

if [[ "${FGDB_STRICT_CCL:-1}" != 0 ]]; then
  if grep -E 'crdb-protobuf-client-ccl|enginepbccl|pkg/ccl([/:]|$)|pkg/ui/distccl' <<<"${nm_output}
${strings_output}" >/dev/null; then
    echo "verify-oss-binary: strict CCL reference is present in ${bin}" >&2
    exit 1
  elif [[ $? -ne 1 ]]; then
    echo "verify-oss-binary: strict CCL scan failed" >&2
    exit 1
  fi
  if [[ ! -s "$ui_bundle" ]]; then
    echo "verify-oss-binary: strict CCL check requires the OSS UI bundle: ${ui_bundle}" >&2
    exit 1
  fi
  if grep -aE 'crdb-protobuf-client-ccl|enginepbccl|pkg/ccl([/:]|$)|pkg/ui/distccl' "$ui_bundle" >/dev/null; then
    echo "verify-oss-binary: strict CCL reference is present in the OSS UI bundle" >&2
    exit 1
  elif [[ $? -ne 1 ]]; then
    echo "verify-oss-binary: strict CCL scan failed" >&2
    exit 1
  fi
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
