#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

# Read the flat setup file accepted by setup-runner.sh.
# See docs/fgdb/runner-config.md for the supported lines.

config_fail() {
  if declare -F die >/dev/null 2>&1; then
    die "$@"
  fi
  printf 'error: %s\n' "$*" >&2
  exit 1
}

runner_config_known_key() {
  case "$1" in
    auto_update | cache_path | allowed_filesystems | local_cpu | local_ram_mb | \
      cpu_quota | memory_high | memory_max | runner_version | bazelisk_version)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

yaml_scalar() {
  local raw=$1 lineno=$2 value rest
  raw=${raw#"${raw%%[![:space:]]*}"}
  if [[ -z "$raw" || "$raw" == "#"* ]]; then
    config_fail "config file line ${lineno}: missing value"
  fi
  if [[ "$raw" == '"'* ]]; then
    raw=${raw:1}
    [[ "$raw" == *'"'* ]] || config_fail "config file line ${lineno}: missing closing double quote"
    value=${raw%%\"*}
    rest=${raw#*\"}
    [[ "$rest" =~ ^[[:space:]]*(#.*)?$ ]] ||
      config_fail "config file line ${lineno}: extra text after the quoted value"
    [[ "$value" != *\\* ]] ||
      config_fail "config file line ${lineno}: backslash escapes are not supported"
    printf '%s' "$value"
    return 0
  fi
  if [[ "$raw" == "'"* ]]; then
    raw=${raw:1}
    [[ "$raw" == *"'"* ]] || config_fail "config file line ${lineno}: missing closing single quote"
    value=${raw%%\'*}
    rest=${raw#*\'}
    [[ "$rest" =~ ^[[:space:]]*(#.*)?$ ]] ||
      config_fail "config file line ${lineno}: extra text after the quoted value"
    printf '%s' "$value"
    return 0
  fi
  if [[ "$raw" == *" #"* ]]; then
    raw=${raw%% #*}
  fi
  raw=${raw%"${raw##*[![:space:]]}"}
  [[ -n "$raw" ]] || config_fail "config file line ${lineno}: missing value"
  case "$raw" in
    "|" | "|-" | "|+" | ">" | ">-" | ">+" | "["* | "{"* | "!"* | "&"* | "*"*)
      config_fail "config file line ${lineno}: this file only supports plain key: value lines and the allowed_filesystems list"
      ;;
  esac
  printf '%s' "$raw"
}

require_int_range() {
  local value=$1 min=$2 max=$3 label=$4 lineno=$5
  [[ "$value" =~ ^[1-9][0-9]*$ ]] ||
    config_fail "config file line ${lineno}: ${label} must be a whole number from ${min} to ${max}"
  if (( value < min || value > max )); then
    config_fail "config file line ${lineno}: ${label} must be from ${min} to ${max}"
  fi
}

validate_runner_value() {
  local key=$1 value=$2 lineno=$3
  case "$key" in
    auto_update)
      [[ "$value" == true || "$value" == false ]] ||
        config_fail "config file line ${lineno}: auto_update must be true or false"
      ;;
    cache_path)
      [[ "$value" =~ ^/[^[:space:][:cntrl:]#\"\']+$ ]] ||
        config_fail "config file line ${lineno}: cache_path must be an absolute path without spaces"
      ;;
    local_cpu)
      require_int_range "$value" 1 1024 "local_cpu" "$lineno"
      ;;
    local_ram_mb)
      require_int_range "$value" 1 4194304 "local_ram_mb" "$lineno"
      ;;
    cpu_quota)
      [[ "$value" =~ ^[1-9][0-9]{0,6}%$ ]] ||
        config_fail "config file line ${lineno}: cpu_quota must be a whole number followed by % (for example 2400%)"
      ;;
    memory_high | memory_max)
      [[ "$value" =~ ^[1-9][0-9]{0,12}([KMGT]|[KMGT]i)?$ ]] ||
        config_fail "config file line ${lineno}: ${key} must be a size such as 80G, 512M, or 81920"
      ;;
    runner_version | bazelisk_version)
      [[ "$value" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
        config_fail "config file line ${lineno}: ${key} must look like 2.338.0"
      ;;
    *)
      config_fail "config file line ${lineno}: unknown config key: ${key}"
      ;;
  esac
}

validate_fstype_name() {
  local value=$1 lineno=$2
  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]] ||
    config_fail "config file line ${lineno}: filesystem type must be a single token such as ext4"
  (( ${#value} <= 64 )) ||
    config_fail "config file line ${lineno}: filesystem type is too long"
}

parse_runner_config() {
  local file=$1 line lineno=0 key rest value list_key="" list_start=0
  [[ -f "$file" ]] || config_fail "config file does not exist: ${file}"
  RC_has_auto_update=0
  RC_auto_update=""
  RC_has_cache_path=0
  RC_cache_path=""
  RC_has_allowed_filesystems=0
  RC_allowed_filesystems=()
  RC_has_local_cpu=0
  RC_local_cpu=""
  RC_has_local_ram_mb=0
  RC_local_ram_mb=""
  RC_has_cpu_quota=0
  RC_cpu_quota=""
  RC_has_memory_high=0
  RC_memory_high=""
  RC_has_memory_max=0
  RC_memory_max=""
  RC_has_runner_version=0
  RC_runner_version=""
  RC_has_bazelisk_version=0
  RC_bazelisk_version=""
  unset RC_SEEN
  declare -gA RC_SEEN=()

  finish_list() {
    if [[ -n "$list_key" ]]; then
      (( ${#RC_allowed_filesystems[@]} > 0 )) ||
        config_fail "config file line ${list_start}: allowed_filesystems needs at least one filesystem type"
      list_key=""
    fi
  }

  remember_key() {
    local seen_key=$1 seen_line=$2
    if [[ -n "${RC_SEEN[$seen_key]:-}" ]]; then
      config_fail "config file line ${seen_line}: duplicate key: ${seen_key}"
    fi
    RC_SEEN[$seen_key]=1
  }

  take_filesystem() {
    local item lineno_item=$1
    item=$(yaml_scalar "$2" "$lineno_item")
    validate_fstype_name "$item" "$lineno_item"
    if (( ${#RC_allowed_filesystems[@]} >= 32 )); then
      config_fail "config file line ${lineno_item}: allowed_filesystems accepts at most 32 types"
    fi
    RC_allowed_filesystems+=("$item")
    RC_has_allowed_filesystems=1
  }

  parse_key_line() {
    local raw_line=$1
    if [[ "$raw_line" == "---" || "$raw_line" == "..." ]]; then
      config_fail "config file line ${lineno}: this file does not support YAML document markers"
    fi
    [[ "$raw_line" =~ ^([A-Za-z][A-Za-z0-9_]*):(.*)$ ]] ||
      config_fail "config file line ${lineno}: expected a key: value line"
    key=${BASH_REMATCH[1]}
    rest=${BASH_REMATCH[2]}
    runner_config_known_key "$key" ||
      config_fail "config file line ${lineno}: unknown config key: ${key}"
    remember_key "$key" "$lineno"
    local rest_trim=${rest#"${rest%%[![:space:]]*}"}
    if [[ -z "$rest_trim" || "$rest_trim" == "#"* ]]; then
      if [[ "$key" == allowed_filesystems ]]; then
        list_key=$key
        list_start=$lineno
        return 0
      fi
      config_fail "config file line ${lineno}: missing value for ${key}"
    fi
    if [[ "$key" == allowed_filesystems ]]; then
      config_fail "config file line ${lineno}: allowed_filesystems must be a list of - items, not a single value"
    fi
    value=$(yaml_scalar "$rest" "$lineno")
    validate_runner_value "$key" "$value" "$lineno"
    case "$key" in
      auto_update)
        RC_has_auto_update=1
        RC_auto_update=$value
        ;;
      cache_path)
        RC_has_cache_path=1
        RC_cache_path=$value
        ;;
      local_cpu)
        RC_has_local_cpu=1
        RC_local_cpu=$value
        ;;
      local_ram_mb)
        RC_has_local_ram_mb=1
        RC_local_ram_mb=$value
        ;;
      cpu_quota)
        RC_has_cpu_quota=1
        RC_cpu_quota=$value
        ;;
      memory_high)
        RC_has_memory_high=1
        RC_memory_high=$value
        ;;
      memory_max)
        RC_has_memory_max=1
        RC_memory_max=$value
        ;;
      runner_version)
        RC_has_runner_version=1
        RC_runner_version=$value
        ;;
      bazelisk_version)
        RC_has_bazelisk_version=1
        RC_bazelisk_version=$value
        ;;
    esac
  }

  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$((lineno + 1))
    line=${line%$'\r'}
    [[ "$line" != *$'\t'* ]] ||
      config_fail "config file line ${lineno}: tabs are not supported; use spaces"
    [[ "$line" =~ ^[[:space:]]*$ || "$line" =~ ^[[:space:]]*# ]] && continue
    if [[ "$line" =~ ^[[:space:]]+-[[:space:]]*$ ]]; then
      config_fail "config file line ${lineno}: empty list item"
    fi
    if [[ "$line" =~ ^[[:space:]]+-(.*)$ && ! "$line" =~ ^[[:space:]]+-[[:space:]] ]]; then
      config_fail "config file line ${lineno}: put a space after the dash in a list item"
    fi
    if [[ "$line" =~ ^[[:space:]]+-[[:space:]]+(.*)$ ]]; then
      [[ -n "$list_key" ]] ||
        config_fail "config file line ${lineno}: list items are only allowed under allowed_filesystems"
      take_filesystem "$lineno" "${BASH_REMATCH[1]}"
      continue
    fi
    if [[ "$line" =~ ^[[:space:]] ]]; then
      config_fail "config file line ${lineno}: only allowed_filesystems list items may be indented"
    fi
    finish_list
    parse_key_line "$line"
  done <"$file"
  finish_list
}

# Copy parsed values onto the setup-runner variables. Call this after the
# environment defaults are set and before command-line flags are applied.
# shellcheck disable=SC2034
apply_runner_config() {
  if [[ "$RC_has_cache_path" -eq 1 ]]; then
    CACHE_DIR_OVERRIDE=$RC_cache_path
  fi
  if [[ "$RC_has_local_cpu" -eq 1 ]]; then
    LOCAL_CPU=$RC_local_cpu
  fi
  if [[ "$RC_has_local_ram_mb" -eq 1 ]]; then
    LOCAL_RAM_MB=$RC_local_ram_mb
  fi
  if [[ "$RC_has_cpu_quota" -eq 1 ]]; then
    CPU_QUOTA=$RC_cpu_quota
  fi
  if [[ "$RC_has_memory_high" -eq 1 ]]; then
    MEMORY_HIGH=$RC_memory_high
  fi
  if [[ "$RC_has_memory_max" -eq 1 ]]; then
    MEMORY_MAX=$RC_memory_max
  fi
  if [[ "$RC_has_runner_version" -eq 1 ]]; then
    RUNNER_VERSION=$RC_runner_version
  fi
  if [[ "$RC_has_bazelisk_version" -eq 1 ]]; then
    BAZELISK_VERSION=$RC_bazelisk_version
  fi
  if [[ "$RC_has_auto_update" -eq 1 ]]; then
    if [[ "$RC_auto_update" == true ]]; then
      AUTO_UPDATE=1
    else
      AUTO_UPDATE=0
    fi
  fi
  if [[ "$RC_has_allowed_filesystems" -eq 1 ]]; then
    ALLOWED_FILESYSTEMS=("${RC_allowed_filesystems[@]}")
  fi
}
