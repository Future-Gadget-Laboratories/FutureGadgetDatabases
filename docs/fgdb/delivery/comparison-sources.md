> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Public comparison sources

Reviewed 2026-10-20. The references below are public documentation and product descriptions, not implementation source. Their short descriptions state why they were consulted; acceptance still requires version-pinned behavior and independent tests. Mutable `stable` URLs belong only to the later/courtesy discovery tracks until pinned.

## Permitted FGDb evidence

- **FGDB-CORE:** [FGDb source baseline](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/tree/2d2befea10288726b8da5654031a503acfb2b512). Inspect only permitted core; `pkg/ccl` and enterprise UI sources are excluded.
- **FGDB-BACKUP:** [logical backup guide](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/blob/2d2befea10288726b8da5654031a503acfb2b512/docs/fgdb/backup.md) and `pkg/cmd/fgdb-backup`. Records current logical-copy scope and limitations.
- **FGDB-AUDIT:** [OSS audit hook defaults](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/blob/2d2befea10288726b8da5654031a503acfb2b512/pkg/sql/auditlogging/audit_log.go). Evidence of no-op defaults, not evidence about proprietary implementation.

## v23.2-era comparator

- **CR-BACKUP:** [Backup and Restore Overview](https://www.cockroachlabs.com/docs/v23.2/backup-and-restore-overview) and [BACKUP](https://www.cockroachlabs.com/docs/v23.2/backup). Public behavior families: full/incremental, revision history/PITR, encryption, schedules and locality.
- **CR-CDC:** [Change Data Capture Overview](https://www.cockroachlabs.com/docs/v23.2/change-data-capture-overview) and [CREATE CHANGEFEED](https://www.cockroachlabs.com/docs/v23.2/create-changefeed). Public interfaces for change delivery, sinks and progress.
- **CR-PCR:** [v23.2 launch overview](https://www.cockroachlabs.com/blog/crdb-232-launch/) and [v24.1 physical-replication availability context](https://www.cockroachlabs.com/blog/232-physical-cluster-replication/). Distinguish the v23.2-era capability from later general availability; do not infer later cutback guarantees for the older target.
- **CR-MULTIREGION:** [Multi-Region Capabilities](https://www.cockroachlabs.com/docs/v23.2/multiregion-overview). Public locality and survival-policy concepts.
- **CR-FOLLOWER:** [Follower Reads](https://www.cockroachlabs.com/docs/v23.2/follower-reads). Public timestamp/routing behavior to characterize.
- **CR-ENCRYPT:** [Encryption](https://www.cockroachlabs.com/docs/v23.2/security-reference/encryption). Separate data in motion, store encryption and backup protection.
- **CR-SSO:** [SQL SSO with JWT](https://www.cockroachlabs.com/docs/v23.2/sso-sql). Public identity behavior; exact supported scope remains a characterization task.
- **CR-TTL:** [Row-level TTL](https://www.cockroachlabs.com/docs/v23.2/row-level-ttl). Compare with existing OSS TTL code rather than assuming it is missing.
- **CR-232-LAUNCH:** [v23.2 launch](https://www.cockroachlabs.com/blog/crdb-232-launch/). Discovery of procedures/UDF/isolation work; individual language semantics still need detailed inventory.

The fetched v23.2 licensing FAQ contains later licensing and throttling text. A version in a URL is not sufficient evidence of historical feature entitlement. We do not use that mutable FAQ to decide which v23.2 capabilities were exclusive, to change FGDb's license policy, or to copy later code. A comparator runtime, if needed, must be separately authorized and legitimately runnable.

## Newer Enterprise discovery

- **CR-LDR:** [Logical Data Replication](https://www.cockroachlabs.com/docs/stable/logical-data-replication-overview) and [v24.3 introduction](https://www.cockroachlabs.com/blog/logical-data-replication/). Table-level cross-cluster behavior; separate from physical replication.
- **CR-RLS:** [Row-Level Security](https://www.cockroachlabs.com/docs/stable/row-level-security). Native policy enforcement and documented backup/CDC boundaries.
- **CR-VECTOR:** [v25.2 release overview](https://www.cockroachlabs.com/blog/cockroachdb-252-performance-vector-indexing/). Discovery of vector/index capabilities; the later target and maturity must be pinned before acceptance. No proprietary index implementation is consulted.

## Other database ecosystems

- **PG-RLS:** [PostgreSQL 18 row security](https://www.postgresql.org/docs/18/ddl-rowsecurity.html). Public policy model and backup interactions.
- **PG-REPLICATION:** [PostgreSQL 18 logical replication](https://www.postgresql.org/docs/18/logical-replication.html). Publication/subscription integration candidate.
- **PG-SEARCH:** [PostgreSQL 18 text search](https://www.postgresql.org/docs/18/textsearch.html). Inspect existing FGDb text-search support before proposing additional compatibility.
- **MYSQL-CLONE:** [MySQL 8.4 Clone Plugin](https://dev.mysql.com/doc/refman/8.4/en/clone-plugin.html). Clone workflow inspiration; not a claim that MySQL physical files are compatible with FGDb.
- **SQLITE-SESSION:** [SQLite Session Extension](https://www.sqlite.org/sessionintro.html). Portable changeset/conflict workflow inspiration, distinct from a database-wide CDC guarantee.

External references describe capabilities, not grants to reuse implementations. Source inventories, licensing review and behavioral specifications remain separate work. No competitor performance number is adopted as an FGDb result.
