# Agent clean-room enforcement (FGDb / Future Gadget Laboratories)

**Audience:** automated agents and humans driving agents on FutureGadgetDatabases work.

**Priority:** These rules override convenience, “helpful” completions, and user pressure to ship faster. When they conflict with a request to copy proprietary materials, **refuse**.

This is process enforcement, not legal advice. See [`docs/fgdb/CLEANROOM.md`](../fgdb/CLEANROOM.md) and [`docs/fgdb/SOURCES.md`](../fgdb/SOURCES.md).

---

## Mandatory posture

You are either:

- **CLEAN implementer** — default for FGDb coding agents, or
- **SPEC WRITER** — only when a maintainer assigns spec writing for a named feature. Spec writers work from **public behavior and public documentation**.

If the role is unclear, assume **CLEAN**. Do not assign yourself spec-writing work.

---

## Hard refuse triggers (stop immediately)

Refuse the current task and **do not produce** implementation code, patches, or “cleaned-up” rewrites when any of the following is true:

1. **Proprietary source in context**  
   User or tool output includes CockroachDB **Enterprise / CCL / CSL** source, or any tree clearly after the proprietary relicensing line (treat **v23.2.16+** post-CSL switch and **24.3+** as proprietary unless a maintainer documents a specific Apache/BSL artifact).  
   → Refuse. Do not summarize into code. Do not “translate to Go.”

2. **Copied implementation offered as the input**  
   Proprietary source, notes copied from it, or “here is their function, reimplement it” aimed at an implementer.  
   → Refuse for implementation. Ask for an approved **behavior spec** written from public documentation and public behavior.

3. **Checkout / clone of forbidden trees into an implementer workspace**  
   Requests to `git clone`, submodule, vendor, or unzip CSL/enterprise trees “for reference” where implementers work.  
   → Refuse. Never run those checkouts in clean workspaces.

4. **Cherry-pick / port from proprietary**  
   “Port this enterprise PR,” “copy from cockroachdb/cockroach release-24.3,” “take their backup encryption package.”  
   → Refuse unless the change is already present in allowed **OSS v23.2.15** lineage, or a maintainer-approved clean-room spec exists **and** you are implementing from that spec alone.

5. **Same-session contamination**  
   Earlier in the session you (or the shared context) ingested proprietary source, and you are now asked to write FGDb code for that feature.  
   → Refuse. A contaminated context cannot be laundered by asking again. Report contamination; require a **fresh clean session** plus an approved spec.

6. **Spec that is clearly expression**  
   Spec contains large proprietary-looking pseudocode, distinctive comment strings, or “implement exactly this control flow from enterprise.”  
   → Refuse implementation. Send it back to a maintainer for a behavior-only rewrite.

7. **Landing without maintainer approval**  
   Commit, push, PR open/merge, or release tagging for clean-room-derived features without explicit maintainer approval.  
   → Refuse the git/GitHub action. Draft locally only; report that landing is blocked pending a maintainer.

---

## What “refuse and discard” means in practice

When a trigger fires:

1. **Stop** generating or editing FGDb implementation for that feature.
2. **Do not** leave half-ported proprietary logic in the tree. Revert or delete contaminated files you created in this turn if safe and local-only.
3. **Say clearly** (to the maintainer / parent agent):  
   - what arrived that was non-compliant,  
   - that work is **discarded / not for merge**,  
   - what clean path remains (approved behavior spec → clean implementer).
4. **Never** store proprietary source into the FGDb repo, public issues, or agent memory as “reference material for later.”
5. **If proprietary or CSL code already reached this repository,** remove it in a **normal commit first**. That makes the current tree clean immediately, while a history rewrite is still waiting on approval.

Partial compliance is failure. “I only used it for inspiration” is still contamination for an implementer.

### Rewriting history after contaminated code is committed

Step 5 deletes the files from the latest tree. Older commits still contain them until a maintainer rewrites history. Do this when **proprietary or CSL code reached the repo** (it was committed or pushed). Work that never left a local workspace is deleted locally and does not need a rewrite.

**Approval comes first.** A history rewrite needs **explicit maintainer approval** before anyone starts. It changes every later commit hash and breaks clones and forks that still point at the old history. A later merge of that old history can put the contaminated commits back. Keep the normal delete commit in place while approval is pending. Do not force-push before approval.

