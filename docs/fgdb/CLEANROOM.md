# Cleanroom reimplementation at Future Gadget Laboratories (FGDb)

**Audience:** hobbyists, junior engineers, and anyone contributing to FutureGadgetDatabases (FGDb / Future Gadget Laboratories).

**Not legal advice.** This document describes an engineering process used in industry to reduce copyright and trade-secret contamination risk. Laws and licenses differ by country and by product. When in doubt, stop and get counsel review before touching proprietary materials.

---

## What “cleanroom” means here

A **cleanroom** (also called a **Chinese wall** or **two-team** process) is how you rebuild *behavior* of a system without copying its *expression* (source, comments, internal structure, distinctive naming, proprietary algorithms as written).

Classic industry pattern (Phoenix BIOS era, still cited today):

1. One group (**spec / analysis / “dirty” team**) may lawfully study the target and write a **functional specification** — what inputs produce what outputs, error codes, timing constraints, wire formats, SQL semantics, etc. They do **not** paste proprietary code into that spec.
2. A separate group (**implementers / “clean” team**) has **never** seen the proprietary source, binaries under analysis, disassembly dumps, or contaminated notes. They implement **only** from the approved spec plus public references (standards, published docs, our own OSS tree).
3. Ideally a **gatekeeper / monitor** reviews every handoff from analysis → implementers so expression (code snippets, comment text, distinctive structure) does not leak into the clean side.
4. You **document** who saw what, when, and which artifacts crossed the wall. If someone contaminates the clean side, you **refuse and discard** that work and start over clean — you do not “edit it a little.”

Cleanroom is useful as evidence of **independent creation** for **copyright**. It does **not** bypass **patents**, and it does **not** override **license contracts** that forbid reverse engineering, decompilation, or competitive use. Treat those as separate constraints.

---

## When this process applies at FGDb

| Situation | Cleanroom required? |
|-----------|---------------------|
| Work entirely inside our OSS fork of CockroachDB **v23.2.15** (Apache after BSL conversion) and public standards | No. Ordinary OSS contribution rules apply. |
| Porting or reimplementing behavior that exists only in CockroachDB **Enterprise / CCL** code, or in trees under the **CockroachDB Software License (CSL)** (generally **v23.2.16+** patches after the CSL switch, and **24.3+**) | **Yes.** Analysis may use approved RE tooling on an isolated FGL server; implementation must be cleanroom. |
| Someone pastes “just a snippet” of proprietary CRDB source into chat, a PR, or an issue | **Stop.** Treat as contamination. Do not merge. Discard or quarantine. |
| Checking out a CSL/`enterprise` tree into an **implementer** workspace “for reference” | **Forbidden.** That collapses the wall. |

**Hard product rule:** FGDb’s licensed base is **CockroachDB OSS v23.2.15 only** (post-conversion Apache). We never copy proprietary Enterprise/CSL code from later trees into FGDb. Features that later CRDB released under proprietary terms are candidates for **cleanroom reimplementation**, not for cherry-picks from forbidden trees.

---

## Roles (keep them separate)

### Spec / analysis team (“dirty” for that feature)

- May observe lawful materials: public docs, OSS v23.2.15, and — when Vincent has authorized a feature — **local** Decode / Ghidra / black-box observation on the **FGL analysis server**.
- Writes **behavior-only** specs: APIs, wire formats, SQL/error semantics, invariants, acceptance tests described as *observable* results.
- Must **not** include: proprietary source lines, decompiled pseudocode that mirrors structure/names, comment text from the target, or “copy this function” guidance.
- Declares in writing (or ticket metadata) that they viewed restricted materials for this feature.

### Implementer team (“clean”)

- Sees **only**: approved specs, public standards, FGDb’s OSS tree, and public CRDB OSS materials that are already in our allowed lineage.
- Never mounts analysis disks, never clones CSL trees, never opens Ghidra projects, never accepts pasted proprietary source.
- Every non-trivial change should cite a **spec clause** or public standard — not “I saw it in enterprise.”

### Gatekeeper / steward (often Vincent or a designated steward)

- Reviews specs before they reach implementers: strip expression leakage.
- Owns contamination response: quarantine, discard, re-spec if needed.
- Approves consequential commits/PRs that land cleanroom work in the public repo (**Vincent OK required**).

