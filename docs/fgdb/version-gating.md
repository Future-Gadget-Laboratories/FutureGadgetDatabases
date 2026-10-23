> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Version gating and feature flags

**Design status:** Proposed; needs maintainer approval  
**design_approved_by:** _unset_

This note defines how FGDb-only behavior will be introduced while preserving the v23.2.15 `cockroach-oss` behavior when the feature is disabled. It is a design contract; this change does not register cluster settings or wire feature code into the server.

## GATE: disabled means today's behavior

Every `fgdb.<feature>.enabled` setting is a boolean with default `false`. When it is false, callers take the existing OSS path. They must not enter FGDb-only code, perform FGDb side effects, or replace the existing error with an FGDb-specific message. Off-path parity covers feature-operation behavior: results, errors, SQLSTATE, and job state. Registering an `fgdb.*` setting adds a settings-catalog entry, so the setting name is visible in `SHOW ALL CLUSTER SETTINGS` even while its value is false.

An entry point must check the flag before FGDb-specific work. For example, a future in-core backup entry point checks `fgdb.backup.enabled` before creating any FGDb job, schedule, manifest, or file. When off, it follows the current `BACKUP`/`RESTORE` path and returns the same OSS/CCL-boundary error as the v23.2.15 oracle. This is the GATE contract. CockroachDB's public v23.2 [`BACKUP`](https://www.cockroachlabs.com/docs/v23.2/backup.html) and [`RESTORE`](https://www.cockroachlabs.com/docs/v23.2/restore.html) references describe those SQL statements; exact parity remains subject to black-box recording.

| Setting | Type | Default | Initial purpose |
| --- | --- | --- | --- |
| `fgdb.backup.enabled` | `ApplicationLevel` boolean cluster setting | `false` | Enable the future in-core FGDb backup implementation for the tenant whose setting is true, after the compatibility checks below. |

Register settings through the existing `pkg/settings` machinery when server wiring is implemented. In this v23.2.15 tree, `ApplicationLevel` is the class name; its setting value is separate for the system tenant/interface and each virtual cluster. Enablement is therefore per tenant: the system tenant and each virtual cluster remain off until that tenant's setting is explicitly enabled. A virtual cluster cannot enable the feature for another tenant. This tenant-local scope is the conservative choice because opt-in cannot silently enable FGDb behavior for other tenants; feature implementations must remain tenant-scoped and must not use this flag to turn on shared storage behavior. The setting name is public and reportable like other feature settings, including its presence in `SHOW ALL CLUSTER SETTINGS` while off. Do not use the upstream feature-flag default or its “disabled by database administrator” error behavior: an off FGDb flag preserves today's feature-operation path and error.

The small `pkg/fgdb/gate` package defines the setting-name and branch contract in pure Go. Registration, SQL `SET CLUSTER SETTING` integration, telemetry, and feature execution are follow-up work.

## Enabling a feature on a cluster

Before a tenant enables a feature, every live, non-decommissioned node in the shared cluster must report an FGDb release at or above that feature's required release **N**. Determine node identity from the existing gossip `roachpb.NodeDescriptor.build_tag`, which is populated from build metadata. In v23.2.15, `crdb_internal.node_build_info` populates `Version` directly from `build.GetInfo().Tag`, so it is build-tag-derived as well and can expose FGDb release identity. Parse the tag strictly. Reject unknown, missing, malformed, or below-required tags when enabling.

This check uses the existing node membership and liveness semantics. It does not add cluster-version keys or bump the cluster `version` setting. The binary's CRDB compatibility version and active cluster version remain on the `V23_2` line; `ServerVersion` continues to use CRDB `23.2.x` semantics. The build tag carries FGDb release identity. Because `node_build_info.Version` is tag-derived, an FGDb tag can be visible to SQL clients querying that virtual table; the exact client-visible effect and any required compatibility treatment remain open for implementation and oracle validation.

The setting transition in the tenant is the control point: check all live nodes, reject the transition with a clear error if any node fails, then persist the tenant's setting through existing cluster settings storage (`system.settings`). Recheck membership and versions when enabling after upgrades; operational guidance for long-lived enabled tenants must describe what happens when an older or unknown node joins. That runtime policy and the error's exact SQLSTATE/message are open for implementation and oracle validation.

The helper in `pkg/fgdb/release` recognizes the current fix-line form and a provisional new-line form. It does not yet decide cross-line ordering or implement node enumeration.

## Rollback matrix

