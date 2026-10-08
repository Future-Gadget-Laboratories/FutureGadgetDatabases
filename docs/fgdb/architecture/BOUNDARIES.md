> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.
>
> This is a first pass. It lists what the OSS binary does when a CCL hook is missing. It does not describe how those features are implemented. `pkg/ccl` is in this git tree. Those files were not read for this page. Directory names were listed only.

# Boundaries

`cockroach-oss` is `pkg/cmd/cockroach-oss`, which imports `pkg/cli` and `pkg/ui/distoss` and nothing under `pkg/ccl` (`pkg/cmd/cockroach-oss/BUILD.bazel`). `pkg/build/info.go` sets `Distribution = "OSS"`. The other entry point, `pkg/cmd/cockroach/main.go`, blank-imports `pkg/ccl`, `pkg/ccl/cliccl`, and `pkg/ui/distccl`. That import is what runs the CCL `init` hooks. This project does not release that binary. The check is [build.md](build.md).

`base.CheckEnterpriseEnabled` (`pkg/base/license.go`) returns `OSS binaries do not include enterprise features` for every caller. The CCL build replaces the function. The `feature` argument is ignored on this binary, so the SQL text does not say which feature you asked for. Call sites wrap it with their own message when they want one.

`pkg/sql/sqlerrors.NewCCLRequiredError` marks the error with SQLSTATE `XXC01` (`pgcode.CCLRequired`).

## How a missing feature fails

Three patterns, all on the OSS side:

1. **The statement is marked `tree.CCLOnlyStatement`** (`pkg/sql/sem/tree/stmt.go`). `planOpaque` in `pkg/sql/opaque.go` calls `maybePlanHook`. If no hook returns a plan, the error is `a CCL binary is required to use this statement type: %T`.
2. **A package-level function variable stays at its OSS default.** The default returns an error or `false`. A CCL `init` would replace it. If you are searching, look for `CCL` in `pkg/sql`, `pkg/storage`, `pkg/server`, and `pkg/kv` — not in `pkg/ccl`.
3. **A DistSQL processor variable is nil.** `newProcessor` in `pkg/sql/rowexec/processors.go` returns `"<Name> processor unimplemented"`. Some of those variables are assigned from OSS `init` functions. The comment above the variable is not always right. The assignment is.

## pkg/ccl directories

Present in this tree, names only: `auditloggingccl`, `backupccl`, `baseccl`, `benchccl`, `buildccl`, `changefeedccl`, `cliccl`, `cloudccl`, `cmdccl`, `gqpccl`, `gssapiccl`, `importerccl`, `jobsccl`, `jwtauthccl`, `kvccl`, `logictestccl`, `multiregionccl`, `multitenantccl`, `oidcccl`, `partitionccl`, `pgcryptoccl`, `plpgsqlccl`, `schemachangerccl`, `securityccl`, `serverccl`, `spanconfigccl`, `sqlitelogictestccl`, `sqlproxyccl`, `storageccl`, `streamingccl`, `telemetryccl`, `testccl`, `testutilsccl`, `utilccl`, `workloadccl`, plus `ccl_init.go`.

`pkg/ui/distccl` is the CCL DB Console bundle. `pkg/ui/distoss` is the one this binary embeds.

What each directory does inside was not read. The OSS behavior below is the contract for `cockroach-oss`.

## Backup, restore, export, changefeeds

These types implement `cclOnlyStatement()` in `pkg/sql/sem/tree/stmt.go`:

| Statement | Type |
| --- | --- |
| `BACKUP` | `Backup` |
| `RESTORE` | `Restore` |
| `SHOW BACKUP` | `ShowBackup` |
| `ALTER BACKUP` | `AlterBackup` |
| scheduled backup | `ScheduledBackup`, `AlterBackupSchedule` |
| `EXPORT` | `Export` |
| `IMPORT` | `Import` — exception, see below |
| `CREATE CHANGEFEED`, `ALTER CHANGEFEED` | `CreateChangefeed`, `AlterChangefeed` |
| scheduled changefeed | `ScheduledChangefeed` |
| physical cluster replication | `CreateTenantFromReplication`, `AlterTenantReplication` |

