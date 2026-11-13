> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.
>
> This is the product brief. Paste it when a chat, a ticket, or a design note needs one description of the project.

# FutureGadgetDatabases — product brief

FutureGadgetDatabases (FGDb) is a public distributed SQL database project from Future Gadget Laboratories.

You talk to it with SQL. Clients use the PostgreSQL wire protocol on port 26257. The data is split into ranges. A **range** is one slice of the sorted key space. Each range is copied to several nodes. A **node** is one running `cockroach-oss` process with its own disk directory. A majority of the copies must agree before a write is committed. That is why one node can die and the database can keep serving, as long as enough copies are left.

## Who it is for

Hobbyists and junior engineers can run a one-node lab, then a three-node lab, from [getting-started.md](getting-started.md).

Operators who need a pinned open-source binary use `v23.2.15-fgdb.1`, described in [releases.md](releases.md). That release adds `fgdb-backup`. The earlier `v23.2.15-oss` release is the database tarball from the first publish.

CipherBank is a design partner and a supported consumer. FGDb's docs, license rules, and roadmap are written for the project itself. CipherBank's install pins are a downstream choice. This repository does not change those pins.

## The pin

| Item | Value |
| --- | --- |
| Source pin | CockroachDB **v23.2.15** |
| Upstream commit | `3497fb02dce0beb6fc2bbf76c1ed7ad69cc31344` |
| Import commit | `a72022ff42bbceb6f0865b3b07e6fd6a9e94a6e1` |
| Binary | `cockroach-oss`, Bazel target `//pkg/cmd/cockroach-oss:cockroach-oss` |
| Current release | `v23.2.15-fgdb.1` (database plus `fgdb-backup`) |
| Earlier release | `v23.2.15-oss` (database tarball from the first publish) |
| Current image | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2.15-fgdb.1` |
| Consumer pin | The image digest in [releases.md](releases.md). For `v23.2.15-fgdb.1` that is `sha256:c71351abe5885f08f1e356c3497e1121591aed2110043d7adbfed577438bb173`. |

`cockroach version` on this pin still says CockroachDB. That string comes from the imported source. The project name is FutureGadgetDatabases.

A tag such as `v23.2-oss` moves when a newer `v23.2.15-oss` image is pushed. A digest stays on the bytes it names. Copy the digest from [releases.md](releases.md) into the deployment.

## License story

v23.2.0 through v23.2.15 shipped under the Business Source License 1.1. The Change Date in [licenses/BSL.txt](../../licenses/BSL.txt) is **2026-10-01**. After that date those files are under the Apache License 2.0 ([licenses/APL.txt](../../licenses/APL.txt)).

The CockroachDB Community License (CCL) does not convert. CCL code lives under `pkg/ccl` and `pkg/ui/distccl`. The official image `cockroachdb/cockroach:v23.2.15` includes it. FGDb releases build `cockroach-oss`, which leaves it out. Check with `cockroach-oss version`. The `Distribution` line should say `OSS`.

The CockroachDB Software License (CSL) is proprietary. It applies to v23.2.16 and to later CockroachDB lines. FGDb does not copy CSL or Cockroach Enterprise source.

Future Gadget Laboratories is not Cockroach Labs.

Full rules: [license.md](license.md).

## How a consumer should run it

1. Prefer the published `v23.2.15-fgdb.1` tarball or its GHCR image. Build from source with `./dev build oss geos` when you are changing the database. The lab steps are in [getting-started.md](getting-started.md).
2. Pin the image by digest.
3. Start nodes with `COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=true`. Then set `diagnostics.reporting.enabled` and `diagnostics.reporting.send_crash_reports.enabled` to `false`. Steps and a sample env file are in [operations.md](operations.md).
4. Leave the license enforcer off. In this tree it stays off unless `COCKROACH_ENABLE_LICENSE_ENFORCER=true`. This line has no license key and does not use that switch.
5. Treat `--insecure` as a laptop-only flag. Any shared network needs certificates. A copy-paste setup is in [operations.md](operations.md).

## What this project is doing next

1. Keep the v23.2.15 pin and the CCL-free build.
2. Add newer behavior later through the cleanroom process in [CLEANROOM.md](CLEANROOM.md). That process rewrites behavior. It does not copy proprietary source.
3. Rename user-facing strings toward FGDb after the behavior is in place.

## Where to send people

| Need | Page |
| --- | --- |
| Run a lab tonight | [getting-started.md](getting-started.md) |
| Understand a crash | [architecture.md](architecture.md) and [operations.md](operations.md) |
| File a bug | [CONTRIBUTING.md](../../CONTRIBUTING.md) and the bug issue template |
| Report a vulnerability | [SECURITY.md](../../SECURITY.md) |
| Ask what help exists | [SUPPORT.md](../../SUPPORT.md) |
