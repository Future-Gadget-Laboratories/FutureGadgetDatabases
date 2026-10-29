> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-005 — `EXPORT INTO` CSV / PARQUET on `cockroach-oss`

| Field | Value |
| --- | --- |
| Spec id | PAR-005 (export lane) |
| Version | v1 |
| Date | 2026-10-26 |
| Role | spec writer (cleanroom; see [CLEANROOM.md](../CLEANROOM.md)) |
| Skill bundles | `database-fgdb@1.0.1`, `impl-functional-design@1.0.1`, `impl-documentation@1.0.1` |
| Sources consulted | This checkout; [oss-capabilities.md](../oss-capabilities.md); public CockroachDB v23.2 EXPORT docs (behavior only) |

## Builder cleanroom boundary (mandatory)

These are behavior-only draft specifications; references to this allowed OSS tree are context, not a substitute for behavioral requirements. Before implementer handoff, a maintainer must review for expression leakage and approve the spec under [CLEANROOM.md](../CLEANROOM.md); record the writer, reviewer, approval, spec version and eventual implementer in the audit trail. Approval is pending.

Oracle baseline provenance: stock CockroachDB OSS v23.2.15 binary, with version and distribution recorded alongside each capture. Record reproducible inputs, settings, outputs or SQLSTATE and message, and capture date in versioned conformance vectors; never copy source or upstream test files. The stock OSS baseline may expose a stub rather than the desired success behavior; it does not establish missing success vectors. Any additional permitted behavioral oracle and its provenance must be identified and approved by the maintainer. Pending oracle data and approval are gates before handoff and implementation, not assumed evidence.

Same as PAR-005 workload loader: Apache-2.0 v23.2.15 tree only; no `pkg/ccl/`; no newer CRDB sources; no GitHub source browsing. Public docs cite **behavior** only.

## 1. Goal, acceptance criteria, GATE decision

**Goal.** Document and lock parity for `EXPORT INTO CSV` and `EXPORT INTO PARQUET` on `cockroach-oss`, with FGDb guard tests and conformance evidence. This lane does **not** assume EXPORT is missing — the survey shows a full OSS implementation — but FGDb must prove it and handle existing admin/feature-flag gates.

**Acceptance criteria.**

1. **EXP-001 (CSV).** `EXPORT INTO CSV 'nodelocal://1/<testdir>/' FROM SELECT 1` succeeds on a disposable OSS node with default cluster settings; output files exist and contain expected CSV bytes (oracle-recorded).
2. **EXP-002 (PARQUET).** Same for `EXPORT INTO PARQUET` with a simple typed row; parquet readable per oracle (row count / column value).
3. **EXP-004 (not CCL gate).** With default `feature.export.enabled` **true**, minimal `EXPORT INTO CSV ... FROM SELECT 1` does **not** return statement-gate `XXC01` / `a CCL binary is required to use this statement type: *tree.Export` on OSS (`*tree.Export` is `cclOnlyStatement` in `pkg/sql/sem/tree/stmt.go` but absent from the opaque list in `pkg/sql/opaque.go`). EXP-004 must run **while the flag is still default-on**, before any disable case — with the flag false, `ConstructExport` returns SQLSTATE `57000` first (`pkg/sql/export.go:116`) and EXP-004 would falsely pass on the wrong error class.
4. **EXP-003 (feature flag).** After EXP-001, EXP-002, and EXP-004 succeed, `SET CLUSTER SETTING feature.export.enabled = FALSE`; EXPORT fails with SQLSTATE `57000` (operator intervention) and message containing `feature EXPORT was disabled by the database administrator` (matches `pkg/sql/export.go` and `TestExportFeatureFlag` at `pkg/sql/importer/exportcsv_test.go:440`). Re-enable the setting (`= true`) before the multi-statement negative case.
5. **Claims.** New `fgdb/test/capabilities` or conformance rows; claims.yaml entries with tests.
6. **M-BULK alignment.** Export leg of M-BULK (fixed dataset round-trip) PENDING vs approved success oracle on build host.

**GATE contract decision: no new PAR-005 feature flag.**

| Item | Decision |
| --- | --- |
| New FGDb flip? | **No.** EXPORT is already OSS-implemented (`feature.export.enabled` in `pkg/sql/export.go`, default on). There is no in-tree PAR-001 parity table to cite; document gates from OSS paths below. |
| Existing gates | Keep upstream `feature.export.enabled` (default on) and privilege / `nodelocal` rules unchanged. |
| Flag-off contract | Disabling `feature.export.enabled` must preserve today's **admin disable** error (not the CCL loader stub). Document in conformance; do not conflate with `XXC01`. |

**Reasons.** Unnecessary duplicate flag would confuse PAR-001; correct contract is parity success + documented operator-disable path.

## 2. Verified survey (this tree only)

