#!/usr/bin/env bash
#
# Checks for changed-paths.sh. No network and no Cockroach build.

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${root}/changed-paths.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
passed=0
failed=0

record_result() {
  local label=$1 status=$2
  if [[ "$status" -eq 0 ]]; then
    echo "PASS: $label"
    passed=$((passed + 1))
  else
    echo "FAIL: $label" >&2
    failed=$((failed + 1))
  fi
}

expect_result() {
  local label=$1 want=$2 mode=$3 status=0
  printf 'fgdb=%s\n' "$want" > "$tmp/expected"
  if [[ "$mode" == file ]]; then
    bash "$script" "$tmp/input" > "$tmp/output" || status=$?
  else
    bash "$script" < "$tmp/input" > "$tmp/output" || status=$?
  fi
  if [[ "$status" -eq 0 ]] && cmp -s "$tmp/expected" "$tmp/output"; then
    record_result "$label" 0
  else
    printf 'expected fgdb=%s; exit=%s; got:\n' "$want" "$status" >&2
    cat "$tmp/output" >&2
    record_result "$label" 1
  fi
}

expect_paths() {
  local label=$1 want=$2
  shift 2
  : > "$tmp/input"
  if [[ "$#" -gt 0 ]]; then
    printf '%s\0' "$@" > "$tmp/input"
  fi
  expect_result "$label" "$want" stdin
}

expect_pr() {
  local label=$1 want=$2
  tr '\n' '\0' > "$tmp/input"
  expect_result "$label" "$want" file
}

must_fail() {
  local label=$1
  shift
  if bash "$script" "$@" > "$tmp/output" 2> "$tmp/error"; then
    record_result "$label" 1
  else
    record_result "$label" 0
  fi
}

# PR #46: GitHub PR files API / refs/pull/46/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #46' true <<'EOF'
docs/fgdb/spec/PAR-208-multifamily-pk.md
pkg/sql/logictest/testdata/logic_test/alter_primary_key
pkg/sql/row/BUILD.bazel
pkg/sql/row/writer.go
pkg/sql/row/writer_test.go
EOF

# PR #50: GitHub PR files API / refs/pull/50/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #50' true <<'EOF'
docs/fgdb/spec/PAR-200-gc-span-guard.md
pkg/kv/kvserver/batcheval/BUILD.bazel
pkg/kv/kvserver/batcheval/cmd_gc.go
pkg/kv/kvserver/batcheval/cmd_gc_test.go
pkg/kv/kvserver/client_split_test.go
EOF

# PR #49: GitHub PR files API / refs/pull/49/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #49' true <<'EOF'
build/fgdb/require-repo-commit.py
build/fgdb/require-repo-commit_test.sh
EOF

# PR #48: GitHub PR files API, Nov 13 2026.
expect_pr 'PR #48' true <<'EOF'
pkg/fgdbtest/cluster/proc_test.go
EOF

# PR #17: GitHub PR files API / refs/pull/17/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #17' true <<'EOF'
README.md
docs/fgdb/backup-cli.md
docs/fgdb/backup-config.md
docs/fgdb/backup.md
docs/fgdb/testing.md
pkg/cmd/fgdb-backup/cli_fixes_test.go
pkg/cmd/fgdb-backup/config.example.yaml
pkg/cmd/fgdb-backup/db.go
pkg/cmd/fgdb-backup/import_array_typmod_test.go
pkg/cmd/fgdb-backup/lock.go
pkg/cmd/fgdb-backup/logic_test.go
pkg/cmd/fgdb-backup/main.go
pkg/cmd/fgdb-backup/prune.go
pkg/cmd/fgdb-backup/restore.go
pkg/cmd/fgdb-backup/restore_listener_order_test.go
pkg/cmd/fgdb-backup/restore_listener_unit_test.go
pkg/cmd/fgdb-backup/restore_swap.go
pkg/cmd/fgdb-backup/s3_test.go
pkg/cmd/fgdb-backup/store.go
pkg/cmd/fgdb-backup/store_s3.go
EOF

# PR #20: GitHub PR files API / refs/pull/20/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #20' true <<'EOF'
.github/workflows/fgdb-oss-release.yml
docs/fgdb/RELEASING.md
docs/fgdb/RUNNER.md
EOF

# PR #27: GitHub PR files API / refs/pull/27/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #27' false <<'EOF'
docs/fgdb/architecture/runtime-contracts.md
docs/fgdb/delivery/README.md
docs/fgdb/development/local-go.md
EOF

# PR #25: GitHub PR files API / refs/pull/25/merge HEAD^1..HEAD, Nov 13 2026.
expect_pr 'PR #25' false <<'EOF'
docs/fgdb/architecture/FOLLOW-UPS.md
docs/fgdb/architecture/OPEN-QUESTIONS.md
docs/fgdb/architecture/README.md
docs/fgdb/architecture/build.md
EOF