No OSS `AddPlanHook` for backup, restore, changefeed, or export showed up under `pkg/sql`. The only `AddPlanHook` there is `"import"` in `pkg/sql/importer/import_planning.go`. So `BACKUP`, `RESTORE`, `EXPORT`, and `CREATE CHANGEFEED` hit the `opaque.go` error on this binary.

Processor variables that stay nil unless something assigns them (`pkg/sql/rowexec/processors.go`):

| Variable | Error if nil |
| --- | --- |
| `NewBackupDataProcessor` | `BackupData processor unimplemented` |
| `NewRestoreDataProcessor` | `RestoreData processor unimplemented` |
| `NewChangeAggregatorProcessor` | `ChangeAggregator processor unimplemented` |
| `NewChangeFrontierProcessor` | `ChangeFrontier processor unimplemented` |
| `NewStreamIngestionDataProcessor` | `StreamIngestionData processor unimplemented` |
| `NewStreamIngestionFrontierProcessor` | same pattern for the frontier processor |
| `NewGenerativeSplitAndScatterProcessor` | `GenerativeSplitAndScatter processor unimplemented` |

`NewCSVWriterProcessor` and `NewParquetWriterProcessor` are assigned from `pkg/sql/importer/exportcsv.go` and `exportparquet.go`. The writers exist. The `EXPORT` statement still has no OSS plan hook, so the statement should fail in `opaque.go` before a processor runs. That combination was not executed. See [OPEN-QUESTIONS.md](OPEN-QUESTIONS.md).

`IMPORT` is the odd one. The AST type is `CCLOnlyStatement`, but `pkg/server/server.go` blank-imports `pkg/sql/importer`, whose `init` registers `importPlanHook` and `rowexec.NewReadImportDataProcessor`. A hook that returns a plan skips the generic CCL error. `pkg/ccl/importerccl` still exists, so some import paths may be extra behavior on top of the OSS importer. This pass did not run `IMPORT`. The workload helper is stricter: `pkg/workload/workload.go` returns ``loading initial data with IMPORT requires a CCL binary`` from `requiresCCLBinaryDataLoader`.

Rangefeed (the KV stream) is OSS. Changefeed SQL is not. See [kv.md](kv.md).

## Multi-region, partitioning, zone configs

| Action | OSS gate | Error |
| --- | --- | --- |
| Create a multi-region database | `InitializeMultiRegionMetadataCCL` in `pkg/sql/descriptor.go` | `creating multi-region databases requires a CCL binary` |
| Add a region | `GetMultiRegionEnumAddValuePlacementCCL` in `pkg/sql/alter_database.go` | `adding regions to a multi-region database requires a CCL binary` |
| `PARTITION BY` | `CreatePartitioningCCL` in `pkg/sql/create_table.go`, and the same string from `pkg/sql/schemachanger/scdeps/build_deps.go` | `creating or manipulating partitions requires a CCL binary` |
| New zone config on an index or partition | `GenerateSubzoneSpans` calls `CheckEnterpriseEnabled` (`pkg/sql/partition_utils.go`) | `OSS binaries do not include enterprise features` |
| Drop an index while another index or partition still has a zone config | `pkg/sql/drop_index.go` | `schema change requires a CCL binary because table %q has at least one remaining index or partition with a zone config` |

Ordinary zone configs on a whole table or database (replica count, range size, lease preferences) are the path in `pkg/config/zonepb` and are what the lab uses. The enterprise check above is for subzones: per-index and per-partition.

## Encryption at rest

`storage.NewEncryptedEnvFunc` is nil. `ResolveEncryptedEnvOptions` in `pkg/storage/pebble.go`:

- `encryption is enabled but no function to create the encrypted env`
- `encryption was used on this store before, but no encryption flags specified. You need a CCL build and must fully specify the --enterprise-encryption flag`

`ConfigureForSharedStorage` is nil. Opening a store with shared storage set returns `shared storage requires CCL features`.