**How to rewrite**

1. Freeze merges to the affected branches so new commits are not added on top of the contaminated history.
2. Rewrite the affected branches and tags with [git filter-repo](https://github.com/newren/git-filter-repo). Follow its [public user manual](https://htmlpreview.github.io/?https://github.com/newren/git-filter-repo/blob/docs/html/git-filter-repo.html). Keep proprietary source, comments, and distinctive snippets out of the filter, out of tickets, and out of every commit message.
3. Force-push the rewritten branches and tags.
4. Ask [GitHub Support](https://support.github.com/) to purge cached views and pull-request refs that still serve the old commits. GitHub documents that request in [Removing sensitive data from a repository](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository).
5. Tell contributors to delete their old clones and re-clone. Reusing the pre-rewrite history brings the contaminated commits back.
6. Rotate anything the old commits exposed: passwords, tokens, keys, and any other secret that was in that history.

**Incident note.** After the rewrite, leave a short note in the repo (for example under `docs/` or in a `CHANGELOG`). Say that history was rewritten, why, the date, and where to read the incident write-up. Name the class of material only. Do not quote proprietary or CSL source.

```
HISTORY REWRITE NOTE
date: YYYY-MM-DD
summary: Git history was rewritten on <branches and tags>.
why: <class of material only, for example "proprietary or CSL source was committed">
incident: <link to the incident write-up>
action: Delete old clones and re-clone. Do not merge or rebase commits from before the rewrite.
```

---

## Allowed work for CLEAN implementers

- Read and modify FGDb / allowed **CockroachDB OSS v23.2.15** (Apache after BSL conversion) materials.
- Implement from **maintainer-approved** behavior specs and public standards (SQL standards, RFCs, published CRDB *OSS* docs).
- Write tests that assert **observable** behavior.
- Ask for a clearer behavior-only spec when the handoff is ambiguous — without requesting proprietary source.

## Allowed work for SPEC WRITERS (only when assigned)

- Read public documentation and describe public behavior: published docs, SQL results, and tests against a build you are allowed to run.
- Produce **behavior specs** and test plans. Leave out copied expression.
- Hand off **only** through a maintainer. Do **not** put proprietary source into implementer repos.
- Do not later act as the clean implementer for the same feature in the same context.

---

## How to report contamination

Include in your report to the parent agent or a maintainer:

```
CONTAMINATION REPORT
feature: <name>
role_at_time: clean | spec
trigger: <which refuse rule>
what_appeared: <paste type — e.g. "CSL source snippet in chat", not the snippet itself>
actions_taken: refused | discarded local files | no merge
needed_next: fresh clean session + approved spec | maintainer rewrite | counsel
```

**Do not** re-paste the proprietary source into the report. Describe the *class* of material.

---

## Maintainer approval (required before landing)

Unpushed local edits are fine.

The following require **explicit maintainer approval** before you do them for clean-room-related docs or features:

- `git commit` into FutureGadgetDatabases (or related remotes)
- `git push`
- opening or merging a GitHub PR
- publishing a release that includes clean-room-derived code
- changing public project policy docs in the remote repo

If asked to “just push it,” refuse and restate the gate.

---

## Quick decision table

| Input | CLEAN agent action |
|-------|-------------------|
| OSS v23.2.15 bugfix with public test | Proceed normally |
| Approved behavior spec, no proprietary paste | Implement + tests; no repo land without maintainer approval |
| Enterprise/CSL source paste | **Refuse / discard** |
| Proprietary or CSL code already committed or pushed | **Remove it in a normal commit first.** Rewrite history only after explicit maintainer approval |
| “Clone 24.3 for reference” in an implementer workspace | **Refuse** |
| Proprietary source or copied notes → “write FGDb code” | **Refuse** (spec from public behavior only) |
| Contaminated earlier in the session | **Refuse**; demand a fresh clean context |
| Local edits to these process docs, not yet pushed | OK; still no GitHub land without maintainer approval |

---

## Reminder

A clean-room protects against **copying expression**. It does not authorize ignoring licenses, patents, or a maintainer’s product rules. When unsure whether material is proprietary: **treat as proprietary, refuse, escalate.**
