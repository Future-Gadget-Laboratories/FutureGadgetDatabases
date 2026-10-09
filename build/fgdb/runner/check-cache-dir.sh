#!/usr/bin/env bash
# Confirm a cache directory is on an allowed local filesystem and has room.
#
# The filesystem type comes from `findmnt -no FSTYPE --target`. Do not use
# `stat -f -c %T` for this: on ext4 that command prints ext2/ext3.
# If findmnt cannot answer, the script reads /proc/self/mountinfo
# (tests may point FGDB_MOUNTINFO at a fixture).

set -euo pipefail

die() {
  local message=$1 code=${2:-1}
  printf 'error: %s\n' "$message" >&2
  exit "$code"
}

# No --allow-fstype flags: ext4, xfs, btrfs, and zfs.
# The first --allow-fstype replaces that list. Each later flag adds a type.
allow=()
explicit_allow=0
while [[ $# -gt 0 && "$1" == --allow-fstype ]]; do
  [[ $# -ge 2 ]] || die "--allow-fstype needs a value" 2
  [[ "$2" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]] || die "filesystem type is not a single token: $2" 2
  if (( explicit_allow == 0 )); then
    allow=()
    explicit_allow=1
  fi
  allow+=("$2")
  shift 2
done
if (( explicit_allow == 0 )); then
  allow=(ext4 xfs btrfs zfs)
fi
[[ $# -ge 1 && $# -le 2 ]] || die "usage: check-cache-dir.sh [--allow-fstype TYPE ...] DIR [MIN_FREE_GIB]" 2
dir=$1
min_gib=${2:-150}
[[ "$min_gib" =~ ^[0-9]+$ ]] || die "minimum free GiB must be an integer" 2

probe=$dir
while [[ ! -e "$probe" && "$probe" != "/" ]]; do
  probe=$(dirname "$probe")
done
probe=$(realpath -m "$probe")

unescape_mount_field() {
  local rest=$1 out="" chunk
  while [[ "$rest" == *\\* ]]; do
    chunk=${rest%%\\*}
    out+=$chunk
    rest=${rest#*\\}
    if [[ "$rest" == 040* ]]; then
      out+=" "
      rest=${rest:3}
    elif [[ "$rest" == 011* ]]; then
      out+=$'\t'
      rest=${rest:3}
    elif [[ "$rest" == 012* ]]; then
      out+=$'\n'
      rest=${rest:3}
    elif [[ "$rest" == 134* ]]; then
      out+="\\"
      rest=${rest:3}
    else
      out+="\\"
      out+=${rest:0:1}
      rest=${rest:1}
    fi
  done
  printf '%s' "${out}${rest}"
}

mount_covers() {
  local path=$1 mnt=$2
  [[ "$mnt" == "/" || "$path" == "$mnt" || "$path" == "$mnt"/* ]]
}

load_mountinfo() {
  local path=$1
  local info=${FGDB_MOUNTINFO:-/proc/self/mountinfo}
  local line left right mnt rest opts fstype source
  local best_len=-1 found=0
  MI_TARGET=""
  MI_SOURCE=""
  MI_FSTYPE=""
  MI_OPTIONS=""
  [[ -r "$info" ]] || return 1
  while IFS= read -r line || [[ -n "$line" ]]; do
    line=${line%$'\r'}
    [[ "$line" == *" - "* ]] || continue
    left=${line%% - *}
    right=${line#* - }
    read -r _ _ _ _ mnt rest <<<"$left" || continue
    read -r opts _ <<<"$rest" || continue
    read -r fstype source _ <<<"$right" || continue
    mnt=$(unescape_mount_field "$mnt")
    source=$(unescape_mount_field "$source")
    if mount_covers "$path" "$mnt" && (( ${#mnt} > best_len )); then
      best_len=${#mnt}
      MI_TARGET=$mnt
      MI_SOURCE=$source
      MI_FSTYPE=$fstype
      MI_OPTIONS=$opts
      found=1
    fi
  done <"$info"
  (( found == 1 ))
}

parse_findmnt_p() {
  local line=$1
  [[ "$line" =~ TARGET=\"([^\"]*)\"[[:space:]]SOURCE=\"([^\"]*)\"[[:space:]]OPTIONS=\"([^\"]*)\" ]] || return 1
  MOUNT_TARGET=${BASH_REMATCH[1]}
  MOUNT_SOURCE=${BASH_REMATCH[2]}
  MOUNT_OPTIONS=${BASH_REMATCH[3]}
}

describe_mount() {
  local path=$1 fstype="" pairs=""
  MOUNT_TYPE=""
  MOUNT_TARGET=""
  MOUNT_SOURCE=""
  MOUNT_OPTIONS=""
  if command -v findmnt >/dev/null 2>&1 \
    && fstype=$(findmnt -no FSTYPE --target "$path" 2>/dev/null) \
    && [[ "$fstype" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]]; then
    MOUNT_TYPE=$fstype
    if pairs=$(findmnt -nP -o TARGET,SOURCE,OPTIONS --target "$path" 2>/dev/null) \
      && parse_findmnt_p "$pairs"; then
      return 0
    fi
  else
    fstype=""
  fi
  if load_mountinfo "$path"; then
    if [[ -n "$fstype" ]]; then
      printf 'note: findmnt reported %s for %s; mount options came from %s\n' \
        "$fstype" "$path" "${FGDB_MOUNTINFO:-/proc/self/mountinfo}" >&2
      MOUNT_TYPE=$fstype
    else
      printf 'note: findmnt did not describe %s; using %s\n' \
        "$path" "${FGDB_MOUNTINFO:-/proc/self/mountinfo}" >&2
      MOUNT_TYPE=$MI_FSTYPE
    fi
    MOUNT_TARGET=$MI_TARGET
    MOUNT_SOURCE=$MI_SOURCE
    MOUNT_OPTIONS=$MI_OPTIONS
    [[ -n "$MOUNT_TYPE" && -n "$MOUNT_TARGET" && -n "$MOUNT_OPTIONS" ]]
    return
  fi
  return 1
}

describe_mount "$probe" || die "could not tell which filesystem holds $probe" 10

allowed=0
for type in "${allow[@]}"; do
  [[ "$type" == "$MOUNT_TYPE" ]] && allowed=1
done
(( allowed == 1 )) || die "filesystem $MOUNT_TYPE at $MOUNT_TARGET is not in the local allowlist" 10
[[ ",${MOUNT_OPTIONS}," != *,ro,* ]] || die "$MOUNT_TARGET is read-only" 13

df_line=$(df -P -B1 "$probe" | awk 'NR == 2') || die "could not read free bytes for $probe" 11
read -r _ _ _ free_bytes _ _ <<<"$df_line"
[[ "$free_bytes" =~ ^[0-9]+$ ]] || die "could not read free bytes for $probe" 11
min_bytes=$((min_gib * 1024 * 1024 * 1024))
(( free_bytes >= min_bytes )) || die "$probe has only $((free_bytes / 1024 / 1024 / 1024)) GiB free; need ${min_gib} GiB" 11

inode_total=""
free_inodes=""
if df_inode=$(df -Pi "$probe" 2>/dev/null | awk 'NR == 2'); then
  read -r _ inode_total _ free_inodes _ _ <<<"$df_inode"
fi
inode_report=$free_inodes
if [[ "$MOUNT_TYPE" == btrfs || "$inode_total" == "0" ]]; then
  printf 'note: skipping inode check for %s because %s reports %s total inodes\n' \
    "$dir" "$MOUNT_TYPE" "${inode_total:-unknown}" >&2
  inode_report=skipped
else
  [[ "$free_inodes" =~ ^[0-9]+$ && "$inode_total" =~ ^[0-9]+$ ]] ||
    die "could not read free inodes for $probe" 12
  min_inodes=$((inode_total / 20))
  (( min_inodes > 5000000 )) || min_inodes=5000000
  (( free_inodes >= min_inodes )) || die "$probe has too few free inodes" 12
fi

printf 'cache ok: path=%s mount=%s source=%s type=%s free=%s inodes=%s\n' \
  "$dir" "$MOUNT_TARGET" "$MOUNT_SOURCE" "$MOUNT_TYPE" "$free_bytes" "$inode_report"
