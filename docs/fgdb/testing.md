> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Testing FGDb

This page is for someone running the suite on a laptop or on a self-hosted runner. It covers the first slice: a 3-node cluster on one Linux machine, a few upstream tests, and a written list of claims.

Fast `fgdb-backup` tests and the `build/fgdb` shell tests are in the Fast checks section. `.github/workflows/fgdb-checks.yml` runs them. Cluster-backed `fgdb-backup` tests stay in `.github/workflows/fgdb-backup-tests.yml`.

The product claims live in `fgdb/test/claims.yaml`. Each claim names at least one test. The checker fails if a claim has no test, if that test is not in the tree, or if the status is `untested`. A waived claim needs an owner, a reason, and an expiry date.

The registry has no field for a downstream product. A downstream need arrives as a feature or fix request issue. If it is accepted, it gets its own claim and its own test.

## What you need

- Linux on x86-64. The 3-node steps start real `cockroach-oss` processes.
- Go 1.21, the same version the release binary reports (`go1.21.12`).
- A `cockroach-oss` binary that prints `Distribution: OSS`. A local build (`./dev build oss`) or the `v23.2.15-fgdb.1` tarball both work. The tarball's `lib/` directory has to sit next to the binary. The suite adds that directory to `LD_LIBRARY_PATH` when it is there.
- For the logictest subset and kvnemesis, the repo's Bazel setup. Those two do not run if `bazel` is not on `PATH`. The report says they were not run. On the interim tier that is a failure, not a pass.

## Run a tier

From the repository root:

```bash
fgdb/test/run.sh pr
```

`pr` checks the claims file, checks that no new production file imports a test package, and runs the new Go tests. It does not start a cluster.

```bash
export FGDB_COCKROACH=/path/to/cockroach
fgdb/test/run.sh interim
```

`interim` is the first release slice. It runs the `pr` checks, then:

- `//pkg/sql/logictest/tests/local:local_test` and the `fakedist` config, filtered to `TestLogic_txn` and `TestLogic_select`
- `//pkg/sql/logictest/tests/5node:5node_test`, filtered to `TestLogic_ranges` and `TestLogic_distsql_stats`
- `//pkg/kv/kvnemesis:kvnemesis_test`
- a 3-node cluster using `FGDB_COCKROACH`

`nightly` is the same shape as `interim`, plus an explicit note that the rest of logictest is not in this slice. `weekly` adds explicit notes for the sqllogictest corpus, the external history checker, clock skew, and lab-only faults. Those notes are `not_tested` lines. They are not silent skips.

`fgdb/test/scripts/run-local.sh --tier=pr` is the same entry point.

The 3-node cluster asks the operating system for free ports and tries a new set if a port is already taken. Temporary files for one run sit under `FGDB_WORK`, or under `RUNNER_TEMP` or `TMPDIR` when that is unset. `run.sh` removes the directory when it created it. The interim workflow removes its directory at the end of the job, including when the suite fails. That workflow does not leave a GitHub token in `.git/config`. `run.sh` starts the suite with `env -i` and the exact names in `pkg/fgdbtest/cluster/allowlist.txt`: `PATH`, `HOME`, `TMPDIR`, `LANG`, `LC_ALL`, the `FGDB_` variables the harness reads, `GOCOVERDIR`, and `GOTRACEBACK`. Database and workload processes get that same list. `LD_LIBRARY_PATH` is not copied from the runner. When the release has a real `lib` directory containing libgeos, the cockroach process gets `LD_LIBRARY_PATH` set to that directory only. A variable whose name contains `TOKEN`, `SECRET`, or `PASSWORD`, or whose name starts with `ACTIONS_`, fails the run.

The run writes `result.json` and `summary.md`. The first two lines of the summary are always:

```text
partition NOT tested
disk faults NOT tested
```

## What the 3-node scenario proves

Set `FGDB_COCKROACH` to the binary under test. For the rolling upgrade, also set `FGDB_PREVIOUS` to a different binary, usually the `v23.2.15-fgdb.1` release. The scenario checks that both print `Distribution: OSS`.

It then:

