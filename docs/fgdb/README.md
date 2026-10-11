> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# FGDb docs

These pages describe FutureGadgetDatabases. The product brief that you can paste into chat is [PROJECT.md](PROJECT.md).

| Page | What it covers |
| --- | --- |
| [PROJECT.md](PROJECT.md) | Short description of the project |
| [getting-started.md](getting-started.md) | Build `cockroach-oss`, run one node, run three nodes, create a table |
| [testing.md](testing.md) | Run the suite locally, and what a run does not prove yet |
| [architecture.md](architecture.md) | SQL, ranges, copies, and what a crash means |
| [architecture/README.md](architecture/README.md) | Code map: statement flow, subsystems, CCL boundaries |
| [license.md](license.md) | BSL change date, CCL, CSL, and what not to ship |
| [releases.md](releases.md) | Tarball URLs, image names, and digest pins |
| [operations.md](operations.md) | Telemetry, certificates, a crash drill, and a careful note on decommission |
| [backup.md](backup.md) | Logical backup and restore to a directory or S3 |
| [contributing.md](contributing.md) | A first evening on the project |
| [security.md](security.md) | How to report a vulnerability |
| [roadmap.md](roadmap.md) | v23.2.15 pin, then cleanroom work, then naming |
| [CLEANROOM.md](CLEANROOM.md) | How to rewrite newer behavior without copying proprietary code |
| [SOURCES.md](SOURCES.md) | Public sources for that cleanroom process |
| [../agent/CLEANROOM-ENFORCEMENT.md](../agent/CLEANROOM-ENFORCEMENT.md) | Rules for people and agents: refuse contaminated work and discard it |
| [RELEASING.md](RELEASING.md) | How maintainers publish `v23.2.15-oss`, including the digest-pinned base image |
| [RUNNER.md](RUNNER.md) | How maintainers set up the release build machine, including the local-disk Bazel cache |
| [ISSUES.md](ISSUES.md) | The short Discord note sent when someone opens an issue |

The release workflow and the runner script are not changed by this index. Follow [RELEASING.md](RELEASING.md) and [RUNNER.md](RUNNER.md) when you cut a release.

Imported Cockroach Labs docs stay in the rest of `docs/`. The index for those files is [../README.md](../README.md).
