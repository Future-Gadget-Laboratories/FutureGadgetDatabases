#!/usr/bin/env bash
# Exercise the release wrapper with fake tools; never invoke real Bazel.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
src=${tmp}/src
export TEST_OUTPUT_BASE=${tmp}/output TEST_BAZEL_LOG=${tmp}/bazel.log
mkdir -p "${tmp}/bin" "$src" "${TEST_OUTPUT_BASE}/external/archived_cdep_libgeos_linux/lib"
touch "${src}/.bazelrc.user"
touch "${TEST_OUTPUT_BASE}/external/archived_cdep_libgeos_linux/lib/"{libgeos.so,libgeos_c.so}
oss_bin=${src}/_bazel/bin/pkg/cmd/cockroach-oss/cockroach-oss_/cockroach-oss
ui_bundle=${src}/_bazel/bin/pkg/ui/workspaces/db-console/db-console-oss/assets/bundle.js
mkdir -p "$(dirname "$oss_bin")" "$(dirname "$ui_bundle")"
echo 'fake executable' >"$oss_bin"
echo 'clean OSS UI' >"$ui_bundle"
cat >"${tmp}/bin/id" <<'TOOL'
#!/usr/bin/env bash
echo 1000
TOOL
cat >"${tmp}/bin/mkdir" <<'TOOL'
#!/usr/bin/env bash
# Keep the wrapper's absolute artifacts-directory attempt inside the fixture.
[[ $1 != /artifacts/tmp ]] || exit 1
exec /bin/mkdir "$@"
TOOL
cat >"${tmp}/bin/bazel" <<'TOOL'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$TEST_BAZEL_LOG"
case "$1" in
  cquery)
    [[ $2 == 'deps(set(//pkg/cmd/cockroach-oss:cockroach-oss //c-deps:libgeos))' && $3 == --output=label ]] || exit 3
    printf '%s\n' "${TEST_GRAPH:-//pkg/ui/workspaces/db-console:db-console-oss (abc123)}"
    exit "${TEST_QUERY_STATUS:-0}"
    ;;
  build) exit 0 ;;
  info) echo "$TEST_OUTPUT_BASE" ;;
  *) exit 3 ;;
esac
TOOL
chmod +x "${tmp}/bin/"*
export PATH="${tmp}/bin:${PATH}"
unset FGDB_STRICT_CCL

run_build() {
  : >"$TEST_BAZEL_LOG"
  bash "${root}/build-oss.sh" "$src" >"${tmp}/out" 2>"${tmp}/err"
}
expect_failure() {
  if run_build; then
    echo "build-oss accepted $1" >&2
    exit 1
  fi
  grep -q "$2" "${tmp}/err"
}

run_build
test -x "${src}/cockroach-oss"
test -f "${src}/lib/libgeos.so"
grep -q '^cquery ' "$TEST_BAZEL_LOG"

for marker in '//pkg/ccl/example:lib' '//pkg/ccl:root' '//pkg/ui/distccl:distccl' '//pkg/ui/workspaces/db-console/ccl/src/js:crdb-protobuf-client-ccl-lib' '//other:enginepbccl_proto'; do
  export TEST_GRAPH=$marker
  expect_failure "CCL graph marker: $marker" 'forbidden OSS dependency'
  if grep -q '^build ' "$TEST_BAZEL_LOG"; then
    echo "build-oss started a build before rejecting the CCL dependency" >&2
    exit 1
  fi
done
unset TEST_GRAPH
export TEST_QUERY_STATUS=2
expect_failure 'a dependency query failure' 'dependency query failed'
unset TEST_QUERY_STATUS

for marker in '@cockroachlabs/crdb-protobuf-client-ccl' 'enginepbccl.EncryptionStatus' 'pkg/ccl/example' '//pkg/ccl:root'; do
  echo "$marker" >"$ui_bundle"
  expect_failure "CCL UI marker: $marker" 'forbidden reference in the OSS UI bundle'
done
for setting in '' 1 false; do
  export FGDB_STRICT_CCL=$setting TEST_GRAPH=//pkg/ccl/example:lib
  expect_failure "CCL graph with strict=$setting" 'forbidden OSS dependency'
done
unset FGDB_STRICT_CCL TEST_GRAPH
rm "$ui_bundle"
expect_failure 'a missing OSS UI bundle' 'requires the OSS UI bundle'

# Scanner errors must fail closed for both the graph and UI scans.
export TEST_REAL_GREP
TEST_REAL_GREP=$(command -v grep)
cat >"${tmp}/bin/grep" <<'TOOL'
#!/usr/bin/env bash
[[ $1 != "${TEST_FAIL_GREP_FLAG:-}" ]] || exit 2
exec "$TEST_REAL_GREP" "$@"
TOOL
chmod +x "${tmp}/bin/grep"
echo 'clean OSS UI' >"$ui_bundle"
for flag in -E -aE; do
  export TEST_FAIL_GREP_FLAG=$flag
  expect_failure "a scanner failure ($flag)" 'strict CCL scan failed'
done
unset TEST_FAIL_GREP_FLAG

# Only literal 0 opts out, including both graph and UI checks.
export FGDB_STRICT_CCL=0 TEST_GRAPH=//pkg/ccl/example:lib TEST_QUERY_STATUS=2
run_build
if grep -q '^cquery ' "$TEST_BAZEL_LOG"; then
  echo 'build-oss queried dependencies despite FGDB_STRICT_CCL=0' >&2
  exit 1
fi
echo 'enginepbccl.EncryptionStatus' >"$ui_bundle"
run_build

echo 'build-oss tests passed'
