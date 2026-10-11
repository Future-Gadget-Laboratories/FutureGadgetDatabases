#!/usr/bin/env bash
# Run one FGDb suite tier on this machine and in CI.
# Usage: fgdb/test/scripts/run-local.sh --tier=pr
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
tier=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tier)
      tier=${2:-}
      shift 2
      ;;
    --tier=*)
      tier=${1#--tier=}
      shift
      ;;
    pr | nightly | weekly | interim)
      tier=$1
      shift
      ;;
    *)
      echo "unknown argument: $1" >&2
      echo "usage: fgdb/test/scripts/run-local.sh --tier=<pr|nightly|weekly|interim>" >&2
      exit 2
      ;;
  esac
done

case "$tier" in
  pr | nightly | weekly | interim) ;;
  *)
    echo "usage: fgdb/test/scripts/run-local.sh --tier=<pr|nightly|weekly|interim>" >&2
    exit 2
    ;;
esac

if [[ -f /etc/fgdb/runner.env ]]; then
  set -a
  # shellcheck disable=SC1091
  . /etc/fgdb/runner.env
  set +a
fi

work=${FGDB_WORK:-$(mktemp -d)}
mkdir -p "$work"
cli=${FGDB_TEST_BIN:-$work/fgdb-test}
if [[ ! -x "$cli" ]]; then
  (cd "$root" && go build -o "$cli" ./pkg/cmd/fgdb-test)
fi

out=${FGDB_OUTPUT:-$work/out}
mkdir -p "$out"
cd "$root"
"$cli" run --tier "$tier" --root "$root" --output "$out"
echo "summary: $out/summary.md"