| Transition | Rule |
| --- | --- |
| Old fix line → new feature line | Normal rolling restart while all FGDb feature flags remain off. |
| New feature line → old fix line | Allowed only when every tenant's `fgdb.<feature>.enabled` setting is false, no tenant has ever enabled one, and no FGDb persisted format exists. |
| First transition of any flag to true in any tenant | Rollback cutover for the whole shared cluster. A durable activation marker or an explicit durable operator record is required; the record must identify the feature, tenant, and first activation and remain verifiable after the setting is turned off. |
| Any FGDb persisted format exists | Blocks downgrade regardless of current flag value. This includes future jobs, schedules, and manifests. Their format compatibility must be designed and tested before implementation. |

`system.settings` stores current values and does not provide value history, so its row alone cannot prove that a flag was never enabled. The implementation design must provide durable evidence independent of the current value, such as a write-once activation marker recorded on the first transition to true, or an explicitly maintained durable operator record with the same information. Downgrade eligibility requires checking that evidence for every feature and tenant and confirming that no marker or record exists. This is a design requirement only: PAR-003 adds no table, column, or marker implementation.

## Protobuf add-only rule

Future FGDb-owned protobuf messages may add fields only. Never renumber or reuse a field number. Readers ignore unknown fields. During a mixed FGDb patch rollout, writers must not require new fields to be understood by old peers. PAR-003 makes no `.proto` changes.

## Future BSL rebase

The gate contract, release parser, and flag definitions belong under Apache-2.0 `pkg/fgdb/` (or in this document). Any future code derived from converted BSL material stays behind the flags and follows the separate cleanroom and review discipline in [CLEANROOM.md](CLEANROOM.md). Feature availability is gated by FGDb release metadata and the flag contract, never by new CockroachDB cluster-version keys.

## Version lines and tag parsing

Decision D1 selected a separate FGDb feature line. The existing `v23.2.15-fgdb.N` tags remain a fixes-only line; they are not the native-feature release line. D1a, the exact name grammar, is still pending.

The options are:

| Option | Example tags | Base version location |
| --- | --- | --- |
| N1, FGDb semver | `fgdb-v1.0.0`, `fgdb-v1.1.0`, `fgdb-v1.0.0-rc.1` | Build metadata and release metadata, for example `fgdb-v1.0.0+crdb.v23.2.15` in long version output and image labels. |
| N2, base-anchored | `v23.2-fgdb2.0.0`, `v23.2-fgdb2.0.0-rc.1` | Tag prefix. |
| N3, calendar line | `fgdb-2026.1`, `fgdb-2026.1.1`, `fgdb-2026.2-rc.1` | Build metadata and release metadata. |

Until D1a is decided, the new-line parser grammar is provisional N1: `fgdb-vMAJOR.MINOR.PATCH` with optional `-rc.N`. The current old-line parser accepts `v23.2.15-fgdb.N`. `pkg/fgdb/release` labels the N1 grammar provisional in code. Once D1a is decided, update the grammar and build-tag metadata contract together.

Both lines keep the binary's CRDB compatibility version and cluster version on the `V23_2` line; existing CRDB-shaped version parsing and `ServerVersion` semantics remain unchanged. The build tag carries FGDb release identity. Since `crdb_internal.node_build_info.Version` is populated from that tag in v23.2.15, the field can display the FGDb tag to SQL clients; the client-visible compatibility impact and any follow-up separation are open items for oracle validation. Tag-filter and release-workflow updates are a follow-up; this change does not edit `.github/`.

## Validation and dependencies

| Evidence | Status |
| --- | --- |
| Black-box comparison against recorded stock v23.2.15 `cockroach-oss` and official full-binary behavior, including results, errors, SQLSTATE, `SHOW` output, and job state | **PENDING** |
| Multi-node mixed-version checks: old line plus new line, enable rejection, partial rollout, and mixed FGDb patch levels with flags on | **PENDING** |
| Oracle recordings and conformance cases | **PENDING**; no oracle conformance run is available yet for this design change. |

PAR-004 supplies benchmark baselines and harness; PAR-008 supplies the secure harness. Persisted jobs, schedules, manifests, system tables, or storage formats remain blocked on maintainer approval of this design and on their own compatibility tests.

## Open decisions and approval

- **D1a:** Select N1, N2, or N3. N1 is the provisional parser grammar, not a final naming decision.
- Decide whether required release N is global or specified per feature.
- Decide whether compatibility is checked only when enabling or also on reads and during an enabled cluster's lifetime.
- Define the homogeneity rejection error and confirm its SQLSTATE against oracle recordings.

**Maintainer approval:** `design_approved_by: _unset_`  
This proposal needs maintainer approval before any risk=yes work depends on it.
