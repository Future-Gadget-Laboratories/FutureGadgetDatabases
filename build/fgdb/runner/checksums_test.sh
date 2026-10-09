#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
awk '
  /^#/ || NF == 0 { next }
  NF != 5 { exit 1 }
  $4 !~ /^[0-9a-f]{64}$/ { exit 1 }
  seen[$1 FS $2 FS $3]++ > 0 { exit 1 }
  END { if (!seen["actions-runner 2.338.0 linux-x64"] || !seen["bazelisk 1.29.0 linux-amd64"]) exit 1 }
' "$ROOT/checksums.txt"
printf 'checksum map tests passed\n'
