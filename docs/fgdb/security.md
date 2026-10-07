> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Security reports

The policy GitHub shows for this repository is [SECURITY.md](../../SECURITY.md). This page is the same process in a few short steps.

## What to report

Report a problem in the `cockroach-oss` line at v23.2.15 when it can leak data, corrupt data, or let someone past a certificate check.

Cockroach Labs ended assistance support for the v23.2 LTS line on 2026-07-08. Later official patches are under the CSL, and this project does not take them. A report can still be useful here. Maintainers will say so if the pin cannot absorb a fix without CSL code.

## Where to send it

Use a private GitHub security advisory:

<https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/security/advisories/new>

A public issue is the wrong place. A public issue is also the wrong place for a proof of concept, a dump of cluster data, or a log that contains passwords.

If the advisory form is unavailable, open a public issue that says only "security report available" and wait for a maintainer to move it to a private advisory. Leave out exploit detail in that issue.

Cockroach Labs' disclosure page is for Cockroach Labs' products. This repository is maintained by Future Gadget Laboratories. Send FGDb reports to the advisory link above.

## What helps

- Output of `cockroach-oss version` (or `cockroach version` if you used the release tarball). The `Distribution` line should be `OSS`. If it is not, say so. You may be running a different binary.
- How many nodes, and whether you used `--insecure` or certificates.
- What you did, in the smallest steps that still show the problem.
- What you expected, and what the cluster did instead.
- Whether data was exposed, changed, or made unavailable.

## What to leave out of the first message

- Customer data and secrets. Describe the shape of the data instead.
- A working exploit pasted into a public ticket.

Maintainers may ask for a log after a private channel is open. Logs can contain SQL text and addresses. Send those only in the advisory.
