#!/usr/bin/env bash
# Usage: fgdb/test/run.sh <pr|nightly|weekly|interim>
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: fgdb/test/run.sh <pr|nightly|weekly|interim>" >&2
  exit 2
fi

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
exec "$here/scripts/run-local.sh" --tier="$1"
