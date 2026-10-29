> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-005 — Workload `IMPORT` fixture loader on `cockroach-oss`

| Field | Value |
| --- | --- |
| Spec id | PAR-005 (workload loader lane) |
| Version | v1 |
| Date | 2026-10-26 |
| Role | spec writer (cleanroom; see [CLEANROOM.md](../CLEANROOM.md)) |
| Skill bundles | `database-fgdb@1.0.1`, `impl-functional-design@1.0.1`, `impl-documentation@1.0.1` |
| Sources consulted | This checkout (OSS v23.2.15 tree); [oss-capabilities.md](../oss-capabilities.md); operator inputs `par005-inputs.md`; public CockroachDB v23.2 `IMPORT` / workload docs (behavior only); PAR-001 gate-contract pattern (co-locate under `docs/fgdb/spec/` when present) |

## Builder cleanroom boundary (mandatory)

These are behavior-only draft specifications; references to this allowed OSS tree are context, not a substitute for behavioral requirements. Before implementer handoff, a maintainer must review for expression leakage and approve the spec under [CLEANROOM.md](../CLEANROOM.md); record the writer, reviewer, approval, spec version and eventual implementer in the audit trail. Approval is pending.

Oracle baseline provenance: stock CockroachDB OSS v23.2.15 binary, with version and distribution recorded alongside each capture. Record reproducible inputs, settings, outputs or SQLSTATE and message, and capture date in versioned conformance vectors; never copy source or upstream test files. The stock OSS baseline may expose a stub rather than the desired success behavior; it does not establish missing success vectors. Any additional permitted behavioral oracle and its provenance must be identified and approved by the maintainer. Pending oracle data and approval are gates before handoff and implementation, not assumed evidence.

- After approval, implement only from this spec, public documentation, and Apache-2.0 paths in this tree (v23.2.15 base).
- **Never** read, list, grep, or import `pkg/ccl/`, `pkg/ui/distccl/`, or any CockroachDB source newer than v23.2.15. Do not browse `cockroachdb/cockroach` on GitHub.
- Do not paste or paraphrase proprietary Enterprise/CCL implementation from memory or external trees. Rebuild behavior from observable SQL and the OSS importer/workload code cited below.
- Record in the PR that no CCL/CSL code was copied (license checklist).

## 1. Goal, acceptance criteria, GATE decision

**Goal.** Restore stock v23.2.15 behavior where `cockroach workload init` with default `--data-loader=AUTO` selects the fast `IMPORT` path for fixture-capable workloads (for example `tpcc`), so benchmarks and soak tests can load comparable data without forcing `--data-loader=INSERT` on every system.

**Acceptance criteria.**

1. **TOOL-001.** On a loopback single-node `cockroach-oss` cluster, `cockroach workload init tpcc --warehouses=1` with default `AUTO` completes without error and leaves a populated `tpcc` schema (row counts consistent with warehouse=1 expectations from the workload generator in this tree). Run the workload `PostLoad` hook after all imports complete; assert that the expected tpcc foreign key constraints exist and are unvalidated (`NOT VALID`), comparing their definitions and validation state with the approved oracle.
2. **GATE-off contract.** For AUTO selecting IMPORT or explicit `--data-loader=import`, read the setting using a connection to an existing database before `runInitImpl` creates the workload database and before `Setup` invokes `PreCreate`. If false, return `loading initial data with IMPORT requires a CCL binary` without creating the database or running hooks, table creation, or IMPORT. This deliberately removes the earlier command’s database/PreCreate side effects. INSERT and AUTO selecting INSERT are unaffected; ad-hoc server IMPORT remains available.
3. **INSERT unchanged.** `--data-loader=INSERT` behavior and performance characteristics remain unchanged when the flag is on or off.
4. **Conformance.** FGL conformance cases (recorded oracle from stock v23.2.15 OSS baseline and an approved success oracle for URI shape, job completion, and row fingerprints) pass on FGDb with flag on; flag-off cases match today's error.
5. **Claims.** `fgdb/test/claims.yaml` lists TOOL-001 with at least one test; interim tier can run the integration test when a binary is supplied.
6. **Benchmarks (PENDING on build host).** Numeric M-BULK / M-TPCC-D comparison versus approved success oracle is **out of scope for this spec** (`BENCHMARK-PLAN` is not in this tree). Record ingest duration in PENDING notes; a follow-up benchmark PR may define thresholds.

