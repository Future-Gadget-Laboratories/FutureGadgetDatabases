> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-001 — Missing-feature contract tests for every CCL gate

| Field | Value |
| --- | --- |
| Spec id | PAR-001 |
| Version | v1.2 |
| Date | 2026-10-22 |
| Role | spec writer (Codex, cleanroom spec role) |
| Sources consulted | `docs/fgdb/*.md`; `pkg/fgdbtest/capabilities`; OSS v23.2.15 SQL tree, parser, statement planner, builtin registrations, license checks, and OSS logic-test expectations |

## Contract

Run each case against one disposable, initialized OSS node as a table-driven case. Each case is reported separately in the existing capability report format. A case passes only when the statement reaches SQL execution and returns the stated SQLSTATE and message substring. Success, syntax/parser errors, permission errors, feature-flag errors, unrelated SQL errors, connection errors, and inability to run setup are failures, never skips or passes.

The common statement-gate output is SQLSTATE `XXC01` and message substring `a CCL binary is required to use this statement type:` **[oss-src]**. The concrete statement type may follow the colon; matching must require the common prefix and the case-specific type where the table specifies one. Exact message text remains an acceptance assertion for the runtime run; no oracle run was available in this job.

`PAR-001` has no assigned PAR-0xx flag-off parity item in the consulted FGDb context. Therefore every row below is `stable / no known PAR-0xx flip`; if a later PAR-0xx changes a row, it must update that row and add a paired on/off claim.

## Maintainer notes (v1.2, 2026-10-22)

- Gate ids renumbered to a contiguous GATE-001..GATE-026 sequence (v1 had duplicates).
- GATE-006 (`IMPORT TABLE ... CREATE USING`) is retired: that form is not in the v23.2 OSS grammar. Its id is not reused; `IMPORT TABLE ... FROM <format> ...` remains an OSS import path, not a CCL gate.
- GATE-011 (virtual-cluster replication syntax) was oracle-pending in v1.1; v1.2 uses the OSS grammar's required `ON <location>` clause and remains `oracle-pending` for the black-box CCL result.
- Exact message text for every row must be confirmed by a Decode oracle run (stock binary as a black box) or by the pinned `cockroach-oss` run in CI before the claims leave `partial`.

## v1.2 changes

- Corrected GATE-005, GATE-007, GATE-008, GATE-009, and GATE-011 to forms accepted by the checked-out v23.2 OSS grammar; corrected rows are marked `oss-src (grammar), oracle-pending`.
- Retired GATE-006 as `retired: not in v23.2 grammar`; its id is not reused.
- Moved GATE-014 out of the claim set as `unreachable on cockroach-oss` because `defaultdb` is not multi-region and the required setup is itself gated.
- Added the observed v23.2.15 black-box failures as parser/setup evidence while retaining `XXC01` as the pending statement-gate assertion for corrected forms.

## Statement-gate cases

The following are parsed CCL-only statement families. The SQL is intentionally minimal and uses no external storage. Where an object name is only a placeholder, the statement gate is expected before object existence or storage access is relevant.

