#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# shellcheck source=checksum-notes.sh
# shellcheck disable=SC1091
source "$ROOT/checksum-notes.sh"

notes=$ROOT/testdata/actions-runner-v2.338.0-release-notes.txt
got=$(runner_linux_x64_sha "$(cat "$notes")" "actions-runner-linux-x64-2.338.0.tar.gz")
[[ "$got" == "af4b794c1bc41d73d40535e3fe092a39f9679cd8d965954c2aca25a05ca41d32" ]]
first=$(grep -aoE '[0-9a-fA-F]{64}' "$notes" | head -1)
[[ "$first" != "$got" ]]
# The published hash is glued to the HTML markers, so a whitespace-field
# match never sees a bare 64-character field. That is the bug.
if awk -v file="actions-runner-linux-x64-2.338.0.tar.gz" '
  index($0, file) { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9a-fA-F]{64}$/) { found = 1 } }
  END { exit found ? 0 : 1 }
' "$notes"; then
  printf 'whitespace field parser matched the real release notes\n' >&2
  exit 1
fi

side=$ROOT/testdata/bazelisk-v1.29.0-linux-amd64.sha256
[[ "$(bazelisk_sidecar_sha "$(cat "$side")")" == "5a408715e932c0250d28bd84555f12edbf70117de42f9181691c736eacc4a992" ]]
[[ "$(bazelisk_sidecar_sha "5a408715e932c0250d28bd84555f12edbf70117de42f9181691c736eacc4a992  bazelisk-linux-amd64")" == "5a408715e932c0250d28bd84555f12edbf70117de42f9181691c736eacc4a992" ]]
if bazelisk_sidecar_sha "not-a-hash" >/dev/null; then
  printf 'sidecar parser accepted garbage\n' >&2
  exit 1
fi

work=$tmp/work
mkdir -p "$work/bin" "$tmp/artifacts"
cp "$ROOT/update-checksums.sh" "$ROOT/checksum-notes.sh" "$ROOT/checksums.txt" "$work/"
printf 'runner-bytes' >"$tmp/artifacts/runner"
printf 'bazel-bytes' >"$tmp/artifacts/bazel"
runner_sha=$(sha256sum "$tmp/artifacts/runner" | awk '{print $1}')
bazel_sha=$(sha256sum "$tmp/artifacts/bazel" | awk '{print $1}')
python3 - "$runner_sha" "$tmp/artifacts/notes.json" <<'PY'
import json, sys
sha, path = sys.argv[1], sys.argv[2]
body = (
    "- actions-runner-linux-x64-9.9.9.tar.gz <!-- BEGIN SHA linux-x64 -->"
    + sha
    + "<!-- END SHA linux-x64 -->\n"
)
with open(path, "w", encoding="utf-8") as fh:
    json.dump({"body": body}, fh)
PY
printf '%s\n' "$bazel_sha" >"$tmp/artifacts/bazel.sha256"
printf '%s\n' "0000000000000000000000000000000000000000000000000000000000000000" >"$tmp/artifacts/bad.sha256"

cat >"$work/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
out=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out=$2
      shift 2
      ;;
    -fsSL | -f | -s | -S | -L)
      shift
      ;;
    -*)
      printf 'unexpected curl flag %s\n' "$1" >&2
      exit 1
      ;;
    *)
      url=$1
      shift
      ;;
  esac
done
printf '%s\n' "$url" >>"${FGDB_CURL_LOG:?}"
case "$url" in
  https://github.com/actions/runner/releases/download/*)
    [[ -n "$out" ]]
    cat "${FGDB_RUNNER_ARTIFACT:?}" >"$out"
    ;;
  https://api.github.com/repos/actions/runner/releases/tags/*)
    [[ -z "$out" ]]
    if [[ "${FGDB_RUNNER_NOTES_FAIL:-}" == 1 ]]; then
      exit 1
    fi
    cat "${FGDB_RUNNER_NOTES:?}"
    ;;
  https://github.com/bazelbuild/bazelisk/releases/download/*.sha256)
    [[ -z "$out" ]]
    if [[ "${FGDB_BAZEL_SIDECAR_FAIL:-}" == 1 ]]; then
      exit 1
    fi
    cat "${FGDB_BAZEL_SIDECAR:?}"
    ;;
  https://github.com/bazelbuild/bazelisk/releases/download/*)
    [[ -n "$out" ]]
    cat "${FGDB_BAZEL_ARTIFACT:?}" >"$out"
    ;;
  *)
    printf 'unexpected url %s\n' "$url" >&2
    exit 1
    ;;
