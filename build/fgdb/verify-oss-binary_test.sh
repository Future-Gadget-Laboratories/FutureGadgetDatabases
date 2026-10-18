#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/verify-oss-binary.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "${tmp}/bin"
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

echo "verify-oss-binary tests passed"
