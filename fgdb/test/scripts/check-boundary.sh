#!/usr/bin/env bash
# Fail when FGDb-owned source imports pkg/ccl or starts with a CSL header.
# Usage: fgdb/test/scripts/check-boundary.sh
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"

header_lines=25
# The phrases are split so this file's own text is not a header match.
csl_header='CockroachDB Software '"License"
ccl_import='github.com/cockroachdb/cockroach/pkg/ccl'
bazel_dep='//pkg/ccl'
self_script='fgdb/test/scripts/check-boundary.sh'

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

while IFS= read -r file; do
  if [[ "$file" == "$self_script" ]]; then
    continue
  fi
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
    *.bazel)
      if bazel_depends_on_ccl "$file"; then
        note "pkg/ccl Bazel dependency in ${file}"
      fi
      ;;
  esac
done < <(find pkg/fgdbtest pkg/cmd/fgdb-test pkg/cmd/fgdb-backup fgdb build/fgdb \
  -type f \( -name '*.go' -o -name '*.sh' -o -name '*.bazel' \) | sort)

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "fgdb boundary check passed"
