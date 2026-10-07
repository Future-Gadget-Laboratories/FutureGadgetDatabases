# FutureGadgetDatabases

FutureGadgetDatabases (FGDb) is a distributed SQL database that you can run yourself.

**Distributed** means the data is split across processes. Each piece is copied to more than one process. One process can stop, and the database can keep answering.

**SQL** is the language you use to create tables and ask questions. FGDb speaks the PostgreSQL wire protocol on port 26257. Many PostgreSQL clients can connect. Not every PostgreSQL feature exists. For the feature list at this version, use the CockroachDB v23.2 docs linked below. Those docs describe the code this tree started from.

Future Gadget Laboratories maintains this repository. Cockroach Labs wrote CockroachDB v23.2.15, which is the code this tree started from. The two projects are separate. The `cockroach` command in this pin still prints the CockroachDB name. That is the program name in the imported source. A later cleanup may change names. It has not happened yet.

CipherBank is a design partner and a supported consumer. FGDb is a public database project with its own docs, license rules, and release pins.

## What you can run today

The version pin is **v23.2.15**.

| Piece | Value |
| --- | --- |
| Upstream tree | [cockroachdb/cockroach](https://github.com/cockroachdb/cockroach) tag `v23.2.15`, commit `3497fb02dce0beb6fc2bbf76c1ed7ad69cc31344` |
| Import commit in this repo | `a72022ff42bbceb6f0865b3b07e6fd6a9e94a6e1` |
| Binary we ship | `cockroach-oss` (CCL-free). Bazel target `//pkg/cmd/cockroach-oss:cockroach-oss` |
| Release tag, when published | `v23.2.15-oss` |
| Image name, when published | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2.15-oss` |

**CCL** means the CockroachDB Community License. It covers enterprise features under `pkg/ccl`. CCL has no date when it becomes Apache-2.0. The official Docker Hub image `cockroachdb/cockroach:v23.2.15` includes that code. FGDb's release builds the open-source binary instead.

The GitHub Release and the image are produced by the workflow in [docs/fgdb/RELEASING.md](docs/fgdb/RELEASING.md). Until that release exists, the download URLs in [docs/fgdb/releases.md](docs/fgdb/releases.md) are the planned names. They are not a promise that the files are already on the server. This document does not invent an image digest. Pin a digest after the release notes publish one.

## License, in short

The core of v23.2.0 through v23.2.15 was under the Business Source License 1.1 (BSL). The BSL **Change Date** is 2026-10-01. On that date the Change License takes over. The Change License is Apache License 2.0. The text is in [licenses/BSL.txt](licenses/BSL.txt) and [licenses/APL.txt](licenses/APL.txt).

That conversion covers files that were under the BSL. It does not cover CCL files. It does not cover later patches. v23.2.16 and every newer CockroachDB line we have checked are under the CockroachDB Software License (CSL). CSL is proprietary. This project does not copy CSL code or Cockroach Enterprise code.

Read [docs/fgdb/license.md](docs/fgdb/license.md) before you ship a binary. The root [LICENSE](LICENSE) file is the notice imported with v23.2.15. [NOTICE](NOTICE) explains how to read it after the Change Date.

## Try it

A full build is heavy. A cold compile wants on the order of 32 GB of RAM, a large disk cache, and a few hours. If release `v23.2.15-oss` is already published, use the tarball in [docs/fgdb/releases.md](docs/fgdb/releases.md). Otherwise build the open-source target:

```bash
./dev doctor
./dev build oss geos
./artifacts/cockroach-oss version
```

The `Distribution` line should say `OSS`. If it does not, you built the wrong target. `./dev build` without `oss` produces a binary that links CCL code. That is not the binary this project ships.

Then start one node on your own computer. `--insecure` means no encryption and no login. Use it only for a local lab. Run this from the repository root:

```bash
mkdir -p "$HOME/fgdb-lab"
./artifacts/cockroach-oss start-single-node \
  --insecure \
  --store="$HOME/fgdb-lab/lab-single" \
  --listen-addr=localhost:26257 \
  --http-addr=localhost:8080
```

In a second terminal, from the same repository root:

```bash
./artifacts/cockroach-oss sql --insecure --host=localhost:26257
```

[docs/fgdb/getting-started.md](docs/fgdb/getting-started.md) has the full lab, a three-node example, and a small schema. [docs/fgdb/operations.md](docs/fgdb/operations.md) shows how to turn telemetry off, what the license enforcer does in this tree, and what happens when you kill one node.

The admin web UI is the DB Console at <http://localhost:8080>. The open-source build serves the open-source UI (`pkg/ui/distoss`). Screens that need CCL code are absent from this binary.

## Docs written for this project

These pages were written by Future Gadget Laboratories. They are marked in the page banner.

| Page | Read it when you want to |
| --- | --- |
| [Product brief](docs/fgdb/PROJECT.md) | Paste a short description into chat or a design note |
| [Getting started](docs/fgdb/getting-started.md) | Build `cockroach-oss` and run one node, then three |
| [Architecture](docs/fgdb/architecture.md) | See how a query, a range, and a crash fit together |
| [License](docs/fgdb/license.md) | Decide what you are allowed to ship |
| [Releases and pins](docs/fgdb/releases.md) | Download a binary or pin an image by digest |
| [Operations](docs/fgdb/operations.md) | Turn telemetry off, use certificates, practice a crash |
| [Contributing](CONTRIBUTING.md) | Send a fix |
| [Security](SECURITY.md) | Report a vulnerability in private |
| [Support](SUPPORT.md) | See what help this project does and does not offer |
| [Governance](GOVERNANCE.md) | See who accepts changes |
| [Roadmap](docs/fgdb/roadmap.md) | See the v23.2.15 pin, the cleanroom process, and later naming work |
| [Cleanroom](docs/fgdb/CLEANROOM.md) | See how newer behavior is rewritten without copying proprietary code |
| [Cleanroom sources](docs/fgdb/SOURCES.md) | Read the public sources for that process |
| [Agent rules](docs/agent/CLEANROOM-ENFORCEMENT.md) | See when to refuse contaminated work |
| [Release procedure](docs/fgdb/RELEASING.md) | Publish `v23.2.15-oss` (maintainers) |
| [Build machine](docs/fgdb/RUNNER.md) | Set up the machine that builds the release (maintainers) |

Imported Cockroach Labs writing is still in the tree. The old root README is [docs/upstream/cockroach-v23.2.15-readme.md](docs/upstream/cockroach-v23.2.15-readme.md). The design note is [docs/design.md](docs/design.md). It still says RocksDB in places. This tree stores data with Pebble. Prefer [docs/fgdb/architecture.md](docs/fgdb/architecture.md) for a current sketch, and the [CockroachDB v23.2 architecture overview](https://www.cockroachlabs.com/docs/v23.2/architecture/overview.html) for the long upstream version. The "stable" docs on that site describe newer CockroachDB releases. Those releases are outside this pin.

## Roadmap

1. Stay on the Apache-2.0 core at v23.2.15 and publish CCL-free `cockroach-oss` builds.
2. Add newer behavior later through the cleanroom process in [docs/fgdb/CLEANROOM.md](docs/fgdb/CLEANROOM.md). One group writes down observable behavior. A different group implements that behavior without seeing proprietary source.
3. Clean up names so the project reads as FGDb in more places.

Details: [docs/fgdb/roadmap.md](docs/fgdb/roadmap.md).

## Project files

- [CONTRIBUTING.md](CONTRIBUTING.md)
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) (Contributor Covenant 2.1)
- [SECURITY.md](SECURITY.md)
- [SUPPORT.md](SUPPORT.md)
- [GOVERNANCE.md](GOVERNANCE.md)
- [NOTICE](NOTICE)
