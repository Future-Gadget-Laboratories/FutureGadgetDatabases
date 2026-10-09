# Clean-room reimplementation at Future Gadget Laboratories (FGDb)

**Audience:** hobbyists, junior engineers, and anyone contributing to FutureGadgetDatabases (FGDb / Future Gadget Laboratories).

**Not legal advice.** This document describes an engineering process used to keep a reimplementation separate from proprietary source. Laws and licenses differ by country and by product. When in doubt, stop and get counsel review before using materials you are not allowed to copy.

---

## What “clean-room” means here

A **clean-room** (also called a **two-team** process) is how you rebuild *behavior* of a system without copying its *expression* (source, comments, internal structure, distinctive naming).

The pattern used here:

1. One group (**spec writers**) studies **public behavior and public documentation** and writes a **functional specification** — what inputs produce what outputs, error codes, timing constraints, wire formats, SQL semantics, and so on. They do **not** paste proprietary code into that spec.
2. A separate group (**implementers**) has **never** seen the proprietary source or notes copied from it. They implement **only** from the approved spec plus public references (standards, published docs, our own OSS tree).
3. A **maintainer** reviews every handoff from spec writers to implementers so expression (code snippets, comment text, distinctive structure) does not leak into the implementation.
4. You **document** who wrote the spec, who implemented it, and which spec version was used. If proprietary source gets into the implementer side, you **refuse and discard** that work and start over — you do not “edit it a little.”

This process is useful as evidence of **independent creation** for **copyright**. It does **not** bypass **patents**, and it does **not** override **license terms**. Treat those as separate constraints.

---

## When this process applies at FGDb

| Situation | Clean-room required? |
|-----------|---------------------|
| Work entirely inside our OSS fork of CockroachDB **v23.2.15** (Apache after BSL conversion) and public standards | No. Ordinary OSS contribution rules apply. |
| Reimplementing behavior that exists only in CockroachDB **Enterprise / CCL** code, or in trees under the **CockroachDB Software License (CSL)** (generally **v23.2.16+** patches after the CSL switch, and **24.3+**) | **Yes.** Spec writers use public documentation and public behavior. Implementers work only from the approved spec. |
| Someone pastes “just a snippet” of proprietary CRDB source into chat, a PR, or an issue | **Stop.** Treat as contamination. Do not merge. Discard or quarantine. |
| Checking out a CSL/`enterprise` tree into an **implementer** workspace “for reference” | **Forbidden.** That collapses the wall. |

**Hard product rule:** FGDb’s licensed base is **CockroachDB OSS v23.2.15 only** (post-conversion Apache). We never copy proprietary Enterprise/CSL code from later trees into FGDb. Features that later CRDB released under proprietary terms are candidates for **clean-room reimplementation from public behavior and public documentation**, not for cherry-picks from forbidden trees.

---

## Roles (keep them separate)

### Spec writers

- Read public docs, OSS v23.2.15, published SQL behavior, and tests you can run against a build you are allowed to use.
- Write **behavior-only** specs: APIs, wire formats, SQL/error semantics, invariants, acceptance tests described as *observable* results.
- Must **not** include: proprietary source lines, comment text from a proprietary tree, internal symbol names taken from that tree, or “copy this function” guidance.
- Record, in the ticket or the spec header, who wrote the spec for this feature.

### Implementers

- See **only**: approved specs, public standards, FGDb’s OSS tree, and public CRDB OSS materials that are already in our allowed lineage.
- Never clone CSL or enterprise trees, and never accept pasted proprietary source.
- Every non-trivial change should cite a **spec clause** or public standard — not “I saw it in enterprise.”

### Maintainer

- Reviews specs before they reach implementers: strip expression leakage.
- Owns contamination response: quarantine, discard, re-spec if needed.
- Approves commits and pull requests that land clean-room work in the public repo.

A hobby project may not hire an independent third-party monitor. That is a known gap versus a fully audited commercial clean-room. Compensate with **strict artifact separation**, **written refuse/discard rules**, and **no landing of this work without a maintainer’s approval**.

---

## Process steps (feature-sized)

1. **Scope.** Name the feature and why it is not already in OSS v23.2.15. Confirm it is not a forbidden cherry-pick.
2. **Assign spec writers.** Record who is writing the spec for this feature.
3. **Describe public behavior.** Use published docs, SQL tests, and inputs and outputs you can observe from a program you are allowed to run.
4. **Write the spec.** Behavior, edge cases, compatibility matrices. No proprietary source. Prefer tables and “when X then Y” over prose that restates someone else’s implementation line-by-line.
5. **Maintainer pass.** Someone who understands both contamination risk and the feature reviews the spec. Reject or rewrite it if it contains copied expression.
6. **Handoff.** Only the approved spec (and public refs) enters implementer channels and repos.
7. **Implement** in FGDb from the spec. New code, new tests. License spirit: as close to BSD/Apache as our project policy allows.
8. **Conformance.** Tests that prove *behavior* matches the spec, without comparing the new code to proprietary source trees.
9. **Record.** Keep: spec version hash, who wrote the spec, who implemented, maintainer notes, test evidence. Treat this as the audit trail.
10. **Land only with maintainer approval.** No silent push of clean-room features to GitHub.

---

## What is forbidden

- Copying, translating, or “lightly rewriting” proprietary CRDB Enterprise/CSL source into FGDb.
- Checking out CSL or enterprise trees into **implementer** workspaces (including “read-only” clones, zip dumps, or “just for grep”).
- Pasting proprietary source into Discord, GitHub issues/PRs, agent chats, or specs.
- Asking an AI agent that has been shown proprietary source to “now write the FGDb version” in the same session without a fresh clean context and an approved spec.
- Putting proprietary source, or notes copied from it, into the public repo or implementer trees.
- Using this process as an excuse to ignore **license terms** or **patents**.

---

## Concrete examples

### Good

- Spec says: “Statement `ALTER …` must return error code `XYZ` when the table is offline; retry after lease transfer succeeds.” Implementers write new Go from that rule and add SQL tests.
- Spec writers turn a published description into a field table. A maintainer checks that the table has no symbol names taken from proprietary source. Implementers code the codec from the table.

### Bad (refuse / discard)

- Contributor opens a PR titled “Port backup encryption from crl-enterprise” with files clearly derived from a CSL tree.
- Chat paste: “Here’s the enterprise function, just change the package name.”
- An implementer is handed proprietary source, or notes copied from it, and told to “clean it up into FGDb style.”
- Spec contains a 40-line block that is recognizably the same control flow and variable names as proprietary source.

When that happens: **stop**, mark the branch or workspace contaminated, **do not merge**, tell a maintainer, and start again from an approved spec if the feature still matters.

---

## Limits (read this twice)

- **Copyright ≠ patents.** Independent implementation can still infringe a patent.
- **License terms still apply.** CSL and enterprise terms are not the BSL→Apache story of v23.2.15. If a license does not let you copy the material, do not copy it.
- **Paper walls fail.** Calling the process a clean-room does not help when the two groups were not actually separate. Separation has to be practiced.
- **This page is not a substitute for a lawyer.** Before a large feature that is not described by the public OSS docs, get counsel review.

---

## Related docs

- Agent mandatory rules: [`docs/agent/CLEANROOM-ENFORCEMENT.md`](../agent/CLEANROOM-ENFORCEMENT.md)
- Research citations: [`docs/fgdb/SOURCES.md`](SOURCES.md)
