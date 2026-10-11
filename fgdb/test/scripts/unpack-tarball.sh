#!/usr/bin/env bash
# List a checksum-verified tarball, reject unsafe members, then extract it.
# Usage: unpack-tarball.sh ARCHIVE PARENT
# Members are written to stderr. The new directory is written to stdout.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=fgdb/test/scripts/scrub-credentials.sh
. "$here/scrub-credentials.sh"

if [[ "$(id -u)" -eq 0 ]]; then
  echo "refusing to extract a tarball as root" >&2
  exit 1
fi

if [[ $# -ne 2 ]]; then
  echo "usage: unpack-tarball.sh ARCHIVE PARENT" >&2
  exit 2
fi

archive=$1
parent=$2

if [[ ! -f "$archive" ]]; then
  echo "tarball not found: $archive" >&2
  exit 1
fi

python3 - "$archive" <<'PY'
import sys
import tarfile

path = sys.argv[1]
with tarfile.open(path, "r:*") as tf:
    members = tf.getmembers()
if not members:
    sys.exit("tarball has no members")
for member in members:
    print(f"member {member.name}", file=sys.stderr)

def reject(member):
    name = member.name
    if name.startswith("/") or name.startswith("\\"):
        sys.exit(f"absolute path: {name}")
    parts = name.replace("\\", "/").split("/")
    if ".." in parts:
        sys.exit(f"path traversal: {name}")
    if member.issym():
        sys.exit(f"symlink: {name}")
    if member.islnk():
        sys.exit(f"hardlink: {name}")
    if member.isdev() or member.ischr() or member.isblk():
        sys.exit(f"device file: {name}")
    if not (member.isfile() or member.isdir()):
        sys.exit(f"unsupported member: {name}")

for member in members:
    reject(member)
PY

dest=$(mktemp -d "${parent}/unpack.XXXXXX")
tar -xzf "$archive" -C "$dest" --no-same-owner --no-same-permissions
printf '%s\n' "$dest"