| Path | What it implies |
| --- | --- |
| `pkg/sql/sem/tree/stmt.go` (`(*Export) cclOnlyStatement`, ~1165) | Marker on `*tree.Export` — **not** used for OSS planning path (see below). |
| `pkg/sql/opaque.go` (`init` opaque list, ~405–417) | Comment: CCL statements **without Export** which has an optimizer operator. `*tree.Export` is **absent** from opaque registration. |
| `pkg/sql/opaque.go` (`case tree.CCLOnlyStatement`, ~287–292) | Statement-gate `XXC01` applies only to opaque-routed CCL statements. |
| `pkg/sql/opt/optbuilder/export.go` (`buildExport`) | Optimizer builds `Export` relational operator. |
| `pkg/sql/opt/exec/execbuilder/statement.go` (`buildExport`, ~379+) | Calls `exec.Factory.ConstructExport`. |
| `pkg/sql/export.go` (`ConstructExport`, `featureExportEnabled`, ~97–131) | Runtime planning: admin feature flag, single-stmt txn check, CSV/parquet suffix validation, privilege / external IO checks, client NOTICE. |
| `pkg/sql/distsql_physical_planner.go` (`createPlanForExport`, ~4697+) | DistSQL physical plan adds `Exporter` processor. |
| `pkg/sql/importer/exportcsv.go`, `exportparquet.go`, `export_base.go` | CSV and Parquet writer processors and flow integration. |
| `pkg/sql/importer/exportcsv_test.go`, `exportparquet_test.go` | Extensive OSS tests including `TestExportFeatureFlag` (~438–458). |
| `pkg/sql/logictest/testdata/logic_test/export` | Logic tests for CSV/PARQUET EXPORT and NOTICE trace. |
| `pkg/sql/importer/import_stmt_test.go` (`TestExportImportRoundTrip`) | Round-trip export/import behavior in OSS tests. |

**Gating summary for `cockroach-oss` today.**

| Gate type | Active on OSS? | Mechanism |
| --- | --- | --- |
| CCL statement gate (`XXC01`, `*tree.Export`) | **No** (not reachable via opaque planner) | `stmt.go` + `opaque.go` survey |
| `feature.export.enabled` | **Yes** (default true) | `pkg/sql/export.go` + `featureflag.CheckEnabled` |
| Privilege / external IO | **Yes** | `ConstructExport` admin or `EXTERNALIOIMPLICITACCESS` or explicit-auth URIs |
| Multi-statement txn | **Yes** | Error if not single-statement txn |

## 3. Deliverables (paths and size targets)

| Deliverable | Path | Target size |
| --- | --- | --- |
| Capability checks | `pkg/fgdbtest/capabilities/export.go` (new, Apache-2.0 header per `pkg/fgdbtest`) | ~80–120 LOC; **`runExportChecks` (or equivalent) called from `runCluster`** in `pkg/fgdbtest/capabilities/run.go`, with errors joined into the same `errors.Join` as `importErr` / `backupErr` (~line 101). `newReport` seeds EXP-001..004 as `not_tested`; on the **success path** require all four ids **`pass`** — `Rejected()` is true for required steps that stay `not_tested`, but `TestOSSCapabilities` stays green if export checks are never invoked. Register EXP-* in `newReport`; extend `TestMissingBinaryDoesNotPass` for new claim IDs. **Order inside one cluster session:** EXP-001, EXP-002, EXP-004 (flag default on) → disable flag → EXP-003 → re-enable flag → multi-statement negative. Use harness **disk** store so `nodelocal://1/` works (`--external-io-dir` is not set for in-memory stores). |
| Claims | `fgdb/test/claims.yaml` | Full rows per §5 (all registry fields). |
| Conformance oracle | `fgdb/test/conformance/par005-export/` | Recorded CSV bytes + parquet metadata from stock OSS v23.2.15 baseline. |
| Doc cross-link | One paragraph in `docs/fgdb/oss-capabilities.md` (follow-up PR if required) | State EXPORT parity is guarded, not BACKUP-gated. |

**No** new EXPORT implementation in `pkg/ccl`. If bugs are found vs oracle, patch only `pkg/sql/export.go` / `pkg/sql/importer/export*.go`.

## 4. Build, tests, conformance, scan gate

**Build.**

```bash
./dev build oss
```

**Upstream OSS tests (representative).**

```bash
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/importer -run 'TestExportFeatureFlag|TestExportImportBank' -timeout 30m
```

**FGDb capability (after implementer adds EXP-*).**

```bash
FGDB_REQUIRE_CLUSTER=1 FGDB_COCKROACH=/absolute/path/to/cockroach-oss \
  go test -count=1 -v -timeout 15m ./pkg/fgdbtest/capabilities -run 'TestOSSCapabilities'
```

**Conformance cases (public docs + oracle).** Same order as capability checks (flag on for success + EXP-004 before disable).