**GATE contract decision: yes — check before command side effects.**

| Item | Decision |
| --- | --- |
| Mechanism | Public cluster setting `feature.workload_import_loader.enabled` (name negotiable with maintainer; must be `settings.ApplicationLevel`, default `true`, documented). Register the setting in the **OSS-only wiring package** (below) so `cockroach-oss` still exposes it. In `pkg/workload/cli/run.go` `runInitImpl`, move AUTO resolution and loader selection ahead of the existing `DROP DATABASE` and `CREATE DATABASE` statements; for an IMPORT selection, query the setting through `initDB` connected to an existing database and return the loader error if false. Only then may the command drop or create the workload database and call `workloadsql.Setup` (which invokes `PreCreate`). Workload init and the deprecated workload run initialization path use `runInitImpl`; any fixtures or other entry point that can select this loader must enforce the same preflight before those side effects. |
| Flag **on** | `workload.ImportDataLoader` runs the cleanroom OSS implementation; server uses existing `IMPORT INTO` + `workload://` support in `pkg/sql/importer`. |
| Flag **off** | The command returns today's loader error string unchanged before database creation and hooks (`loading initial data with IMPORT requires a CCL binary`) and does not run `IMPORT`. Server `IMPORT` for HTTP/`nodelocal` (OSS-002) stays unaffected. |
| PAR-001 linkage | This is a **new** PAR-0xx flip for workload-loader only; update PAR-001 maintainer notes when the setting lands. Not a statement-gate `XXC01` row. |

**Reasons.** Rollback without reverting the binary; explicit parity test for “today's error”; separates CLI fixture loading from server-side `IMPORT INTO` already proven by OSS-002.

## 2. Verified survey (this tree only)

| Path | What it implies |
| --- | --- |
| `pkg/workload/cli/run.go` (`runInitImpl`, ~323–345) | `AUTO` → `import` when `workload.SupportsFixtures(gen)`; loader is `workload.ImportDataLoader`. |
| `pkg/workload/workload.go` (`ImportDataLoader`, `requiresCCLBinaryDataLoader`, ~370–379) | Default loader returns `loading initial data with %s requires a CCL binary` — **only** the workload CLI hook is missing on OSS. |
| `pkg/workload/workload.go` (`SupportsFixtures`, ~47–57) | Fixtures require every table to have `InitialRows.FillBatch != nil`. |
| `pkg/cli/cli.go` → `pkg/workload/cli/run.go` → `workloadsql` | Every `cockroach-oss` / `cockroach` / `cockroach-short` and `go test` importing `pkg/cli` already loads `workloadsql`; **`init` that sets `ImportDataLoader` must not live there** (races CCL hook on the same var). |
| `pkg/workload/workloadsql/workloadsql.go` (`Setup`) | Calls `PreCreate`, then `InitialDataLoad`, then splits, then `PostLoad` — **does not** create tables itself. |
| `pkg/fgdbtest/cluster/sql.go` (`WorkloadURL` vs `SQLURL`) | `WorkloadURL` has **no** database segment (required for `tpcc` init); `SQLURL` uses `/defaultdb` and fails `SanitizeUrls` for tpcc. |
| `pkg/workload/workloadsql/dataload.go` (`InsertsDataLoader`) | **`CREATE TABLE`** and INSERT load live here; import loader must mirror the same `CREATE TABLE IF NOT EXISTS` schema strings, then `IMPORT INTO` (table must exist before import). |
| `pkg/sql/importer/import_planning.go` (`importPlanHook`, `init` `AddPlanHook("import", ...)`, ~357+, ~1236–1237) | **`IMPORT INTO` is planned on OSS** via plan hook (not the opaque `XXC01` statement gate). |
| `pkg/sql/importer/read_import_workload.go` (`parseWorkloadConfig`, `workloadReader`, ~133+) | Server parses `workload:///<format>/<generator>/<table>?version=...` and reads generator data in-process — **no CCL package required**. Generator flags are parsed onto a **new** generator instance from URI query params only (CLI `--warehouses` etc. are **not** inherited unless duplicated in the URI). `row-end` is an **exclusive** batch index (`BatchEnd`); use `InitialRows.NumBatches`, not a row count. Stop when batch index reaches `row-end` (~259, ~327–332). |
| `pkg/sql/importer/import_processor.go` (~309–319) | `workloadReader` is selected only when the import file format is **CSV** — fixture loads must use the `csv` segment in the URI. |
| `pkg/cloud/uris.go` (~62–63) | `workload` scheme treated as non-secret for URI display. |
| `pkg/cli/zip_test.go` (~999) | Shortened `rows=` example — **do not copy** (do-not-copy oracle). Conformance must use the full `CSV DATA` shape in §2.1. |
| `docs/fgdb/oss-capabilities.md` (OSS-002) | HTTP CSV `IMPORT INTO` already a capability claim; workload path is orthogonal client wiring. |

