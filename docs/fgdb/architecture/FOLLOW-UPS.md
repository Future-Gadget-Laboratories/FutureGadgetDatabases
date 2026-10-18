> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Architecture verification work queue

This queues the numbered questions in [OPEN-QUESTIONS.md](OPEN-QUESTIONS.md). It does not close them or claim runtime coverage. The map was reconciled against `main` at `2d2befea10288726b8da5654031a503acfb2b512` on 2026-10-17. Recheck the source and PR heads when work starts.

The Phase 0 suite is documented in [testing.md on main](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/blob/main/docs/fgdb/testing.md). On main it lives under `fgdb/test/` and `pkg/fgdbtest/`. Those paths are not in this branch. Extend that suite and its claim registry rather than creating another harness. The `v23.2.15-fgdb.1` pin is recorded in [releases.md on main](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/blob/main/docs/fgdb/releases.md). [releases.md](../releases.md) in this branch documents `v23.2.15-oss`. Architecture documentation can land with open verification questions, provided the evidence limits remain explicit.

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

## Skills and native tooling

For agents with the supplied collection installed, start with `cb-project-engineering`. For everyone else, the repository's own build/test instructions remain the executable workflow; these skills are guidance, not new dependencies.

| Work | Applicable skill guidance | Native implementation/checks |
| --- | --- | --- |
| All architecture follow-ups | `code-review-checklist`, relevant stack add-ons, `code-review-static-tools` | Existing Go tests, Bazel targets, claims checker, `go vet`; compatible staticcheck if installed. |
| Review an existing or new PR | `code-review-composer-index` plus its checklist and static tools | Exact base/head, prior findings, concrete test results; independent final review when required. |
| CI/build wiring | Review add-ons and static tools | actionlint, ShellCheck, existing fixture tests; preserve manual self-hosted execution and checkout restrictions. |
| Push or ready code | `local-sonar-scan` | Record the actual local Sonar quality gate, or leave scan status pending. |
| Later performance investigation | `benchmarking-advisor`, only for the methodology/tool in use | Go benchmarks/profiling and representative SQL workloads. No unmeasured speedup claims. |

FGDb is primarily Go. The supplied C++23 and .NET overlays do not authorize a toolchain migration, and CipherBank's `make-module` generator is not FGDb's Go/Bazel module workflow. Use Python or C/C++ specialists only when changing those actual components.

## Separate feature tracks

These architecture tests establish existing OSS behavior. They do not implement Nexus compatibility, PL/pgSQL, incremental backup, or point-in-time restore. Those features need separately scoped work and, where required, maintainer-approved behavior specifications under the repository's cleanroom process.
