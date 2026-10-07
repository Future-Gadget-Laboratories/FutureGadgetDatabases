# Sources — FGDb cleanroom documentation research

Research date: **2026-10-06** (America/Denver).

These are references used while drafting [`docs/fgdb/CLEANROOM.md`](CLEANROOM.md) and [`docs/agent/CLEANROOM-ENFORCEMENT.md`](../agent/CLEANROOM-ENFORCEMENT.md). Prefer primary / reputable commentary. **This list is not legal advice** and does not assert that any case controls FGDb’s facts. Have counsel review before relying on any doctrine for reverse engineering or relicensing decisions.

---

## Industry practice and process descriptions

1. **Bob Zeidman, “Clean Room Development to Prevent the Spread of ‘Infectious IP’”** — IPWatchdog, 2023-04-22.  
   https://ipwatchdog.com/2023/04/22/clean-room-development-to-prevent-the-spread-of-infectious-ip/  
   Describes dirty room / clean room / independent **monitor** protocol; warns against using a non-independent employee as monitor; cites use of cleanroom concepts in litigation context including *NEC v. Intel* and *Computer Associates v. Altai*.

2. **Wikipedia: Clean-room design** (also titled / redirected in some indexes as clean-room reverse engineering).  
   https://en.wikipedia.org/wiki/Clean-room_design  
   Overview of Chinese-wall technique; notes cleanroom helps copyright independent-creation arguments but **not** patent circumvention; summarizes Phoenix BIOS-style two-team practice and *NEC v. Intel* context. Verify claims against primary sources before citing in legal filings.

3. **Mathew Schwartz, “Reverse-Engineering”** — Computerworld, 2001-11-12.  
   https://www.computerworld.com/article/1349695/reverse-engineering.html  
   Widely cited description of Phoenix Technologies’ cleanroom BIOS: spec team documents behavior without code; second team with no prior IBM BIOS exposure implements from specs.

4. **David S. Elkins, “NEC v. Intel: A Guide to Using ‘Clean Room’ Procedures as Evidence,”** 10 Computer L.J. 453 (1990).  
   UIC John Marshall Journal of Information Technology & Privacy Law (open repository):  
   https://repository.law.uic.edu/cgi/viewcontent.cgi?article=1423&context=jitpl  
   Scholarly walkthrough of cleanroom requirements as trial evidence: (1) clean programmer unfamiliar with target code; (2) separation of spec writers and implementers; (3) **gatekeeper** screening communications; preserve logs and drafts.

---

## Case law and case commentary (do not invent holdings)

> **Caution:** Case names and holdings below are from the cited public materials. Secondary summaries can oversimplify. Confirm with primary opinions and counsel before relying on them.

5. ***NEC Corp. v. Intel Corp.***, 10 U.S.P.Q.2d 1177 (N.D. Cal. 1989) — discussed in Elkins (1990) and Zeidman (2023). Often cited as early U.S. judicial attention to cleanroom-developed microcode as evidence related to independent development / constraints. Read the opinion and commentary; do not treat blog paraphrases as the holding.

6. ***Sega Enterprises Ltd. v. Accolade, Inc.***, 977 F.2d 1510 (9th Cir. 1992).  
   Justia: https://law.justia.com/cases/federal/appellate-courts/F2/977/1510/305345/  
   Resource.org text: https://law.resource.org/pub/us/case/reporter/F2/977/977.F2d.1510.92-15655.html  
   Ninth Circuit discussion of intermediate disassembly and fair use in a video-game compatibility context; also notes that a cleanroom does not eliminate the need to discover functional specs (disassembly may still be involved on the analysis side). **Fact-specific; not a blanket RE license.**

7. ***Sony Computer Entertainment, Inc. v. Connectix Corp.***, 203 F.3d 596 (9th Cir. 2000).  
   Justia: https://law.justia.com/cases/federal/appellate-courts/F3/203/596/474793/  
   Fair-use analysis of intermediate copying during reverse engineering for an emulator where the final product did not contain Sony’s copyrighted material. Again **fact-specific**.

8. ***Computer Associates International, Inc. v. Altai, Inc.***, 982 F.2d 693 (2d Cir. 1992) — referenced in Zeidman (2023) among cleanroom-related litigation context; better known for the abstraction-filtration-comparison approach to software copyright. Confirm relevance with counsel; do not assume cleanroom was the centerpiece holding.

9. **UK / IBM mainframe reverse-engineering commentary (*IBM United Kingdom Ltd v LzLabs GmbH & Ors* and related reporting).**  
   - Lexology overview: https://www.lexology.com/library/detail.aspx?g=24adb7b6-9bc8-43c9-92c7-ff2244a131cf  
   - RPC commentary: https://www.rpclegal.com/thinking/tech/reverse-engineering-of-ibm-mainframe-software-in-breach-of-software-licence-ibm-v-lzlabs-part-1/  
   Takeaway emphasized in commentary: **paper “clean rooms” that are not operationally separate fail**; license terms and statutory software exceptions are construed carefully. Not U.S. precedent for FGDb, but a warning that labeling a process “cleanroom” without real separation is weak.

---

## CockroachDB licensing context (product rules for FGDb)

10. **Cockroach Labs — BSL relicensing announcement (historical).**  
    https://cockroachlabs-www-prod.netlify.app/blog/oss-relicensing-cockroachdb/  
    Explains BSL with rolling conversion to Apache 2.0; enterprise features under separate community/enterprise licensing that did **not** convert the same way.

11. **Example BSL text in-tree (illustrative; check the exact tag you fork).**  
    e.g. https://github.com/cockroachdb/cockroach/blob/v19.2.0/licenses/BSL.txt  
    and later versioned `licenses/BSL.txt` files for specific releases.

12. **CSL transition notes (24.x era).**  
    - Example PR removing BSL/CCL files under CSL: https://github.com/cockroachdb/cockroach/pull/132057  
    - Cockroach Labs licensing FAQs (versioned docs; select the version you care about): https://www.cockroachlabs.com/docs/stable/licensing-faqs  
    FGDb’s project rule is to base on **OSS v23.2.15** after Apache conversion and to treat later **CSL / Enterprise** materials as **out of bounds** for direct copying — cleanroom only for new behavior.

> **Uncertainty note:** Exact conversion dates and which patch lines remained Apache vs moved to CSL should be verified against the specific git tags and LICENSE files FGDb vendors. Do not rely on memory or third-party blogs alone when tagging a release.

---

## What we deliberately did *not* invent

- No fabricated case citations.
- No claim that cleanroom defeats patents.
- No claim that U.S. fair-use software cases authorize every RE technique worldwide or under every EULA/CSL.
- No assertion that FGDb’s hobby-scale gatekeeping equals a fully independent commercial cleanroom monitor engagement.

---

## Recommended next step before large RE campaigns

Have qualified counsel review: (1) CSL / enterprise license restrictions on the artifacts you might observe; (2) patent exposure for specific features; (3) whether FGDb’s documented two-team process and audit trail are adequate for the project’s risk tolerance.
