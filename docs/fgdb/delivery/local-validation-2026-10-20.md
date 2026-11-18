> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Local validation: 2026-10-20

This records actual local execution while preparing the documentation stack. The implementation under test is main `2d2befea10288726b8da5654031a503acfb2b512`; only documentation changed in the stack. It does not test unmerged #17/#24 implementation or approve Enterprise parity.

## Toolchain and binary provenance

- Local compiler: `go version go1.21.12 linux/amd64`; `GOTOOLCHAIN=local`.
- Official archive: `https://go.dev/dl/go1.21.12.linux-amd64.tar.gz`.
- Verified archive SHA256: `121ab58632787e18ae0caa8ae285b581f9470d0f6b3defde9e1600e211f583c5`.
- Test server archive: FGDb release `v23.2.15-fgdb.1`, as pinned in [releases.md](../releases.md).
- Verified server archive SHA256: `5f9c36263bdbbfd1d8e6781fe09920fac33d4c608b4743be6681009f4215a379`.
- Executed server identity: Build Tag `v23.2.15`, Distribution `OSS`, Platform `linux amd64`, Go Version `go1.21.12 X:nocoverageredesign`, Build Type `release`.
- The binary's embedded Build Commit ID is `a72022ff42bbceb6f0865b3b07e6fd6a9e94a6e1`. This is distinct from the release-packaging source commit in releases.md; preserve both rather than inferring the embedded server was rebuilt from a documentation candidate.

The Go toolchain, dependency caches and extracted OSS release live outside the source checkout. A local activation file sets PATH and cache directories; workstation paths and raw logs are not committed here.

## Executed checks

1. `go test -count=1 -timeout 5m -json ./pkg/fgdbtest/...`: **exit 0**. Captured 44 passing named test/subtest events and one named skip, `TestThreeNodeSlice`. Twelve packages reported pass; three packages had no tests. `FGDB_COCKROACH` was unset for this command. The three-node scenario was not executed.
2. `go vet ./pkg/fgdbtest/... ./pkg/cmd/fgdb-test/...`: **exit 0**.
3. In `pkg/cmd/fgdb-backup`, `go test -count=1 -timeout 5m ./...` without a binary: **exit 0**. This initial unit run does not count as the cluster evidence.
4. In the same module, with `FGDB_COCKROACH` pointing to the verified OSS release and its libraries, `go test -count=1 -timeout 20m -json ./...`: **exit 0**, package elapsed 114.049 seconds. Captured 68 passing named test/subtest events, zero failed and zero skipped. This run includes the existing cluster-backed backup tests on disposable local nodes.
5. In the backup module, `go vet ./...`: **exit 0**.
6. `bash fgdb/test/run.sh pr`: **exit 0**. Reported CHECK-CLAIMS, CHECK-IMPORTS and UNIT pass. The report explicitly says network partitions and disk faults were not tested.
7. Local activation file shell syntax check: **exit 0**. Documentation and register checks are recorded in the PR description after final validation.

Cold dependency downloads and loopback socket fixtures required the approved host execution path. A sandbox-only helper-suite repeat failed to bind `127.0.0.1:0`; the host rerun passed. Preserve that distinction rather than treating an environmental refusal as a database regression.

Counts refer to Go's named test/subtest result events, including fuzz seed cases where emitted; they are not statement coverage, feature count or a percentage of Enterprise functionality.

## Not established by these checks

- No full candidate server Bazel compile or core-wide test run.
- No execution of PR #24's new `repstream_test` target or base-failing/head-passing hook regression.
- No execution of the three-node interim suite, real network partition, disk-fault or clock-skew campaign.
- No secure multi-node deployment, region survival, cross-key serializability proof or rolling-upgrade run.
- No Enterprise comparator runtime or excluded-feature conformance.
- No release publication or CI Sonar quality-gate claim.
- Existing backup tests do not cover all the value and destructive-failure cases required by B01/B02; a green baseline does not erase the known #17 findings.

Raw local JSONL results and the PR-tier summary are retained in the task's analysis artifacts. A future CI run should attach equivalent results to the candidate revision so evidence is independently retrievable, rather than treating this local record as a permanent CI gate.
