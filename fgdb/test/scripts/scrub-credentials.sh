#!/usr/bin/env bash
# Allowlisted environment for candidate binaries and workload processes.
# Source this file, then start those programs with fgdb_exec.
# fgdb_exec uses env -i. A credential name or an Actions variable is refused
# even if an allowlist rule would have copied it.

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
  local key item

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

  for key in PATH HOME TMPDIR TEMP TMP LANG LC_ALL LC_CTYPE LANGUAGE \
    USER LOGNAME SHELL TZ LD_LIBRARY_PATH PWD \
    CGO_ENABLED CC CXX PKG_CONFIG_PATH \
    SSL_CERT_FILE SSL_CERT_DIR \
    HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
    fgdb_take "$key" || return 1
  done
  while IFS= read -r key; do
    case "$key" in
      FGDB_* | LC_* | GO*)
        case "$key" in
          GOOGLE*) ;;
          *) fgdb_take "$key" || return 1 ;;
        esac
        ;;
    esac
  done < <(compgen -e)
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
