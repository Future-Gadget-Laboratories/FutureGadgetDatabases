#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/cache" "$tmp/tools"

# findmnt -no FSTYPE --target prints only the type. On ext4 that is "ext4".
# stat -f -c %T prints "ext2/ext3" for the same disk. A fake that prints
# "ext4" for stat hides the bug this test is here to catch.
cat >"$tmp/bin/findmnt" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FGDB_FINDMNT_FAIL:-}" == 1 ]]; then
  echo "findmnt failed" >&2
  exit 1
fi
if [[ "${1:-}" == "-no" && "${2:-}" == "FSTYPE" && "${3:-}" == "--target" ]]; then
  printf '%s\n' "${FGDB_FSTYPE:-ext4}"
  exit 0
fi
if [[ "${1:-}" == "-nP" && "${2:-}" == "-o" && "${3:-}" == "TARGET,SOURCE,OPTIONS" && "${4:-}" == "--target" ]]; then
  printf 'TARGET="%s" SOURCE="%s" OPTIONS="%s"\n' \
    "${FGDB_MOUNT_TARGET:-/}" \
    "${FGDB_MOUNT_SOURCE:-/dev/sda1}" \
    "${FGDB_MOUNT_OPTS:-rw,relatime}"
  exit 0
fi
printf 'unexpected findmnt args: %s\n' "$*" >&2
exit 1
EOF
cat >"$tmp/bin/stat" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-f" && "${2:-}" == "-c" && "${3:-}" == "%T" ]]; then
  case "${FGDB_FSTYPE:-ext4}" in
    ext2 | ext3 | ext4) printf 'ext2/ext3\n' ;;
    *) printf '%s\n' "${FGDB_FSTYPE}" ;;
  esac
  exit 0
fi
exec /usr/bin/stat "$@"
EOF
cat >"$tmp/bin/df" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-P" && "${2:-}" == "-B1" ]]; then
  printf '%s\n' 'Filesystem 1B-blocks Used Available Use% Mounted-on'
  printf '%s\n' "/dev/sda1 200000000000 1000000000 ${FGDB_FREE_BYTES:-199000000000} 1% /"
  exit 0
fi
if [[ "${1:-}" == "-Pi" ]]; then
  printf '%s\n' 'Filesystem Inodes IUsed IFree IUse% Mounted-on'
  printf '%s\n' "/dev/sda1 ${FGDB_INODE_TOTAL:-10000000} 100000 ${FGDB_FREE_INODES:-9900000} 1% /"
  exit 0
fi
printf 'unexpected df args: %s\n' "$*" >&2
exit 1
EOF
chmod +x "$tmp/bin/"*
ln -s /usr/bin/awk /usr/bin/realpath /usr/bin/bash "$tmp/tools/"

expect_ok() {
  local out err
  err=$(mktemp)
  if ! out=$(PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$@" 2>"$err"); then
    cat "$err" >&2
    rm -f "$err"
    printf 'expected success: %s\n' "$*" >&2
    exit 1
  fi
  printf '%s\n' "$out"
  cat "$err" >&2
  rm -f "$err"
}

expect_fail() {
  local err
  err=$(mktemp)
  if PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$@" >"$tmp/stdout" 2>"$err"; then
    printf 'expected failure: %s\n' "$*" >&2
    cat "$err" >&2
    rm -f "$err"
    exit 1
  fi
  cat "$err"
  rm -f "$err"
}

out=$(expect_ok "$tmp/cache" 1)
[[ "$out" == *"type=ext4"* ]]
[[ "$out" != *"ext2/ext3"* ]]

# The four flags setup-runner passes must all stick. Keeping only the last
# one (zfs) would reject this ext4 disk.
out=$(expect_ok --allow-fstype ext4 --allow-fstype xfs --allow-fstype btrfs --allow-fstype zfs "$tmp/cache" 1)
[[ "$out" == *"type=ext4"* ]]

expect_fail --allow-fstype zfs "$tmp/cache" 1 >/dev/null
FGDB_FSTYPE=zfs expect_ok --allow-fstype zfs "$tmp/cache" 1 >/dev/null
FGDB_FSTYPE=xfs expect_ok --allow-fstype ext4 --allow-fstype xfs "$tmp/cache" 1 >/dev/null
FGDB_FSTYPE=nfs expect_fail "$tmp/cache" 1 >/dev/null
FGDB_MOUNT_OPTS=ro,relatime expect_fail "$tmp/cache" 1 >/dev/null
FGDB_FREE_BYTES=1 expect_fail "$tmp/cache" 1 >/dev/null
FGDB_FREE_INODES=1 expect_fail "$tmp/cache" 1 >/dev/null

# btrfs reports inodes, but the count is not meaningful. Skip even when the
# free count would fail the normal test.
skip=$(FGDB_FSTYPE=btrfs FGDB_INODE_TOTAL=10000000 FGDB_FREE_INODES=1 \
  PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$tmp/cache" 1 2>&1)
[[ "$skip" == *"type=btrfs"* ]]
[[ "$skip" == *"skipping inode check"* ]]
[[ "$skip" == *"inodes=skipped"* ]]

skip=$(FGDB_FSTYPE=xfs FGDB_INODE_TOTAL=0 FGDB_FREE_INODES=0 \
  PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$tmp/cache" 1 2>&1)
[[ "$skip" == *"skipping inode check"* ]]
[[ "$skip" == *"reports 0 total inodes"* ]]

# findmnt says ext4. A contradictory mountinfo must not win, and stat's
# ext2/ext3 label must not either.
mkdir -p "$tmp/my disk/cache"
probe=$(realpath -m "$tmp/my disk")
escaped=${probe// /\\040}
{
  printf '1 0 8:0 / / rw,relatime - nfs4 /dev/nfs rw\n'
  printf '2 1 8:1 / %s rw,relatime - xfs /dev/sdb rw\n' "$escaped"
} >"$tmp/mountinfo"
out=$(FGDB_MOUNTINFO="$tmp/mountinfo" expect_ok "$tmp/my disk/cache" 1)
[[ "$out" == *"type=ext4"* ]]

# findmnt fails. The longest mountinfo prefix wins, including a space in the path.
out=$(FGDB_FINDMNT_FAIL=1 FGDB_MOUNTINFO="$tmp/mountinfo" \
  PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$tmp/my disk/cache" 1 2>"$tmp/err")
[[ "$out" == *"type=xfs"* ]]
[[ "$out" != *"type=nfs4"* ]]
grep -q 'findmnt did not describe' "$tmp/err"

# The checker runs under bash without the executable bit.
cp "$ROOT/check-cache-dir.sh" "$tmp/nocheck.sh"
chmod a-x "$tmp/nocheck.sh"
out=$(PATH="$tmp/bin:$PATH" bash "$tmp/nocheck.sh" "$tmp/cache" 1)
[[ "$out" == *"type=ext4"* ]]

# findmnt is not on PATH at all.
rm -f "$tmp/bin/findmnt"
out=$(FGDB_MOUNTINFO="$tmp/mountinfo" \
  PATH="$tmp/bin:$tmp/tools" "$ROOT/check-cache-dir.sh" "$tmp/my disk/cache" 1 2>"$tmp/err")
[[ "$out" == *"type=xfs"* ]]
grep -q 'findmnt did not describe' "$tmp/err"

if PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" >/dev/null 2>&1; then
  printf 'expected usage failure\n' >&2
  exit 1
fi

printf 'check-cache-dir tests passed\n'
