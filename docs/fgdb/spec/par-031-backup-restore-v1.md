<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- SPDX-FileCopyrightText: Future Gadget Laboratories -->

> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-031: native BACKUP/RESTORE behavior specification

| Record | Value |
| --- | --- |
| Status | **Draft v1** — unsealed; not an implementation handoff |
| Item | PAR-031; S1–S3 (S3 also informs PAR-035) |
| Date | 2026-10-21 |
| Spec writer | FGL spec team (agent-drafted); no implementation role for this feature |
| Skill bundles used | database-fgdb@1.0.1; impl-documentation |
| Oracle target | Official stock CockroachDB v23.2.15 full binary; not executed |
| Runtime evidence | None. No runner, cluster, or database binary invoked |
| Gatekeeper / seal / spec hash | Pending maintainer review; not assigned / not sealed / not recorded |
| License | New FGL documentation and case data: Apache-2.0 |

## Sources used and evidence rules

Read on 2026-10-21. Only the versioned public documentation below, the local
FGDb documentation, and FGL scope decisions (R1) were used for behavior and scope.
No proprietary source, source after v23.2.15, backup file contents, or secrets
were read. CCL means CockroachDB Community License; CSL means CockroachDB
Software License; BSL means Business Source License. No CCL or CSL source was
copied. No implementation code is part of this draft.

