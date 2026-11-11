> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Initial OSS capability checks

This slice tests existing OSS behavior. It does not add enterprise replication, SQL BACKUP, or newer PostgreSQL features. The logical `fgdb-backup` client remains a separate command.

## Stage 3: independent replication manager hooks

`GetStreamIngestManager` must check `GetStreamIngestManagerHook`, not the producer hook. The regression tests in `pkg/repstream/api_test.go` cover neither hook, producer only, ingest only, both hooks, argument forwarding, and error propagation. Their global hooks are restored after the tests; the cases are not parallel.

The production fix changes only that guard. With both hooks nil, the normal OSS error stays unchanged. No replication implementation or proprietary source is needed for this correction.

This package depends on generated SQL types. Run it through Bazel rather than assuming a plain `go test` can build it from a clean checkout:

```bash
bazel test //pkg/repstream:repstream_test --test_output=errors
```

The manual self-hosted compile runs this target after its existing OSS build and disallowed-imports check. If `pkg/repstream/api_test.go` is missing, that step fails unless the dispatch input `allow_missing_hook_test` is true. That input defaults to false. Set it only for an older commit that predates the test. The step then prints `repstream regression NOT tested` and writes a job summary line that the green compile is not a hook pass. The interim, nightly, and weekly tiers also require the target. A `pkg/repstream/**` change triggers the GitHub-hosted fast checks. Those jobs do not format, vet, or Bazel-test `pkg/repstream`. A green fast-check job is not evidence that the hook regression ran.

## Stage 4: the first observable capability matrix

`pkg/fgdbtest/capabilities` uses the existing process-identity-aware cluster launcher. It starts one disposable local node and records these outcomes separately in the normal report format:

| Claim | Check | Passing evidence |
| --- | --- | --- |
| OSS-001 | Candidate version identity | The supplied binary reports `Distribution: OSS`; `FGDB_EXPECT_GO`, when set, must match the existing version helper's check. |
| OSS-002 | CSV IMPORT through the OSS path | A loopback HTTP fixture supplies two rows; the imported table contains exactly those IDs and values, with no extra rows. |
| OSS-003 | Unsupported SQL BACKUP | `BACKUP INTO` returns SQLSTATE `XXC01` and the OSS statement-gate message. Success, syntax errors, permission errors, and connection failures all fail this check. |

`REP-001` records the separate hook regression. These new registry entries remain `partial` until the recorded tests actually run. Adding tests does not itself demonstrate a passing candidate.

The HTTP fixture serves a fixed in-memory CSV, supports HEAD and ranges through Go's standard HTTP handler, and listens on loopback. It accepts no external URL or file path. SQL values are read through pgx, including structured PostgreSQL errors.

## PAR-001 CCL gate contract

The runtime slice also records the 24 active PAR-001 gates as separate claims: GATE-001..GATE-005, GATE-007..GATE-013, and GATE-015..GATE-026. These table-driven cases exercise the approved statement, hook, and builtin CCL gates and require SQLSTATE `XXC01` plus the spec's message substring. Setup, transport, parser, permission, and other mismatches fail closed; a setup failure leaves later gate claims `not_tested`. The corrected replication form is runtime-observed against the released OSS binary (spec v1.3). Retired grammar and unreachable OSS rows are excluded from the claim set. CSV and Parquet export parity rows are not gate claims; PARITY-001 is deferred in this slice.

## Run it

Cluster-free helper/unit checks:

```bash
go test -count=1 ./pkg/fgdbtest/capabilities ./pkg/fgdbtest/harness
bash fgdb/test/run.sh pr
```

`TestOSSCapabilities` explicitly skips when no binary is supplied on this optional unit path. That skip is not runtime capability evidence. The required runtime path fails when the binary is missing:

```bash
FGDB_REQUIRE_CLUSTER=1 \
FGDB_COCKROACH=/absolute/path/to/cockroach \
FGDB_EXPECT_GO=go1.21.12 \
go test -count=1 -v -timeout 15m ./pkg/fgdbtest/capabilities -run '^TestOSSCapabilities$'
```

Use the OSS binary from the verified release archive or an OSS candidate build. Keep its `lib/` directory beside it. The cluster helper applies the existing environment allowlist and release-library handling. The test never connects to a user-supplied live database URL.

Set `FGDB_OUTPUT` to retain `result.json` and `summary.md` outside the Go test's temporary directory. Report `source_sha` identifies the suite checkout supplied by the caller; `candidate_sha256` identifies the supplied candidate tarball. They are different provenance fields. Do not describe a pinned-release run as a test of a newly built database candidate.

`fgdb/test/run.sh interim` (also nightly and weekly) runs the same capability implementation and merges its per-claim outcomes into the tier report. The PR tier runs only its helper tests. The required capability step reports missing setup and omitted checks explicitly, and fails the tier rather than accepting a skipped runtime check.

The patch-test workflow runs `TestOSSCapabilities` against its existing checksum-pinned `v23.2.15-fgdb.1` binary after the backup integration tests. It uploads the capability report on completion. It does not build the changed server; the manual compile and interim candidate run provide that separate evidence.

## Boundaries and cleanup

All node stores are temporary and owned by this test. SQL and startup have bounded contexts. Shutdown uses the existing stop and final-reap mechanisms; cleanup errors fail the run. A failure before a check leaves that claim `not_tested` rather than marking it successful.

This adds no secure-mode, upgrade, network-partition, disk-fault, TTL, GEOS, auth-fallback, or full statement-coverage claim. The report keeps its existing partition/disk NOT-tested lines. Expand those capabilities in later slices with their own evidence and cleanup constraints.