**Implication.** The server can ingest `workload://` URIs on `cockroach-oss` today; PAR-005 is almost entirely a **cleanroom `InitialDataLoader`** plus wiring `workload.ImportDataLoader` for the OSS binary.

### 2.1 Required `IMPORT INTO` URI (per table)

After `CREATE TABLE IF NOT EXISTS`, issue **one** statement per fixture table (same `WITH` / job polling as stock oracle). Format is fixed:

```sql
IMPORT INTO <table> CSV DATA (
  'workload:///csv/<generator>/<table>?version=<gen.Meta().Version>&row-start=0&row-end=<InitialRows.NumBatches>&<flag>=<value>'
)
```

| Param | Rule |
| --- | --- |
| `<format>` | Must be `csv` (workload reader path). |
| `version` | `gen.Meta().Version` from the workload generator in this tree (e.g. `tpcc` → `2.2.0`, `bank` → `1.0.0`). |
| `row-start` | `0` for full fixture load. |
| `row-end` | `table.InitialRows.NumBatches` (exclusive batch bound — **not** total row count). |
| `<flag>=<value>` | Repeat every generator flag the stock CLI would pass (e.g. `warehouses=1` for `tpcc`); omit only when the generator has no flags. |

Record the exact URI strings in conformance oracle JSON from the approved success oracle; FGDb must match param-for-param.

## 3. Deliverables (paths and size targets)

| Deliverable | Path | Target size |
| --- | --- | --- |
| OSS import loader implementation | `pkg/workload/workloadsql/import_loader.go` (new, ~120–200 LOC, Apache-2.0 header per `pkg/fgdbtest`) | Preflight in the command before database creation and `PreCreate`; package `workloadsql`: `InitialDataLoad` (per table `CREATE TABLE IF NOT EXISTS`, then `IMPORT INTO` per §2.1). **No** `init` that assigns `workload.ImportDataLoader` in this package — `pkg/cli/cli.go` already pulls `pkg/workload/cli` → `workloadsql`, so an `init` here would run for `cockroach-oss`, `cockroach`, `cockroach-short`, and `go test` binaries that import `pkg/cli`, racing the CCL hook on `workload.ImportDataLoader`. |
| OSS-only wiring | `pkg/workload/importoss/` (new small package, Apache-2.0 header) | `init` assigns `workload.ImportDataLoader` to the `workloadsql` implementation; register `feature.workload_import_loader.enabled` here. **Blank import only** from `pkg/cmd/cockroach-oss/main.go` **and** add `//pkg/workload/importoss` to `pkg/cmd/cockroach-oss/BUILD.bazel` `deps` (today only `//pkg/cli` and `//pkg/ui/distoss` — blank import does not link until the dep exists). |
| Cluster setting | Same `pkg/workload/importoss/` package | Registered at process start for `cockroach-oss`; read during command preflight via SQL on a connection to an existing database. Server `workload://` import stays OSS-safe; gate is loader-only. |
| Integration test | `pkg/workload/workloadsql/import_loader_oss_test.go` | **Exec** `cockroach-oss workload init` subprocess (not in-process `InitialDataLoad`). Name: `TestImportDataLoaderOSS`. Start harness with `cluster.Spec{Binary: os.Getenv("FGDB_COCKROACH"), ...}` — `cluster.Start` does **not** read `FGDB_COCKROACH`; `checkBinary` errors when `Spec.Binary` is empty. `t.Skip` when the env var is unset. Use `WorkloadURL` (no database segment). |
| Bazel test target | `pkg/workload/workloadsql/BUILD.bazel` | Add a **separate** `go_test` (e.g. `import_loader_oss_test`) with `size = "large"` and a long `-test.timeout` for `TestImportDataLoaderOSS` only. Leave the existing `go_test` as `size = "small"` / ~55s — do not attach this subprocess test to that target. |
| Claims | `fgdb/test/claims.yaml` | Full row (registry requires all fields): see §5 example `TOOL-001`. |
| Conformance fixtures | `fgdb/test/conformance/par005-workload-loader/` (new) | Oracle JSON from approved success oracle (operator-recorded); no copied upstream test files. |

