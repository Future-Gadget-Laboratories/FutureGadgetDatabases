#!/usr/bin/env bash
set -euo pipefail

die() {
  local message=$1 code=${2:-1}
  printf 'error: %s\n' "$message" >&2
  exit "$code"
}

allow=(ext4 xfs btrfs zfs)
while [[ $# -gt 0 && "$1" == --allow-fstype ]]; do
  [[ $# -ge 2 ]] || die "--allow-fstype needs a value" 2
  allow=("$2")
  shift 2
done
[[ $# -ge 1 ]] || die "usage: check-cache-dir.sh [--allow-fstype TYPE] DIR [MIN_FREE_GIB]" 2
dir=$1
min_gib=${2:-150}
[[ "$min_gib" =~ ^[0-9]+$ ]] || die "minimum free GiB must be an integer" 2

probe=$dir
while [[ ! -e "$probe" && "$probe" != "/" ]]; do
  probe=$(dirname "$probe")
done
probe=$(realpath -m "$probe")

mount_line=$(findmnt -n -T "$probe" -o TARGET,SOURCE,FSTYPE,OPTIONS) ||
  die "findmnt could not describe $probe" 10
read -r mount_target mount_source mount_type mount_options <<<"$mount_line"
[[ -n "$mount_type" ]] || die "could not parse findmnt output for $probe" 10

stat_type=$(stat -f -c %T "$probe") || die "stat could not describe $probe" 10
[[ "$stat_type" == "$mount_type" ]] ||
  die "findmnt reports $mount_type but stat reports $stat_type for $probe" 10

allowed=0
for type in "${allow[@]}"; do
  [[ "$type" == "$mount_type" ]] && allowed=1
done
(( allowed == 1 )) || die "filesystem $mount_type at $mount_target is not in the local allowlist" 10
[[ ",$mount_options," != *,ro,* ]] || die "$mount_target is read-only" 13

read -r _ _ free_bytes _ _ < <(df -P -B1 "$probe" | awk 'NR == 2')
[[ "$free_bytes" =~ ^[0-9]+$ ]] || die "could not read free bytes for $probe" 11
min_bytes=$((min_gib * 1024 * 1024 * 1024))
(( free_bytes >= min_bytes )) || die "$probe has only $((free_bytes / 1024 / 1024 / 1024)) GiB free; need ${min_gib} GiB" 11

read -r _ inode_total inode_used free_inodes _ _ < <(df -Pi "$probe" | awk 'NR == 2')
[[ "$free_inodes" =~ ^[0-9]+$ && "$inode_total" =~ ^[0-9]+$ ]] ||
  die "could not read free inodes for $probe" 12
min_inodes=$((inode_total / 20))
(( min_inodes > 5000000 )) || min_inodes=5000000
(( free_inodes >= min_inodes )) || die "$probe has too few free inodes" 12

printf 'cache ok: path=%s mount=%s source=%s type=%s free=%s inodes=%s\n' \
  "$dir" "$mount_target" "$mount_source" "$mount_type" "$free_bytes" "$free_inodes"
