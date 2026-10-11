> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Testing FGDb

This page is for someone running the suite on a laptop or on a self-hosted runner. It covers the first slice: a 3-node cluster on one Linux machine, a few upstream tests, and a written list of claims.

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

`FGDB_ONLY_STEPS` limits the fault steps. Example: `node-kill,node-pause`. The replication setup and the clients still run. A limited run does not mark the other claims as failed.

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

## Where CI runs it

`.github/workflows/fgdb-suite-interim.yml` is manual (`workflow_dispatch` only). It does not run on pull requests. It runs on a self-hosted runner labeled `fgdb-build`, reads `/etc/fgdb/runner.env` for the Bazel cache and the 24-thread / 96 GB limits, and has no secrets. You pass the candidate tarball URL and its sha256. The previous binary is the `v23.2.15-fgdb.1` release, checked against its published sha256.
