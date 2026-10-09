#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/cache"

cat >"$tmp/bin/findmnt" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${FGDB_FINDMNT:-/dev/sda /dev/sda ext4 rw}"
EOF
cat >"$tmp/bin/stat" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == "-f -c %T "* ]]; then
  printf '%s\n' "${FGDB_STAT_TYPE:-ext4}"
else
  /usr/bin/stat "$@"
fi
EOF
cat >"$tmp/bin/df" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "-P" ]]; then
  printf '%s\n' 'Filesystem 1024-blocks Used Available Capacity Mounted on'
  printf '%s\n' "/dev/sda 200000000000 1000000000 ${FGDB_FREE_BYTES:-199000000000} 1% /"
else
  printf '%s\n' 'Filesystem Inodes IUsed IFree IUse% Mounted on'
  printf '%s\n' "/dev/sda 10000000 100000 ${FGDB_FREE_INODES:-9900000} 1% /"
fi
EOF
chmod +x "$tmp/bin/"*

run_ok() {
  PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$tmp/cache" 1 >/dev/null
}

run_fail() {
  if PATH="$tmp/bin:$PATH" "$ROOT/check-cache-dir.sh" "$tmp/cache" 1 >/dev/null 2>&1; then
    exit 1
  fi
}

run_ok
FGDB_FINDMNT='/dev/nfs /dev/nfs nfs rw' run_fail
FGDB_FINDMNT='/dev/sda /dev/sda ext4 ro' run_fail
FGDB_FREE_INODES=1 run_fail
FGDB_STAT_TYPE=xfs run_fail
printf 'check-cache-dir tests passed\n'