Do **not** add dependencies outside Go stdlib and existing module deps already used by `pkg/workload` / `pkg/sql/importer`.

## 4. Build, tests, conformance, scan gate

**Build (build host).**

```bash
./dev build oss
./artifacts/cockroach-oss version   # expect Distribution: OSS
```

**Unit / package tests.**

```bash
FGDB_COCKROACH=/absolute/path/to/cockroach-oss \
  GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/workload/workloadsql -run 'TestImportDataLoaderOSS' -timeout 30m
```

(Bazel: `dev bazel test //pkg/workload/workloadsql:import_loader_oss_test` or the target name chosen in `BUILD.bazel`. Test skips when `FGDB_COCKROACH` is unset; when set, pass it as `Spec.Binary`.)

**Broader regression (when binary exists).**

```bash
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/workload/... -short
```

**Manual acceptance.**

Use `pkg/fgdbtest/cluster` (disk store, reserved loopback address port per `cluster/ports.go`) or pass **explicit** `--listen-addr` and `--sql-addr` — do **not** use `--listen-addr=localhost:0` alone (advertised SQL address is copied before bind in `pkg/cli/flags.go`). In-memory `--store=type=mem` is fine for `workload://` (no `nodelocal` needed); it is unsuitable for export-style `nodelocal` tests.

```bash
# Example: harness-started node; use cluster.WorkloadURL (no database segment).
# SanitizeUrls rejects /defaultdb for tpcc (expects database "tpcc"); SQLURL is the /defaultdb form.
./artifacts/cockroach-oss workload init tpcc --warehouses=1 '<workload-url-from-harness>'
```

**TBD oracle gate.** The additional success-oracle source and its approval are not yet identified. Before implementation or handoff, the maintainer must approve the permitted source and its provenance; record the source, version/distribution, approver, approval date and approval reference in a versioned `fgdb/test/conformance/par005-workload-loader/README.md`, alongside the captured inputs and vectors in that directory. Capture URI shape, job completion, row fingerprints and FK definitions/validation state before comparing FGDb. Until the source, approval record and captures exist, this gate is open.

**Conformance cases (record oracle on approved success oracle, then diff FGDb).**

1. `workload init tpcc --warehouses=1` (AUTO) — exit 0; `SELECT count(*) FROM tpcc.warehouse` (and one more fact table) vs oracle.
2. Same with `--data-loader=import` explicit.
3. Add a command-level test for `runInitImpl` in `pkg/workload/cli/run_test.go`: use an instrumented fixture-capable generator with `PreCreate` and `PostLoad` counters and a recording SQL connection returning false for the setting. Exercise AUTO selecting IMPORT and explicit import, with absent and existing workload databases and with `--drop` enabled. Assert the exact loader error, zero hook calls, and no DROP, CREATE DATABASE, table creation, or IMPORT statements; verify the simulated existing database schema and configuration remain unchanged. Also exercise INSERT and AUTO selecting INSERT to show the gate does not block them. A cluster conformance case must additionally run tpcc with an option that exercises `PreCreate` and verify the absent database stays absent and relevant configuration is unchanged; repeat against an existing database and verify its schema and configuration remain unchanged.
4. `workload init bank` (small fixture) — row count oracle.
5. Negative: workload without fixtures still uses INSERT on AUTO (e.g. generator without `FillBatch`).

**Public doc references (behavior only).** CockroachDB v23.2 docs: `IMPORT INTO`, bulk data loading; workload fixture URLs as documented for v23.2.

