# Support

FutureGadgetDatabases is a public project maintained by Future Gadget Laboratories. Help is best-effort through this repository.

## What you can ask here

| Need | Where |
| --- | --- |
| The database returns a wrong result, crashes, or loses availability | A GitHub issue, using the bug template. Add the `reliability` label if you can. |
| You want a change | The feature template |
| You think you found a vulnerability | [SECURITY.md](SECURITY.md), in private |
| You want to know what the project is | [docs/fgdb/PROJECT.md](docs/fgdb/PROJECT.md) |
| You want to run a lab | [docs/fgdb/getting-started.md](docs/fgdb/getting-started.md) |

Include `cockroach-oss version` output, the number of nodes, and the smallest steps that show the problem.

## What this repository does not provide

Cockroach Labs support, Cockroach Cloud, and enterprise contracts are handled by Cockroach Labs. This project is a separate effort. The v23.2 manuals on their site are still the right reference for SQL behavior at this pin: <https://www.cockroachlabs.com/docs/v23.2/>.

CipherBank runs this database as a consumer. CipherBank on-call is CipherBank's process. A CipherBank outage is not, by itself, a ticket in this repository. A defect in the `cockroach-oss` pin is.

There is no paid support offer in this repository.

## Versions

The pin is v23.2.15, built as `cockroach-oss`. Official patches after v23.2.15 are under the CockroachDB Software License. This project does not ship them. If your binary's `Distribution` line does not say `OSS`, you are on a different build. Say so in the issue.
