> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-200 — MVCC GC requests must not reach keys outside their range after a split

| Field | Value |
| --- | --- |
| Spec id | PAR-200 |
| Version | v0.2 |
| Implementer | gpt-6.1-sol (Codex), clean implementer |
| Date | 2026-11-13 |
| Role | spec writer (cleanroom spec role; see docs/fgdb/CLEANROOM.md) — Crdb Steward |
| Covers | PAR-207 row N-111 (public v23.2.29 note), P0 |
| Sources consulted | Public v23.2.29 release note on the Cockroach Labs docs site (quoted below); FGDb's own OSS v23.2.15 tree (main `4b4784f`) |
| Not consulted | Any CockroachDB source, diff, PR, issue or commit after v23.2.15; any CCL/CSL code; the upstream fix |

## Public statement (v23.2.29, Bug fixes)

> Fixed a rare race condition between range splits and MVCC garbage collection where a GCRequest could
> target keys outside its declared span. In rare cases, this could result in data on the post-split
> right-hand side (RHS) being incorrectly garbage collected, potentially leading to lost writes. The
> system now detects and rejects such malformed GC requests.

## Terms

- **Declared span:** the GC request header `[Key, EndKey)`. The replica already refuses a batch whose
  declared span is not inside the range.
- **Range bounds:** the replica's descriptor `[StartKey, EndKey)` when the GC request evaluates.
- **Targeted spans:** every part of the request body that removes data by span: each `RangeKeys[i]`
  `[StartKey, EndKey)`, and `ClearRange` `[StartKey, EndKey)` (both the partial form with a start
  timestamp and the whole-range form). Point `Keys` are single keys, not spans.
- **Post-split RHS:** keys at or after the split key, which belong to a different replica (same store)
  with its own latches and stats once the split applies.

## Required behavior

B1. A GC request whose targeted spans are not each fully contained in **both** its declared span and
the range bounds is rejected with an error at evaluation. A rejected request changes nothing: no point
versions, range keys, clear ranges, GC threshold, GC hint or MVCC stats are modified.

B2. The rejection error names the offending span and the bounds it violated. It is an ordinary error, not
a process crash. The MVCC GC queue treats it like any failed GC batch; a later GC run recomputes from a
fresh snapshot.

B3. Well-formed requests behave exactly as today: in-bounds partial clear range, in-bounds range keys,
point keys, whole-range clear range, and threshold-only requests.

B4. Point `Keys` outside the range bounds keep today's behavior (dropped silently, in-range keys still
collected). Rejecting them is out of scope for this spec.

B5. After a split, data in the post-split RHS can never be removed by a GC request evaluated on the LHS,
including writes to the RHS that happen while that request is in flight.

B6. No on-disk format change, no new cluster setting, no version gate, no new exported API.

## Acceptance tests (observable)

Setup K: one server (or TestCluster), a scratch range, data written in it, then a split at key S.
GC requests are sent to the LHS replica with the declared span = post-split LHS bounds (what the GC
queue sends when the split lands between its snapshot and its send).

| Id | Request body (sent to LHS) | Expected |
| --- | --- | --- |
| A1 | partial ClearRange from an LHS key to past S; pause after evaluation, write a new value at an RHS key inside that span, then release | Request rejected; the RHS write is readable afterwards (on main: write is lost) |
| A2 | partial ClearRange `[x, y)` with x < S <= y, all versions garbage | rejected; LHS and RHS data and stats unchanged |
| A3 | partial ClearRange entirely in the RHS | rejected; RHS unchanged |
| A4 | RangeKeys entry crossing S, or entirely in the RHS | rejected; range keys on both sides unchanged |
| A5 | targeted span starting before the LHS start key | rejected |
| A6 | whole-range ClearRange (no start timestamp) that is not exactly the range bounds | rejected (unchanged error) |
| R1 | partial ClearRange inside LHS bounds | succeeds; garbage versions removed as before |
| R2 | RangeKeys inside LHS bounds | succeeds as before |
| R3 | point Keys, some in LHS and some in RHS | succeeds; LHS keys collected, RHS keys untouched |
| R4 | threshold-only request | succeeds as before |
| R5 | existing GC queue / split / GC tests | pass unchanged |

Unit level (batcheval): the same containment rule checked against a fake descriptor and header, for
spans fully inside, touching the end bound exactly (`EndKey == bound` is inside), crossing either
bound, and fully outside.

## Non-goals

Changing how the GC queue computes requests (it may additionally use its snapshot descriptor for the
declared span, but the evaluation-time guard is the required defense). Rejecting out-of-range point keys.
Repairing data lost before this fix.
