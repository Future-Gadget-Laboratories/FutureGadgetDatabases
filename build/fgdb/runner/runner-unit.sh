#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

# Helpers for the cache check command written into the runner service.

config_fail() {
  if declare -F die >/dev/null 2>&1; then
    die "$@"
  fi
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_cache_check_script() {
  local path=$1
  if [[ ! -f "$path" ]]; then
    config_fail "cache check script is missing: ${path}"
  fi
  if [[ ! -r "$path" ]]; then
    config_fail "cache check script cannot be read: ${path}"
  fi
  if ! bash -n "$path"; then
    config_fail "cache check script cannot be run: ${path}"
  fi
}

build_cache_check_args() {
  local dir=$1 min_gib=$2 fstype
  CACHE_CHECK_ARGS=()
  (( ${#ALLOWED_FILESYSTEMS[@]} > 0 )) || config_fail "the filesystem allowlist is empty"
  for fstype in "${ALLOWED_FILESYSTEMS[@]}"; do
    [[ "$fstype" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]] ||
      config_fail "filesystem type is not a single token: ${fstype}"
    CACHE_CHECK_ARGS+=(--allow-fstype "$fstype")
  done
  CACHE_CHECK_ARGS+=("$dir" "$min_gib")
}

build_exec_start_pre() {
  local output_base=$1 min_gib=$2 fstype
  local line="/usr/bin/bash /usr/local/lib/fgdb/check-cache-dir.sh"
  [[ "$output_base" == /* ]] || config_fail "cache directory must be an absolute path"
  [[ "$output_base" != *[[:space:]]* ]] || config_fail "cache directory cannot contain whitespace"
  [[ "$min_gib" =~ ^[0-9]+$ ]] || config_fail "minimum free GiB must be an integer"
  (( ${#ALLOWED_FILESYSTEMS[@]} > 0 )) || config_fail "the filesystem allowlist is empty"
  for fstype in "${ALLOWED_FILESYSTEMS[@]}"; do
    [[ "$fstype" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]] ||
      config_fail "filesystem type is not a single token: ${fstype}"
    line+=" --allow-fstype ${fstype}"
  done
  line+=" ${output_base} ${min_gib}"
  printf '%s\n' "$line"
}
