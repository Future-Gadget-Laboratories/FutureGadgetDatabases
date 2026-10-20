> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Enterprise comparison and evidence rules

The machine-readable [comparison log](comparison-log.json) is a dated, versioned inventory. Its first target is **v23.2-era self-hosted Enterprise observable behavior**, including shared core behavior. Newer Enterprise capabilities have a separate track. Nothing in this register claims that every listed capability is Enterprise-exclusive, that the inventory is exhaustive, or that FGDb has achieved parity.

Public source references are catalogued in [comparison-sources.md](comparison-sources.md). The source code evidence is the permitted FGDb baseline at `2d2befea10288726b8da5654031a503acfb2b512`. PR #17 and #24 evidence is explicitly identified as branch/released-binary evidence, not merged behavior.

## What counts as parity

For each feature, separately decide:

- Functional scope: accepted inputs, outputs, SQLSTATE/errors, cancellation and privileges.
- SQL/driver compatibility: syntax, result shapes, catalog behavior and client protocols.
- Persistence and artifacts: restart, versioned formats and restore compatibility. Enterprise backup-file interoperability is a separate opt-in requirement, not assumed from functional backup support.
- Correctness and faults: atomicity, ordering, deletes, duplicate effects, crash and partition behavior.
- Operations: progress, metrics, schedules, cleanup, upgrade and rollback.
- Performance: target data size, concurrency and topology, with measured bounds rather than a vendor headline.

Every requested dimension must have scoped acceptance cases. A package name, parsed statement, hook, successful build or green helper test is not a completed feature. A capability can be functionally useful while its compatibility status remains partial.

## Current comparison summary

**Inherited core:** SQL, PostgreSQL wire access, distributed transactions, range replication, vectorized execution, jobs, metrics, and TTL machinery already exist in the permitted tree. Characterize and test them before commissioning replacements. Procedures, isolation options and advanced diagnostics need a more detailed inventory instead of a blanket missing label.

**Backup:** FGDb has a separate full logical copy tool. Native BACKUP/RESTORE SQL remains gated. Incremental chains, retained revision-history PITR, distributed locality-aware backups, complete cluster operational recovery, and native job/schedule behavior are distinct gaps. S3 server-side encryption is not an encrypted portable backup format with key rotation. This is the first delivery track because recoverability must precede risky engine changes.

**Streaming:** OSS rangefeed-related code and replication manager interfaces are substrates. They do not establish a durable SQL changefeed or physical-cluster replication implementation. PR #24's guard correction does not fill either product gap.

**Placement and reads:** multi-region policy DDL, survival goals and follower-read behavior require precise tests. A cluster stretched over regions is not automatically equivalent to declared table locality or residency policies. A historical query does not establish that a follower served it.

**Security:** TLS, certificate/password authentication and structured logs must be inventoried as existing behavior. Role-based SQL auditing has no-op hook behavior in the OSS side. Native store encryption and enterprise identity options need their own evidence; a filesystem adapter, external login proxy or S3 setting cannot silently stand in for the native guarantee.

**Later track:** logical cross-cluster replication, row-level policies, and vector/index compatibility are useful newer comparison candidates. Pin their exact target release, public behavior and availability when they enter scope. They do not move the v23.2 baseline's goalposts.

## Status vocabulary

- `inherited-needs-conformance`: implementation exists, but target behavior is not yet accepted as equivalent.
- `inherited-partial-evidence`: some named runtime evidence exists; the whole contract remains unverified.
- `bolt-on-partial`: an independent tool provides a subset, with explicit exclusions.
- `absent-gated` / `absent-from-oss-release`: the shipped OSS binary does not link the corresponding product implementation or rejects the native entry point.
- `not-implemented` / `not-established`: absent from the current product plan/evidence; not a proof that no useful substrate exists.
- `scope-unverified` / `inventory-required`: leave uncertainty visible until precise source/runtime characterization resolves it.
- `oss-noop-hook`: the inspected permitted hook defaults to no effect; logs elsewhere do not fill the promised policy.
- `external-orchestration-candidate`: a proposed workflow, not shipped native scheduling.

All rows currently retain `not_verified_for_parity`. Existing tests can be recorded without promoting a whole row. Preserve negative cases and skipped checks; distinguish a released binary from a server compiled at the candidate SHA.

## Update log, not just checkboxes

A row update should record target release/edition, source revision, spec revision and approval, implemented dimensions, test identifiers, binary SHA/checksum, exact command, result and evidence URL. Commit the change as a dated log entry. An expected OSS rejection proves the rejection contract; it does not prove the requested feature was added.

Before calling the baseline complete, finish an explicit public-documentation inventory audit and have a maintainer accept its scope. Do not compute a parity percentage from these seed rows. They differ substantially in size and risk, and `inventory_complete` is false.

These planning descriptions are not approved cleanroom specifications. Apply [CLEANROOM.md](../CLEANROOM.md) before implementing excluded behavior. Public behavior research and permitted OSS code are the inputs; proprietary source is not.