esac
EOF
cat >"$work/bin/jq" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "-r" && "$2" == ".body" ]]
python3 -c 'import json,sys; print(json.load(sys.stdin)["body"])'
EOF
chmod +x "$work/bin/curl" "$work/bin/jq"

run_update() {
  PATH="$work/bin:$PATH" \
    FGDB_CURL_LOG="$tmp/curl.log" \
    FGDB_RUNNER_ARTIFACT="$tmp/artifacts/runner" \
    FGDB_RUNNER_NOTES="${FGDB_RUNNER_NOTES:-$tmp/artifacts/notes.json}" \
    FGDB_BAZEL_ARTIFACT="$tmp/artifacts/bazel" \
    FGDB_BAZEL_SIDECAR="${FGDB_BAZEL_SIDECAR:-$tmp/artifacts/bazel.sha256}" \
    bash "$work/update-checksums.sh" "$@"
}

: >"$tmp/curl.log"
umask 077
run_update actions-runner 9.9.9
grep -q "actions-runner 9.9.9 linux-x64 ${runner_sha} " "$work/checksums.txt"
[[ "$(stat -c '%a' "$work/checksums.txt")" == "644" ]]
grep -q 'api.github.com/repos/actions/runner/releases/tags/v9.9.9' "$tmp/curl.log"

: >"$tmp/curl.log"
cp "$ROOT/checksums.txt" "$work/checksums.txt"
chmod 0644 "$work/checksums.txt"
python3 - "$tmp/artifacts/notes-bad.json" <<'PY'
import json, sys
body = "- actions-runner-linux-x64-9.9.9.tar.gz <!-- BEGIN SHA linux-x64 -->0000000000000000000000000000000000000000000000000000000000000000<!-- END SHA linux-x64 -->\n"
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    json.dump({"body": body}, fh)
PY
if FGDB_RUNNER_NOTES="$tmp/artifacts/notes-bad.json" run_update actions-runner 9.9.9; then
  printf 'mismatched runner notes were accepted\n' >&2
  exit 1
fi
[[ "$(cat "$work/checksums.txt")" == "$(cat "$ROOT/checksums.txt")" ]]

: >"$tmp/curl.log"
if FGDB_BAZEL_SIDECAR_FAIL=1 run_update bazelisk 9.9.9; then
  printf 'bazelisk update trusted a download with no published checksum\n' >&2
  exit 1
fi
[[ "$(cat "$work/checksums.txt")" == "$(cat "$ROOT/checksums.txt")" ]]
grep -q 'bazelisk-linux-amd64.sha256' "$tmp/curl.log"

: >"$tmp/curl.log"
umask 077
run_update bazelisk 9.9.9
grep -q "bazelisk 9.9.9 linux-amd64 ${bazel_sha} " "$work/checksums.txt"
[[ "$(stat -c '%a' "$work/checksums.txt")" == "644" ]]

: >"$tmp/curl.log"
cp "$ROOT/checksums.txt" "$work/checksums.txt"
if FGDB_BAZEL_SIDECAR="$tmp/artifacts/bad.sha256" run_update bazelisk 9.9.9; then
  printf 'mismatched bazelisk sidecar was accepted\n' >&2
  exit 1
fi
[[ "$(cat "$work/checksums.txt")" == "$(cat "$ROOT/checksums.txt")" ]]

printf 'update-checksums tests passed\n'
