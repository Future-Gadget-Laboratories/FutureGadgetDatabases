> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Architecture verification work queue

This queues the numbered questions in [OPEN-QUESTIONS.md](OPEN-QUESTIONS.md). It does not close them or claim runtime coverage. The map was reconciled against `main` at `2d2befea10288726b8da5654031a503acfb2b512` on 2026-10-17. Recheck the source and PR heads when work starts.

The Phase 0 suite is documented in [testing.md](../testing.md) and lives under `fgdb/test/` and `pkg/fgdbtest/`. Extend that suite and its claim registry rather than creating another harness. The `v23.2.15-fgdb.1` release (source commit `56ce139286c504cd61ccc1da1d8e03b644afa131`, tarball sha256 `5f9c36263bdbbfd1d8e6781fe09920fac33d4c608b4743be6681009f4215a379`) and the earlier `v23.2.15-oss` release are recorded in [releases.md](../releases.md). Architecture documentation can land with open verification questions, provided the evidence limits remain explicit.

## Order and completion evidence

All rows are proposed and unstarted. No owner has been assigned.

| Work item | Open questions | Scope | Completion evidence |
| --- | --- | --- | --- |
| ARCH-01 | 1–3, 8, 13 | Start an OSS capability matrix: version identity, a supported IMPORT case, and unsupported statements. Expand the enterprise-gate caller inventory using only the OSS side. | Pinned OSS binary and exact statements; expected result or SQLSTATE per case; independent test results and linked claims. Do not assume every unsupported statement uses the same error path. |
| ARCH-02 | 9 | Investigate the ingest hook guard in `pkg/repstream/api.go`. | Confirm the caller contract. A base-failing/head-passing regression covers both hooks nil, only the producer hook, only the ingest hook, both hooks, and returned errors. No CCL implementation is needed. Restore global hooks after each test and do not run those cases in parallel. |
| ARCH-03 | 4–7, 12, 14 | Extend capability tests to TTL, GEOS present/missing, auth fallback, license-enforcer behavior, and explicit versus implicit schema changes. | Disposable cluster/store and certificate fixtures; bounded cleanup; actual results and errors. Fault and enforcer experiments never target a shared cluster. |
| ARCH-04 | 10, 11, 15, 16 | Trace span-config propagation, engine ownership, vectorized fallback, and commit paths. | Version-pinned traces or targeted tests linked to the relevant source. Treat two-round-trip drawings as explanatory shapes, not measured timings. Do not infer behavior from package comments alone. |
| ARCH-05 | 17, 20 | Triage historical capacity language and conflicting comments. | Source-backed corrections or clearly labeled unresolved historical statements. Keep documentation claims separate from measured capacity or performance. |
| ARCH-06 | 18, 19 | Preserve source boundaries and distinguish build/publish evidence. | No proprietary implementation input. Record source SHA, artifact checksums, run links, and which checks ran. A compile-only run does not demonstrate publishing or a full interim pass. |

The original question numbers remain stable. Record each result with its test identifier, binary/source revision, command, exit code, and artifact or run link. A test merely existing, a skipped test, or an unavailable dependency is not passing evidence. Update the public claims only when the corresponding evidence supports the change.

## Before writing the first test

1. Read the current source, applicable repository instructions, and existing nearby tests.
2. Check which PR tier and workflow paths will actually run the new package. For example, a new `pkg/repstream` regression is outside the current FGDb-specific fast-check package list and pull-request path filters unless explicitly wired in.
3. Add the smallest observable assertion. Keep test-only code out of the release binary and preserve the `cockroach-oss` disallowed-imports gate.
4. Run the scoped tests and the relevant existing checks. Report unavailable tools and omitted cluster checks explicitly.

## Tooling

Use the repository's own build and test instructions. They are the executable workflow; nothing here adds a new dependency.

| Work | Checks |
| --- | --- |
| All architecture follow-ups | Existing Go tests, Bazel targets, the claims checker, `go vet`; a compatible staticcheck if installed. |
| Review an existing or new PR | Exact base/head, prior findings, concrete test results; an independent final review when required. |
| CI/build wiring | actionlint, ShellCheck, existing fixture tests; preserve manual self-hosted execution and checkout restrictions. |
| Push or mark ready | Record the actual static-analysis quality gate result, or leave its status pending. |
| Later performance investigation | Go benchmarks/profiling and representative SQL workloads. No unmeasured speedup claims. |

FGDb is primarily Go. Do not migrate toolchains or adopt another project's module generator; Go/Bazel is the module workflow here. Change Python or C/C++ only where those actual components need it.

## Separate feature tracks

These architecture tests establish existing OSS behavior. They do not implement third-party product compatibility layers, PL/pgSQL, incremental backup, or point-in-time restore. Those features need separately scoped work and, where required, maintainer-approved behavior specifications under the repository's cleanroom process.
