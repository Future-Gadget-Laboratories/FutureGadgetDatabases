> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# pkg/jobs

Long work does not stay inside the SQL connection that started it. A **job** is a row in `system.jobs` plus a resumer that some node will pick up, including after a restart.

Schema changes, `IMPORT`, statistics collection, row-level TTL, and garbage collection of dropped tables are jobs. `BACKUP` and changefeeds are also jobs in a CCL build, but their resumers are not registered in `cockroach-oss`. The registry that would run them is still here.

## Purpose

- Persist job state in `system.jobs` (payload, progress, status, claim).
- Claim a job on one node at a time, using a SQL-liveness session.
- Call the `Resumer` for that job type until it finishes, fails, or is canceled.

## Key types and entry points

| Piece | Where | Role |
| --- | --- | --- |
| `jobs.Record` | `pkg/jobs/jobs.go` | What you pass to create a job: description, username, `jobspb.Details`, progress. The details payload selects the type. |
| `jobs.Job` | `pkg/jobs/jobs.go` | In-memory handle. Payload and progress sit under a mutex. |
| `jobs.Registry` | `pkg/jobs/registry.go` | Create, adopt, update. Holds `sqlliveness.Session`, an internal SQL db, the clock, and a `stop.Stopper`. |
| `Registry.CreateJobWithTxn` | `pkg/jobs/registry.go` | Inserts the row in the caller's transaction, status running, claimed by this session. |
| `Registry.CreateStartableJobWithTxn` | `pkg/jobs/registry.go` | Same, but the resumer does not run until the transaction commits and the caller starts it. |
| `Registry.Start` | `pkg/jobs/registry.go` | Background loops: claim jobs, resume claimed jobs, drop claims whose session is dead. |
| `claimJobs` / `resumeClaimedJobs` | `pkg/jobs/adopt.go` | The SQL that sets `claim_session_id`, then the Go that calls `Resume`. |
| `JobUpdater` / `UpdateFn` | `pkg/jobs/update.go` | Transactional updates of status, payload, and progress. |
| `Resumer` | `pkg/jobs/registry.go` | `Resume`, `OnFailOrCancel`, `CollectProfile`. |
| `RegisterConstructor` | `pkg/jobs/registry.go` | `init` functions call this with a `jobspb.Type` and a constructor. |

Startup wiring is in `pkg/server/server_sql.go`: `jobRegistry.Start` runs inside `sqlServer.preStart`. `initJobScheduler` (`pkg/server/server.go`) starts scheduled jobs (`pkg/scheduledjobs` plus the registry daemon).

## How data and control enter and leave

**In.** SQL calls `CreateJobWithTxn` or a helper that does. The insert commits with the user's transaction, so a job does not exist if the `CREATE` / `ALTER` / `IMPORT` rolls back.

**Claim.** `Registry.Start` looks for rows whose claim is empty or whose claim session is no longer alive, then writes this node's session id. Two nodes should not resume the same job. The liveness session, not the process pid, is the lock.

**Run.** The constructor registered for that `jobspb.Type` returns a `Resumer`. `Resume` gets an execution context that can run internal SQL and KV. Progress is written back through `JobUpdater`. On failure, `OnFailOrCancel` runs.

**Out.** For a schema change, the output is updated descriptors and, when the change is done, a job status of succeeded. For TTL, the output is deleted rows. The client that started a detached job only gets a job id; `SHOW JOBS` reads `system.jobs`.

### Schema changes

Two resumers, both OSS:

| Job type | Registered in | Changer |
| --- | --- | --- |
| `NEW_SCHEMA_CHANGE` | `pkg/sql/schemachanger/scjob/job.go` (`newSchemaChangeResumer`) | Declarative. State lives in the descriptor's declarative schema changer state. The package comment is `pkg/sql/schemachanger/doc.go`. |
| `SCHEMA_CHANGE` | `pkg/sql/schema_changer.go` | Legacy. Descriptor `mutations` (add index, drop column, …) are walked by `SchemaChanger`. |
| `SCHEMA_CHANGE_GC` | `pkg/sql/gcjob` | Deletes the data left behind after a drop. |
| `TYPEDESC_SCHEMA_CHANGE` | `pkg/sql/type_change.go` | Enum and type changes. |

Which changer creates the job is the session setting `use_declarative_schema_changer`. Default `on` uses declarative for supported statements that can auto-commit (a single implicit statement). An explicit transaction uses the legacy job. See [sql.md](sql.md).

### Other OSS resumers

`pkg/server/server.go` blank-imports:

- `pkg/sql/importer` — `IMPORT` (and the CSV / Parquet export writer processors).
- `pkg/sql/ttl/ttljob` and `pkg/sql/ttl/ttlschedule` — row-level TTL. The processor variable in `pkg/sql/rowexec/processors.go` has a comment that says the implementation is CCL. The assignment `rowexec.NewTTLProcessor = newTTLProcessor` is in `pkg/sql/ttl/ttljob/ttljob_processor.go`. On this tree, TTL is OSS. The job can still be disabled by `sql.ttl.job.enabled`.

Statistics jobs and the GC job register from `pkg/sql` as well.

## What it depends on

- `system.jobs` and internal SQL (`pkg/sql/isql`).
- `pkg/sql/sqlliveness` for the claim session.
- `pkg/kv` for the transaction that writes the job row.
- `pkg/util/stop` so adoption shuts down with the node.
- Constructors from `pkg/sql/...`. The registry does not import those packages; they import `pkg/jobs` and register in `init`, which is why the blank imports in `server.go` matter. A binary that linked `pkg/jobs` but not `pkg/sql/importer` would still parse `IMPORT` and then fail to plan or run it.

## Invariants

- A job row and the descriptor mutation that needs it commit in one transaction. Otherwise a restart would see a half-applied schema change with no job, or a job with no mutation.
- At most one live claim per job. Adoption checks the SQL-liveness session, not "is the process still in `ps`".
- `Resume` must be safe to call again after a crash. The resumer reads progress and continues. Schema changers are written that way; a new resumer that is not idempotent will double-apply on the next adoption.
- Canceled and failed jobs run `OnFailOrCancel` once so side effects (backfills, GC) can stop.

## Gotchas

- **`SHOW JOBS` on a fresh node can sit at `running` with no progress** when no registry has adopted it yet. Adoption is asynchronous, inside `Registry.Start`.
- **Detached vs not.** `IMPORT ... WITH DETACHED` returns a job id immediately (`import_planning.go` switches the result header). Without it, the SQL session blocks in the job. A client timeout is not a job cancel unless the statement was written to cancel.
- **The legacy and declarative changers are different job types.** A stack trace in `schema_changer.go` is the legacy one. A stack trace in `pkg/sql/schemachanger/scjob` is the new one. The setting comment in `exec_util.go` that says the new changer is off by default is wrong; the default value is `on`.
- **CCL job types have no constructor here.** Creating the statement fails earlier, at planning (`opaque.go`), which is a better error than a job that can never resume. If you ever see `BackupData processor unimplemented` or `ChangeAggregator processor unimplemented` (`pkg/sql/rowexec/processors.go`), a plan was built without the matching `init`. That should not happen for `BACKUP` / `CREATE CHANGEFEED` on `cockroach-oss`, because those plan hooks are not registered in this binary.
- **Scheduled backups** are CCL statements (`ScheduledBackup` implements `cclOnlyStatement` in `pkg/sql/sem/tree/stmt.go`). The scheduler daemon itself is OSS. It will run OSS schedules (TTL) and has nothing to call for a backup schedule.