| Gate | Minimal trigger | Preconditions/setup SQL | Expected SQLSTATE | Expected message substring | PAR-0xx flip |
| --- | --- | --- | --- | --- | --- |
| GATE-001 | `BACKUP DATABASE defaultdb TO 'nodelocal://1/par001'` | none | `XXC01` **[oss-src]** | `a CCL binary is required to use this statement type: *tree.Backup` **[oss-src]** | none known |
| GATE-002 | `RESTORE DATABASE defaultdb FROM 'nodelocal://1/par001'` | none | `XXC01` **[oss-src]** | `a CCL binary is required to use this statement type: *tree.Restore` **[oss-src]** | none known |
| GATE-003 | `SHOW BACKUP FROM LATEST IN 'nodelocal://1/par001'` | none | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.ShowBackup` **[oss-src]** | none known |
| GATE-004 | `CREATE CHANGEFEED FOR TABLE defaultdb.public.par001` | `CREATE TABLE par001 (id INT PRIMARY KEY)` | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.CreateChangefeed` **[oss-src]** | none known |
| GATE-005 | `ALTER CHANGEFEED 1 SET min_checkpoint_frequency = '1s'` | none; job lookup must not be required for the gate | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.AlterChangefeed` **[oss-src]**; provenance: `oss-src (grammar), oracle-pending` | none known |
| GATE-007 | `ALTER BACKUP 'nodelocal://1/par001' ADD NEW_KMS = 'kms://local/new' WITH OLD_KMS = 'kms://local/old'` | none; no file or KMS access should be required | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.AlterBackup` **[oss-src]**; provenance: `oss-src (grammar), oracle-pending` | none known |
| GATE-008 | `CREATE SCHEDULE FOR CHANGEFEED TABLE defaultdb.public.par001 INTO 'nodelocal://1/par001' RECURRING '@hourly'` | `CREATE TABLE par001 (id INT PRIMARY KEY)` | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.ScheduledChangefeed` **[oss-src]**; provenance: `oss-src (grammar), oracle-pending` | none known |
| PARITY-001 | `EXPORT INTO CSV 'nodelocal://1/par001/out' FROM SELECT 1` | none | success is **not** a gate: OSS CSV EXPORT is implemented **[oss-src]** | not applicable; a successful CSV export is required parity evidence **[oss-src]** | none known |
| PARITY-002 | `EXPORT INTO PARQUET 'nodelocal://1/par001/out' FROM SELECT 1` | none | success is **not** a gate: OSS Parquet export processor is registered **[oss-src]** | not applicable; a successful Parquet export or a non-CCL data/format error is not gate evidence **[oss-src]** | none known |
| GATE-009 | `CREATE SCHEDULE FOR BACKUP DATABASE defaultdb INTO 'nodelocal://1/par001' RECURRING '@daily'` | none | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.ScheduledBackup` **[oss-src]**; provenance: `oss-src (grammar), oracle-pending` | none known |
| GATE-010 | `ALTER BACKUP SCHEDULE 1 SET INTO 'nodelocal://1/par001'` | none; schedule lookup must not be required for the gate | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.AlterBackupSchedule` **[oss-src]** | none known |
| GATE-011 | `CREATE VIRTUAL CLUSTER v FROM REPLICATION OF source ON 'postgresql://127.0.0.1:26257'` | none; no remote connection may be attempted | `XXC01` **[oss-src]** | common statement-gate prefix; type `*tree.CreateTenantFromReplication` **[oss-src]**; provenance: `oss-src (grammar), oracle-pending` | none known |

The `IMPORT INTO` path used by `OSS-002` is deliberately excluded from GATE-006: its CSV reader is an OSS implementation. Likewise, supported CSV and Parquet `EXPORT` are not missing-feature gates. An unsupported option or disabled feature must be reported as its own non-gate negative case, not relabeled `XXC01`.

## Retired and non-claim notes

These entries are retained for traceability and are not acceptance claims. Their ids are not reused.

| Id | Status | Reason / replacement |
| --- | --- | --- |
| GATE-006 | `retired: not in v23.2 grammar` | `IMPORT TABLE ... CREATE USING` is not a v23.2 OSS statement form. The valid `IMPORT TABLE <table> FROM <format> <file>` family is an OSS import path, so it is not a CCL gate. |
| GATE-014 | `unreachable on cockroach-oss` | `ALTER DATABASE defaultdb ADD REGION 'us-east1'` reaches the multi-region precondition first: `defaultdb` is not multi-region. Creating a multi-region database is itself a CCL-gated operation. No equivalent reachable v23.2 OSS trigger was identified. |

## Specific hook-gate cases

| Gate | Minimal trigger | Preconditions/setup SQL | Expected SQLSTATE | Expected message substring | PAR-0xx flip |
| --- | --- | --- | --- | --- | --- |
| GATE-012 | `CREATE DATABASE mr PRIMARY REGION 'us-east1' SURVIVE ZONE FAILURE` | none | `XXC01` **[oss-src]** | `creating multi-region databases requires a CCL binary` **[oss-src]** | none known |
| GATE-013 | `ALTER DATABASE defaultdb PRIMARY REGION 'us-east1'` | none | `XXC01` **[oss-src]** | `creating multi-region databases requires a CCL binary` **[oss-src]** | none known |
| GATE-015 | `CREATE TABLE p (a INT PRIMARY KEY) PARTITION BY LIST (a) (PARTITION p1 VALUES IN (1))` | none | `XXC01` **[oss-src]** | `creating or manipulating partitions requires a CCL binary` **[oss-src]** | none known |
| GATE-016 | `CREATE TABLE p2 (a INT PRIMARY KEY); ALTER TABLE p2 PARTITION BY LIST (a) (PARTITION p1 VALUES IN (1))` | `CREATE TABLE p2 (a INT PRIMARY KEY)`; run the ALTER as the case statement after setup | `XXC01` **[oss-src]** | `creating or manipulating partitions requires a CCL binary` **[oss-src]** | none known |
| GATE-017 | `CREATE FUNCTION f() RETURNS RECORD AS $$ BEGIN RETURN ROW(0); END $$ LANGUAGE PLpgSQL` | none | `XXC01` **[oss-src]** | `using PL/pgSQL requires a CCL binary` **[oss-src]** | none known |
| GATE-018 | `CREATE PROCEDURE p() AS $$ BEGIN RAISE NOTICE '1'; END $$ LANGUAGE PLpgSQL` | none | `XXC01` **[oss-src]** | `using PL/pgSQL requires a CCL binary` **[oss-src]** | none known |
| GATE-019 | `SET plan_cache_mode = force_generic_plan` | none | `XXC01` **[oss-src]** | `plan_cache_mode=force_generic_plan and plan_cache_mode=auto require a CCL binary` **[oss-src]** | none known |
| GATE-020 | `SET plan_cache_mode = auto` | none | `XXC01` **[oss-src]** | `plan_cache_mode=force_generic_plan and plan_cache_mode=auto require a CCL binary` **[oss-src]** | none known |

## Builtin gates

| Gate | Minimal trigger | Preconditions/setup SQL | Expected SQLSTATE | Expected message substring | PAR-0xx flip |
| --- | --- | --- | --- | --- | --- |
| GATE-021 | `SELECT encrypt('abc', 'key', 'aes')` | none | `XXC01` **[oss-src]** | `encrypt can only be used with a CCL distribution` **[oss-src]** | none known |
| GATE-022 | `SELECT encrypt_iv('abc', 'key', '123', 'aes')` | none | `XXC01` **[oss-src]** | `encrypt_iv can only be used with a CCL distribution` **[oss-src]** | none known |
| GATE-023 | `SELECT decrypt('\xdb5f149a7caf0cd275ca18c203a212c9', 'key', 'aes')` | none | `XXC01` **[oss-src]** | `decrypt can only be used with a CCL distribution` **[oss-src]** | none known |
| GATE-024 | `SELECT decrypt_iv('\x91b4ef63852013c8da53829da662b871', 'key', '123', 'aes')` | none | `XXC01` **[oss-src]** | `decrypt_iv can only be used with a CCL distribution` **[oss-src]** | none known |
| GATE-025 | `SELECT with_min_timestamp(now())` | none | `XXC01` **[oss-src]** | `with_min_timestamp can only be used with a CCL distribution` **[oss-src]** | none known |
| GATE-026 | `SELECT with_max_staleness('1s')` | none | `XXC01` **[oss-src]** | `with_max_staleness can only be used with a CCL distribution` **[oss-src]** | none known |

## Enterprise-license checks: separate from the CCL gate

These are OSS-source checks that call the enterprise-enable hook. They are not GATE claims because the stock OSS hook returns the OSS feature-disabled error, not `XXC01`; the `XXC02` license-expiry/throttling paths belong to a CCL binary and are unreachable on `cockroach-oss` under this contract.

| Case | Minimal trigger | Preconditions/setup SQL | Expected OSS result |
| --- | --- | --- | --- |
| LIC-001 | `ALTER TABLE a CONFIGURE ZONE USING global_reads = true` | `CREATE TABLE a (id INT PRIMARY KEY)` | error text `OSS binaries do not include enterprise features` **[oss-src]**; SQLSTATE is not a PAR-001 `XXC01` claim **[oss-src]** |
| LIC-002 | add a zone config to an index/partition subzone | a partitioned descriptor and existing subzone metadata; use only local setup | error text `OSS binaries do not include enterprise features` **[oss-src]**; not a statement-gate claim **[oss-src]** |

## Acceptance and reporting

Each GATE claim must be a distinct table entry and report row; PARITY rows are not GATE claims. The runtime helper must retain the existing `pgx` structured-error checks: require a PostgreSQL error, exact SQLSTATE `XXC01` for gate rows, and the required message substring. The helper must fail closed on nil error, syntax error, permission error, transport/connection error, or any other SQLSTATE. Setup and cleanup failures fail the suite and leave later claims `not_tested`; they do not become passing claims.

## Open questions / oracle-pending

- Confirm every trigger's parser acceptance and the exact concrete statement type in a black-box Decode run of the stock OSS binary.
- Confirm the exact SQLSTATE for LIC-001 and LIC-002; the OSS source establishes the message but not the wire wrapping at the call site.
- Confirm whether any future FGDb PAR-0xx feature flag intentionally flips a row; none is defined in the consulted context.
- Confirm the corrected scheduled-backup and replication forms in the stock-binary oracle run without permitting a job, file, or network side effect.

Acceptance claim ids: GATE-001, GATE-002, GATE-003, GATE-004, GATE-005, GATE-007, GATE-008, GATE-009, GATE-010, GATE-011, GATE-012, GATE-013, GATE-015, GATE-016, GATE-017, GATE-018, GATE-019, GATE-020, GATE-021A, GATE-022A, GATE-021, GATE-022, GATE-023, GATE-024, GATE-025, GATE-026.

Summary line 1: PAR-001 defines table-driven OSS CCL-gate contract cases.
Summary line 2: It preserves the existing per-claim capability report format.
Summary line 3: Statement gates require SQLSTATE XXC01.
Summary line 4: Generic gates require the documented CCL message prefix.
Summary line 5: Multi-region and partitioning hooks have specific messages.
Summary line 6: PL/pgSQL and generic-plan settings have specific messages.
Summary line 7: Four pgcrypto encryption builtins are covered.
Summary line 8: Two AS OF SYSTEM TIME builtins are covered.
Summary line 9: OSS CSV and Parquet paths are explicitly non-gates.
Summary line 10: License checks are separated and marked oracle-pending.

## Corrected rows (v1.2)

| Gate | Corrected minimal trigger | Status / provenance |
| --- | --- | --- |
| GATE-005 | `ALTER CHANGEFEED 1 SET min_checkpoint_frequency = '1s'` | corrected; `oss-src (grammar), oracle-pending` |
| GATE-006 | `IMPORT TABLE t CREATE USING 'nodelocal://1/par001/schema.sql'` | `retired: not in v23.2 grammar`; id not reused |
| GATE-007 | `ALTER BACKUP 'nodelocal://1/par001' ADD NEW_KMS = 'kms://local/new' WITH OLD_KMS = 'kms://local/old'` | corrected; `oss-src (grammar), oracle-pending` |
| GATE-008 | `CREATE SCHEDULE FOR CHANGEFEED TABLE defaultdb.public.par001 INTO 'nodelocal://1/par001' RECURRING '@hourly'` | corrected; `oss-src (grammar), oracle-pending` |
| GATE-009 | `CREATE SCHEDULE FOR BACKUP DATABASE defaultdb INTO 'nodelocal://1/par001' RECURRING '@daily'` | corrected; `oss-src (grammar), oracle-pending` |
| GATE-011 | `CREATE VIRTUAL CLUSTER v FROM REPLICATION OF source ON 'postgresql://127.0.0.1:26257'` | corrected; `oss-src (grammar), oracle-pending` |
| GATE-014 | `ALTER DATABASE defaultdb ADD REGION 'us-east1'` | `unreachable on cockroach-oss`; non-claim note, id not reused |
