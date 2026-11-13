#!/usr/bin/env bash
# require-repo-commit.py accepts only a full SHA that is already on a
# branch or tag of this repository.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/require-repo-commit.py
repo=Future-Gadget-Laboratories/FutureGadgetDatabases

run_check() {
  COMMIT_SHA=$1 GITHUB_REPOSITORY=$2 python3 "$script"
}

if run_check abc "$repo" > /tmp/fgdb-require-bad.out 2> /tmp/fgdb-require-bad.err; then
  echo "short sha was accepted" >&2
  exit 1
fi
grep -q '40 hexadecimal' /tmp/fgdb-require-bad.err

missing=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
if run_check "$missing" "$repo" > /tmp/fgdb-require-miss.out 2> /tmp/fgdb-require-miss.err; then
  echo "unknown sha was accepted" >&2
  exit 1
fi
grep -q 'not in' /tmp/fgdb-require-miss.err

# Parent of the current main tip. It is in this repository and is not a
# branch tip, so the check must follow history rather than branch heads.
on_main=c3f0732a7b105180a463429266f2d88a539443b1
run_check "$on_main" "$repo" > /tmp/fgdb-require-ok.out
grep -q 'is on branch' /tmp/fgdb-require-ok.out

# A commit from another repository must not be treated as present here.
foreign=$(python3 - <<'PY'
import json, urllib.request
url = "https://api.github.com/repos/octocat/Hello-World/commits/7fd1a60b01f91b314f59955a4e4d4e80d8edf11d"
with urllib.request.urlopen(url, timeout=30) as resp:
    print(json.load(resp)["sha"])
PY
)
if run_check "$foreign" "$repo" > /tmp/fgdb-require-foreign.out 2> /tmp/fgdb-require-foreign.err; then
  echo "foreign sha was accepted" >&2
  exit 1
fi
grep -q 'not in' /tmp/fgdb-require-foreign.err
echo "require-repo-commit tests passed"
