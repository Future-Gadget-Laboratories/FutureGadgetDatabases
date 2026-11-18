#!/usr/bin/env bash
# require-repo-commit.py accepts only a full SHA that is already on a
# branch or tag of this repository.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/require-repo-commit.py
repo=Future-Gadget-Laboratories/FutureGadgetDatabases

run_check() {
  COMMIT_SHA=$1 GITHUB_REPOSITORY=$2 python3 "$script"
}

# Exercise HTTP failures offline, including errors from compare lookups.
python3 - "$script" "$repo" <<'PY'
from collections import Counter
from contextlib import redirect_stderr
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
import runpy
import subprocess
import sys
import threading
import urllib.error
from unittest.mock import patch

script, repo = sys.argv[1:]
sha = "b" * 40
counts = Counter()
statuses = {}

# Avoid real timeout waits and make non-HTTP network failures deterministic.
request = runpy.run_path(script)["request"]
for error in (TimeoutError("timed out"), urllib.error.URLError("connection failed")):
    stderr = io.StringIO()
    with patch("urllib.request.urlopen", side_effect=error) as urlopen, redirect_stderr(stderr):
        try:
            request("http://127.0.0.1/unreachable")
        except SystemExit as exc:
            assert exc.code != 0
        else:
            raise AssertionError("network error was accepted")
    assert "GitHub API request failed" in stderr.getvalue(), stderr.getvalue()
    assert "is not in" not in stderr.getvalue(), stderr.getvalue()
    assert urlopen.call_count == 1


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        operation = self.path.split("/")[4].split("?")[0]
        counts[operation] += 1
        responses = statuses.get(operation, [200])
        status = responses[min(counts[operation] - 1, len(responses) - 1)]
        data = {
            "commits": {"sha": "c" * 40 if mismatch else sha},
            "branches": [{"name": "main"}],
            "compare": {"ahead_by": 0},
            "tags": [],
        }[operation]
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())


server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
thread = threading.Thread(target=server.serve_forever, daemon=True)
thread.start()
env = dict(os.environ, COMMIT_SHA=sha, GITHUB_REPOSITORY=repo,
           NO_PROXY="127.0.0.1", no_proxy="127.0.0.1",
           FGDB_GITHUB_API=f"http://127.0.0.1:{server.server_port}")
try:
    cases = []
    for operation in ("commits", "compare"):
        for status in (404, 422, 403, 429, 500, 502, 503):
            missing = status in (404, 422)
            message = ("not in" if operation == "commits" else "not on a branch or tag") \
                if missing else f"HTTP {status}"
            cases.append((operation, [status], False, message, False))
        # A transient error should also recover when the next request succeeds.
        cases.append((operation, [502, 200], False, "is on branch main", True))
    cases.append(("commits", [200], True, "different commit SHA", False))
    for operation, responses, mismatch, message, success in cases:
        counts.clear()
        statuses = {operation: responses}
        result = subprocess.run([sys.executable, script], env=env,
                                capture_output=True, text=True, timeout=15)
        label = f"{operation} {responses}, mismatch={mismatch}"
        assert (result.returncode == 0) == success, (label, result.stderr)
        output = result.stdout if success else result.stderr
        assert message in output, (label, output)
        if responses[0] not in (404, 422) or operation != "commits":
            assert "is not in" not in result.stderr, (label, result.stderr)
        expected = 2 if success else (3 if responses[0] in (429, 502, 503) else 1)
        assert counts[operation] == expected, (label, counts)
        if operation == "commits" and not success:
            assert counts["branches"] == 0, (label, counts)
finally:
    server.shutdown()
    server.server_close()
    thread.join()
print("offline HTTP-status tests passed")
PY

if run_check abc "$repo" > /tmp/fgdb-require-bad.out 2> /tmp/fgdb-require-bad.err; then
  echo "short sha was accepted" >&2
  exit 1
fi
grep -q '40 hexadecimal' /tmp/fgdb-require-bad.err

missing=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
if run_check "$missing" "$repo" > /tmp/fgdb-require-miss.out 2> /tmp/fgdb-require-miss.err; then
  echo "unknown sha was accepted" >&2
  exit 1
fi
grep -q 'not in' /tmp/fgdb-require-miss.err

# Parent of the current main tip. It is in this repository and is not a
# branch tip, so the check must follow history rather than branch heads.
on_main=c3f0732a7b105180a463429266f2d88a539443b1
run_check "$on_main" "$repo" > /tmp/fgdb-require-ok.out
grep -q 'is on branch' /tmp/fgdb-require-ok.out

# A commit from another repository must not be treated as present here.
foreign=$(python3 - <<'PY'
import json, urllib.request
url = "https://api.github.com/repos/octocat/Hello-World/commits/7fd1a60b01f91b314f59955a4e4d4e80d8edf11d"
with urllib.request.urlopen(url, timeout=30) as resp:
    print(json.load(resp)["sha"])
PY
)
if run_check "$foreign" "$repo" > /tmp/fgdb-require-foreign.out 2> /tmp/fgdb-require-foreign.err; then
  echo "foreign sha was accepted" >&2
  exit 1
fi
grep -q 'not in' /tmp/fgdb-require-foreign.err
echo "require-repo-commit tests passed"