1. Starts three insecure nodes on one machine, initializes the cluster, and sets the internal ranges to 3 copies so a 3-node lab can become fully replicated.
2. Loads the bank workload and the kv workload.
3. Starts three operating-system processes: bank against two nodes, kv against the third, and a history recorder that does single-key reads and writes through all three.
4. If `FGDB_PREVIOUS` is a different binary, drains and restarts each node onto `FGDB_COCKROACH`. The clients keep running.
5. Kills one node with `SIGKILL`, checks the bank total on a live node, and starts that node again on the same store.
6. Pauses one node with `SIGSTOP` and resumes it with `SIGCONT`.
7. Kills two of the three nodes. New acknowledged writes should stop, because three copies need two votes. It starts both nodes again and checks that every acknowledged write is still present.
8. Stops the clients and checks the bank total on every node: the row count is the loaded size and the sum of balances is 0.
9. Runs Porcupine (MIT, copied under `pkg/fgdbtest/third_party/porcupine`) on the recorder's history. An empty history, a history with no successful call, or a checker timeout is a failure. The check does not skip.
10. Runs `crdb_internal.check_consistency`. If that call cannot run, the step fails.
11. Builds `fgdb-backup` if `FGDB_BACKUP_BIN` is unset, backs up the bank database from one node, restores it on another, and compares the count, the sum, and a checksum.

`FGDB_ONLY_STEPS` limits the fault steps on a local tier such as `pr`. Example: `FGDB_TIER=pr FGDB_ONLY_STEPS=node-kill,node-pause`. The replication setup and the clients still run. Checks that were left out are marked not tested, and the summary result is not a pass. The interim tier, the release tier, and `FGDB_REQUIRE_CLUSTER=1` reject `FGDB_ONLY_STEPS`.

You can run the same scenario as a Go test:

```bash
FGDB_COCKROACH=/path/to/cockroach \
  go test -count=1 -timeout 45m ./pkg/fgdbtest/scenarios/cluster -run TestThreeNodeSlice
```

Without `FGDB_COCKROACH` the test skips. `FGDB_REQUIRE_CLUSTER=1` turns that skip into a failure. The interim tier sets that variable.

## What is not tested yet

These are stated in the report. They are not passes.

- **Network partition.** That needs preconfigured root units on separate machines. Those units are not installed. One machine also shares a loopback interface, so this run cannot show a real split.
- **Disk faults** (slow, stalled, or full). Same reason: they need those root units.
- **Clock skew.** The three processes share one clock.
- **Secure mode.** This slice uses `--insecure`.
- **Cross-key serializability.** The external checker is not in this repository. Porcupine checks one register.
- **The rest of logictest, and the sqllogictest corpus.** The corpus is an external input and is not copied into this repository.
- **Statement coverage.**

`fgdb/test/skips.yaml` is empty. A skip needs a test name, a reason, and an issue link. Adding a skip without those fails the claims check.

## How to add a test

Put new Go in `pkg/fgdbtest/` or `pkg/cmd/fgdb-test`. Mark the Bazel target `testonly = True`. Call upstream Apache-licensed tests through their existing targets (`pkg/sql/logictest`, `pkg/kv/kvnemesis`, `cockroach workload`). Do not copy code from `pkg/ccl`. Do not vendor an EPL checker. Porcupine is the MIT checker already copied under `pkg/fgdbtest/third_party/porcupine`.

A function stays at cognitive complexity 15 or below, takes at most 7 parameters, and does not repeat a long literal.

## How to add a claim

Add a row to `fgdb/test/claims.yaml`:

- `id` looks like `FT-001`.
- `claim` is one sentence a reader can check.
- `source` is a file under `docs/fgdb/`, optionally with `#L` and a line number.
- `tests` lists `pkg/path:TestName` or a `//pkg/path:target` Bazel target. Each one has to exist.
- `status` is `tested`, `partial`, `untested`, or `waived`. `untested` fails the check. `waived` needs `owner`, `reason`, and `expiry` (`YYYY-MM-DD`).

Then run:

```bash
fgdb/test/run.sh pr
```

A doc line under `docs/fgdb/` that says "always", "guarantees", "survives", or "default is" is a hint for the reviewer. The hint does not fail the run. The reviewer decides whether the registry needs a new row.

## No test code in production

`fgdb/test/test-code-in-prod-baseline.txt` is the frozen list of production Go files that already import `pkg/testutils` or `pkg/fgdbtest`. The check fails when a new file does that. Removing an old line is allowed. `pkg/testutils`, `pkg/fgdbtest`, `pkg/cmd/fgdb-test`, and `pkg/cmd/roachtest` are the trees that are allowed to import test helpers.

`//pkg/cmd/cockroach-oss:cockroach-oss` also rejects an import of `pkg/fgdbtest`.

## Fast checks

`.github/workflows/fgdb-checks.yml` runs these checks on GitHub-hosted runners. `.github/workflows/fgdb-backup-tests.yml` is the job that starts `cockroach-oss`. Unit tests, `go vet`, and the shell scripts are in `fgdb-checks.yml`, not in the cluster job.

