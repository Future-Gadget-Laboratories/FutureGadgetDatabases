# Sources — FGDb clean-room documentation research

Research date: **2026-10-06** (America/Denver).

These are references used while drafting [`docs/fgdb/CLEANROOM.md`](CLEANROOM.md) and [`docs/agent/CLEANROOM-ENFORCEMENT.md`](../agent/CLEANROOM-ENFORCEMENT.md). Prefer primary / reputable commentary. **This list is not legal advice** and does not assert that any case controls FGDb’s facts. Have counsel review before relying on any of it for a licensing decision.

The process in those pages is a clean-room reimplementation from **public behavior and public documentation**, with separate spec-writing and implementation roles.

---

## Industry practice and process descriptions

1. **Bob Zeidman, “Clean Room Development to Prevent the Spread of ‘Infectious IP’”** — IPWatchdog, 2023-04-22.  
   https://ipwatchdog.com/2023/04/22/clean-room-development-to-prevent-the-spread-of-infectious-ip/  
   Describes a clean-room protocol: one group writes a specification, a different group writes the code, and an independent monitor screens what passes between them. Warns against using a non-independent employee as the monitor. Cites clean-room concepts in litigation, including *NEC v. Intel* and *Computer Associates v. Altai*.

2. **Wikipedia: Clean-room design.**  
   https://en.wikipedia.org/wiki/Clean-room_design  
   Overview of the two-team technique. Notes that a clean-room can support a copyright independent-creation argument and does **not** avoid patents. Summarizes the practice of writing a behavior spec, then implementing from that spec. Verify claims against primary sources before citing them in a legal filing.

3. **David S. Elkins, “NEC v. Intel: A Guide to Using ‘Clean Room’ Procedures as Evidence,”** 10 Computer L.J. 453 (1990).  
   UIC John Marshall Journal of Information Technology & Privacy Law (open repository):  
   https://repository.law.uic.edu/cgi/viewcontent.cgi?article=1423&context=jitpl  
   Scholarly walkthrough of clean-room requirements as trial evidence: (1) the programmer who writes the code is unfamiliar with the target code; (2) spec writers and implementers are separate; (3) a **gatekeeper** screens communications. Preserve logs and drafts.

---

## Case law and case commentary (do not invent holdings)

> **Caution:** Case names below are from the cited public materials. Secondary summaries can oversimplify. Confirm with primary opinions and counsel before relying on them.

4. ***NEC Corp. v. Intel Corp.***, 10 U.S.P.Q.2d 1177 (N.D. Cal. 1989) — discussed in Elkins (1990) and Zeidman (2023). Often cited for judicial attention to clean-room-developed code as evidence related to independent development. Read the opinion and commentary; do not treat a blog paraphrase as the holding.

5. ***Computer Associates International, Inc. v. Altai, Inc.***, 982 F.2d 693 (2d Cir. 1992) — referenced in Zeidman (2023). Better known for the abstraction-filtration-comparison approach to software copyright. Confirm relevance with counsel.

---

## CockroachDB licensing context (product rules for FGDb)

6. **Cockroach Labs — BSL relicensing announcement (historical).**  
   https://cockroachlabs-www-prod.netlify.app/blog/oss-relicensing-cockroachdb/  
   Explains BSL with rolling conversion to Apache 2.0; enterprise features under separate community/enterprise licensing that did **not** convert the same way.

7. **Example BSL text in-tree (illustrative; check the exact tag you fork).**  
   e.g. https://github.com/cockroachdb/cockroach/blob/v19.2.0/licenses/BSL.txt  
   and later versioned `licenses/BSL.txt` files for specific releases.

8. **CSL transition notes (24.x era).**  
   - Example PR removing BSL/CCL files under CSL: https://github.com/cockroachdb/cockroach/pull/132057  
   - Cockroach Labs licensing FAQs (versioned docs; select the version you care about): https://www.cockroachlabs.com/docs/stable/licensing-faqs  
   FGDb’s project rule is to base on **OSS v23.2.15** after Apache conversion and to treat later **CSL / Enterprise** materials as **out of bounds** for direct copying. New behavior comes from a clean-room spec based on public behavior and public documentation.

> **Uncertainty note:** Exact conversion dates and which patch lines remained Apache vs moved to CSL should be verified against the specific git tags and LICENSE files FGDb vendors. Do not rely on memory or third-party blogs alone when tagging a release.

---

## What we deliberately did *not* invent

- No fabricated case citations.
- No claim that a clean-room defeats patents.
- No assertion that FGDb’s hobby-scale review equals a fully independent commercial clean-room monitor engagement.

---

## Recommended next step before a large new feature

Have qualified counsel review: (1) license restrictions on any material that is not public OSS; (2) patent exposure for the specific feature; (3) whether FGDb’s documented two-team process and audit trail are adequate for the project’s risk tolerance.
