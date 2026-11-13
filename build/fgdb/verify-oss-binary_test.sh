#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/verify-oss-binary.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "${tmp}/bin"
# Do not let the caller's opt-out weaken default-mode regression coverage.
unset FGDB_STRICT_CCL
ui_bundle=${tmp}/_bazel/bin/pkg/ui/workspaces/db-console/db-console-oss/assets/bundle.js
mkdir -p "$(dirname "$ui_bundle")"
echo 'clean OSS UI' >"$ui_bundle"
cat >"${tmp}/binary" <<'EOF'
#!/usr/bin/env bash
cat <<'VERSION'
Build Tag: v23.2.15
Distribution: OSS
Build Type: release
Go Version: go1.21.12
VERSION
EOF
chmod +x "${tmp}/binary"

cat >"${tmp}/bin/file" <<'EOF'
#!/usr/bin/env bash
echo "ELF 64-bit LSB pie executable, x86-64"
EOF
cat >"${tmp}/bin/nm" <<'EOF'
#!/usr/bin/env bash
echo "scanner failed" >&2
exit 2
EOF
cat >"${tmp}/bin/strings" <<'EOF'
#!/usr/bin/env bash
echo "scanner failed" >&2
exit 2
EOF
chmod +x "${tmp}/bin/file" "${tmp}/bin/nm" "${tmp}/bin/strings"

if PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
  echo "verify-oss-binary accepted an nm failure" >&2
  exit 1
fi
grep -q 'nm failed' "${tmp}/err"

cat >"${tmp}/bin/nm" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${tmp}/bin/nm"
if PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
  echo "verify-oss-binary accepted a strings failure" >&2
  exit 1
fi
grep -q 'strings failed' "${tmp}/err"

cat >"${tmp}/bin/nm" <<'EOF'
#!/usr/bin/env bash
echo "cockroach/pkg/ccl/example"
EOF
chmod +x "${tmp}/bin/nm"
if PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
  echo "verify-oss-binary accepted a CCL symbol" >&2
  exit 1
fi
grep -q 'CCL symbols are present' "${tmp}/err"

cat >"${tmp}/bin/nm" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"${tmp}/bin/strings" <<'EOF'
#!/usr/bin/env bash
echo "github.com/cockroachdb/cockroach/pkg/ccl/example"
EOF
chmod +x "${tmp}/bin/nm" "${tmp}/bin/strings"
if PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
  echo "verify-oss-binary accepted a CCL string" >&2
  exit 1
fi
grep -q 'pkg/ccl import path is present' "${tmp}/err"

cat >"${tmp}/bin/strings" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${tmp}/bin/strings"
PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out"
grep -q 'CCL-free checks passed' "${tmp}/out"

# Both protobuf markers must fail in binary strings and in the uncompressed
# UI, since the embedded assets in the binary may be compressed.
cat >"${tmp}/bin/strings" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${SCANNER_TEXT:-}"
EOF
cat >"${tmp}/bin/nm" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${NM_TEXT:-}"
EOF
for marker in '@cockroachlabs/crdb-protobuf-client-ccl' 'enginepbccl.EncryptionStatus' 'pkg/ccl/example' '//pkg/ccl:root'; do
  if SCANNER_TEXT=$marker PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
    echo "verify-oss-binary accepted binary marker: $marker" >&2
    exit 1
  fi
  grep -q 'strict CCL reference is present' "${tmp}/err"
  if NM_TEXT=$marker PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
    echo "verify-oss-binary accepted a symbol marker: $marker" >&2
    exit 1
  fi
  grep -q 'strict CCL reference is present' "${tmp}/err"
  echo "$marker" >"$ui_bundle"
  for setting in '' 1 false; do
    if FGDB_STRICT_CCL=$setting PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
      echo "verify-oss-binary accepted UI marker with strict=$setting: $marker" >&2
      exit 1
    fi
    grep -q 'strict CCL reference is present in the OSS UI bundle' "${tmp}/err"
  done
  SCANNER_TEXT=$marker FGDB_STRICT_CCL=0 PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out"
  echo 'clean OSS UI' >"$ui_bundle"
done

# Scanner errors must fail closed for both the symbol/string and UI scans.
export TEST_REAL_GREP
TEST_REAL_GREP=$(command -v grep)
cat >"${tmp}/bin/grep" <<'EOF'
#!/usr/bin/env bash
[[ $1 != "${TEST_FAIL_GREP_FLAG:-}" ]] || exit 2
exec "$TEST_REAL_GREP" "$@"
EOF
chmod +x "${tmp}/bin/grep"
for flag in -E -aE; do
  if TEST_FAIL_GREP_FLAG=$flag PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
    echo "verify-oss-binary accepted a scanner failure ($flag)" >&2
    exit 1
  fi
  grep -q 'strict CCL scan failed' "${tmp}/err"
done

# An explicit bundle path supports verification after relocating a binary.
mv "$ui_bundle" "${tmp}/relocated.js"
PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" "${tmp}/relocated.js" >"${tmp}/out"
if PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
  echo "verify-oss-binary accepted a missing OSS UI bundle" >&2
  exit 1
fi
grep -q 'requires the OSS UI bundle' "${tmp}/err"
FGDB_STRICT_CCL=0 PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out"

# Opting out of the new strict checks must retain the original binary checks.
if SCANNER_TEXT=github.com/cockroachdb/cockroach/pkg/ccl/example FGDB_STRICT_CCL=0 PATH="${tmp}/bin:${PATH}" "$script" "${tmp}/binary" >"${tmp}/out" 2>"${tmp}/err"; then
  echo "verify-oss-binary opt-out bypassed the original import check" >&2
  exit 1
fi
grep -q 'pkg/ccl import path is present' "${tmp}/err"

echo "verify-oss-binary tests passed"
