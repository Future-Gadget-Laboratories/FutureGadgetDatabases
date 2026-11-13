> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# From documented behavior to a native feature

This is the proposed delivery order, not permission to implement all listed features. [slices.json](slices.json) records dependencies and completion evidence; [comparison-log.json](comparison-log.json) records capabilities. Start with v23.2 Enterprise behavior, then create a separately pinned newer target. Calendar estimates wait for feature scoping and baseline measurements.

## 0. Make the baseline executable

Use [local-go.md](../development/local-go.md), record [local validation](local-validation-2026-10-20.md), and keep the OSS binary pin. Resolve remaining #17 backup safety findings and execute #24's production-package hook regression before those changes are treated as validated. Helper checks are not candidate-server checks. Preserve the OSS import boundary and test-only harness boundary.

## 1. Document the feature before building it

Reconcile #8's runtime contracts and #25's older stacked descriptions. Audit inherited support; do not rewrite working TTL, text search or materialized-view code because the comparison has a generic feature name. Select one capability row and one useful minimum scope.

Use [feature-handoff.md](feature-handoff.md) to prepare the decision record. For excluded capabilities, assign the named spec writer and separate implementer through the repository's cleanroom process. This planning stack is not an approved behavioral spec. Ordinary fixes inside the permitted lineage do not require inventing a new proprietary-feature specification.

Completion: target release/edition, observable inputs/results/errors, privilege model, consistency/durability requirements, format scope, exclusions, acceptance cases and maintainer decision are recorded. Explicitly decide whether SQL compatibility, Enterprise backup-file compatibility or only functional substitution is requested.

## 2. Build a useful external feature

The first candidate is **reliable scheduled logical backup with restore rehearsal**, built around the existing `fgdb-backup` executable after safety repairs. This advances a usable product without pretending that native BACKUP, incrementals or PITR already exist.

Split it into reviewable changes: restore refusal/type safety (B01), value/schema corpus and rehearsal (B02), external scheduling/retention (B03), and optional portable artifact encryption (B04). Keep configuration and manifests versioned, define cancellation/retry and expose useful progress. Secrets stay out of artifacts, logs and SQL URLs. Scheduling must handle overlapping workers and stale ownership; retaining old database copies is not backup-chain retention.

A read-only plan must identify the proposed work and refusals. Running plan/preflight is not a proof that the database nodes can reach every future URL or that a later network failure cannot occur. Dangerous restore operations need an explicit recovery contract after their destructive commit boundary.

For a feature that cannot be equivalent externally—native row security, store encryption or locality policies—the external stage may be a conformance harness or workflow prototype. Label the limitation. Do not ship a proxy/polling substitute under the native feature's guarantee.

## 3. Test the external feature as a product

Test exact values, not only table sizes and hashes. Include BYTES, CHAR/BIT/DECIMAL arrays and type modifiers, collations, NULL versus empty values, generated columns, identity/sequence state, views, FKs and grants. Reject unsupported combinations before destructive work. Test both new and old artifact versions; a fixed exporter does not fix old data files.

Exercise local and S3-compatible stores, partial multipart uploads, corrupted/truncated artifacts, interrupted publication, expired historical snapshots, lock expiry/overlap, scheduler restart, bad TLS, invalid advertise URLs, permission failures and imports that cannot reach their source. Run against a disposable cluster and verify the original rows after every expected preflight refusal. Test post-drop failures separately: do not claim an in-place IMPORT is atomic.

Completion: reproducible corpus, test identifiers, binary/checksum/source provenance, executed/skipped/failed evidence and scoped performance measurements. Decide the acceptable backup/restore data sizes and resource budgets from measurements. Do not reuse a green unit test as proof that the cluster cases ran.

## 4. Integrate natively while keeping bolt-on backup