expect_paths 'empty stdin' false
: > "$tmp/input"
expect_result 'empty file' false file
expect_paths 'docs path with a space' false 'docs/a space.md'
expect_paths 'SQL path with a space' true 'pkg/sql/a space.go'
expect_paths 'docs path with a newline' false $'docs/a\nb.md'
expect_paths 'newline does not start another path' false $'docs/x\npkg/sql/y.go'
expect_paths 'SQL path with a newline' true $'pkg/sql/a\nb.go'
expect_paths 'rename out of pkg (delete plus add)' true 'pkg/sql/old.go' 'docs/old.go'
expect_paths 'workflow only' true '.github/workflows/fgdb-checks.yml'
expect_paths 'root markdown only' false 'README.md' 'FGL-README.md'
expect_paths 'go.sum only' true 'go.sum'
expect_paths 'go.mod only' true 'go.mod'
expect_paths 'pkg look-alike' false 'pkgx/foo.go'
expect_paths 'nested pkg look-alike' false 'docs/pkg/sql/x.md'
expect_paths 'file look-alikes' false 'go.sum.bak' 'docs/go.mod' '.github/workflowsx/check.yml' 'fgdbx/test.sh' 'build/fgdbx/test.sh'

# The old filter's roots all remain included; pkg/sql is a new member.
expect_paths 'old scope: FGDb' true 'fgdb/test/tiers/pr.yaml'
expect_paths 'old scope: build scripts' true 'build/fgdb/build-oss.sh'
expect_paths 'old scope: fgdbtest' true 'pkg/fgdbtest/test.go'
expect_paths 'old scope: repstream' true 'pkg/repstream/test.go'
expect_paths 'old scope: fgdb-test' true 'pkg/cmd/fgdb-test/main.go'
expect_paths 'old scope: fgdb-backup' true 'pkg/cmd/fgdb-backup/main.go'
expect_paths 'new scope: SQL only' true 'pkg/sql/row/writer.go'
expect_paths 'new scope: KV only' true 'pkg/kv/kvserver/batcheval/cmd_gc.go'
expect_paths 'new scope: other pkg roots' true 'pkg/util/example.go'

printf '%s' 'pkg/sql/final.go' > "$tmp/input"
expect_result 'final matching record without NUL via file' true file
expect_result 'final matching record without NUL via stdin' true stdin
printf '%s' 'docs/final.md' > "$tmp/input"
expect_result 'final docs record without NUL' false stdin
printf '%s\0%s' 'docs/first.md' 'pkg/kv/final.go' > "$tmp/input"
expect_result 'unterminated matching record after docs' true stdin

must_fail 'two arguments' "$tmp/input" "$tmp/input"
must_fail 'missing file' "$tmp/missing"

# Mirror the workflow's first-parent diff of a PR merge commit.
merge_paths() (
  local repo=$1 first_path=$2 second_path=$3 base
  export GIT_AUTHOR_NAME='Changed Paths Test'
  export GIT_AUTHOR_EMAIL='changed-paths@example.invalid'
  export GIT_COMMITTER_NAME=$GIT_AUTHOR_NAME
  export GIT_COMMITTER_EMAIL=$GIT_AUTHOR_EMAIL
  # Ignore developer hooks and signing settings without writing git config.
  export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
  export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=/dev/null
  # expect_merge calls this under ||, so errexit cannot guard setup failures.
  git init -q --initial-branch=base "$repo" || exit 1
  cd "$repo" || exit 1
  printf 'base\n' > README.md || exit 1
  git add README.md || exit 1
  git commit --no-gpg-sign -qm base || exit 1
  base=$(git rev-parse HEAD) || exit 1
  git checkout -qb feature || exit 1
  mkdir -p "$(dirname "$second_path")" || exit 1
  printf 'PR change\n' > "$second_path" || exit 1
  git add "$second_path" || exit 1
  git commit --no-gpg-sign -qm 'change second parent' || exit 1
  git checkout -q --detach "$base" || exit 1
  mkdir -p "$(dirname "$first_path")" || exit 1
  printf 'base change\n' > "$first_path" || exit 1
  git add "$first_path" || exit 1
  git commit --no-gpg-sign -qm 'advance first parent' || exit 1
  git merge --no-gpg-sign -q --no-ff feature -m 'merge PR' || exit 1
  git diff --no-renames --name-only -z HEAD^1 HEAD | bash "$script"
)

expect_merge() {
  local label=$1 want=$2 repo=$3 first_path=$4 second_path=$5 status=0
  printf 'fgdb=%s\n' "$want" > "$tmp/expected"
  merge_paths "$repo" "$first_path" "$second_path" > "$tmp/output" || status=$?
  if [[ "$status" -eq 0 ]] && cmp -s "$tmp/expected" "$tmp/output"; then
    record_result "$label" 0
  else
    printf 'expected fgdb=%s; exit=%s; got:\n' "$want" "$status" >&2
    cat "$tmp/output" >&2
    record_result "$label" 1
  fi
}

expect_merge 'merge commit first-parent diff pipeline' true \
  "$tmp/merge-code-pr" main.md pkg/kv/x.go
expect_merge 'merge commit ignores base-only SQL change' false \
  "$tmp/merge-docs-pr" pkg/sql/base.go docs/guide.md

printf 'changed-paths tests: %s passed, %s failed\n' "$passed" "$failed"
[[ "$failed" -eq 0 ]]
