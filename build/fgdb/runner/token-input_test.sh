#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf 'token-from-file\n' >"$tmp/token"
chmod 600 "$tmp/token"
mkdir "$tmp/bin"
cat >"$tmp/bin/stat" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"%u"*) printf '0\n' ;;
  *"%a"*) printf '600\n' ;;
esac
EOF
chmod +x "$tmp/bin/stat"
# shellcheck source=token-input.sh
# shellcheck disable=SC1091
source "$ROOT/token-input.sh"
PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/token"
[[ "$REPLY_TOKEN" == token-from-file ]]

printf 'token-from-file\r\n' >"$tmp/cr"
PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/cr"
[[ "$REPLY_TOKEN" == token-from-file ]]

printf '  token-from-file \r\n' >"$tmp/padded"
PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/padded"
[[ "$REPLY_TOKEN" == token-from-file ]]

printf '\r\n' >"$tmp/empty"
if PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/empty"; then
  printf 'empty token was accepted\n' >&2
  exit 1
fi

PATH="$tmp/bin:$PATH" read_registration_token_stdin <<<'token-from-stdin'
[[ "$REPLY_TOKEN" == token-from-stdin ]]
PATH="$tmp/bin:$PATH" read_registration_token_stdin <<<"token-from-stdin"$'\r'
[[ "$REPLY_TOKEN" == token-from-stdin ]]

ln -s "$tmp/token" "$tmp/link"
if PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/link"; then
  printf 'symlink token was accepted\n' >&2
  exit 1
fi

msg=$(registration_token_required_error /opt/fgdb-actions-runner)
[[ "$msg" == *"--token-file"* ]]
[[ "$msg" == *"--token-stdin"* ]]
[[ "$msg" == *"GH_RUNNER_REGISTRATION_TOKEN"* ]]
[[ "$msg" != *"pass --token or set GH_RUNNER_REGISTRATION_TOKEN"* ]]
grep -q 'registration_token_required_error' "$ROOT/setup-runner.sh"

python3 - "$ROOT/token-input.sh" <<'PY'
import os, select, sys, time

script = sys.argv[1]
master, slave = os.openpty()
pid = os.fork()
if pid == 0:
    os.setsid()
    os.dup2(slave, 0)
    os.dup2(slave, 1)
    os.dup2(slave, 2)
    os.close(master)
    os.close(slave)
    os.execv(
        "/bin/bash",
        [
            "bash",
            "-c",
            'source "$1"; printf "READY\\n"; read_registration_token_stdin; printf "GOT=%s\\n" "$REPLY_TOKEN"',
            "bash",
            script,
        ],
    )
os.close(slave)
buf = b""
deadline = time.time() + 5
while time.time() < deadline and b"READY" not in buf:
    ready, _, _ = select.select([master], [], [], 0.2)
    if ready:
        buf += os.read(master, 4096)
os.write(master, b"secret-token\n")
deadline = time.time() + 5
while time.time() < deadline and b"GOT=" not in buf:
    ready, _, _ = select.select([master], [], [], 0.2)
    if ready:
        chunk = os.read(master, 4096)
        if not chunk:
            break
        buf += chunk
_, status = os.waitpid(pid, 0)
text = buf.decode("utf-8", "replace")
if status != 0 or text.count("secret-token") != 1 or "GOT=secret-token" not in text:
    sys.stderr.write(text + "\n")
    sys.exit(1)
PY

[[ ! -x "$ROOT/token-input.sh" ]]
printf 'token input tests passed\n'
