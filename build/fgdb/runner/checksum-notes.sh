#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

# Pull published checksums out of upstream release text.
# Actions Runner v2 notes wrap each hash as:
#   <!-- BEGIN SHA linux-x64 -->HASH<!-- END SHA linux-x64 -->
# Bazelisk publishes a one-line bazelisk-linux-amd64.sha256 file.

runner_linux_x64_sha() {
  local notes=$1 filename=$2 line sha
  notes=${notes//$'\r'/}
  line=$(printf '%s\n' "$notes" | awk -v file="$filename" '
    index($0, file) && index($0, "BEGIN SHA linux-x64") { print; exit }
  ')
  [[ -n "$line" ]] || return 1
  [[ "$line" =~ BEGIN\ SHA\ linux-x64\ --\>([0-9a-fA-F]{64})\<\!--\ END\ SHA\ linux-x64 ]] || return 1
  sha=${BASH_REMATCH[1]}
  printf '%s\n' "${sha,,}"
}

bazelisk_sidecar_sha() {
  local text=$1 line sha
  text=${text//$'\r'/}
  line=$(printf '%s\n' "$text" | awk 'NF { print; exit }')
  [[ -n "$line" ]] || return 1
  read -r sha _ <<<"$line"
  [[ "$sha" =~ ^[0-9a-fA-F]{64}$ ]] || return 1
  printf '%s\n' "${sha,,}"
}