**Scan gate (operator on build host; not in sandbox).**

```bash
mkdir -p /tmp/par005-scope && rsync -a \
  ./pkg/workload ./fgdb/test ./docs/fgdb/spec/PAR-005-workload-import-loader.md \
  /tmp/par005-scope/
easyscan-scan --workspace /tmp/par005-scope \
  --project-key Future-Gadget-Laboratories_FutureGadgetDatabases \
  --disable sonar --fail-on-error --fail-on-severity medium
```

(`easyscan-check --require-local` may fail in restricted sandboxes; operator runs check/Sonar.)

## 5. Risks, maintainer questions, PENDING

**Risks.**

- **URI / flag parity:** Wrong query params (`version`, generator flags) → silent data skew vs stock; mitigate with oracle-recorded URIs per workload.
- **Job polling:** Detached `IMPORT` timing and error surfacing must match stock; use same `WITH` options as oracle.
- **Storage layout:** IMPORT vs INSERT layout differs (benchmark plan); document that PAR-005 enables cross-system compare only when all systems use IMPORT with flag on.
- **Error text:** Flag-off must preserve exact `requires a CCL binary` substring for GATE tests.

**Security.** No new network endpoints; SQL uses existing `IMPORT` privilege model. Workload URIs are in-process (`workload://`), not user-supplied remote URLs in the default tpcc path.

**Maintainer questions.**

1. Preferred setting name (`feature.workload_import_loader.enabled` vs shorter alias).
2. Minimum warehouse count / workloads required for TOOL-001 beyond `tpcc` and `bank`.
3. *(Reviewer: server-side deny for `workload://` when flag off — **declined** for v1; loader gate matches today's user-visible behavior without touching OSS-002 server paths.)*

**Claims registry example (`TOOL-001`).**

```yaml
- id: TOOL-001
  claim: cockroach-oss workload init with default AUTO completes IMPORT fixture load for tpcc warehouse=1.
  source: docs/fgdb/spec/PAR-005-workload-import-loader.md
  category: tooling
  tests:
    - pkg/workload/workloadsql:TestImportDataLoaderOSS
  tier: interim
  status: partial
```

**PENDING (build host).**

- `./dev build oss` cold compile.
- `TestImportDataLoaderOSS` (subprocess) and optional ingest timing notes (no in-tree benchmark threshold).
- Stock OSS baseline and approved success oracle captures for conformance JSON.
- `go vet` / `staticcheck` on touched packages.
- `fgdb/test/run.sh pr` and interim tier with `FGDB_COCKROACH` set.

## 6. Review checklist

- [ ] No `pkg/ccl` imports; Bazel/oss graph still excludes CCL.
- [ ] Flag off reproduces pre-change loader error verbatim.
- [ ] TOOL-001 test uses `fgdbtest/cluster` (or explicit listen/sql addrs), execs `cockroach-oss workload init`, leaktest/log.Scope per FGDb rules.
- [ ] `workload.ImportDataLoader` `init` and cluster setting live in `pkg/workload/importoss/`; blank import **only** from `pkg/cmd/cockroach-oss/main.go` (not in `workloadsql`).
- [ ] Preflight checks the setting before database creation and `PreCreate`; flag-off side-effect tests cover absent and existing databases.
- [ ] `PostLoad` runs after all imports; tpcc foreign keys are present and unvalidated (`NOT VALID`).
- [ ] `TestImportDataLoaderOSS` on large Bazel target; `Spec.Binary` from `FGDB_COCKROACH`; skip when unset; `WorkloadURL` not `SQLURL` / `/defaultdb`.
- [ ] `pkg/cmd/cockroach-oss/BUILD.bazel` lists `//pkg/workload/importoss`.
- [ ] `IMPORT INTO` URIs match §2.1 (`csv`, `version`, `row-end=NumBatches`, generator flags in query).
- [ ] Claims.yaml updated; PR states license boundary.
- [ ] TBD gate closed: maintainer-approved success-oracle source and approval recorded in the versioned conformance README, with baseline provenance and required captures (not FGDb candidate).
- [ ] easyscan scope copied to `/tmp/par005-scope` before scan.
- [ ] No internal hostnames, tokens, or private consumer names in docs or tests.
