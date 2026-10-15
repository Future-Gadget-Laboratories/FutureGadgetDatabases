> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Open questions

Follow-ups for a test-coverage pass. Nothing here was treated as a fact in the other pages unless the page says the code was read. The binary was not built or started for this map.

## Run on a real `cockroach-oss`

1. **`IMPORT`.** `importPlanHook` and `NewReadImportDataProcessor` are registered from `pkg/sql/importer`, which `pkg/server` blank-imports. The AST type is still `CCLOnlyStatement`. Confirm a small `IMPORT INTO` of a local CSV succeeds, and write down the error for a URL or format that does not. `pkg/ccl/importerccl` was not read, so "which formats are only in CCL" is unknown.
2. **`EXPORT`.** No `AddPlanHook("export")` under `pkg/sql`. The CSV and Parquet writers are still assigned in `pkg/sql/importer`. Confirm the user-facing error is the `opaque.go` CCL message (`a CCL binary is required to use this statement type: *tree.Export`) and not `processor unimplemented`.
3. **`BACKUP` / `CREATE CHANGEFEED` / `PARTITION BY` / `CREATE FUNCTION` in PL/pgSQL.** Confirm the strings in [BOUNDARIES.md](BOUNDARIES.md) are what `psql` prints, including SQLSTATE `XXC01`.
4. **TTL.** `ALTER TABLE ... SET (ttl = ...)` or the equivalent this version accepts, plus a job that deletes an expired row. The processor comment says CCL; the assignment in `pkg/sql/ttl` says OSS. A test settles it.
5. **Spatial.** With the image's `/usr/local/lib/cockroach/libgeos.so`, `SELECT ST_IsValid('POINT(0 0)'::geometry)` should work. With the libraries renamed, the public error should be `geos: this operation is not available`. Confirm the search path `findLibraryDirectories` actually uses in the image, not only under `bazel test`.
6. **JWT and OIDC.** JWT should fail with `JWT token authentication requires CCL features`. OIDC should leave the console with SSO disabled and the node still starting.
7. **License enforcer off by default.** `SHOW` or the logs on a normal `start-single-node` should not mention throttling. Then, only if it is safe in a throwaway directory, `COCKROACH_ENABLE_LICENSE_ENFORCER=true` and record whether `No license installed…` appears. Do not point that experiment at a shared cluster.
8. **`version` output.** `Distribution: OSS`, build type `release` on a release build, Go `go1.21.12`. This is what `verify-oss-binary.sh` checks. It was not run here.

## Code that was only partly traced

9. **`GetStreamIngestManager` checks the wrong variable.** In `pkg/repstream/api.go` the ingest function tests `GetReplicationStreamManagerHook` and then calls `GetStreamIngestManagerHook`. Both are nil in this binary, so the error path works. A test that sets only one hook would crash or skip. Worth a unit test that does not need CCL code: set the producer hook to a non-nil stub and call the ingest function.
10. **Span configs versus gossip.** `KeyDeprecatedSystemConfig` is documented as unused after 22.1. The split/merge queue still has a gossip callback (`store_gossip.go`). Which updates actually come from the span-config subscriber (`pkg/spanconfig`, wired in `pkg/server/server.go`) was not walked end to end.
11. **`TODOEngine` versus `StateEngine` / `LogEngine`.** Replica MVCC uses `TODOEngine()`. Which keys, if any, are already on the other two engines in a v23.2.15 store was not measured.
12. **GSSAPI.** No `RegisterAuthMethod("gss")` in `pkg/sql/pgwire`. `pkg/ccl/gssapiccl` was not opened. Confirm an HBA line with method `gss` fails in a clear way on `cockroach-oss`.
13. **Every `CheckEnterpriseEnabled` caller.** Known ones are in [BOUNDARIES.md](BOUNDARIES.md). A test-coverage pass should grep `CheckEnterpriseEnabled` and `NewCCLRequiredError` under `pkg/` excluding `pkg/ccl`, and try each statement once.
14. **Declarative schema changer fallback.** Default is `on`, and `planner.SchemaChange` returns `nil, nil` for explicit transactions. A test should `BEGIN; ALTER TABLE ...; COMMIT;` and show a `SCHEMA_CHANGE` job rather than `NEW_SCHEMA_CHANGE`, then the same `ALTER` outside a transaction and show the opposite. The setting's description string says the feature is off by default. That string is wrong; the test is the demonstration.
15. **Vectorized fallback list.** `colflow.IsSupported` decides per processor. Which common plans (zigzag join, some window functions, CDC) still use `rowexec` was not enumerated.
16. **1PC versus parallel commit on a one-row `INSERT`.** The committer supports both (`pkg/kv/kvclient/kvcoord/txn_interceptor_committer.go`). Parallel commits default on. The user-visible setting is `kv.transaction.parallel_commits.enabled` (internal key `kv.transaction.parallel_commits_enabled`). A trace of `INSERT INTO t VALUES (1)` on an implicit transaction should show whether the client waited for one Raft round trip or two. The sequence diagram in [README.md](README.md) draws two steps and calls out the shortcut. A trace would pick the real shape.
17. **`docs/design.md` capacity claim.** It says the design scales toward 4 exabytes. No constant matching that number was looked up. Treat it as an old design note.

## Not done, on purpose

18. **No file under `pkg/ccl` or `pkg/ui/distccl` was read.** Directory names are listed in [BOUNDARIES.md](BOUNDARIES.md). A later pass still should not summarize CCL internals. It can add OSS tests that assert the error strings.
19. **The release workflow was not executed.** [build.md](build.md) is from the scripts and the Dockerfile. A publish run is a separate exercise.
20. **Package comments that disagree with defaults** (schema changer, TTL processor) are called out where they were seen. There are certainly more stale comments. When a comment and a registration disagree, the registration is what this map trusted.
