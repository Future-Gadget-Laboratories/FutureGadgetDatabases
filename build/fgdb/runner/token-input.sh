#!/usr/bin/env bash

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
  IFS= read -r REPLY_TOKEN <"$path" || true
}

read_registration_token_stdin() {
  IFS= read -r REPLY_TOKEN || true
}
