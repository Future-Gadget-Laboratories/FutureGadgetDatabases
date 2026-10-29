> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Feature scoping and handoff checklist

Use this to prepare a **named** feature's documentation and approval record. It is a checklist, not a completed spec. Follow [CLEANROOM.md](../CLEANROOM.md); do not use this file to assign yourself the spec-writer role or to waive separation of roles.

1. **Identity and target:** comparison ID; exact reference version/edition; user problem; compatibility dimensions requested; in/out-of-scope behavior.
2. **Existing behavior:** permitted source revision; relevant package/hook; released and candidate runtime observations; inherited support to reuse; known safety defects and uncertainty.
3. **Source and role record:** public reference URLs and observation dates; named spec writer when required; maintainer review; approved spec hash; separate clean implementer and implementation revision. Record ordinary permitted-core classification if that is the route.
4. **Observable contract:** inputs/results/errors; privileges; catalog changes; timing/cancellation; transaction/ordering/consistency guarantees; data formats; recovery and unsupported cases. Keep it behavior-only, without proprietary control flow or code.
5. **External milestone:** minimum useful tool/adapter; versioning; deployment; credentials; bounded resource use; explicit differences from native behavior. If external equivalence is impossible, state why and define the limited prototype/test harness.
6. **Acceptance corpus:** smallest positive case; adversarial/negative cases; exact values; deletes and DDL; cancellation/crash/retry; corrupted/truncated inputs; source/target version matrix; rollback and no-data-loss assertions.
7. **Native milestone:** responsible server layer; state migration; job persistence and idempotency; ownership/cancellation/cleanup; privilege boundary; format compatibility. Architecture choices are made independently inside permitted implementation work after the behavior handoff.
8. **Fallback:** standalone backup support matrix, recovery procedure and rehearsal, old/new artifacts and mixed-version behavior. State the conditions for refusing unsafe recovery.
9. **Evidence and approval:** source/binary checksums, commands, test IDs, skips/failures, artifacts and measured resource/performance bounds; independent review for high-risk work; maintainer acceptance and release gate.

Advance the comparison row only for the dimensions actually demonstrated. A “not supported” response is valid baseline evidence, not successful implementation of that feature.