### On a laptop

From the repository root. The Go version in the workflow is 1.21.12.

```bash
cd pkg/cmd/fgdb-backup
go test -count=1 ./...
go vet ./...
```

`go test -count=1 ./...` is the command in the `backup-unit` job. It runs the in-process tests: config parsing, SQL rewrite, the local store, the backup lease, and S3 against an in-process fake server. It also runs the seed inputs of `FuzzArrayStreamMatchesReference`. Tests in `roundtrip_test.go`, `review_test.go`, `safety_test.go`, `cli_fixes_test.go`, and `restore_listener_order_test.go` start real `cockroach-oss` processes. They skip unless `FGDB_COCKROACH` is set, or unless `/tmp/fgdb-bin/cockroach` is present. A skip is not a failure. `FGDB_REQUIRE_CLUSTER` is not read by these tests. `fgdb-checks.yml` does not set `FGDB_COCKROACH`, so those tests skip there.

The same workflow also runs these from the repository root. `gofmt -l` has to print nothing.

```bash
gofmt -l pkg/fgdbtest pkg/cmd/fgdb-test pkg/cmd/fgdb-backup
go vet ./pkg/fgdbtest/... ./pkg/cmd/fgdb-test/...
bash fgdb/test/run.sh pr
```

`fgdb/test/run.sh pr` is the pr tier described above. The boundary scripts are in the tree:

```bash
bash fgdb/test/scripts/check-boundary_test.sh
bash fgdb/test/scripts/check-boundary.sh
```

The script exits 1 if a file under `pkg/fgdbtest`, `pkg/cmd/fgdb-test`, `pkg/cmd/fgdb-backup`, `fgdb`, or `build/fgdb` imports `pkg/ccl`, lists `//pkg/ccl` in a Bazel file, or starts with a CockroachDB Software License header.

The shell tests use fixtures. They do not build Cockroach, and they do not need a GitHub token. From the repository root, each one exits 0 when its checks pass:

```bash
bash build/fgdb/release-tag_test.sh
bash build/fgdb/skip-duplicate-tag-build_test.sh
bash build/fgdb/verify-oss-binary_test.sh
bash build/fgdb/runner/check-cache-dir_test.sh
bash build/fgdb/runner/checksums_test.sh
bash build/fgdb/runner/token-input_test.sh
bash build/fgdb/runner/read-config_test.sh
bash build/fgdb/runner/update-checksums_test.sh
bash build/fgdb/runner/runner-unit_test.sh
```

- `release-tag_test.sh` checks `build/fgdb/release-tag.sh`: which git tag is published, and which image tags that publish pushes.
- `skip-duplicate-tag-build_test.sh` checks `build/fgdb/skip-duplicate-tag-build.sh` with a fixture in place of `curl`.
- `verify-oss-binary_test.sh` checks `build/fgdb/verify-oss-binary.sh` against a stand-in binary that prints `Distribution: OSS`.
- `check-cache-dir_test.sh` checks `build/fgdb/runner/check-cache-dir.sh` with stand-in `findmnt` and `stat` programs.
- `checksums_test.sh` checks `build/fgdb/runner/checksums.txt`: five fields per row, a 64-character hash, no duplicate keys, and rows for `actions-runner 2.338.0 linux-x64` and `bazelisk 1.29.0 linux-amd64`.
- `token-input_test.sh` checks `build/fgdb/runner/token-input.sh`. An empty token and a symlink are refused. A token from a file or from stdin is trimmed.
- `read-config_test.sh` checks `build/fgdb/runner/read-config.sh`, the parser for the runner setup file.
- `update-checksums_test.sh` checks `build/fgdb/runner/update-checksums.sh` and `checksum-notes.sh`. A downloaded file is refused when the publisher hash does not match.
- `runner-unit_test.sh` checks `build/fgdb/runner/runner-unit.sh`, including the cache-check command line and the refusal of a missing or broken check script.

### `.github/workflows/fgdb-checks.yml`

The workflow is named FGDB checks. It runs on GitHub-hosted `ubuntu-latest`. A pull request runs it when the change touches `.github/workflows/**`, `fgdb/**`, `build/fgdb/**`, `pkg/fgdbtest/**`, `pkg/cmd/fgdb-test/**`, `pkg/cmd/fgdb-backup/**`, `go.mod`, or `go.sum`. Every push to `main` runs it, with no path filter. It has no secrets. The workflow permission set is empty, and each job sets `contents: read`. Checkouts set `persist-credentials: false`.

