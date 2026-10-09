#!/usr/bin/env bash

trim_registration_token() {
  local value=$1
  value=${value#"${value%%[![:space:]]*}"}
  value=${value%"${value##*[![:space:]]}"}
  printf '%s' "$value"
}

read_registration_token_file() {
  local path=$1 mode
  [[ -f "$path" && ! -L "$path" ]] || {
    printf 'error: token file must be a regular, non-symlink file\n' >&2
    return 1
  }
  [[ "$(stat -c '%u' "$path")" == "0" ]] || {
    printf 'error: token file must be owned by root\n' >&2
    return 1
  }
  mode=$(stat -c '%a' "$path")
  [[ "$mode" == "400" || "$mode" == "600" ]] || {
    printf 'error: token file must have mode 0400 or 0600\n' >&2
    return 1
  }
  # shellcheck disable=SC2034
  IFS= read -r REPLY_TOKEN <"$path" || true
  REPLY_TOKEN=$(trim_registration_token "$REPLY_TOKEN")
  if [[ -z "$REPLY_TOKEN" ]]; then
    printf 'error: the registration token is empty\n' >&2
    return 1
  fi
}

read_registration_token_stdin() {
  if [[ -t 0 ]]; then
    # shellcheck disable=SC2034
    IFS= read -rs REPLY_TOKEN || true
    printf '\n' >&2
  else
    # shellcheck disable=SC2034
    IFS= read -r REPLY_TOKEN || true
  fi
  REPLY_TOKEN=$(trim_registration_token "$REPLY_TOKEN")
  if [[ -z "$REPLY_TOKEN" ]]; then
    printf 'error: the registration token is empty\n' >&2
    return 1
  fi
}

registration_token_required_error() {
  printf '%s' "a registration token is required. Pass --token-file or --token-stdin, or set GH_RUNNER_REGISTRATION_TOKEN. The --token flag still works, but the shell keeps it in history and other people can see it in the process list. You do not need a token on a later run once ${1}/.runner exists."
}
