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

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=fgdb/test/scripts/scrub-credentials.sh
. "$script_dir/scrub-credentials.sh"

base=${RUNNER_TEMP:-${TMPDIR:-/tmp}}
own_work=0
if [[ -z "${FGDB_WORK:-}" ]]; then
  FGDB_WORK=$(mktemp -d "${base}/fgdb-suite.XXXXXX")
  own_work=1
fi
export FGDB_WORK
cleanup_work() {
  if [[ "$own_work" -eq 1 ]]; then
    rm -rf "$FGDB_WORK"
  fi
}
trap cleanup_work EXIT

cli=${FGDB_TEST_BIN:-$FGDB_WORK/fgdb-test}
if [[ ! -x "$cli" ]]; then
  (cd "$root" && fgdb_exec go build -o "$cli" ./pkg/cmd/fgdb-test)
fi

out=${FGDB_OUTPUT:-$FGDB_WORK/out}
case "$out" in
  "$FGDB_WORK" | "$FGDB_WORK"/*) ;;
  *)
    echo "FGDB_OUTPUT must stay inside FGDB_WORK" >&2
    exit 1
    ;;
esac
mkdir -p "$out"
cd "$root"
# Candidate binaries and workloads are started with an allowlisted environment.
fgdb_exec "$cli" run --tier "$tier" --root "$root" --output "$out"
echo "summary: $out/summary.md"
if [[ -f "$out/summary.md" ]]; then
  cat "$out/summary.md"
fi
