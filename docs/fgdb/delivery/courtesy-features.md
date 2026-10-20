> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Courtesy-feature discovery

Initial research, 2026-10-20. These are candidate benefits for FGDb users, not promises or an assertion that the underlying feature is missing. Effort bands are relative scoping estimates, not delivery dates: S = bounded documentation/tooling, M = adapter or contained subsystem change, L/XL = protocol, security or storage work. Confirm them with a spike before scheduling. Recovery/correctness blockers take precedence over implementation; research can proceed now.

## C-01: Disposable development clones and restore rehearsal

**Reference:** [MySQL 8.4 Clone Plugin](https://dev.mysql.com/doc/refman/8.4/en/clone-plugin.html). The transferable user benefit is a repeatable, observable clone workflow. MySQL's physical file copying is not compatible with FGDb's engine.

**FGDb opportunity:** orchestrate the existing logical backup/restore into an isolated disposable database/cluster, run value/schema checks, then expose connection information. Default to restoring a test fixture; real-data clones require an approved data-handling scope and redaction before access. Never copy production credentials into a developer clone.

**Reuse/dependencies:** B01–B03 and the existing cluster harness. **Effort/risk:** M / medium. **First evidence:** failed clone leaves the source unchanged; teardown is bounded; rehearsal identifies corrupt/unsupported artifacts and preserves exact values. This is the best initial courtesy candidate because it strengthens recovery evidence.

## C-02: PostgreSQL publication/subscription interoperability

**Reference:** [PostgreSQL 18 logical replication](https://www.postgresql.org/docs/18/logical-replication.html).

**Opportunity:** evaluate whether a narrowly scoped replication adapter would simplify migrations and downstream integrations. A PostgreSQL wire connection alone does not implement its replication protocol, publications or subscriptions. Keep FGDb's native CDC guarantees separate from protocol compatibility.

**Dependencies:** P05–P06, a pinned protocol/semantic scope and separate feature approval. **Effort/risk:** XL / high. **First evidence:** snapshot-to-stream handoff, deletes, primary-key changes, type mappings, schema changes, resume after disconnect and declared ordering. Discovery only until the prerequisite stream is dependable.

## C-03: Text-search compatibility and ergonomics

**Reference:** [PostgreSQL 18 text search](https://www.postgresql.org/docs/18/textsearch.html).

**Existing FGDb substrate:** `pkg/sql/sem/builtins/tsearch_builtins.go`, inverted text-search planning and `tsvector` logic tests already exist. Do not propose a new search subsystem without identifying an actual gap.

**Opportunity:** a compatibility corpus and usable examples for tokenization, ranking, language handling and indexed plans. **Dependencies:** P00/P11. **Effort/risk:** S–M for inventory/examples; potentially L for semantic gaps. **First evidence:** pin expected results and inspect plans on representative languages; do not present an example as a throughput benchmark.

## C-04: Materialized-view refresh behavior

**Reference:** [PostgreSQL 18 REFRESH MATERIALIZED VIEW](https://www.postgresql.org/docs/18/sql-refreshmaterializedview.html).

**Existing FGDb substrate:** `pkg/sql/refresh_materialized_view.go`, materialized-view tests and backup handling. The open question is compatibility of refresh, concurrency, privileges and operational behavior, not whether FGDb has materialized views at all.

**Opportunity:** document and test refresh visibility and failure/restart semantics before adding ergonomic scheduling or additional syntax. **Dependencies:** P00/P11 and native scheduling if selected. **Effort/risk:** M / medium, potentially L for engine behavior. **First evidence:** concurrent readers observe the declared contract during refresh; canceled/failed refresh preserves a usable prior view where promised.

## C-05: Columnar-file analytics bridge

**Reference:** [DuckDB Parquet documentation](https://duckdb.org/docs/extensions/parquet), whose public search-index description identifies projection/filter pushdown. Direct page retrieval returned a redirect during this review, so deeper format/version verification remains open.

**Opportunity:** inspect existing FGDb importer/exporter support, then consider a standalone Parquet interchange/analytics bridge before proposing native external scans. File interoperability is distinct from integrating another engine into transactions.

**Dependencies:** B02, actual importer inventory, approved type/null/timezone mappings. **Effort/risk:** M for a bounded export adapter; XL for native scans/pushdown. **First evidence:** exact decimals, timestamps, NULLs and nested types; explain which filters execute where and measure bytes read. Discovery confidence is lower until the direct source/version is captured.

## C-06: Portable changesets for edge/offline workflows

**Reference:** [SQLite Session Extension](https://www.sqlite.org/sessionintro.html).

**Opportunity:** evaluate an independently specified changeset import/export tool for bounded datasets. Its usefulness would come from explicit conflict reporting and validation, not from claiming offline SQL replicas have the same transaction model as FGDb.

**Dependencies:** P05–P06, artifact versioning, an approved conflict policy. **Effort/risk:** L/XL / high. **First evidence:** duplicates, missing rows, FK conflicts, concurrent changes, partial apply and retry. State whether a whole changeset is atomic; do not silently drop conflicts.

## C-07: Native row-policy conformance suite

**Reference:** [PostgreSQL 18 row security](https://www.postgresql.org/docs/18/ddl-rowsecurity.html).

**Opportunity:** use a public-behavior corpus to make the later L02 policy feature safer and familiar to PostgreSQL users. This is the same underlying track as the newer Enterprise comparison, not a second implementation project.

**Dependencies:** L02 scope, privilege model and backup/CDC policy boundaries. **Effort/risk:** XL / high. **First evidence:** default deny, role combinations, write checks, owner/bypass scope, views and prepared plans; backup must not silently omit protected rows. A proxy is insufficient if direct connections bypass it.

## C-08: Generated-column and identity migration diagnostics

**Reference:** [PostgreSQL 18 generated columns](https://www.postgresql.org/docs/18/ddl-generated-columns.html).

**Existing FGDb behavior:** computed columns and identity handling already occur in SQL and the logical backup code. `GENERATED ALWAYS AS IDENTITY` is currently an explicit backup limitation. Generated columns and identity columns are separate concepts and must not be conflated.

**Opportunity:** a migration/preflight report that names exact incompatible constructs before backup or destructive restore; later implement selected compatibility gaps. **Dependencies:** B02/P00/P11. **Effort/risk:** S–M for diagnostics; larger for execution changes. **First evidence:** supported computed values round-trip, unsupported constructs produce clear object-specific errors, and no diagnostic probe mutates live tables.

## Selection rule

For each candidate, record a real user need, existing-support inventory, functional benefit, maintenance cost, failure/security risk, architecture owner and measurable acceptance. Prefer C-01 and bounded C-03/C-08 improvements after the backup safety gates. Larger replication, policy and external-scan ideas stay discovery work until their substrate and specification gates are satisfied.

All sources are public behavior descriptions. No competitor implementation has been copied or translated. A future dependency choice requires its own license/security review; these references do not select a library or authorize an implementation.
