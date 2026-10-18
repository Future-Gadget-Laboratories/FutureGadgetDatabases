#!/usr/bin/env bash
# Fail when FGDb-owned source imports pkg/ccl or starts with a CSL header.
# Starlark files (BUILD, WORKSPACE, .bazel, .bzl, .star) are included.
# Usage: fgdb/test/scripts/check-boundary.sh
# Set FGDB_BOUNDARY_ROOT to scan a fixture tree instead of this repository.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)

header_lines=25
# The phrases are split so this file's own text is not a header match.
csl_header='CockroachDB Software '"License"
ccl_import='github.com/cockroachdb/cockroach/pkg/ccl'
bazel_dep='//pkg/ccl'

fail=0

note() {
  echo "$1" >&2
  fail=1
}

header_has_csl() {
  local file=$1
  head -n "$header_lines" "$file" | grep -F -q -- "$csl_header"
}

go_imports_ccl() {
  local file=$1
  grep -E -q "^[[:space:]]*(_ )?\"[^\"]*${ccl_import}" "$file"
}

bazel_depends_on_ccl() {
  local file=$1
  grep -F -q -- "$bazel_dep" "$file"
}

if [[ -n "${FGDB_BOUNDARY_ROOT:-}" ]]; then
  cd "$FGDB_BOUNDARY_ROOT"
  find_roots=(.)
else
  cd "$repo_root"
  find_roots=(pkg/fgdbtest pkg/cmd/fgdb-test pkg/cmd/fgdb-backup fgdb build/fgdb)
fi

while IFS= read -r file; do
  file=${file#./}
  case "$file" in
    fgdb/test/scripts/check-boundary.sh|fgdb/test/scripts/check-boundary_test.sh)
      continue
      ;;
  esac
  case "$file" in
    *.go)
      if header_has_csl "$file"; then
        note "CSL header in ${file}"
      fi
      if go_imports_ccl "$file"; then
        note "pkg/ccl import in ${file}"
      fi
      ;;
    *.sh)
      if header_has_csl "$file"; then
        note "CSL header in ${file}"
      fi
      ;;
    *.bazel|*.bzl|*.star|BUILD|WORKSPACE|*/BUILD|*/WORKSPACE)
      if bazel_depends_on_ccl "$file"; then
        note "pkg/ccl Bazel dependency in ${file}"
      fi
      ;;
  esac
done < <(find "${find_roots[@]}" -type f \( \
  -name '*.go' -o -name '*.sh' -o -name '*.bazel' -o -name '*.bzl' -o -name '*.star' \
  -o -name 'BUILD' -o -name 'WORKSPACE' \
\) | sort)

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "fgdb boundary check passed"
