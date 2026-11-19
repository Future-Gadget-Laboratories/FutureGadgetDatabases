> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Runtime contracts and feature insertion points

Source baseline: `2d2befea10288726b8da5654031a503acfb2b512`, inspected 2026-10-20. This additive guide complements architecture PR [#8](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/8) and its follow-up [#25](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/25). It does not replace their package maps or declare their open questions closed. See [the short architecture guide](../architecture.md).

## Follow one request

1. `pkg/server`, `pkg/cli` and `pkg/sql/pgwire` assemble the node and accept a PostgreSQL wire connection. Authentication and session privileges apply before a statement becomes execution work.
2. `pkg/sql/parser` creates an AST. Semantic analysis resolves names, checks types and applies SQL rules. A parsed keyword is not proof that a usable implementation is linked.
3. `pkg/sql/opt` transforms relational expressions, retaining alternatives in memo groups. Statistics and a cost model guide selection. `xform/join_order_builder.go` explicitly uses a hypergraph, conflict rules and **DPSube** join enumeration. DPHyp is a TODO in this pin, not an implemented algorithm to advertise.
4. Physical planning arranges row or vectorized operators into execution flows. `pkg/sql/rowexec`, `colexec`, `execinfra` and `flowinfra` own hash/merge joins, aggregation, sorting, top-K, flow transport and spilling. Vectorization can fall back; an operator's presence does not prove it serves every query shape.
5. `pkg/kv/kvclient/kvcoord` routes batches across ranges and coordinates transactions. Refresh/retry, intents, write pipelining and qualifying one-phase/parallel commit paths provide behavior above individual consensus groups.
6. `pkg/kv/kvserver` evaluates and replicates range operations. Each range has its own Raft group. Leases and timestamps govern serving authority. Raft orders replicated state changes; not every read traverses Raft.
7. `pkg/storage` implements MVCC integration over Pebble. A store engine contains many ranges; the system does not create a separate Pebble database for every range. Versioned keys, WAL, SSTables, compaction, Bloom filters and version GC support the storage path.
8. The gateway delivers rows or an error. Cancellation, statement retry, ambiguous commit outcomes and client reconnects are observable contracts. An independent feature must define these, not just its happy-path result.

## Durability, transactions and time are different contracts

Per-range consensus is not cross-range transaction atomicity. A majority for one range does not imply the whole cluster can serve every table. Availability depends on the ranges involved, placement, leases and surviving replicas. Replication also copies a committed deletion, so it cannot replace independent backup.

HLC combines physical time with a logical component. `pkg/kv/kvserver/store_send.go` advances it from incoming KV timestamps; responses propagate timing information back through the client path. RPC heartbeats use physical clocks to monitor skew. They are not the mechanism that advances HLC from observed writes. State the required consistency model explicitly when testing real-time histories; do not infer strict serializability from the mere presence of HLC or Raft.

Gossip distributes bootstrap, descriptors and cluster information. A missing sentinel wakes bootstrap and marks gossip stalled. That state does not universally disable SQL/KV admission. Source anchors: `pkg/gossip/gossip.go`, `pkg/gossip/client.go`, `pkg/rpc/heartbeat.go`, `pkg/util/hlc/doc.go`.

## Long-running work must survive re-execution

`pkg/jobs` persists state and claims jobs with SQL liveness. A new process can adopt a job after failure. Both `Resume` and `OnFailOrCancel` must tolerate repeat invocation. In `Registry.stepThroughStateMachine`, unsuccessful cleanup or lost context can leave a job reverting for later execution; cleanup is not exactly-once.

`Stopper.Quiesce` closes `ShouldQuiesce`, cancels contexts registered through `WithCancelOnQuiesce`, invokes quiesce callbacks and waits for tasks. It does not cancel arbitrary caller contexts passed to `RunAsyncTask`. A long-running feature must watch the proper signal or obtain a registered context. Calling `Stop` from a task the same stopper waits for can deadlock.

For a new native job, document durable progress, retry boundaries, ownership transfer, duplicate external effects, cancellation, cleanup and mixed-version behavior before implementation. Merely moving an external process into a goroutine loses the operational contract.

## Backup is currently an independent SQL client

`pkg/cmd/fgdb-backup` is a separate Go module and executable. It reads user schema and data through ordinary SQL. It captures a timestamp for historical row reads, budgets work against GC lifetime, streams PGCOPY data, optionally compresses it, and records file hashes and row counts in a manifest. It can publish to local storage or S3. Its temporary GC TTL changes are not a general protected-timestamp or revision-history service.

Restore checks artifacts, recreates compatible objects, loads rows, and handles dependencies in phases. Views, foreign keys, sequence state, grants and zones have distinct treatment. Grants and zones can be best-effort warnings; passwords and full cluster operational state are not faithfully cloned. A data-row snapshot is not proof that every administrative object was captured at the same instant.

Swap restore stages a separate database and renames after loading; the old copy is retained according to policy. In-place IMPORT commits destructive changes before loading. Listener, advertised-host, TLS, artifact and privilege refusals must happen before that destructive boundary. Even correct preflight does not make later failures transactional; the recovery policy must state what happens after the drop commits.

PR #17 repairs listener ordering and width-bearing array exports, but its reviewed head still permits an unusable advertised host to bypass the guard, and still accepts known-unsafe COPY loading. Block promotion on repairs and value-level round trips. SHA256 and equal row counts do not detect a BYTES value restored as escaped text or a truncated CHAR array element. Old artifacts do not change when an exporter is fixed.

## Where a feature belongs

- **External adapter:** SQL client, object storage, scheduler or monitoring integration with an explicit privilege boundary. It is deployable separately and has a separate version.
- **Native control plane:** job persistence, privileges, progress, cancellation, retries and scheduling in the server. Keep behavior/artifact contracts versioned. Do not shell-execute the backup CLI inside a node as a shortcut to native integration.
- **Native SQL semantics:** parser, catalog, planner, execution, authorization and result/error contracts. New syntax must accurately describe its guarantees; using the `BACKUP` keyword does not prove Enterprise equivalence.
- **Storage/replication substrate:** MVCC export/import, change capture, GC protection, range split/merge behavior, transaction boundaries and replication state. This requires multi-node failure tests and upgrade design, not only client tests.

Some features cannot safely be delivered as equivalent bolt-ons. A SQL proxy cannot enforce native row security if clients can connect directly; SQL polling is not lossless CDC; OS disk encryption is not proof of native store-key rotation. A prototype may establish workflows or benchmarks while the compatibility register continues to say partial or unsupported.

## Architecture PR alignment

For #8, correct the retry, shutdown, HLC and gossip wording before using it as an implementation contract. For #25, preserve those corrections when refreshing its conflicted parent relationship, and reconcile its packaging/import-boundary claims with the actual tooling revision. Do not blindly replace the parent files with the older stacked snapshots.

The delivery work should extend `pkg/fgdbtest` and `fgdb/test` rather than introducing a second cluster harness. Keep it test-only: the OSS target's disallowed-import test on main excludes `pkg/fgdbtest` along with the prohibited enterprise paths. Add claims only with test identifiers; retain `partial` until the stated runtime evidence exists.
