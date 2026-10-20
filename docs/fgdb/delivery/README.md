> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Documentation and delivery stack

Planning baseline agreed on 2026-10-20: establish **v23.2-era Enterprise behavior coverage first**, then evaluate newer Enterprise capabilities separately. This is a planning inventory, not an approved cleanroom feature specification or a claim of completed parity.

The documentation is being reviewed in three layers:

1. **Foundation:** [local Go setup](../development/local-go.md) and [runtime contracts](../architecture/runtime-contracts.md). Base: current main, independently mergeable. Follow architecture PRs [#8](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/8) and [#25](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/25) as related work; do not inherit #25's conflict.
2. **Comparison:** [capability register and evidence rules](enterprise-parity.md), [comparison log](comparison-log.json), and [public sources](comparison-sources.md). Base: foundation branch.
3. **Delivery:** [stages and gates](stages.md), [31 dependency-linked slices](slices.json), [feature handoff checklist](feature-handoff.md), [courtesy discovery](courtesy-features.md), and [actual local validation](local-validation-2026-10-20.md). Base: comparison branch.

Each later layer must be reviewed as its incremental diff. Merge foundation first; retarget comparison to main once foundation lands, then retarget delivery after comparison lands. Refresh later branches when an earlier layer changes. No existing PR branch is force-updated by this stack.

Implementation dependencies are not the same as Git branch dependencies. The documents can be reviewed now. Backup product promotion waits on #17's remaining safety repairs; hook validation waits on #24's exact production-package tests; source contracts remain linked to the corrections in #8/#25.

## Before a feature enters implementation

Use [CLEANROOM.md](../CLEANROOM.md) and [agent enforcement](../../agent/CLEANROOM-ENFORCEMENT.md). Record the named feature, public behavior sources, spec author, maintainer approval, spec revision and separate implementer. This planning stack does not assign a spec writer or waive that handoff. Ordinary permitted-core fixes remain ordinary OSS contributions.

Keep these milestones separate: source inspection, helper/unit tests, released-binary integration, candidate OSS build, candidate integration, fault/upgrade tests and release publication. Do not report a later milestone because an earlier one passed.
