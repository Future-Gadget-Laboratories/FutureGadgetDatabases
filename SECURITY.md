# Security policy

Future Gadget Laboratories maintains FutureGadgetDatabases. Report vulnerabilities in this repository to the maintainers, in private.

## Supported line

The line this project publishes is the CCL-free `cockroach-oss` build of CockroachDB v23.2.15 (release tag `v23.2.15-oss`, when that release exists).

**CCL** is the CockroachDB Community License. Official CockroachDB images include CCL code. A finding that exists only in CCL code is outside the binary this project ships. Say which binary you ran. `cockroach-oss version` prints a `Distribution` line. `OSS` means you are on the binary this policy covers.

Cockroach Labs ended assistance support for the v23.2 LTS line on 2026-07-08. Later official patches are under the CockroachDB Software License. This project does not merge those patches. Maintainers will still read a report about the v23.2.15 open-source pin.

## How to report

Open a private advisory:

<https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/security/advisories/new>

Do not file a public issue with exploit details, passwords, or customer data.

If the advisory form is unavailable, open a public issue whose body is only "security report available" and wait for a maintainer to move the conversation. Leave the details out of that issue.

Cockroach Labs' disclosure process is for Cockroach Labs products. Use the advisory link above for this repository.

A step-by-step of what to include is in [docs/fgdb/security.md](docs/fgdb/security.md).

## Conduct reports

Reports about harassment belong to [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). You can use the same advisory form. Start the title with `Code of Conduct:` so it is routed as a conduct report.
