#!/usr/bin/env bash
# Allowlisted environment for candidate binaries and workload processes.
# Source this file, then start those programs with fgdb_exec.
# Names are the exact lines in pkg/fgdbtest/cluster/allowlist.txt.
# fgdb_exec uses env -i. A credential name on that list is refused.

_fgdb_script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
_fgdb_allow_file="$_fgdb_script_dir/../../../pkg/fgdbtest/cluster/allowlist.txt"

fgdb_deny_name() {
  case "$1" in
    *TOKEN* | *SECRET* | *PASSWORD* | ACTIONS_* | actions_*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

fgdb_exec() {
  local -a clean=()
  local -A seen=()
  local line item

  fgdb_take() {
    local name=$1
    if [[ -n ${seen[$name]+x} ]]; then
      return 0
    fi
    seen[$name]=1
    if fgdb_deny_name "$name"; then
      echo "refusing environment variable $name" >&2
      return 1
    fi
    if [[ -n ${!name+x} ]]; then
      clean+=("${name}=${!name}")
    fi
  }

  if [[ ! -f "$_fgdb_allow_file" ]]; then
    echo "allowlist not found: $_fgdb_allow_file" >&2
    return 1
  fi
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      '' | \#*) continue ;;
    esac
    case "$line" in
      *'*'* | *'?'* | *'='*)
        echo "allowlist entry is not an exact name: $line" >&2
        return 1
        ;;
    esac
    fgdb_take "$line" || return 1
  done < "$_fgdb_allow_file"
  for item in "${clean[@]}"; do
    if fgdb_deny_name "${item%%=*}"; then
      echo "refusing environment variable ${item%%=*}" >&2
      return 1
    fi
  done
  if [[ ${#clean[@]} -eq 0 ]]; then
    echo "refusing to run with an empty environment" >&2
    return 1
  fi
  env -i "${clean[@]}" "$@"
}
