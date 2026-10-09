#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/skip-duplicate-tag-build.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "${tmp}/bin"
cat >"${tmp}/bin/curl" <<'EOF'
#!/usr/bin/env bash
cat "${FGDB_RUNS_FIXTURE}"
EOF
chmod +x "${tmp}/bin/curl"

cat >"${tmp}/runs.json" <<'EOF'
{
  "workflow_runs": [
    {
      "id": 100,
      "head_sha": "same",
      "event": "workflow_dispatch",
      "name": "FGDB OSS publish=true",
      "display_title": "v23.2.15",
      "status": "completed",
      "conclusion": "success"
    },
    {
      "id": 101,
      "head_sha": "same",
      "event": "workflow_dispatch",
      "name": "FGDB OSS publish=true",
      "display_title": "v23.2.15-fgdb.1",
      "status": "completed",
      "conclusion": "success"
    }
  ]
}
EOF

run() {
  GITHUB_EVENT_NAME=push \
  GITHUB_TOKEN=test \
  GITHUB_REPOSITORY=example/repo \
  GITHUB_SHA=same \
  GITHUB_RUN_ID=999 \
  GITHUB_REF_NAME="$1" \
  FGDB_RUNS_FIXTURE="${tmp}/runs.json" \
  PATH="${tmp}/bin:${PATH}" \
  bash "$script"
}

[[ "$(run v23.2.15-oss)" == true ]]
[[ "$(run v23.2.15-fgdb.1)" == true ]]

python3 - "${tmp}/runs.json" <<'PY'
import json
import sys

path = sys.argv[1]
payload = json.load(open(path))
payload["workflow_runs"] = [payload["workflow_runs"][1]]
json.dump(payload, open(path, "w"))
PY

[[ "$(run v23.2.15-oss)" == false ]]
[[ "$(run v23.2.15-fgdb.1)" == true ]]

echo "duplicate-tag-build tests passed"
