> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-210 — gated role-based SQL audit logging

Version: v2, 2026-11-18. Behavior authority: the maintainer-provided PAR-210
packet task. Implementation and this behavior record: Codex, using that handoff
and the allowed OSS tree. Maintainer review and build-host conformance remain
pending; this document records the supplied requirements, not an independently
approved expansion of their scope. See [CLEANROOM.md](../CLEANROOM.md).

See the [role-based SQL audit logging feature guide](../features/role-audit-log.md) for implementation details and test evidence.

## Settings and gate

All three settings are public application-level settings, listed by
`SHOW CLUSTER SETTINGS` and configurable independently inside a virtual cluster
through the normal cluster-setting privileges.

| Setting | Type | Default | Behavior |
| --- | --- | --- | --- |
| `fgdb.role_audit_log.enabled` | boolean | `false` | Enables this FGDb feature when the configuration string is also nonempty. |
| `fgdb.sql.log.user_audit` | string | empty | Ordered, newline-separated `USER/ROLE STATEMENT_FILTER` rows. Setting validation and runtime loading reject values over 64 KiB or 1,000 non-empty rows with SQLSTATE `53400`. The existing OSS parser validates values within both limits. |
| `fgdb.sql.log.user_audit.reduced_config.enabled` | boolean | `false` | Caches the matching rule once per session, at its first eligible SQL audit event. |

With the gate off, SQL execution retains its OSS behavior and emits no
role-based audit events, even with a nonempty configuration or a cached match.
An empty configuration also disables this audit path while the gate is on.
The setting names intentionally use the `fgdb.` prefix. Upstream setting names
are not aliases.

Example configuration (requires the appropriate cluster-setting privilege):

```sql
SET CLUSTER SETTING fgdb.sql.log.user_audit = e'test_role ALL\nALL NONE';
SET CLUSTER SETTING fgdb.role_audit_log.enabled = true;
```

## Matching and updates

Rows are considered in their written order. A row matches the executing username,
any role membership recognized by the OSS membership resolver, or the special
role `ALL`, which matches everyone. The first matching row wins. Its filter is
`ALL` to log statements or `NONE` to suppress them. No match produces no event.
Names and filters follow the OSS parser's normalization and validation rules;
duplicate roles, reserved names, missing filters, and unknown filters are rejected.

Without reduced configuration, existing sessions use the updated configuration
as soon as the setting is applied on their SQL server. Configuration is also
loaded when the server starts. A runtime parse failure preserves the last
successfully parsed configuration and produces a warning without the raw input
or role names.

With reduced configuration, a session retains its first matching rule, including
a `NONE` rule or no match. Later configuration and membership changes do not
recompute that cached decision. New sessions use the current configuration.
The gate and the nonempty-string check still apply to every statement, including
sessions with a cached rule. Disabling the gate stops their audit events.

The existing matching engine scans ordered rows, O(rows) per match. This feature
adds no alternative data structure or matching algorithm. Reduced configuration
reuses the OSS session cache; no performance improvement is claimed without
measurement.

## Events and scope

Eligible statements emit the existing `role_based_audit_event` structured event
on `SENSITIVE_ACCESS`, including the matched role and existing SQL event details.
The OSS event builder and redaction behavior are unchanged. These are external
log events, not rows in `system.eventlog`. Configure log sinks and protect access
to sensitive SQL logs using the existing logging facilities.

The existing OSS exclusions for internal executors and reserved users remain.
Table-access and admin audit logging are independent and unchanged. This slice
adds no license check, CCL interoperability guarantee, upstream setting aliases,
new event format, persistent audit store, or additional statement filters.

## Tests

- `TestSettingNamesAndDefaults`: names, empty/off defaults, application-level class,
  and public visibility.
- `TestValidation`: valid inputs and rejection of malformed or reserved inputs.
- `TestHookSemantics`: disabled/configured combinations, enabled-but-empty, and
  the reduced-configuration boolean.
- `TestConfigureAndOnChange`: initial configuration, live replacement, and clearing.
- `TestBadRuntimeConfigKeepsPrevious`: invalid runtime input retains the previous rule.
- `TestMatchingRules`: first matching role, username precedence, `ALL`, and `NONE`.
- `TestRoleAuditEvents`: a real user with role membership; sensitive-access event
  contents; gate off; immediate config updates on an existing session; empty
  config; reduced caching for both included and excluded sessions; new-session
  matching; gate off after a session has cached an included rule. Runs inside a
  shared-process secondary tenant to cover application-level settings, using
  the embedded client certificate for `testuser`. After setting changes, waits
  for parsed configuration publication or the corresponding boolean hook before
  running statements that assert audit behavior.
- Logic file `fgdb_audit` (local): public listing, defaults, invalid `SET`, SQL
  success with the gate off and a nonempty config, and ordinary cluster-setting
  authorization.

Build-host commands (not executed by the restricted builder):

```sh
bazel test //pkg/fgdb/auditlog:auditlog_test
bazel test //pkg/sql/logictest/tests/local:local_test --test_filter=TestLogic_fgdb_audit
./dev testlogic --files=fgdb_audit
```

The local logic-test registration is included in this change. The direct Bazel
target runs it without regenerating wrappers. No Bazel or Go tests were run in
the builder workspace under the packet rules; these commands are for the build host.

## Cleanroom sources

- Maintainer handoff: PAR-210 task packet, public behavior and FGDb settings design.
- Public behavioral reference supplied in that handoff:
  <https://www.cockroachlabs.com/docs/v23.2/role-based-audit-logging>.
  No network retrieval was performed in this task.
- Allowed OSS v23.2.15 lineage in this worktree: `pkg/sql/auditlogging`,
  `pkg/sql/audit_logging.go`, `pkg/sql/exec_log.go`, OSS SQL audit tests,
  `pkg/settings`, `pkg/server/server_sql.go`, and `pkg/fgdb/gate`.
- FGDb gate context: [PAR-001](PAR-001-gate-contract.md) and
  [PAR-005](PAR-005-pgcrypto.md). Default-off preserves the existing SQL audit path.

No proprietary source or newer upstream source was consulted. The maintainer
must review this local change and run conformance before landing it.
