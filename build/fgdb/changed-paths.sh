#!/usr/bin/env bash
#
# Detect FGDb changes in a NUL-separated path list from a file or stdin.
# Prints one fgdb=true or fgdb=false line for GitHub Actions outputs.

set -euo pipefail

if [[ $# -gt 1 ]]; then
  echo 'usage: changed-paths.sh [path-list-file]' >&2
  exit 2
fi
if [[ $# -eq 1 ]]; then
  exec < "$1"
fi

fgdb=false
# Also process a final record without NUL. Drain the stream even after a match
# so a piped git diff can finish successfully under pipefail.
while IFS= read -r -d '' path || [[ -n "$path" ]]; do
  case "$path" in
    .github/workflows/*|fgdb/*|build/fgdb/*|pkg/*|go.mod|go.sum)
      fgdb=true
      ;;
  esac
done
printf 'fgdb=%s\n' "$fgdb"
