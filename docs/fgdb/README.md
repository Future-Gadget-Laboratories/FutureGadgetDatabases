> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# FGDb docs

These pages describe FutureGadgetDatabases. The product brief that you can paste into chat is [PROJECT.md](PROJECT.md).

| Page | What it covers |
| --- | --- |
| [PROJECT.md](PROJECT.md) | Short description of the project |
| [using-fgdb.md](using-fgdb.md) | Feature status and common database tasks |
| [getting-started.md](getting-started.md) | Build `cockroach-oss`, run one node, run three nodes, create a table |
| [testing.md](testing.md) | Run the suite locally, and what a run does not prove yet |
| [architecture.md](architecture.md) | SQL, ranges, copies, and what a crash means |
| [architecture/README.md](architecture/README.md) | Code map: statement flow, subsystems, CCL boundaries |
| [license.md](license.md) | BSL change date, CCL, CSL, and what not to ship |
| [releases.md](releases.md) | Tarball URLs, image names, and digest pins |
| [version-gating.md](version-gating.md) | Feature flags, release compatibility, and rollback rules |
| [operations.md](operations.md) | Telemetry, certificates, a crash drill, and a careful note on decommission |
| [backup-cli.md](backup-cli.md) | Current logical-backup command flags, exit codes, and prune behavior |
| [backup.md](backup.md) | Logical backup and restore to a directory or S3 |
| [backup-config.md](backup-config.md) | Backup configuration file and flags |
| [contributing.md](contributing.md) | A first evening on the project |
| [security.md](security.md) | How to report a vulnerability |
| [roadmap.md](roadmap.md) | v23.2.15 pin, then cleanroom work, then naming |
| [spec/PAR-004-benchmark-harness.md](spec/PAR-004-benchmark-harness.md) | Proposed benchmark harness, result schema, and baseline gates |
| [parity/PAR-207-v23.2.16-29-triage.md](parity/PAR-207-v23.2.16-29-triage.md) | Public v23.2.16–v23.2.29 fix-note triage against the v23.2.15 OSS pin |
| [CLEANROOM.md](CLEANROOM.md) | How to rewrite newer behavior without copying proprietary code |
| [SOURCES.md](SOURCES.md) | Public sources for that cleanroom process |
| [../agent/CLEANROOM-ENFORCEMENT.md](../agent/CLEANROOM-ENFORCEMENT.md) | Rules for people and agents: refuse contaminated work and discard it |
| [RELEASING.md](RELEASING.md) | How maintainers publish OSS and FGDb patch releases, including the digest-pinned base image |
| [RUNNER.md](RUNNER.md) | How maintainers set up the release build machine, including the local-disk Bazel cache |
| [runner-config.md](runner-config.md) | Runner setup configuration and resource limits |
| [ISSUES.md](ISSUES.md) | The short Discord note sent when someone opens an issue |

The current release is `v23.2.15-fgdb.1`; `v23.2.15-oss` is the earlier release. See [releases.md](releases.md) for checksums and digest pins. The fast checks run on GitHub-hosted runners; the separate heavy compile is manual, as described in [testing.md](testing.md).

The release workflow and the runner script are not changed by this index. Follow [RELEASING.md](RELEASING.md) and [RUNNER.md](RUNNER.md) when you cut a release.

Imported Cockroach Labs docs stay in the rest of `docs/`. The index for those files is [../README.md](../README.md).
