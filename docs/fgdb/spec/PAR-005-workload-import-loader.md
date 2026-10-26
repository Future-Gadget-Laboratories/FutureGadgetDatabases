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

- Implement only from this approved spec, public documentation, and Apache-2.0 paths in this tree (v23.2.15 base).
- **Never** read, list, grep, or import `pkg/ccl/`, `pkg/ui/distccl/`, or any CockroachDB source newer than v23.2.15. Do not browse `cockroachdb/cockroach` on GitHub.
- Do not paste or paraphrase proprietary Enterprise/CCL implementation from memory or external trees. Rebuild behavior from observable SQL and the OSS importer/workload code cited below.
- Record in the PR that no CCL/CSL code was copied (license checklist).

## 1. Goal, acceptance criteria, GATE decision

**Goal.** Restore stock v23.2.15 behavior where `cockroach workload init` with default `--data-loader=AUTO` selects the fast `IMPORT` path for fixture-capable workloads (for example `tpcc`), so benchmarks and soak tests can load comparable data without forcing `--data-loader=INSERT` on every system.

**Acceptance criteria.**

1. **TOOL-001.** On a loopback single-node `cockroach-oss` cluster, `cockroach workload init tpcc --warehouses=1` with default `AUTO` completes without error and leaves a populated `tpcc` schema (row counts consistent with warehouse=1 expectations from the workload generator in this tree).
2. **GATE-off parity.** When `feature.workload_import_loader.enabled` is **false**, `workload init` with `AUTO` or explicit `--data-loader=import` fails with the **same** error text as today: `loading initial data with IMPORT requires a CCL binary`. Inside `InitialDataLoad`, run `SHOW CLUSTER SETTING` (or equivalent read on the existing `*gosql.DB`) **first** — **before** any `CREATE TABLE` — and return that error when the setting is false (today’s stub returns at the start of `InitialDataLoad` and creates no tables; `CREATE TABLE` before the check would leave tables the current error path does not clean up). Ad-hoc `IMPORT INTO` on the server (for example HTTP CSV per OSS-002) stays available and is **not** gated by this setting.
3. **INSERT unchanged.** `--data-loader=INSERT` behavior and performance characteristics remain unchanged when the flag is on or off.
4. **Conformance.** FGL conformance cases (recorded oracle from stock v23.2.15 **full** binary for URI shape, job completion, and row fingerprints) pass on FGDb with flag on; flag-off cases match today's error.
5. **Claims.** `fgdb/test/claims.yaml` lists TOOL-001 with at least one test; interim tier can run the integration test when a binary is supplied.
6. **Benchmarks (PENDING on build host).** Numeric M-BULK / M-TPCC-D comparison versus stock v23.2.15 full binary is **out of scope for this spec** (`BENCHMARK-PLAN` is not in this tree). Record ingest duration in PENDING notes; a follow-up benchmark PR may define thresholds.

**GATE contract decision: yes — cluster setting read from `InitialDataLoad`.**

| Item | Decision |
| --- | --- |
| Mechanism | Public cluster setting `feature.workload_import_loader.enabled` (name negotiable with maintainer; must be `settings.ApplicationLevel`, default `true`, documented). Register the setting in the **OSS-only wiring package** (below) so `cockroach-oss` still exposes it. `InitialDataLoad` reads it via SQL on the existing `*gosql.DB` **before** `CREATE TABLE` and before any `IMPORT INTO`. |
| Flag **on** | `workload.ImportDataLoader` runs the cleanroom OSS implementation; server uses existing `IMPORT INTO` + `workload://` support in `pkg/sql/importer`. |
| Flag **off** | `InitialDataLoad` returns today's loader error string unchanged (`loading initial data with IMPORT requires a CCL binary`) and does not run `IMPORT`. Server `IMPORT` for HTTP/`nodelocal` (OSS-002) stays unaffected. |
| PAR-001 linkage | This is a **new** PAR-0xx flip for workload-loader only; update PAR-001 maintainer notes when the setting lands. Not a statement-gate `XXC01` row. |

**Reasons.** Rollback without reverting the binary; explicit parity test for “today's error”; separates CLI fixture loading from server-side `IMPORT INTO` already proven by OSS-002.

## 2. Verified survey (this tree only)

| Path | What it implies |
| --- | --- |
| `pkg/workload/cli/run.go` (`runInitImpl`, ~323–345) | `AUTO` → `import` when `workload.SupportsFixtures(gen)`; loader is `workload.ImportDataLoader`. |
| `pkg/workload/workload.go` (`ImportDataLoader`, `requiresCCLBinaryDataLoader`, ~370–379) | Default loader returns `loading initial data with %s requires a CCL binary` — **only** the workload CLI hook is missing on OSS. |
| `pkg/workload/workload.go` (`SupportsFixtures`, ~47–57) | Fixtures require every table to have `InitialRows.FillBatch != nil`. |
| `pkg/cli/cli.go` → `pkg/workload/cli/run.go` → `workloadsql` | Every `cockroach-oss` / `cockroach` / `cockroach-short` and `go test` importing `pkg/cli` already loads `workloadsql`; **`init` that sets `ImportDataLoader` must not live there** (races CCL hook on the same var). |
| `pkg/workload/workloadsql/workloadsql.go` (`Setup`) | Calls `PreCreate`, then `InitialDataLoad`, then splits — **does not** create tables itself. |
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

