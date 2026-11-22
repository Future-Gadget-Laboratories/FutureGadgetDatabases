> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Role-based SQL audit logging

## Overview

Database administrators can audit SQL statements executed by selected users or
members of selected roles. FGDb emits a structured `role_based_audit_event` on
the `SENSITIVE_ACCESS` logging channel when an eligible statement matches an
inclusive rule. The feature defaults to off.

## Spec

[PAR-210](../spec/PAR-210-role-audit-log.md) defines the FGDb settings and gate,
ordered rule matching, live configuration updates, optional session caching, and
event scope. The [cross-reference](#spec--code--test-cross-reference) maps those
requirements to the implementation and targeted tests.

## How it works

1. `pkg/server/server_sql.go` imports `pkg/fgdb/auditlog` for its `init` hooks.
   Each SQL server calls `ConfigureRoleBasedAuditClusterSettings` during setup.
   `configure` subscribes to setting changes before loading the initial value.
   `applyConfig` publishes a successfully parsed configuration under the write
   lock. A runtime parse failure retains the last good configuration and logs a
   warning that omits the raw value and parser error.
2. `planner.maybeLogStatement` calls `planner.maybeAuditRoleBasedAuditEvent`.
   `planner.shouldNotRoleBasedAudit` checks the boolean gate and the raw string
   before role membership lookup, then excludes reserved users and internal
   executors. The OSS membership resolver supplies the user's role memberships.
   `AuditConfigLock.GetMatchingAuditSetting` uses the first row that matches a
   membership, the username, or `ALL` (everyone). The filter `ALL` includes the
   statement and `NONE` suppresses it. No matching row produces no event. The
   ordered scan takes O(number of rows) per match.
3. With reduced configuration off, an existing session uses the current parsed
   configuration for each eligible statement. With it on, the first eligible
   SQL event initializes `planner.reducedAuditConfig` with an `ALL` rule, a
   `NONE` rule, or no match. Configuration and membership changes leave that
   decision in place. New sessions initialize from the current configuration.
4. `auditevents.RoleBasedAuditEvent.BuildAuditEvent` supplies the matched role
   and common SQL event details via the existing event and redaction machinery.
   `planner.logEventsOnlyExternally` routes the event through configured log
   sinks and bypasses `system.eventlog`.
   Cluster settings store the configuration. The parsed rules and the session
   decision live in memory, and events are written only to log sinks.

## External references

- CockroachDB v23.2 documents `sql.log.user_audit`, the ordered rule syntax, and
  the optional reduced configuration. [Role-based SQL Audit Logging](https://docs.cockroachlabs.com/docs/v23.2/role-based-audit-logging)
- CockroachDB describes role-based audit events and the other events that share
  `SENSITIVE_ACCESS`. [Logging Levels and Channels](https://docs.cockroachlabs.com/docs/stable/logging)
- CockroachDB documents the separate per-table audit control,
  `ALTER TABLE ... EXPERIMENTAL_AUDIT`. [ALTER TABLE](https://docs.cockroachlabs.com/docs/stable/alter-table)
- PostgreSQL's pgAudit extension offers session and object audit logging and
  provides a separate model for comparison. [pgAudit](https://github.com/pgaudit/pgaudit)
- PostgreSQL's built-in statement logging selects by statement class through
  `log_statement`. [PostgreSQL Error Reporting and Logging](https://www.postgresql.org/docs/18/runtime-config-logging.html#GUC-LOG-STATEMENT)

## Configuration and gating

| Setting | Default | Effect |
| --- | --- | --- |
| `fgdb.role_audit_log.enabled` | `false` | Allows role auditing when the configuration string is nonempty. |
| `fgdb.sql.log.user_audit` | empty string | Ordered, newline-separated `USER/ROLE STATEMENT_FILTER` rows. The limit is 64 KiB (65,536 bytes) and 1,000 non-empty rows. |
| `fgdb.sql.log.user_audit.reduced_config.enabled` | `false` | Caches the first eligible audit decision in the session. |

Setting validation and runtime loading enforce both limits before parsing.
Whitespace-only lines count toward the byte limit. Each line with non-whitespace
content counts toward the row limit. A limit violation returns SQLSTATE `53400`
and names the exceeded limit without including the setting value. A rejected
runtime load retains the last successfully parsed configuration.

All three settings are public `ApplicationLevel` settings and can be configured
per virtual cluster with ordinary cluster-setting privileges. For example, to
audit a role named `test_role`, set the configuration before enabling the gate.

```sql
SET CLUSTER SETTING fgdb.sql.log.user_audit = e'test_role ALL\nALL NONE';
SET CLUSTER SETTING fgdb.role_audit_log.enabled = true;
```

Set `fgdb.role_audit_log.enabled` to `false` to turn role auditing off while
keeping the rules. Clearing `fgdb.sql.log.user_audit` also stops role audit
events, including in sessions with cached inclusive rules. The gate-off query
path retains ordinary SQL behavior. The gate evaluates the flag and string
without a license or a mixed-version activation check.

**Proposed, pending.** The owner has yet to decide whether FGDb should accept
the upstream `sql.log.user_audit*` setting names. The current registrations
use the `fgdb.` prefix exclusively.

## Spec → code → test cross-reference

| Spec requirement | Code | Test |
| --- | --- | --- |
| [Public settings, defaults, and virtual-cluster scope](../spec/PAR-210-role-audit-log.md#settings-and-gate) | [`enabled`, `userAudit`, `reducedConfig`](../../../pkg/fgdb/auditlog/auditlog.go#L28) | [`auditlog_test.go` `TestSettingNamesAndDefaults`](../../../pkg/fgdb/auditlog/auditlog_test.go#L35); [`fgdb_audit` public listing and three default-value queries](../../../pkg/sql/logictest/testdata/logic_test/fgdb_audit#L4); [`integration_test.go` `TestRoleAuditEvents`](../../../pkg/fgdb/auditlog/integration_test.go#L46) runs in a shared-process secondary tenant. |
| [Ordinary setting authorization](../spec/PAR-210-role-audit-log.md#settings-and-gate) | [`checkPrivilegesForSetting`](../../../pkg/sql/set_cluster_setting.go#L70) | [`fgdb_audit` `testuser` attempts to enable the gate and receives a privilege error](../../../pkg/sql/logictest/testdata/logic_test/fgdb_audit#L47). |
| [Gate off with configured rules preserves query results and suppresses events, even with a cached match](../spec/PAR-210-role-audit-log.md#settings-and-gate) | [`UserAuditEnabled` hook](../../../pkg/fgdb/auditlog/auditlog.go#L57), [`planner.shouldNotRoleBasedAudit`](../../../pkg/sql/audit_logging.go#L151) | [`auditlog_test.go` `TestHookSemantics`](../../../pkg/fgdb/auditlog/auditlog_test.go#L137); [`integration_test.go` `TestRoleAuditEvents`, `gate off`](../../../pkg/fgdb/auditlog/integration_test.go#L65) and [`reduced config caches per session`, gate disabled after caching](../../../pkg/fgdb/auditlog/integration_test.go#L208); [`fgdb_audit` `SELECT 210` as `testuser` after configuring rules](../../../pkg/sql/logictest/testdata/logic_test/fgdb_audit#L37). |
| [Empty configuration suppresses events when enabled, even with a cached match](../spec/PAR-210-role-audit-log.md#settings-and-gate) | [`UserAuditEnabled` hook](../../../pkg/fgdb/auditlog/auditlog.go#L57), [`planner.shouldNotRoleBasedAudit`](../../../pkg/sql/audit_logging.go#L151) | [`auditlog_test.go` `TestHookSemantics`, `enabled empty`](../../../pkg/fgdb/auditlog/auditlog_test.go#L137); [`integration_test.go` `TestRoleAuditEvents`, `enabled empty`](../../../pkg/fgdb/auditlog/integration_test.go#L76) and [`reduced config caches per session`, clearing after caching](../../../pkg/fgdb/auditlog/integration_test.go#L200). |
| [Rule validation and normalization](../spec/PAR-210-role-audit-log.md#matching-and-updates) | [`userAudit` validator](../../../pkg/fgdb/auditlog/auditlog.go#L39), [`auditlogging.Parse`](../../../pkg/sql/auditlogging/parser.go#L24) | [`auditlog_test.go` `TestValidation`](../../../pkg/fgdb/auditlog/auditlog_test.go#L63) covers valid rows and missing, invalid, extra, duplicate, and reserved inputs; [`fgdb_audit` invalid `SET` with filter `SELECT`](../../../pkg/sql/logictest/testdata/logic_test/fgdb_audit#L28). Case normalization is inherited and has no dedicated case in these targets. |
| [Configuration size limits](../spec/PAR-210-role-audit-log.md#settings-and-gate) | [`parseUserAuditConfig`](../../../pkg/fgdb/auditlog/auditlog.go#L97) | [`TestUserAuditConfigLimits`](../../../pkg/fgdb/auditlog/auditlog_test.go#L86) checks both inclusive limits, blank lines, SQLSTATE, and omission of role names from limit errors. [`TestOversizedRuntimeConfigKeepsPrevious`](../../../pkg/fgdb/auditlog/auditlog_test.go#L207) checks that runtime rejection preserves the previous configuration. |
| [First matching username, membership, or `ALL` row wins; `ALL`, `NONE`, or no match determines emission](../spec/PAR-210-role-audit-log.md#matching-and-updates) | [`AuditConfig.getMatchingAuditSetting`](../../../pkg/sql/auditlogging/audit_log.go#L113), [`planner.maybeAuditRoleBasedAuditEvent`](../../../pkg/sql/audit_logging.go#L44) | [`auditlog_test.go` `TestMatchingRules`](../../../pkg/fgdb/auditlog/auditlog_test.go#L225) covers ordering and both filters; [`TestConfigureAndOnChange`](../../../pkg/fgdb/auditlog/auditlog_test.go#L167) checks no match after clearing; [`integration_test.go` `TestRoleAuditEvents`, `enabled role event`, `live config NONE`, and `live config ALL`](../../../pkg/fgdb/auditlog/integration_test.go#L67). No dedicated nonempty unmatched configuration case. |
| [Load at startup and apply live updates to existing sessions in normal mode](../spec/PAR-210-role-audit-log.md#matching-and-updates) | [SQL startup hook](../../../pkg/server/server_sql.go#L1384), [`configure`](../../../pkg/fgdb/auditlog/auditlog.go#L66) | [`auditlog_test.go` `TestConfigureAndOnChange`](../../../pkg/fgdb/auditlog/auditlog_test.go#L167) exercises initial load, replacement, and clearing; [`integration_test.go` `TestRoleAuditEvents`, `live config NONE` and `live config ALL`](../../../pkg/fgdb/auditlog/integration_test.go#L68). |
| [Retain the last good configuration and omit input from runtime warnings](../spec/PAR-210-role-audit-log.md#matching-and-updates) | [`configure`, `applyConfig`](../../../pkg/fgdb/auditlog/auditlog.go#L66) | [`auditlog_test.go` `TestBadRuntimeConfigKeepsPrevious`](../../../pkg/fgdb/auditlog/auditlog_test.go#L185) asserts retention at the application boundary. Warning contents have no dedicated assertion. |
| [Reduced mode caches a session decision and uses current rules for new sessions](../spec/PAR-210-role-audit-log.md#matching-and-updates) | [`UserAuditReducedConfigEnabled` hook](../../../pkg/fgdb/auditlog/auditlog.go#L60), [`planner.logReducedAuditConfig`, `planner.initializeReducedAuditConfig`](../../../pkg/sql/audit_logging.go#L94) | [`auditlog_test.go` `TestHookSemantics`](../../../pkg/fgdb/auditlog/auditlog_test.go#L137) checks the boolean hook; [`integration_test.go` `TestRoleAuditEvents`, `reduced config caches per session`](../../../pkg/fgdb/auditlog/integration_test.go#L80) checks cached `ALL` and `NONE` plus new sessions. Cached no-match decisions and membership changes have no dedicated case. |
| [Structured sensitive-access event with the matched role and existing SQL fields](../spec/PAR-210-role-audit-log.md#events-and-scope) | [`BuildAuditEvent`](../../../pkg/sql/auditlogging/auditevents/role_based_audit_event.go#L28), [`RoleBasedAuditEvent.LoggingChannel`](../../../pkg/util/log/eventpb/eventlog_channels_generated.go#L62), [`planner.logEventsOnlyExternally`](../../../pkg/sql/exec_log.go#L525) | [`integration_test.go` `TestRoleAuditEvents`, `enabled role event`](../../../pkg/fgdb/auditlog/integration_test.go#L67) captures the type, channel, user, role, and statement. Sink redaction and absence from `system.eventlog` have no dedicated assertions. |
| [Internal executor and reserved-user exclusions; table and admin audit remain independent](../spec/PAR-210-role-audit-log.md#events-and-scope) | [`planner.shouldNotRoleBasedAudit`](../../../pkg/sql/audit_logging.go#L151), [`planner.maybeAuditSensitiveTableAccessEvent`](../../../pkg/sql/audit_logging.go#L26), [admin audit path](../../../pkg/sql/exec_log.go#L290) | These are inherited OSS paths. The PAR-210 targets have no dedicated assertions for them. |
| [FGDb names and existing filters, event format, and storage](../spec/PAR-210-role-audit-log.md#events-and-scope) | [Setting registrations](../../../pkg/fgdb/auditlog/auditlog.go#L28), [`parseStatementFilter`](../../../pkg/sql/auditlogging/parser.go#L116), [`planner.logEventsOnlyExternally`](../../../pkg/sql/exec_log.go#L525) | [`auditlog_test.go` `TestSettingNamesAndDefaults`](../../../pkg/fgdb/auditlog/auditlog_test.go#L35) asserts the FGDb names and [`TestValidation`](../../../pkg/fgdb/auditlog/auditlog_test.go#L63) checks accepted and rejected filters. There is no dedicated alias or audit-storage test. |

## Test evidence

The round 2 builder compiled `auditlog.go` and `auditlog_test.go` with Go
1.21.12 and cached OSS dependencies. Both limit tests failed against `12ccd8ae`
before the change. All eight unit tests passed with the limits in place.
The `fgdb_audit` fixture declares the `local` config and is registered by
[`TestLogic_fgdb_audit`](../../../pkg/sql/logictest/tests/local/generated_test.go#L844).
Round 2 build-host results are pending.

| Orchestrator target | Build-host command | Result |
| --- | --- | --- |
| `//pkg/fgdb/auditlog:auditlog_test` | `bazel test //pkg/fgdb/auditlog:auditlog_test` | Pending for round 2. Covers configuration limits, the hook and parser integration, and an actual SQL session with captured logs. |
| Logictest `fgdb_audit`, config `local` | `bazel test //pkg/sql/logictest/tests/local:local_test --test_filter=TestLogic_fgdb_audit` | Pending. Covers discovery, defaults, validation, authorization, and query success with configured rules and the gate off. |

The Go integration test checks the absence of role-based events both before
enabling and after disabling the gate on a session with a cached inclusive rule.
The logictest asserts `SELECT 210` still returns `210` with rules configured and
the gate off. Event absence is asserted by the Go test.

## Limitations and differences

- FGDb provides the separate default-off feature gate and uses `fgdb.` setting
  names. It supports only the `ALL` and `NONE` filters.
- Events use the OSS structured format and redaction behavior, with delivery
  through the existing log sinks. This feature adds no persistent audit store.
- Table-access audit and admin audit use their existing controls. The feature
  adds neither pgAudit compatibility nor a guarantee of CCL interoperability.
- Reduced configuration uses the OSS session cache. The feature makes no
  measured performance claim.

## Change log

- 2026-11-22, PAR-210 / PR #59. Documented the initial default-off feature, its
  FGDb settings, and the corresponding test evidence. Build-host conformance
  remains pending.