In a hobby lab we may not hire an independent third-party monitor like a Fortune-500 cleanroom. That is a known gap versus the “gold standard.” Compensate with **strict artifact separation**, **written refuse/discard rules**, and **no consequential landing without Vincent OK**. Do not pretend a weaker setup is the same as a fully audited corporate cleanroom.

---

## Process steps (feature-sized)

1. **Scope.** Name the feature and why it is not already in OSS v23.2.15. Confirm it is not a forbidden cherry-pick.
2. **Authorize analysis.** Only on the FGL analysis environment. Record who is contaminated for this feature.
3. **Observe.** Prefer black-box (inputs/outputs, SQL tests, network traces). Use Decode/Ghidra only as needed for behavior inference — not as a copy-paste source.
4. **Write the spec.** Behavior, edge cases, compatibility matrices. No proprietary source. Prefer tables and “when X then Y” over prose that restates algorithms line-by-line.
5. **Gatekeeper pass.** Someone who understands both contamination risk and the feature reviews the spec. Reject or rewrite if it smells like expression.
6. **Handoff.** Only the approved spec (and public refs) enters implementer channels/repos.
7. **Implement** in FGDb from the spec. New code, new tests. License spirit: as close to BSD/Apache as our project policy allows.
8. **Conformance.** Black-box tests that prove *behavior* match without comparing to proprietary source trees.
9. **Record.** Keep: spec version hash, who analyzed, who implemented, gatekeeper notes, test evidence. Treat this as the audit trail.
10. **Land only with Vincent OK.** No silent push of cleanroom features to GitHub.

---

## What is forbidden

- Copying, translating, or “lightly rewriting” proprietary CRDB Enterprise/CSL source into FGDb.
- Checking out CSL or enterprise trees into **implementer** workspaces (including “read-only” clones, zip dumps, or “just for grep”).
- Pasting proprietary source into Discord, GitHub issues/PRs, agent chats, or specs.
- Asking an AI agent that has been shown proprietary source to “now write the FGDb version” in the same session without a fresh clean context and approved spec.
- Shipping intermediate RE artifacts (Ghidra databases, decompilation dumps) into the public repo or implementer trees.
- Using cleanroom as an excuse to ignore **license terms** or **patents**. If a license forbids the analysis you want, do not do that analysis.

---

## Concrete examples

### Good

- Spec says: “Statement `ALTER …` must return error code `XYZ` when the table is offline; retry after lease transfer succeeds.” Implementers write new Go from that rule and add SQL tests.
- Analysis team on FGL server notes wire-frame field widths from observation; gatekeeper rewrites notes into a field table with no symbol names from the proprietary binary; implementers code the codec from the table.

### Bad (refuse / discard)

- Contributor opens a PR titled “Port backup encryption from crl-enterprise” with files clearly derived from a CSL tree.
- Chat paste: “Here’s the enterprise function, just change the package name.”
- Implementer agent is given a Ghidra decompilation and told to “clean it up into FGDb style.”
- Spec contains a 40-line pseudocode block that is recognizably the same control flow and variable names as proprietary source.

When bad happens: **stop**, mark the branch/workspace contaminated, **do not merge**, open a contamination report for the steward, and re-do from an approved clean spec if the feature still matters.

---

## Limits (read this twice)

- **Copyright ≠ patents.** Independent implementation can still infringe a patent.
- **Fair use / interoperability** case law (e.g. U.S. cases discussing intermediate copying for interoperability) is **fact-specific** and **jurisdiction-specific**. Do not assume “Sega says we can decompile anything.”
- **Contracts win fights.** A license that bans reverse engineering can create liability even when copyright doctrine might otherwise be friendlier. CSL and enterprise terms are not the BSL→Apache story of v23.2.15.
- **Paper walls fail.** Courts have rejected “we had a cleanroom” claims when operational separation was not real (see commentary on *IBM v LzLabs* / related UK proceedings in [SOURCES.md](SOURCES.md)). Separation must be practiced, not merely named.
- **This lab doc is not a substitute for a lawyer.** Before large-scale RE of proprietary CRDB, get counsel review.

---

## Related docs

- Agent mandatory rules: [`docs/agent/CLEANROOM-ENFORCEMENT.md`](../agent/CLEANROOM-ENFORCEMENT.md)
- Research citations: [`docs/fgdb/SOURCES.md`](SOURCES.md)