`pkg/cli/debug.go` leaves `PopulateStorageConfigHook` and `EncryptedStorePathsHook` unset. Debug commands that need those hooks do not gain encryption flags from OSS code.

## Replication streaming and multi-tenancy

`pkg/repstream/api.go`:

- `GetReplicationStreamManager` returns `replication streaming requires a CCL binary` when `GetReplicationStreamManagerHook` is nil.
- `GetStreamIngestManager` checks **`GetReplicationStreamManagerHook`**, not `GetStreamIngestManagerHook`, and returns the same string. If the producer hook were ever set and the ingest hook were not, the ingest function would call a nil function. On this binary both are nil, so the error is the one above. The swapped check is still a bug in the OSS file.

`pkg/kv/kvclient/kvtenant/connector.go`: `tenant connector with remote KV addresses requires a CCL binary`.

`pkg/server/node.go` has a `dummyTenantUsageServer` that returns `tenant usage requires a CCL binary` and `tenant resource limits require a CCL binary`.

A single-tenant `cockroach start` does not need those hooks. A SQL-only tenant process pointed at remote KV does.

## SQL features that are hooks, not whole subsystems

| Feature | Where | OSS result |
| --- | --- | --- |
| PL/pgSQL | `pkg/sql/plpgsql/plpgsql.go` `CheckClusterSupportsPLpgSQL` | `using PL/pgSQL requires a CCL binary` |
| Generic query plans | `pkg/sql/gpq/gpq.go` | `plan_cache_mode=force_generic_plan and plan_cache_mode=auto require a CCL binary` |
| `pgcrypto` encrypt / decrypt | `pkg/sql/sem/builtins/pgcrypto/pgcrypto.go` | `decrypt can only be used with a CCL distribution` (and `decrypt_iv`, `encrypt`, `encrypt_iv`) |
| Follower reads | `WithMinTimestamp`, `WithMaxStaleness` in `pkg/sql/sem/builtins/builtins.go` | `%s can only be used with a CCL distribution` |
| Role-based audit | `pkg/sql/auditlogging/audit_log.go` | `UserAuditEnabled` returns false. `ConfigureRoleBasedAuditClusterSettings` is an empty function. |
| JWT login | `ConfigureJWTAuth` in `pkg/sql/pgwire/auth_methods.go` | `JWT token authentication requires CCL features` |
| OIDC for the console | `ConfigureOIDC` in `pkg/server/authserver/authentication.go` | returns a config with `Enabled: false` |
| GSSAPI | directory `gssapiccl` exists; `loadDefaultMethods` does not register `gss` | no OSS method found. Unconfirmed beyond that. |
| License enforcer | `pkg/server/license/enforcer.go` `getInitialIsDisabledValue` | off unless `COCKROACH_ENABLE_LICENSE_ENFORCER=true`. `start-single-node` also calls `disableLicenseEnforcement` (`pkg/server/initial_sql.go`). |
| License type string | `base.LicenseType` | returns `"OSS"`. `GetLicenseTTL` returns 0. |

Row-level TTL is **not** in this table. `pkg/sql/ttl` assigns `NewTTLProcessor`. The comment in `processors.go` that says the processor is CCL is stale relative to that assignment. TTL can still be disabled with `sql.ttl.job.enabled` (`pkg/sql/ttl/ttlbase`).

Spatial types are OSS. They need `libgeos` on disk. See [geo.md](geo.md).

## Hard limits and assumptions the code states

These are defaults and caps in this tree, not goals.

