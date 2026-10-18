#!/usr/bin/env bash
#
# When a workflow_dispatch publish creates git tag v*-oss, GitHub starts a
# second run from that tag push. If a dispatch run for the same commit already
# has publish=true (still running or recently succeeded), skip the second build.
#
# Prints "true" or "false".

set -euo pipefail

if [[ "${GITHUB_EVENT_NAME:-}" != "push" ]]; then
  echo false
  exit 0
fi

: "${GITHUB_TOKEN:?GITHUB_TOKEN is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"

api="https://api.github.com/repos/${GITHUB_REPOSITORY}/actions/workflows/fgdb-oss-release.yml/runs?per_page=30"
body=$(curl -fsSL \
  -H "Authorization: Bearer ${GITHUB_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "$api")

FGDB_RUNS_JSON=$body \
FGDB_SHA=$GITHUB_SHA \
FGDB_RUN_ID=$GITHUB_RUN_ID \
python3 - <<'PY'
import json
import os

payload = json.loads(os.environ["FGDB_RUNS_JSON"])
sha = os.environ["FGDB_SHA"]
this_run = int(os.environ["FGDB_RUN_ID"])
tag = os.environ["GITHUB_REF_NAME"]
active = {"queued", "in_progress", "waiting", "pending", "requested"}
for run in payload.get("workflow_runs", []):
    if run.get("id") == this_run:
        continue
    if run.get("head_sha") != sha:
        continue
    if run.get("event") != "workflow_dispatch":
        continue
    title = " ".join(
        part for part in (run.get("name"), run.get("display_title")) if part
    )
    if "publish=true" not in title:
        continue
    # A dispatch for an fgdb tag must not suppress a later OSS tag push at
    # the same commit. For fgdb tags, require the exact tag in the dispatch
    # run name; an OSS push is associated with every non-fgdb dispatch.
    if "-fgdb." in tag:
        title_words = set(title.split())
        if tag not in title_words and f"refs/tags/{tag}" not in title_words:
            continue
    elif "-fgdb." in title:
        continue
    if run.get("status") in active or run.get("conclusion") == "success":
        print("true")
        raise SystemExit(0)
print("false")
PY