| ID | Source and use |
| --- | --- |
| R1 | FGL scope decisions for PAR-031 v1, stated in this document: slice boundaries, the job-result convention, deferred surfaces, case design and comparison rules, role separation, and FGDb's independent backup format. These are FGL choices, not claims about stock behavior |
| D1 | [v23.2 BACKUP](https://www.cockroachlabs.com/docs/v23.2/backup): Targets, Options, Object dependencies, Viewing and controlling backup jobs, Examples |
| D2 | [v23.2 RESTORE](https://www.cockroachlabs.com/docs/v23.2/restore): Parameters, Options, Functional details, Examples |
| D3 | [v23.2 SHOW BACKUP](https://www.cockroachlabs.com/docs/v23.2/show-backup): Parameters, Response, Validate a backup's files |
| L1 | Repository [CLEANROOM.md](../CLEANROOM.md), [SOURCES.md](../SOURCES.md), and [agent enforcement](../../agent/CLEANROOM-ENFORCEMENT.md): provenance and review conventions |
| L2 | Repository [backup.md](../backup.md), [testing.md](../testing.md), root README.md and CONTRIBUTING.md: current capability, FGL banner, evidence conventions |
| K1 | FGL skill bundles database-fgdb@1.0.1 and impl-documentation: task and documentation conventions |

**DOC** means supported by a cited public page, or an FGL scope decision when marked R1; it does not mean recorded
on v23.2.15. **UNVERIFIED — needs black-box run** marks unresolved behavior.
Every example case is runtime-unverified, including examples of DOC behavior.
Unknown values are null or explicit pending fields, never guessed observations.
The live v23.2 pages contain later-version annotations; those annotations are
excluded. The backup-validation URL could not be retrieved and is not evidence.

## Scope and comparison boundary

This is a SQL reference and conformance-data proposal. FGL scope decision (R1): v1 covers full backup,
restore, and inspection. Incremental chains (S4), schedules (S5), and revision
history/PITR (S6) belong to later specs. Encryption/KMS, execution locality,
locality-aware storage URIs, external connections, and tenant/virtual-cluster
backup are deferred. `skip_localities_check` remains an S2 option to specify,
with its exact applicability awaiting evidence.

FGDb has its own backup format. Each product backs up and restores its own
collections. Do not cross-restore stock and FGDb artifacts or analyze stock
file formats. Collection paths returned through SQL are observable values;
internal object names, byte layouts, compression, and progress granularity are
free. Output columns, stable SQL results, and recorded errors are comparison
contracts. Masking must not conceal missing columns, lost rows, or wrong targets.

The current logical `fgdb-backup` command is separate (L2). This draft does not
claim native support exists. FGDb gates the feature behind the proposed `fgdb.backup.enabled` setting; later conformance
must cover the disabled `XXC01` boundary separately from enabled native cases.
No claims registry entry is promoted to tested by these examples.

## S1 — full BACKUP

### S1.1 Statement forms

```sql
BACKUP INTO '<collection>';
BACKUP DATABASE d INTO '<collection>';
BACKUP TABLE d.public.t INTO '<collection>';
BACKUP d.public.t INTO '<collection>';
BACKUP DATABASE d, e INTO '<collection>' AS OF SYSTEM TIME '<timestamp>';
BACKUP TABLE d.public.t, d.public.u INTO '<collection>' WITH detached;
```

D1 defines, and FGL scope (R1) adopts, cluster, database, and whole-table targets. Omission of a target
means cluster; `(cluster)` is explanatory notation, not SQL. `TABLE`-less table
syntax appears in D1. Database backup covers its objects; table backup includes
indexes but does not recursively acquire referenced tables, sequences, or views.
An explicit `AS OF SYSTEM TIME` selects a consistent snapshot. No partial-table
predicate is specified. D1 describes `detached` as asynchronous but inconsistently
says its ID returns after completion: exact return timing is **UNVERIFIED —
needs black-box run**. FGL scope decision: assert a job ID only.

For v1, `detached` is the in-scope statement option. `revision_history`,
`incremental_location`, `encryption_passphrase`, `kms`, and `EXECUTION LOCALITY`
are documented surfaces that FGL defers out of v1 (R1). Unsupported-option SQLSTATEs and messages
are **UNVERIFIED — needs black-box run**; this draft does not invent rejection
semantics. Storage URI query parameters are separate from `WITH` options.

### S1.2 Result and postconditions

FGL scope decision (R1), consistent with the D1 examples: the synchronous result column order is

```text
job_id, status, fraction_completed, rows, index_entries, bytes
```

A successful blocking operation is expected to report `status = succeeded` and
`fraction_completed = 1`. Numeric types, zero-table counts, and exact count
accounting are **UNVERIFIED — needs black-box run**. `detached` returns only
`job_id`; returning an ID alone is not evidence of success. Preserve the ID as a
binding for subsequent job checks even when masking it for comparison.

FGL acceptance checks (R1): restore the product's own backup, compare
rows and all index fingerprints at the snapshot, inspect object lists, and ensure
that a failed backup does not become `LATEST`. A test dataset should include
NULLs, empty strings, computed columns, indexes, sequences, and dependencies.
These are test obligations, not asserted execution results.

### S1.3 Errors and job observations

The following are mandatory future recording categories. Unless a DOC outcome
is stated, the whole outcome is **UNVERIFIED — needs black-box run**. All exact
SQLSTATEs, message text, error precedence, and whether failure occurs before or
after job creation remain unverified.

| Trigger | Draft expectation / evidence |
| --- | --- |
| Missing object, malformed target, malformed option, invalid AOST | Record error and absence/presence of job; no guessed code |
| Timestamp older than retained data or in future | Record rejection, error phase, and collection visibility |
| Insufficient SQL privileges or unreadable/unwritable storage | Record authorization/storage failure independently |
| Repeated full backup to the same collection | D1's uniqueness warning needs reconciliation with collection-listing examples; do not assert unconditional success or failure |
| Dependency excluded from table backup | D1 does not auto-include it; later restore dependency behavior is tested in S2 |
| Invalid URI, missing destination, transient failure, disk full | Record error, job state, retry behavior, and `LATEST` afterward |

D1 exposes jobs through `SHOW JOBS` and job control. FGL scope (R1) requires observations of
pause, resume, cancellation, coordinator/worker interruption, and rerun behavior.
Record terminal status and stable transitions; do not impose an invented full
state sequence or timeout. Eventual cleanup and publication behavior need fault
runs. Detached statements in explicit transactions need commit/rollback cases;
blocking forms and exact transaction errors remain unverified.

## S2 — RESTORE FROM LATEST IN

### S2.1 Statement forms and target rules

```sql
RESTORE FROM LATEST IN '<collection>';
RESTORE DATABASE d FROM LATEST IN '<collection>' WITH new_db_name = 'copy_d';
RESTORE TABLE d.public.t FROM LATEST IN '<collection>' WITH into_db = 'dst';
RESTORE DATABASE d FROM '<subdir>' IN '<collection>';
```

FGL scope (R1) covers these cluster/database/table forms, explicit subdirectory selection,
and existing-target errors. `LATEST` selection with multiple fulls, failed jobs,
or concurrent publication is **UNVERIFIED — needs black-box run**.

D2: full-cluster restore requires no user-created databases/tables. Database
restore creates a database; existing names conflict. Table restore requires a
matching destination database and unoccupied object names. `into_db` needs an
existing database and applies only to table restores. `new_db_name` renames a
database restore.

### S2.2 Options

D2 defines the following option effects. No value is needed except for the two
name options. Exact boolean-value acceptance and invalid combinations are
**UNVERIFIED — needs black-box run**.

| Option | Observable effect |
| --- | --- |
| `new_db_name = 'name'` | Rename restored database |
| `into_db = 'name'` | Redirect table restore |
| `skip_missing_foreign_keys` | Remove missing foreign-key constraints |
| `skip_missing_sequences` | Drop defaults depending on missing sequences |
| `skip_missing_sequence_owners` | Permit omitted sequence ownership dependencies; exact resulting ownership unverified |
| `skip_missing_views` | Omit views with unavailable dependencies |
| `skip_missing_udfs` | Drop column defaults using unavailable UDFs |
| `skip_localities_check` | Bypass locality mismatch check; applicability unverified |
| `schema_only` | Restore schema without user rows |
| `detached` | Return job ID after job creation, without completion information |
| `verify_backup_table_data` | Requires `schema_only`; read/checksum/discard user data |

### S2.3 Results, errors, and integrity checks

FGL scope decision (R1): the provisional restore expectation reuses the S1.2 job-result convention: the six
columns when blocking and `job_id` when detached. Exact restore result metadata
and counter meaning are **UNVERIFIED — needs black-box run**. Track the job to
completion before querying restored data.

Required future error recordings: destination database exists; table name is
occupied; destination database missing; database requested from table-only backup;
missing FK/sequence/owner/view/UDF dependency; conflicting types; empty collection;
missing subdirectory; malformed/incompatible option pairs; missing/corrupt backup;
insufficient privileges. The exact SQLSTATE and message for every category are
**UNVERIFIED — needs black-box run**. Include `verify_backup_table_data` without
`schema_only` as an invalid combination. Record errors rather than silently
repairing names, adding dependency objects, or guessing ownership.

Each `skip_*` recording must pair the failure without the option with a success
using it. Check that unrelated defaults, valid FKs, and independent views survive.
For ownership, query schema metadata rather than inferring from successful INSERT.
For schema-only cases, check table definitions and zero rows. For ordinary
restores, compare per-table rows and per-index fingerprints, object inventories,
grants, owners, sequence behavior, and zone configurations against the intended
snapshot. Exact grant/zone replay expectations require separate recordings.

FGL scope (R1) additionally requires cleanup observations after cancellation/failure and a
successful rerun. Atomic visibility, cleanup deadlines, rollback behavior, and
job state transitions are **UNVERIFIED — needs black-box run**. Schema-only
validation can create schema objects; use a fresh destination for each run.

## S3 — inspection and validation

### S3.1 SQL and output contract

D3 defines these inspection forms:

```sql
SHOW BACKUPS IN '<collection>';
SHOW BACKUP FROM LATEST IN '<collection>';
SHOW BACKUP FROM '<subdir>' IN '<collection>' WITH check_files;
SHOW BACKUP SCHEMAS FROM LATEST IN '<collection>';
```

`SHOW BACKUPS` returns `path`; `SHOW BACKUP` describes contained objects.
The documented default fields are:

```text
database_name, parent_schema_name, object_name, object_type, backup_type,
start_time, end_time, size_bytes, rows, is_full_cluster, regions
```

`SCHEMAS` adds `create_statement`. Full backups have null `start_time`.
`check_files` checks file presence/location and adds `file_bytes` (physical size
estimate); `size_bytes` is logical size. Missing/unreadable files produce errors.
D3 examples disagree about `regions` inclusion: exact variant column order,
nullability, types, and row order are **UNVERIFIED — needs black-box run**.

FGL scope (R1) also includes `SHOW BACKUP FILES` and `SHOW BACKUP RANGES` with the same
`FROM '<subdir>' IN '<collection>'` selection. Their exact syntax and outputs
are **UNVERIFIED — needs black-box run**; no guessed field lists are supplied.
`privileges` and `debug_ids` need later column recordings. `as_json` and internal
metadata inspection are excluded from this behavior-only draft under the FGL behavior-not-format decision (R1).

### S3.2 Validation tiers

The `schema_only` validation cases (`s3-schema-only`, `s3-schema-verify-data`,
`s3-verify-requires-schema-only`) sit under S3 because they exercise the option
as a backup-inspection tool (does the backup restore and checksum?), while the
S2 option table still owns `schema_only` as a RESTORE option.

Presence checks do not establish checksum validity. D2's
`RESTORE ... WITH schema_only, verify_backup_table_data` supplies the stronger
read/checksum check while leaving user tables empty. FGL scope (R1) includes both tiers.
A later isolated fault fixture must distinguish unavailable storage, absent data,
and corrupted data; it must not expose or analyze stock file contents.

Record successful inspection, empty collection, invalid collection, denied
storage access, missing subdirectory, and failed validation. Exact SQLSTATEs and
error patterns are **UNVERIFIED — needs black-box run**. Do not hard-code internal
storage filenames. Paths and byte counts are masked; object identity, backup
scope, and zero-versus-nonzero restored row assertions remain meaningful.

## Conformance-case YAML schema v1 (runner contract)

The adjacent [cases/](cases/) files are independent examples, not a runner or a
claim of executed tests. This section is the normative contract a future runner
must implement; [tools/validate_cases.py](tools/validate_cases.py) is a
docs-side checker for it (`python3 tools/validate_cases.py`, needs PyYAML).

**Rules for every level.** Each file holds exactly one YAML document, a mapping.
Every mapping listed below is closed: **a key not listed as required or optional
is an error and the case is rejected**, never ignored. Required keys must be
present even when their value is null. "Text" means a non-empty string. "Scalar"
means string, integer, float, boolean, or null. No case depends on another file.
The examples use fixed literal rows; larger generators must record a fixed seed.

| Path | Required keys | Optional keys | Value types and constraints |
| --- | --- | --- | --- |
| (top) | `schema_version`, `id`, `slice`, `description`, `fixtures`, `variables`, `setup`, `statement`, `expect`, `post`, `oracle` | — | `schema_version`: integer `1`. `id`: text equal to the file name without `.yaml`, unique, prefixed by the lowercase slice (`s1-`). `slice`: `S1`, `S2`, or `S3`. `description`: text |
| `fixtures` | `source` | `target` | Text describing a fresh isolated cluster. `target` is required whenever any step uses `on: target` |
| `variables` | `collection` | any other binding name | Name matches `^[a-z][a-z0-9_]*$`; value is nonsecret text with no `${...}` references |
| `setup[]` | `on`, `sql` | — | Non-empty ordered list. `on`: `source` or `target` (must name a declared fixture). `sql`: text, one statement |
| `statement` | `on`, `sql` | — | Same as a setup step: the operation being compared |
| `expect` | — | `result`, `error`, `masks` | Exactly one of `result` or `error` |
| `expect.result` | `columns`, `rows`, `row_match` | `values`, `capture` | `columns`: null (unknown) or non-empty list of unique text. `rows`: null or list of mappings whose keys equal `columns` and whose values are scalars. `row_match`: `exact`, `contains`, or `pending`; `pending` if and only if `rows` is null. `values`: mapping of column to scalar that every returned row must have. `capture`: mapping of column to new binding name. Both require known `columns` that contain the named column |
| `expect.error` | `sqlstate`, `message_pattern` | `pending` | `sqlstate`: null or `^[0-9A-Z]{5}$`. `message_pattern`: null or anchored regex text (`^...$`). `pending` text is required if either is null, and forbidden otherwise |
| `expect.masks[]` | `field`, `kind` | — | Non-empty list, only with `result`. `kind`: `job_id`, `timestamp`, `size`, or `path`. `field` unique; must be in `columns` once columns are known |
| `post[]` | `on`, `sql`, `expect` | `pending` | List, may be empty. `expect` uses the grammar above. `pending` text is required when the step's result has null `columns` or `rows` |
| `oracle` | `stock_version`, `doc_urls`, `verification`, `recorded_at`, `license_key_state`, `evidence_note` | `pending_fields` | `stock_version`: `v23.2.15`. `doc_urls`: non-empty list of public `https://www.cockroachlabs.com/docs/v23.2/` URLs. `verification`: `unverified` or `verified`. `recorded_at`: null while unverified, ISO-8601 date text when verified. `license_key_state`: `not_checked`, `not_required`, or `present_redacted`. `evidence_note`: text. `pending_fields`: non-empty list of text, required when the main `expect` is incomplete |

**Binding rules.** SQL text is the only place `${name}` is substituted. A
binding is in scope from the point it is defined: `variables` before `setup`;
a `capture` in `expect` before the first `post` step; a `capture` in `post[i]`
from `post[i+1]` onward. A reference to an unbound name is an error. Binding
names are unique per case; a capture may not shadow a variable or an earlier
capture. Captures take the raw value before masking. Fixture strings are
requirements for the runner to provision, not bindings, and are never
substituted.

**Pending rules.** A null `columns`, `rows`, `sqlstate`, or `message_pattern`,
or `row_match: pending`, is an incomplete comparator and must have a matching
explanation: `expect.error.pending`, `post[].pending`, or
`oracle.pending_fields` for the main statement. A case with any incomplete
comparator reports **not_tested** and never passes. The checks that are
complete (for example `values`) are still evaluated, and a mismatch fails.

`on` names `source` or `target`. Only SQL text receives `${name}` substitution;
variables must be safely bound/quoted by the future runner, never evaluated as
shell. `nodelocal://1/par031/<case>` is a synthetic disposable URI, not a real
host or production path. Fresh source/target fixtures must share access to the
case's product-local storage when a restore crosses clusters.

A mask preserves SQL NULL. Otherwise it replaces a value with `<job_id>`,
`<timestamp>`, `<size>`, or `<path>` after validating its type. Bind captures
before masking, retaining equality relationships. Size masks do not authorize
ignoring row counts or integrity. Fingerprints are compared between source and
restored logical data, never replaced with a wildcard. Default row comparison
is unordered; `exact` means equal row multisets with no extras. `contains`
permits additional rows and must be justified for inventories/system objects.

Null columns, rows, SQLSTATEs, or message patterns plus `pending` comparisons
are incomplete assertions: report **not_tested**, never pass. A verified case
must contain no pending comparator. `verification` is `unverified` or `verified`;
all supplied cases are `unverified`. `recorded_at` is null until a real recording;
`license_key_state` is `not_checked`, `not_required`, or `present_redacted`.
Never store license values or credential-bearing URIs.

The evidence note for all current examples is **UNVERIFIED — needs black-box
run**. Future verified records must identify the v23.2.15 oracle, date, reviewed
columns, stable errors and job observations, and a gatekeeper-approved evidence
reference. Record FGDb replay evidence separately. A docs read is not a runtime
recording. Preserve raw observations outside implementer handoff; only reviewed
behavior cases may enter a sealed spec.

## Open questions / gaps

- Record exact SQLSTATEs, anchored error patterns, result types and column orders
  for all S1–S3 variants. Never infer a code from PostgreSQL conventions.
- Resolve the live docs' detached-BACKUP timing contradiction and `regions`
  column differences. Confirm flag forms and invalid option combinations.
- Confirm repeated full backups in one collection, `LATEST` publication under
  failures/concurrency, empty collections, and explicit-subdirectory equivalence.
- Record sequence-owner and locality edge cases. Clarify `new_db_name` scope for
  multi-database selections and interaction with `into_db`.
- Add paired cases for every missing dependency, schema changes, privileges,
  grants, owners, zones, all data types/index families, and error categories.
  The supplied examples are a starting set, not exhaustive acceptance coverage.
- Add job pause/resume/cancel, transactional detached jobs, node interruptions,
  storage faults, cleanup, retry, and no-partial-publication recordings. Timing
  and progress internals remain free unless public evidence establishes a bound.
- Confirm FILES/RANGES syntax and results without analyzing file contents; agree
  the review boundary for externally injected validation faults.
- Establish key-free versus licensed stock options operationally; no key was
  read or used. Add secure-storage/redaction coverage with the later harness.
- Define runner integration and promotion into a future FGDb conformance layout (proposed: `fgdb/test/conformance/backup/cases/`);
  preserve `not_tested` handling for pending evidence. No runner is implemented.
- Obtain maintainer gatekeeper review, approved spec hash, named independent
  implementer, and sealed handoff. This draft is not authorization to implement.
