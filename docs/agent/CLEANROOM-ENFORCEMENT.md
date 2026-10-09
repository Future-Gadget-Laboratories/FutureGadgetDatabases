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
5. **If proprietary or CSL code already reached this repository,** remove it in a **normal commit first**. That makes the current tree clean immediately, while a history rewrite is still waiting on approval. If that history contains a password, token, key, or other secret, revoke or rotate it in this same step. A rewrite does not un-expose a secret.

Partial compliance is failure. “I only used it for inspiration” is still contamination for an implementer.

### Rewriting history after contaminated code is committed

Step 5 deletes the files from the latest tree. Older commits still contain them until a maintainer rewrites history. Do this when **proprietary or CSL code reached the repo** (it was committed or pushed). Work that never left a local workspace is deleted locally and does not need a rewrite.

The steps below follow GitHub’s public guide, [Removing sensitive data from a repository](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository), and the [git-filter-repo user manual](https://htmlpreview.github.io/?https://github.com/newren/git-filter-repo/blob/docs/html/git-filter-repo.html) ([project](https://github.com/newren/git-filter-repo)).

**Rotate exposed secrets immediately.** If the old history contains a password, token, key, or other secret, revoke or rotate it before any rewrite and before waiting on rewrite approval. A history rewrite does not invalidate a credential, and GitHub’s purge can take time. After the secret is revoked, it can no longer be used for access. GitHub treats this as the first step and says a rewrite may be unnecessary once the secret is dead. After the later purge, verify again that each exposed secret was rotated and that the old value no longer works.

**Approval comes first for the rewrite.** A history rewrite needs **explicit maintainer approval** before anyone starts it. It changes every later commit hash and breaks clones and forks that still point at the old history. A later merge of that old history can put the contaminated commits back. Keep the normal delete commit in place while approval is pending. Do not force-push before approval. Secret rotation does not wait on this approval.

**How to rewrite**

1. Freeze merges to the affected branches. Merge or close open pull requests before the rewrite. GitHub recommends that because every later commit hash changes, so open review comments and diffs will not match the new history. Commits added on the old history during the cleanup force you to start over.
2. Install **git-filter-repo 2.47 or newer**. That is the first version with `--sensitive-data-removal`, which GitHub’s guide requires. Install instructions: [INSTALL.md](https://github.com/newren/git-filter-repo/blob/main/INSTALL.md).
3. Clone a fresh copy of the repository. From that clone, remove the contaminated paths from every branch, tag, and ref:

   ```
   git-filter-repo --sensitive-data-removal --invert-paths --path PATH-TO-YOUR-FILE-WITH-SENSITIVE-DATA
   ```

   `PATH-TO-YOUR-FILE-WITH-SENSITIVE-DATA` is the path recorded in git (GitHub’s example is `src/module/phone-numbers.txt`), not only the file name. If the file was renamed or moved, pass every former path with another `--path`, or run the command once per path. To replace secret strings listed in a file that stays outside the repository, use:

   ```
   git-filter-repo --sensitive-data-removal --replace-text ../passwords.txt
   ```

   Leave that list outside the repository. Do not commit it, and do not put proprietary source or distinctive snippets in the filter.
4. Check the rewritten history before you push. This lists any remaining commits that touch the path:

   ```
   git log --all --name-status -- PATH-TO-YOUR-FILE-WITH-SENSITIVE-DATA
   ```

   If the path is still there, run filter-repo again for the missed path. In the filter-repo output, save the lines that begin with `NOTE: First Changed Commit(s)`.
5. Count the pull requests this rewrite will affect, from the repo root:

   ```
   grep -c '^refs/pull/.*/head$' .git/filter-repo/changed-refs
   ```

   Drop `-c` to list them (`refs/pull/NUMBER/head`). You will give this count and the first changed commits to GitHub Support. If the count is larger than you expect, delete this clone and stop. Until you push, discarding the clone throws the rewrite away.
6. On this rewritten clone, commit the rewrite record from the next section before you push, so the edited history itself says why the material was removed and where to read about it. git-filter-repo removes the `origin` remote on purpose. Add it back, then force-push with the command GitHub documents. `--mirror` updates branches, tags, and other refs, and it drops remote commits that are not in this clone:

   ```
   git remote add origin https://github.com/OWNER/REPOSITORY.git
   git push --force --mirror origin
   ```

   Pushes of `refs/pull/*` fail because GitHub marks those refs read-only. That failure is expected; Support handles them in the next step. If any other ref fails, branch protection is blocking the force-push. Turn that protection off temporarily, run the push again, and turn the protection back on. Repeat until the only failures are refs that start with `refs/pull/`.
7. Ask GitHub Support, through the [GitHub Support portal](https://support.github.com/), to purge cached views and the read-only pull-request refs. Support does this only after the repository refs are cleaned, and only when they decide that rotating the affected credentials is not enough on its own. They do not remove non-sensitive data. In the ticket, give them the owner and repository name, the number of affected pull requests from step 5, and the First Changed Commit(s) from the filter-repo output. If the output says `NOTE: There were LFS Objects Orphaned by this rewrite`, say so and attach the file the tool names.
8. Tell contributors to delete old clones and re-clone, or to rebase onto the new history. A merge of the pre-rewrite history puts the contaminated commits back. Forks still hold the old commits until their owners remove the data or delete the fork. The filter-repo manual section “Make sure other copies are cleaned up” has the steps for a colleague’s existing clone.
9. Verify again that every secret from the old history was rotated or revoked and that the old value no longer works.

**History note.** The history edit itself must say why the material was removed and where to read what happened and how it was addressed. Put the class of material, the date, and the incident URL in the cleanup commit message (step 5 under “refuse and discard”). On the rewritten clone, before the mirror push, commit a rewrite record with the same facts. That record is the note in the edited history: it says why history was rewritten and links to the incident write-up. Both notes are allowed in commit messages because they name only the class of material. They never quote or reproduce proprietary or CSL source.

Cleanup commit message:

```
Remove contaminated material from the tree.

date: YYYY-MM-DD
why: <class of material only, for example "proprietary or CSL source was committed">
incident: <link to the incident write-up>
```

Rewrite record (commit this on the rewritten clone before the mirror push):

```
Rewrite history to remove contaminated material.

date: YYYY-MM-DD
why: <class of material only, for example "proprietary or CSL source was committed">
incident: <link to the incident write-up>
```

Also leave the same facts in the repo (for example under `docs/` or in a `CHANGELOG`) with this template:

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
| Proprietary or CSL code already committed or pushed | **Remove it in a normal commit first.** If a secret was exposed, rotate it immediately. Rewrite history only after explicit maintainer approval |
| “Clone 24.3 for reference” in an implementer workspace | **Refuse** |
| Proprietary source or copied notes → “write FGDb code” | **Refuse** (spec from public behavior only) |
| Contaminated earlier in the session | **Refuse**; demand a fresh clean context |
| Local edits to these process docs, not yet pushed | OK; still no GitHub land without maintainer approval |

---

## Reminder

A clean-room protects against **copying expression**. It does not authorize ignoring licenses, patents, or a maintainer’s product rules. When unsure whether material is proprietary: **treat as proprietary, refuse, escalate.**
