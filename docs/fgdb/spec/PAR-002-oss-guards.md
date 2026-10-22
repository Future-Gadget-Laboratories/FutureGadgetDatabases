> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-002 — Guard tests for features already in the OSS binary

| Field | Value |
| --- | --- |
| Spec id | PAR-002 |
| Version | v1 |
| Date | 2026-10-22 |
| Role | spec writer (Codex, cleanroom spec role) |
| Sources consulted | `pkg/fgdbtest/capabilities`; `docs/fgdb/oss-capabilities.md`; `docs/fgdb/getting-started.md`; OSS v23.2.15 SQL, importer, copy, TTL, transaction, and spatial behavior in this checkout |

## Scope and common setup

These are black-box checks against one disposable, single-node OSS process. The
test owns the node and its temporary store, connects through its loopback SQL
address, and uses only loopback HTTP fixtures or `nodelocal://1/` data. A check
is `pass` only when every listed statement and result matches. A setup, process,
connection, timeout, or cleanup failure is `fail` for the claim; it is never a
successful skip.

Each expected-result cell below includes its provenance. `oss-src` means the
observable contract is derived from the OSS v23.2.15 tree; `docs` means it is
derived from the public FGDb documentation; `oracle-pending` would require a
stock-binary Decode run, but no such run is available for this spec.

The harness should first create the default database and verify SQL readiness,
then execute the claim's setup SQL. Use fresh table names or drop each table at
cleanup. All polling uses a monotonic deadline and a bounded query context.

## Maintainer notes (v1.2, 2026-10-22)

- OSS-008: `MYSQLDUMP` and `PGDUMP` need the `IMPORT TABLE`/bundle form, which PAR-001 GATE-006 lists as possibly gated. They are **oracle-pending**: implement `CSV`, `DELIMITED`, `PGCOPY` and `AVRO` through `IMPORT INTO` as required cases. Report the dump formats as `not_tested` with a reason until oracle evidence decides them. The PARQUET rejection case stays required.
- Erratum (v1.2): OSS-007 COPY TO must use the query form `COPY (SELECT id, v FROM copy_guard ORDER BY id) TO STDOUT`. A black-box run showed that `COPY copy_guard ORDER BY id TO STDOUT` is a syntax error (42601).
- Open (v1.2): OSS-004 timing. On a fresh single node, the row-level TTL schedule had a `next_run` but did not delete the expired row within 300 s (black-box run of the shipped OSS binary). Until the cause is known, the guard is opt-in (`FGDB_TTL_GUARD=1`) and otherwise reports `not_tested`. The spec writer should research the scheduler start-up delay and the TTL read lag for the next spec version.
- OSS-005: the GEOS area check accepts `1` within float tolerance 1e-9 only.
- Every `[oss-src]` result must be confirmed by a Decode oracle run, or by the pinned `cockroach-oss` CI run, before the claim leaves `partial`.

## Acceptance claims

### OSS-004 — Row-level TTL deletes expired rows

| Item | Requirement |
| --- | --- |
| Preconditions | Start one node with the normal OSS binary. Keep TTL enabled. The table must have an ascending primary key. |
| Setup SQL | `SET CLUSTER SETTING sql.ttl.job.enabled = true;`<br>`SET CLUSTER SETTING jobs.registry.interval.adopt = '1s';`<br>`CREATE TABLE ttl_guard (id INT PRIMARY KEY, v STRING) WITH (ttl_expire_after = '1 second', ttl_job_cron = '* * * * *');`<br>`INSERT INTO ttl_guard VALUES (1, 'expired'), (2, 'live');`<br>`UPDATE ttl_guard SET crdb_internal_expiration = now() - INTERVAL '1 minute' WHERE id = 1;` |
| Expected observable result | Repeatedly run `SELECT id, v FROM ttl_guard ORDER BY id` until it returns exactly `(2, 'live')`; the expired row is absent. **[oss-src]** |
| Timing | The five-field cron runs at most one minute after its scheduled boundary; the adoption interval is lowered to 1 second. Allow up to 90 seconds after setup, with a query timeout on each poll. |
| Must not count as pass | A row hidden only by a test query predicate; the schedule exists but no deletion is observed; TTL is disabled; a timeout; or a job/setup error. Restore `jobs.registry.interval.adopt` before teardown when the harness changes it. |

### OSS-005 — GEOS-backed spatial functions work with `lib/` beside the binary

| Item | Requirement |
| --- | --- |
| Preconditions | Launch the binary from a directory whose sibling `lib/` contains both `libgeos.so` and `libgeos_c.so`. The paths must be the files supplied with the OSS build, not a system fallback. **[docs]** |
| Setup SQL | `SELECT ST_Area(ST_GeomFromText('POLYGON ((0 0, 0 1, 1 1, 1 0, 0 0))'));` |
| Expected observable result | One row with one numeric value equal to `1`. **[oss-src]** |
| Timing | The query must complete within 10 seconds after SQL readiness. |
| Must not count as pass | A missing-library startup skip, a test marked “not applicable,” a fallback that does not use the beside-binary `lib/`, a NULL/error result, or a result accepted only by rounding an incorrect area. Missing or unloadable libraries are reported `not_tested`, never `pass`; the claim then fails the required runtime tier. |

### OSS-006 — READ COMMITTED works when enabled

