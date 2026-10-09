#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# shellcheck source=read-config.sh
# shellcheck disable=SC1091
source "$ROOT/read-config.sh"
# Assigned by parse_runner_config. Declared here so static checks see them.
RC_auto_update=""
RC_cache_path=""
RC_allowed_filesystems=()
RC_local_cpu=""
RC_local_ram_mb=""
RC_cpu_quota=""
RC_memory_high=""
RC_memory_max=""
RC_runner_version=""
RC_bazelisk_version=""

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

expect_fail() {
  local file=$1 needle=$2
  local err
  err=$(mktemp)
  if bash -c 'source "$1"; parse_runner_config "$2"' bash "$ROOT/read-config.sh" "$file" 2>"$err"; then
    fail "expected $file to be rejected"
  fi
  if ! grep -q "$needle" "$err"; then
    cat "$err" >&2
    fail "missing error text: $needle"
  fi
  rm -f "$err"
}

write_cfg() {
  local name=$1
  cat >"$tmp/$name"
}

parse_runner_config "$ROOT/config.example.yaml"
[[ "$RC_auto_update" == true ]]
[[ "$RC_cache_path" == /var/cache/fgdb ]]
[[ "${RC_allowed_filesystems[*]}" == "ext4 xfs btrfs zfs" ]]
[[ "$RC_local_cpu" == 24 ]]
[[ "$RC_local_ram_mb" == 81920 ]]
[[ "$RC_cpu_quota" == "2400%" ]]
[[ "$RC_memory_high" == 80G ]]
[[ "$RC_memory_max" == 96G ]]
[[ "$RC_runner_version" == 2.338.0 ]]
[[ "$RC_bazelisk_version" == 1.29.0 ]]
grep -q 'environment' "$ROOT/config.example.yaml"

write_cfg quotes.yaml <<'EOF'
cache_path: "/mnt/disk:1/fgdb" # keep the colon
auto_update: "false"
local_cpu: '8' # quoted number
allowed_filesystems:
  - "ext4" # root
  - 'xfs'
EOF
parse_runner_config "$tmp/quotes.yaml"
[[ "$RC_cache_path" == "/mnt/disk:1/fgdb" ]]
[[ "$RC_auto_update" == false ]]
[[ "$RC_local_cpu" == 8 ]]
[[ "${RC_allowed_filesystems[*]}" == "ext4 xfs" ]]

kept=$(yaml_scalar '"/var/cache/fgdb # ssd"' 1)
[[ "$kept" == "/var/cache/fgdb # ssd" ]]
kept=$(yaml_scalar '/var/cache/fgdb # ssd' 1)
[[ "$kept" == /var/cache/fgdb ]]

write_cfg crlf.yaml <<'EOF'
cache_path: /var/cache/fgdb
local_cpu: 4
EOF
sed -i 's/$/\r/' "$tmp/crlf.yaml"
parse_runner_config "$tmp/crlf.yaml"
[[ "$RC_cache_path" == /var/cache/fgdb ]]
[[ "$RC_local_cpu" == 4 ]]

# File overrides environment. A later flag assignment overrides the file.
LOCAL_CPU=${FGDB_LOCAL_CPU:-24}
FGDB_LOCAL_CPU=3
LOCAL_CPU=${FGDB_LOCAL_CPU:-24}
AUTO_UPDATE=1
ALLOWED_FILESYSTEMS=(ext4 xfs btrfs zfs)
write_cfg prec.yaml <<'EOF'
local_cpu: 8
auto_update: false
allowed_filesystems:
  - xfs
EOF
parse_runner_config "$tmp/prec.yaml"
apply_runner_config
[[ "$LOCAL_CPU" == 8 ]]
[[ "$AUTO_UPDATE" == 0 ]]
[[ "${ALLOWED_FILESYSTEMS[*]}" == xfs ]]
LOCAL_CPU=12
[[ "$LOCAL_CPU" == 12 ]]

apply_line=$(grep -n 'apply_runner_config' "$ROOT/setup-runner.sh" | head -1 | cut -d: -f1)
flag_line=$(grep -n 'while \[\[ $# -gt 0 \]\]' "$ROOT/setup-runner.sh" | head -1 | cut -d: -f1)
[[ -n "$apply_line" && -n "$flag_line" && "$apply_line" -lt "$flag_line" ]]

write_cfg unknown.yaml <<'EOF'
lokc: false
EOF
expect_fail "$tmp/unknown.yaml" "unknown config key"

write_cfg nested.yaml <<'EOF'
cache_path: /var/cache/fgdb
  extra: true
EOF
expect_fail "$tmp/nested.yaml" "indented"

write_cfg flow.yaml <<'EOF'
allowed_filesystems: [ext4, xfs]
EOF
expect_fail "$tmp/flow.yaml" "list"

write_cfg block.yaml <<'EOF'
cache_path: |
  /var/cache/fgdb
EOF
expect_fail "$tmp/block.yaml" "plain key: value"

write_cfg dup.yaml <<'EOF'
local_cpu: 2
local_cpu: 4
EOF
expect_fail "$tmp/dup.yaml" "duplicate key"

write_cfg cpu0.yaml <<'EOF'
local_cpu: 0
EOF
expect_fail "$tmp/cpu0.yaml" "local_cpu"

write_cfg cpuhi.yaml <<'EOF'
local_cpu: 1025
EOF
expect_fail "$tmp/cpuhi.yaml" "local_cpu"

write_cfg ram.yaml <<'EOF'
local_ram_mb: 0
EOF
expect_fail "$tmp/ram.yaml" "local_ram_mb"

write_cfg quota.yaml <<'EOF'
cpu_quota: lots
EOF
expect_fail "$tmp/quota.yaml" "cpu_quota"

write_cfg mem.yaml <<'EOF'
memory_high: 80GB
EOF
expect_fail "$tmp/mem.yaml" "memory_high"

write_cfg ver.yaml <<'EOF'
runner_version: latest
EOF
expect_fail "$tmp/ver.yaml" "runner_version"

write_cfg bool.yaml <<'EOF'
auto_update: yes
EOF
expect_fail "$tmp/bool.yaml" "auto_update"

write_cfg bool2.yaml <<'EOF'
auto_update: FALSE
EOF
expect_fail "$tmp/bool2.yaml" "auto_update"

write_cfg empty.yaml <<'EOF'
allowed_filesystems:
local_cpu: 4
EOF
expect_fail "$tmp/empty.yaml" "at least one"

printf 'local_cpu:\t4\n' >"$tmp/tabs.yaml"
expect_fail "$tmp/tabs.yaml" "tabs"

write_cfg scalarfs.yaml <<'EOF'
allowed_filesystems: ext4
EOF
expect_fail "$tmp/scalarfs.yaml" "must be a list"

printf 'read-config tests passed\n'