| Item | Value | Where |
| --- | --- | --- |
| Default replicas | 3 | `zonepb.DefaultZoneConfig` in `pkg/config/zonepb/zone.go` |
| System-range replicas | 5 | `DefaultSystemZoneConfig` in the same file |
| `start-single-node` | sets replication to 1 via `disableReplication` | `pkg/server/initial_sql.go` |
| Default range size | min 128 MiB, max 512 MiB | `RangeMinBytes`, `RangeMaxBytes` in `DefaultZoneConfig` |
| Default GC TTL | 4 hours | `GC.TTLSeconds` in `DefaultZoneConfig` |
| Max clock offset | 500 ms default, refused above 5 s | `base.DefaultMaxClockOffset`, `maximumMaxClockOffset` in `pkg/server/config.go` |
| Tolerated offset | 80% of max offset, then the node fatals if half the peers are outside it | `ToleratedOffset`, `pkg/rpc/clock_offset.go`, `pkg/rpc/peer.go` |
| Transaction heartbeat | 1 s; 5× that and a conflict may abort it | `base.DefaultTxnHeartbeatInterval` |
| Raft command max | 64 MiB default, 4 MiB floor | `kvserverbase.MaxCommandSize` (`kv.raft.command.max_size`) |
| Chunk threshold | 256000 bytes | `base.ChunkRaftCommandThresholdBytes` |
| SQL work memory | 64 MiB before a processor spills | `sql.distsql.temp_storage.workmem` in `pkg/sql/exec_util.go` |
| Vectorize | `on` | `sql.defaults.vectorize` |
| DistSQL | `auto` | `sql.defaults.distsql` |
| Experimental DistSQL planning | `off` | `sql.defaults.experimental_distsql_planning` |
| Declarative schema changer | `on` | `sql.defaults.use_declarative_schema_changer` |
| Parallel commits | on (`kv.transaction.parallel_commits.enabled`; stored key `kv.transaction.parallel_commits_enabled`) | `txn_interceptor_committer.go` |
| Pebble data block | 32 KiB | `DefaultPebbleOptions` in `pkg/storage/pebble.go` |
| Gossip diameter | 5 hops | `maxHops` in `pkg/gossip/gossip.go` |
| Settings slots | 1023 | `settings.MaxSettings` |
| SQL / gRPC port | 26257 | `base.DefaultPort` |
| HTTP port | 8080 | `base.DefaultHTTPPort` |
| Store directory | `cockroach-data` | `DefaultStorePath` in `pkg/server/config.go` |
| Minimum store size, if set | 640 MiB | `pkg/base/store_spec.go` |
| Network FDs | minimum 256, recommended 5000 | `pkg/server/config.go` |
| Recorded spans per trace | 1000 | `pkg/util/tracing/tracer.go` |
| Query cache | 8 MiB (`defaultSQLQueryCacheSize`) | `pkg/server/config.go` |

`docs/design.md` still says RocksDB and a default range of 64 MiB, and it sketches a 4 exabyte ceiling. The range size and the storage engine disagree with this tree. The exabyte sentence was not checked against a constant. Do not cite `design.md` for those numbers.

Assumptions that are easy to miss:

- A write commits when a **majority of voting replicas** have the Raft entry. Replication factor 3 survives one lost replica, not two. Factor 1 (single-node) survives zero.
- Reads go to the **leaseholder**, unless you are on the CCL follower-read builtins, which this binary rejects.
- Clocks must stay within the max offset. The process is written to exit rather than serve when they do not. See [rpc.md](rpc.md).
- Every node runs the same binary. Mixed `cockroach` and `cockroach-oss` is not a configuration this tree documents, and the version gate on heartbeats (`checkVersion` in `pkg/rpc/peer.go`) is about cluster version, not about which hooks were linked.
- The PostgreSQL wire protocol is not full PostgreSQL. PL/pgSQL is the clearest example: the parser may know the syntax, and `CheckClusterSupportsPLpgSQL` still errors.

## Uncertain

- Exactly which `IMPORT` formats and URLs succeed on `cockroach-oss`. The plan hook is OSS. `importerccl` exists and was not read.
- Whether `EXPORT` can be reached through any hook outside `pkg/sql`. None was found there.
- GSSAPI, and any HBA method registered only from a CCL `init`.
- Every call site of `CheckEnterpriseEnabled`. The ones above are the ones this pass opened. More may exist.
- Behavior of `COCKROACH_ENABLE_LICENSE_ENFORCER=true` on a cluster with no license, beyond the error strings in `enforcer.go` (`License expired on %s…`, `No license installed…`). Not executed.