| Item | Requirement |
| --- | --- |
| Preconditions | Use two SQL connections to the same single node. Before opening the test transaction, execute `SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = true;`. **[oss-src]** |
| Setup SQL | On connection A: `CREATE TABLE rc_guard (id INT PRIMARY KEY); INSERT INTO rc_guard VALUES (1);`<br>On A: `BEGIN TRANSACTION ISOLATION LEVEL READ COMMITTED; SELECT count(*) FROM rc_guard;`<br>On B: `INSERT INTO rc_guard VALUES (2);` and commit. |
| Expected observable result | A's first count is `1`; after B commits, A executes `SELECT count(*) FROM rc_guard;` and receives `2`; A then commits. **[oss-src]** |
| Timing | B's insert and commit must complete within 10 seconds; A's second read must complete within 10 seconds after B commits. |
| Must not count as pass | A transaction silently mapped to SERIALIZABLE, a result obtained after committing A, a retry that changes the sequence without recording it, an unset/failed cluster setting, or a connection failure. With the setting disabled, this claim is not tested; it is not evidence of support. |

### OSS-007 — COPY FROM and COPY TO work

| Item | Requirement |
| --- | --- |
| Preconditions | Use a PostgreSQL wire client capable of COPY-in and COPY-out. Do not substitute ordinary INSERT/SELECT for the COPY protocol. |
| Setup SQL and input | `CREATE TABLE copy_guard (id INT PRIMARY KEY, v STRING NOT NULL);`<br>Send `COPY copy_guard FROM STDIN` with text rows `1\talpha\n2\tbeta\n`, then send COPY-done. |
| Expected observable result | COPY-in reports two rows. `SELECT id, v FROM copy_guard ORDER BY id` returns `(1, 'alpha')`, `(2, 'beta')`. `COPY copy_guard ORDER BY id TO STDOUT` returns exactly `1\talpha\n2\tbeta\n`. **[oss-src]** |
| Timing | Each COPY operation and the verification query must finish within 10 seconds. |
| Must not count as pass | Data loaded by INSERT, a client-side transformed output, a truncated stream, a result with an extra row, or success that omits either COPY direction. |

### OSS-008 — OSS IMPORT format matrix

| Item | Requirement |
| --- | --- |
| Preconditions | Use a fresh target table for each format. Every source is a loopback HTTP fixture or `nodelocal://1/` file owned by the test. The fixture must contain exactly two rows matching the target schema, unless the format requires its own schema/table declaration. |
| Setup SQL | For row formats: `CREATE TABLE import_guard (id INT PRIMARY KEY, v STRING NOT NULL);` For dump formats, provide a minimal valid dump whose table/data correspond to `import_guard`, or use the format's `IMPORT INTO` form where accepted. |
| Expected observable result — accepted formats | `CSV`, `DELIMITED`, `PGCOPY`, `MYSQLDUMP`, `PGDUMP`, and `AVRO` are recognized OSS formats. With a valid fixture and format-specific required options, the statement succeeds and a query of the imported target returns exactly `(1, 'alpha')`, `(2, 'beta')`, with count `2`. **[oss-src]** |
| Expected observable result — rejected format | `IMPORT INTO import_guard PARQUET DATA ('nodelocal://1/par002/unused')` fails with SQLSTATE `0A000` and message containing `unsupported import format: "PARQUET"`. **[oss-src]** |
| Timing | Each accepted-format import has a 60-second deadline; each rejected-format assertion has a 10-second deadline. Poll job completion where IMPORT is asynchronous, then verify rows. |
| Must not count as pass | Parser success without successful load, an empty target, extra/incorrect rows, a fixture served from a non-loopback source, treating a malformed fixture as evidence that the format is rejected, or accepting a format because the server skipped the import. A format is `not_tested` if its valid fixture is unavailable. |

The matrix is about SQL format recognition and successful loading, not every
option combination. Invalid options, malformed data, permission failures, and
unreachable sources are separate negative tests and do not establish that a
recognized format is unsupported.

## Reporting and cleanup

Record one outcome per claim: `pass`, `fail`, or `not_tested`. Include the
statement, SQLSTATE and message for every error, row values for every successful
query, elapsed time, binary identity, and whether the required beside-binary
GEOS libraries were present. A missing binary, missing `lib/`, failed cluster
setting, absent fixture, or skipped execution must be visible in the report.

Drop created tables, restore changed cluster settings, stop the node, reap its
process, and remove only the test-owned temporary directory. Cleanup failure
turns the run into a failure even if SQL assertions passed.

## Open questions / oracle-pending

- Confirm exact stock-binary error text for a missing or unloadable GEOS library.
- Confirm exact stock-binary result formatting for numeric `ST_Area` over the wire.
- Confirm the minimal valid fixtures and completion behavior for each accepted
  IMPORT format on the shipped OSS binary.
- Confirm whether a stock Decode oracle reports any additional format-specific
  notices that the guard should record.

Acceptance claim ids:

- OSS-004
- OSS-005
- OSS-006
- OSS-007
- OSS-008

## 10-line summary

1. PAR-002 guards existing OSS behavior only.
2. OSS-004 checks deletion of an expired TTL row.
3. TTL polling uses one-second job adoption and a bounded deadline.
4. OSS-005 checks a GEOS area calculation.
5. GEOS libraries must be beside the binary under `lib/`.
6. Missing GEOS libraries are `not_tested`, never a pass.
7. OSS-006 enables and exercises READ COMMITTED across two connections.
8. OSS-007 verifies both COPY FROM and COPY TO.
9. OSS-008 covers six recognized IMPORT formats and one rejected format.
10. Every result, error, timing, and skip has an explicit acceptance rule.