1. Minimal CSV export (single column) — CockroachDB v23.2 EXPORT docs baseline (EXP-001).
2. PARQUET export with `compression = gzip` option (one case) (EXP-002).
3. EXP-004: explicitly inspect the minimal CSV export result and assert it is not `XXC01` with `a CCL binary is required to use this statement type: *tree.Export` while `feature.export.enabled` is default **true**.
4. `SET CLUSTER SETTING feature.export.enabled = false` — SQLSTATE `57000` and operator intervention message (EXP-003).
5. `SET CLUSTER SETTING feature.export.enabled = true` again, then negative: `EXPORT` inside explicit multi-statement txn — exact error: `EXPORT cannot be used inside a multi-statement transaction` (`pkg/sql/export.go:134`).
6. Negative control: same session `BACKUP` still `XXC01` (proves suite distinguishes gates).

**Scan gate.** Same `/tmp/par005-scope` + `easyscan-scan` pattern as workload loader spec; include only changed `pkg/fgdbtest/**` and spec file.

## 5. Risks, maintainer questions, PENDING

**Risks.**

- **False assumption EXPORT is gated:** Teams may expect `XXC01`; document `stmt.go` / `opaque.go` survey so “ungating” work is not mis-scoped.
- **File layout / `nodelocal` paths:** Tests must use temp external IO dir from harness, not hard-coded ports.
- **NOTICE text:** Export emits deprecation NOTICE; conformance should allow optional NOTICE match per logic test.
- **Parquet compatibility:** Apache parquet in `exportparquet.go` — oracle compares hashes, not hand-rolled expectations.

**Maintainer questions.**

1. Is M-BULK export leg in scope for the same PR as capabilities or a follow-up benchmark PR?
2. *(Reviewer: dedicated `TestOSSExport` — **declined**; spec requires `runCluster` integration so published `TestOSSCapabilities` exercises EXP-*.)*

**Claim-to-check mapping.** EXP-001 asserts CSV success and expected output bytes; EXP-002 asserts PARQUET success and the expected readable row; EXP-003 asserts SQLSTATE `57000` and the administrator-disabled message with the flag off. EXP-004 explicitly inspects the default-on CSV statement result and asserts it is not SQLSTATE `XXC01` with `a CCL binary is required to use this statement type: *tree.Export`. The same execution may support EXP-001 and EXP-004, but record each assertion separately; absence of that gate alone does not establish export success.

**Claims registry examples (`EXP-001`..`EXP-004`).** Each row uses `tests: pkg/fgdbtest/capabilities:TestOSSCapabilities` and `source` under `docs/fgdb/` (e.g. this spec or `oss-capabilities.md` once cross-linked).

```yaml
- id: EXP-001
  claim: EXPORT INTO CSV to nodelocal succeeds on cockroach-oss with default settings.
  source: docs/fgdb/spec/PAR-005-export.md
  category: capabilities
  tests:
    - pkg/fgdbtest/capabilities:TestOSSCapabilities
  tier: interim
  status: partial
- id: EXP-002
  claim: EXPORT INTO PARQUET succeeds on cockroach-oss with default settings.
  source: docs/fgdb/spec/PAR-005-export.md
  category: capabilities
  tests:
    - pkg/fgdbtest/capabilities:TestOSSCapabilities
  tier: interim
  status: partial
- id: EXP-003
  claim: Disabling feature.export.enabled returns operator intervention on EXPORT.
  source: docs/fgdb/spec/PAR-005-export.md
  category: capabilities
  tests:
    - pkg/fgdbtest/capabilities:TestOSSCapabilities
  tier: interim
  status: partial
- id: EXP-004
  claim: EXPORT does not hit the CCL statement gate on cockroach-oss.
  source: docs/fgdb/spec/PAR-005-export.md
  category: capabilities
  tests:
    - pkg/fgdbtest/capabilities:TestOSSCapabilities
  tier: interim
  status: partial
```

**PENDING.**

- Run importer export tests on build host.
- Record stock OSS v23.2.15 oracle for CSV/PARQUET bytes.
- Benchmark M-BULK export segment vs B0-full.

## 6. Review checklist

- [ ] Export checks run from `runCluster` with `errors.Join`; success path requires EXP-001..004 all `pass`.
- [ ] Check order: EXP-001/002/004 with flag on → EXP-003 with flag off → re-enable → multi-statement negative.
- [ ] Tests prove success path, not only absence of `XXC01` on a disabled flag.
- [ ] Harness uses disk store / `nodelocal://1/`, not in-memory-only cluster.
- [ ] `feature.export.enabled = false` case included.
- [ ] No new cluster version keys or persisted format changes.
- [ ] Claims registry updated; capability report format unchanged.
- [ ] Cleanroom boundary stated in PR.
- [ ] easyscan on scoped copy only.