- `actionlint` (10 minutes) installs actionlint 1.7.7 and runs `actionlint -color`.
- `shellcheck` (10 minutes) installs ShellCheck 0.10.0 and runs it at warning severity on every `*.sh` file under `fgdb/` and `build/fgdb/`.
- `gofmt-vet` (15 minutes) installs Go 1.21.12, fails if `gofmt -l` prints a file under `pkg/fgdbtest`, `pkg/cmd/fgdb-test`, or `pkg/cmd/fgdb-backup`, and runs `go vet` on those trees. `go vet` for `fgdb-backup` uses that module's directory.
- `pr-tier` (15 minutes) runs `bash fgdb/test/run.sh pr`. No Bazel and no cluster.
- `backup-unit` (15 minutes) runs `go test -count=1 ./...` in `pkg/cmd/fgdb-backup`, then the nine shell scripts above.
- `boundary` (10 minutes) runs `bash fgdb/test/scripts/check-boundary_test.sh` and then `bash fgdb/test/scripts/check-boundary.sh`.

### Cluster-backed patch tests

`.github/workflows/fgdb-backup-tests.yml` is named FGDB patch tests, and it has one job, `integration`. A pull request runs it when the change touches that workflow or `pkg/cmd/fgdb-backup/**`. It does not run on a push. It does not run `go vet` or the shell scripts. Those moved to `fgdb-checks.yml`.

The job limit is 30 minutes. It downloads the public `v23.2.15-fgdb.1` linux-amd64 tarball, checks the sha256 written in the workflow, and unpacks it. It sets `FGDB_COCKROACH` to `cockroach` in that tree and `LD_LIBRARY_PATH` to the `lib` directory beside it. In `pkg/cmd/fgdb-backup` it runs:

```bash
go test -count=1 -v ./...
```

To run that on a laptop, point the variables at a binary that prints `Distribution: OSS`. When the binary comes from the release tarball, `LD_LIBRARY_PATH` is that tarball's `lib` directory. The test process passes its environment through to `cockroach`.

```bash
export FGDB_COCKROACH=/path/to/cockroach
export LD_LIBRARY_PATH=/path/to/lib
cd pkg/cmd/fgdb-backup
go test -count=1 -v ./...
```

## Where CI runs it

The laptop command, the GitHub-hosted checks, and the heavy compile are separate. A pull request that changes the checked paths runs the fast checks on `ubuntu-latest`. A pull request does not start a self-hosted job.

`fgdb/test/run.sh pr` is the check you can run on a laptop. It needs Go 1.21. It does not need Bazel, and it does not start a cluster. The claims check, the production-import baseline, and the Go tests in `fgdb/test/tiers/pr.yaml` are the whole tier.

`.github/workflows/fgdb-checks.yml` runs that tier, plus actionlint, ShellCheck, `gofmt`, `go vet`, the `fgdb-backup` unit tests, and a scan that FGDb paths do not import `pkg/ccl`. Every job uses GitHub-hosted `ubuntu-latest`. A pull request runs the workflow when it changes those paths. Every push to `main` runs it, including a push that only changes docs. The workflow has no secrets and does not use a self-hosted runner. The workflow file is in this tree.

`.github/workflows/fgdb-pr-build.yml` compiles `cockroach-oss` when a maintainer starts it from the Actions tab (`workflow_dispatch`). The input is the 40-character SHA of a commit that is already on a branch or tag in this repository. The workflow has no repository-name input, so a commit that exists only on a fork is refused before checkout. Opening, updating, or marking a pull request ready leaves this workflow idle. The job asks for a self-hosted runner labeled `fgdb-pr-ephemeral`. It does not use the persistent runner labeled `fgdb-build`. The checkout has no credentials and the job has no secrets. Each commit gets its own Bazel output directory, named with that SHA. Registering the ephemeral runner is a separate step. The workflow file does not do it. How to send a change is [contributing.md](contributing.md).

`.github/workflows/fgdb-suite-interim.yml` is manual (`workflow_dispatch` only). It does not run on pull requests. It runs on a self-hosted runner labeled `fgdb-build` and reads `/etc/fgdb/runner.env`. Bazel is capped at 24 threads and 81920 MB of RAM. The runner slice itself has a 96G hard cap. Both numbers are in [RUNNER.md](RUNNER.md). The job has no secrets. You pass the candidate tarball URL and its sha256. The previous binary is the `v23.2.15-fgdb.1` release from [releases.md](releases.md), checked against sha256 `5f9c36263bdbbfd1d8e6781fe09920fac33d4c608b4743be6681009f4215a379`.