Record the exact URI strings in conformance oracle JSON from the stock full binary; FGDb must match param-for-param.

## 3. Deliverables (paths and size targets)

| Deliverable | Path | Target size |
| --- | --- | --- |
| OSS import loader implementation | `pkg/workload/workloadsql/import_loader.go` (new, ~120–200 LOC, Apache-2.0 header per `pkg/fgdbtest`) | Package `workloadsql` only: `InitialDataLoad` (setting check **first**, then per table `CREATE TABLE IF NOT EXISTS`, then `IMPORT INTO` per §2.1). **No** `init` that assigns `workload.ImportDataLoader` in this package — `pkg/cli/cli.go` already pulls `pkg/workload/cli` → `workloadsql`, so an `init` here would run for `cockroach-oss`, `cockroach`, `cockroach-short`, and `go test` binaries that import `pkg/cli`, racing the CCL hook on `workload.ImportDataLoader`. |
| OSS-only wiring | `pkg/workload/importoss/` (new small package, Apache-2.0 header) | `init` assigns `workload.ImportDataLoader` to the `workloadsql` implementation; register `feature.workload_import_loader.enabled` here. **Blank import only** from `pkg/cmd/cockroach-oss/main.go` **and** add `//pkg/workload/importoss` to `pkg/cmd/cockroach-oss/BUILD.bazel` `deps` (today only `//pkg/cli` and `//pkg/ui/distoss` — blank import does not link until the dep exists). |
| Cluster setting | Same `pkg/workload/importoss/` package | Registered at process start for `cockroach-oss`; read in `InitialDataLoad` via SQL on `*gosql.DB`. Server `workload://` import stays OSS-safe; gate is loader-only. |
| Integration test | `pkg/workload/workloadsql/import_loader_oss_test.go` | **Exec** `cockroach-oss workload init` subprocess (not in-process `InitialDataLoad`). Name: `TestImportDataLoaderOSS`. Start harness with `cluster.Spec{Binary: os.Getenv("FGDB_COCKROACH"), ...}` — `cluster.Start` does **not** read `FGDB_COCKROACH`; `checkBinary` errors when `Spec.Binary` is empty. `t.Skip` when the env var is unset. Use `WorkloadURL` (no database segment). |
| Bazel test target | `pkg/workload/workloadsql/BUILD.bazel` | Add a **separate** `go_test` (e.g. `import_loader_oss_test`) with `size = "large"` and a long `-test.timeout` for `TestImportDataLoaderOSS` only. Leave the existing `go_test` as `size = "small"` / ~55s — do not attach this subprocess test to that target. |
| Claims | `fgdb/test/claims.yaml` | Full row (registry requires all fields): see §5 example `TOOL-001`. |
| Conformance fixtures | `fgdb/test/conformance/par005-workload-loader/` (new) | Oracle JSON from stock full binary (operator-recorded); no copied upstream test files. |

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

**Conformance cases (record oracle on stock v23.2.15 full binary, then diff FGDb).**

1. `workload init tpcc --warehouses=1` (AUTO) — exit 0; `SELECT count(*) FROM tpcc.warehouse` (and one more fact table) vs oracle.
2. Same with `--data-loader=import` explicit.
3. `SET CLUSTER SETTING feature.workload_import_loader.enabled = false` (or pre-PR binary): AUTO → exact CCL-binary loader error string.
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
- Stock full-binary oracle capture for conformance JSON.
- `go vet` / `staticcheck` on touched packages.
- `fgdb/test/run.sh pr` and interim tier with `FGDB_COCKROACH` set.

## 6. Review checklist

- [ ] No `pkg/ccl` imports; Bazel/oss graph still excludes CCL.
- [ ] Flag off reproduces pre-change loader error verbatim.
- [ ] TOOL-001 test uses `fgdbtest/cluster` (or explicit listen/sql addrs), execs `cockroach-oss workload init`, leaktest/log.Scope per FGDb rules.
- [ ] `workload.ImportDataLoader` `init` and cluster setting live in `pkg/workload/importoss/`; blank import **only** from `pkg/cmd/cockroach-oss/main.go` (not in `workloadsql`).
- [ ] `InitialDataLoad` checks cluster setting before `CREATE TABLE`.
- [ ] `TestImportDataLoaderOSS` on large Bazel target; `Spec.Binary` from `FGDB_COCKROACH`; skip when unset; `WorkloadURL` not `SQLURL` / `/defaultdb`.
- [ ] `pkg/cmd/cockroach-oss/BUILD.bazel` lists `//pkg/workload/importoss`.
- [ ] `IMPORT INTO` URIs match §2.1 (`csv`, `version`, `row-end=NumBatches`, generator flags in query).
- [ ] Claims.yaml updated; PR states license boundary.
- [ ] Conformance oracle provenance documented (stock binary version, not FGDb candidate).
- [ ] easyscan scope copied to `/tmp/par005-scope` before scan.
- [ ] No internal hostnames, tokens, or private consumer names in docs or tests.
