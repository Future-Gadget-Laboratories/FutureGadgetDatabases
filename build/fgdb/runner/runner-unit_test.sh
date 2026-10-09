#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO=$(cd "$ROOT/../../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# shellcheck source=runner-unit.sh
# shellcheck disable=SC1091
source "$ROOT/runner-unit.sh"

ALLOWED_FILESYSTEMS=(xfs btrfs)
line=$(build_exec_start_pre /var/cache/fgdb/bazel-output-base 50)
[[ "$line" == "/usr/bin/bash /usr/local/lib/fgdb/check-cache-dir.sh --allow-fstype xfs --allow-fstype btrfs /var/cache/fgdb/bazel-output-base 50" ]]
[[ "$line" != *"ext4"* ]]

# shellcheck disable=SC2034
ALLOWED_FILESYSTEMS=(ext4 xfs btrfs zfs)
build_cache_check_args /var/cache/fgdb 150
[[ "${CACHE_CHECK_ARGS[*]}" == "--allow-fstype ext4 --allow-fstype xfs --allow-fstype btrfs --allow-fstype zfs /var/cache/fgdb 150" ]]

cp "$ROOT/check-cache-dir.sh" "$tmp/check.sh"
chmod a-x "$tmp/check.sh"
require_cache_check_script "$tmp/check.sh"

if bash -c 'source "$1"; require_cache_check_script "$2"' bash "$ROOT/runner-unit.sh" "$tmp/missing.sh" 2>"$tmp/err"; then
  printf 'missing script was accepted\n' >&2
  exit 1
fi
grep -q 'cache check script is missing' "$tmp/err"

printf 'if\n' >"$tmp/bad.sh"
if bash -c 'source "$1"; require_cache_check_script "$2"' bash "$ROOT/runner-unit.sh" "$tmp/bad.sh" 2>"$tmp/err"; then
  printf 'broken script was accepted\n' >&2
  exit 1
fi
grep -q 'cannot be run' "$tmp/err"

mkdir -p "$tmp/setup"
cp "$ROOT/setup-runner.sh" "$ROOT/token-input.sh" "$ROOT/read-config.sh" "$ROOT/runner-unit.sh" "$tmp/setup/"
if bash "$tmp/setup/setup-runner.sh" --dry-run >"$tmp/out" 2>"$tmp/err"; then
  printf 'dry-run without the cache check script succeeded\n' >&2
  exit 1
fi
grep -q 'cache check script is missing' "$tmp/err"

# shellcheck disable=SC2016
grep -q 'bash "$cache_check"' "$ROOT/setup-runner.sh"
if grep 'check-cache-dir' "$ROOT/setup-runner.sh" | grep -q -- '-x'; then
  printf 'setup still decides whether to run the cache check from the exec bit\n' >&2
  exit 1
fi
grep -q 'build_exec_start_pre' "$ROOT/setup-runner.sh"

[[ ! -x "$ROOT/token-input.sh" ]]
for test_file in "$ROOT"/*_test.sh; do
  head -n 5 "$test_file" | grep -q 'Apache License, Version 2.0'
done
head -n 8 "$ROOT/config.example.yaml" | grep -q 'environment'

grep -q 'ACTIONS_RUNNER_INPUT_TOKEN' "$REPO/docs/fgdb/RUNNER.md"
grep -q 'findmnt' "$REPO/docs/fgdb/RUNNER.md"
grep -q -- '--token-file' "$REPO/docs/fgdb/RUNNER.md"
grep -q 'BEGIN SHA linux-x64' "$REPO/docs/fgdb/RUNNER.md"
grep -q 'btrfs' "$REPO/docs/fgdb/RUNNER.md"
grep -q 'auto_update' "$REPO/docs/fgdb/runner-config.md"
grep -q 'allowed_filesystems' "$REPO/docs/fgdb/runner-config.md"
grep -q 'key: value' "$REPO/docs/fgdb/runner-config.md"

printf 'runner-unit tests passed\n'