N01 is an engine-managed logical-backup job/control plane. Extract a reusable, reviewed boundary where appropriate; keep the standalone CLI versioned and independently usable. Native work adds persisted progress, privilege checks, job ownership, repeat-safe external effects, bounded cancellation and scheduling. Do not have server processes shell-execute the CLI as the integration mechanism.

N02 builds the actual OSS candidate through Bazel and packages the compatible external backup executable alongside it. Preserve the existing release target and disallowed imports. Explicitly version the server, CLI and logical artifact compatibility matrix.

This is an intermediate native milestone. N04 separately completes the approved native SQL entry-point contract after N03 candidate testing and before the incremental-backup implementation slice. An internal job is not yet native Enterprise BACKUP semantics, MVCC incremental export, PITR or Enterprise artifact interchange. A dedicated FGDb API can be useful while an Enterprise-compatible SQL contract is still being specified. Do not attach the BACKUP keyword to weaker guarantees without making the scope clear.

Keep the external backup path available through stabilization and upgrades. Its removal, if ever proposed, requires a separate decision after native restore coverage, operational support and rollback evidence are accepted. An emergency recovery path should not disappear in the same change that introduces a new engine path.

## 5. Test the native candidate and external fallback together

N03 uses the same behavioral corpus across both paths, with explicitly different guarantees where needed. Add crash/re-adoption, node loss, cancellation during external writes, repeated cleanup, privilege revocation, upgrade/rollback and artifact migration. Mixed-version nodes must either interoperate or reject unsafe job admission explicitly. External fallback must read the supported logical formats without depending on the failed native path.

Build and inspect the candidate that actually contains the feature. Then test the packaged tarball/image, not only developer binaries. Publish only after these gates; a compile pass and a successful release upload are separate evidence.

## 6. Deliver the v23.2 parity slices

Follow dependency order in the slice register; independent tracks can progress after their prerequisites, rather than forcing one giant PR.

- **P00–P01:** inherited SQL/driver contracts and distributed correctness evidence.
- **P02–P04:** incremental chains, retained history/PITR, locality-aware and whole-cluster operational recovery. Incrementals must capture deletes and DDL and protect needed history; timestamp polling or repeated full copies do not satisfy this contract.
- **P05–P06:** durable changefeeds and checkpoint/frontier/topology behavior. Specify duplicates and ordering; do not promise exactly-once external delivery without a sink transaction/idempotency design and tests.
- **P07:** separately scoped physical cross-cluster recovery. It can reuse fault-testing infrastructure but does not require pretending that logical and physical replication are the same feature.
- **P08:** region/locality policy and follower-read evidence with real placement/fault topology.
- **P09–P10:** native key lifecycle, identity and audit gaps, with independent failure/security review.
- **P11–P12:** remaining SQL compatibility and observability gaps after inherited behavior is inventoried.
- **P13:** inventory audit and acceptance of the declared profile. Resolve every in-scope unknown, collect candidate fault/upgrade/recovery evidence and name exclusions. Excluded requirements mean a limited profile, not unqualified full Enterprise parity.

## 7. Newer Enterprise and courtesy discovery

L00 freezes the exact newer target after the baseline. L01–L03 seed logical replication, row security and vector/index evaluation. Expand the inventory rather than assuming these three examples cover the successor release.

[courtesy-features.md](courtesy-features.md) begins the other-database discovery now, so opportunities are visible early. It is not an implementation commitment. Prefer useful operational/compatibility improvements with reuse of existing behavior; approve implementation order after recovery and current correctness blockers are addressed.

## Review and release discipline

Keep each PR about one behavioral slice. Its description names the acceptance clauses, failure behavior, rollback, executed tests and known skips. Do not merge the planning PRs as though they contain implementation. Retarget the documentation stack in order; do not force-update #8/#25. Revisit source references and test provenance whenever a dependent PR changes.

The stopping condition is evidence for an explicitly accepted scope, not exhaustion of a feature-name checklist. Never turn `partial`, unrun or skipped evidence into an achieved guarantee.
