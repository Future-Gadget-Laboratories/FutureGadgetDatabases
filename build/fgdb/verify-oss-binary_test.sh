#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/verify-oss-binary.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "${tmp}/bin"
touch "${tmp}/binary"

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

echo "verify-oss-binary tests passed"
