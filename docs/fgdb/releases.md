> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Releases and pinning

This page is for people who run the binary. A laptop walk-through is [getting-started.md](getting-started.md). Maintainers who cut the next release follow [RELEASING.md](RELEASING.md). The workflow file is `.github/workflows/fgdb-oss-release.yml`. A normal push does not build it.

Two releases are published. Both images are public on GHCR. The database binary in both is `cockroach-oss` v23.2.15 (`Distribution: OSS`, Go `go1.21.12`).

## v23.2.15-fgdb.1 (current)

Use this one. The tarball adds `fgdb-backup` next to `cockroach`. [backup.md](backup.md) is how to run it. The phase 0 suite uses this tarball as the previous binary. See [testing.md](testing.md).

| Artifact | Value |
| --- | --- |
| GitHub Release | [`v23.2.15-fgdb.1`](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/tag/v23.2.15-fgdb.1) |
| Source commit | `56ce139286c504cd61ccc1da1d8e03b644afa131` |
| Tarball | `cockroach-oss-v23.2.15.linux-amd64.tgz` |
| Tarball sha256 | `5f9c36263bdbbfd1d8e6781fe09920fac33d4c608b4743be6681009f4215a379` |
| Image tag | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2.15-fgdb.1` |
| Commit tag | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:sha-56ce139286c504cd61ccc1da1d8e03b644afa131` |
| Image digest | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:c71351abe5885f08f1e356c3497e1121591aed2110043d7adbfed577438bb173` |

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-fgdb.1/cockroach-oss-v23.2.15.linux-amd64.tgz
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-fgdb.1/SHA256SUMS
```

`SHA256SUMS` is two-space `sha256sum` format. Inside the archive: `cockroach`, `fgdb-backup`, `lib/libgeos.so`, `lib/libgeos_c.so`, `LICENSE`, `licenses/`, and `OSS-BUILD.txt`. The image puts `fgdb-backup` on `PATH` next to `cockroach`.

This publish left git tag `v23.2.15-oss` and image tags `:v23.2.15-oss` and `:v23.2-oss` where they were.

Example pin for a Kubernetes container:

```yaml
image: ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:c71351abe5885f08f1e356c3497e1121591aed2110043d7adbfed577438bb173
```

## v23.2.15-oss (earlier)

This is the first published build. The tarball has `cockroach` and the GEOS libraries. It has no `fgdb-backup` binary. The image tags from that publish are `:v23.2.15-oss`, `:sha-2df632c521c7266d268ba76f25b17bd1fc4c02a9`, and the moving alias `:v23.2-oss`.

| Artifact | Value |
| --- | --- |
| GitHub Release | [`v23.2.15-oss`](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/tag/v23.2.15-oss) |
| Source commit | `2df632c521c7266d268ba76f25b17bd1fc4c02a9` |
| Tarball sha256 | `02a6b3dfdd69bcd54996a741cae3393202ba49560fc671f482b9a9ae61cef40f` |
| Image digest | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:937ed06722111fe0a73a9bebb8b551edcda7169f75a4ceb02a906a2d89490828` |

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/cockroach-oss-v23.2.15.linux-amd64.tgz
```

Installers that expect a file named `cockroach` inside the archive can use either tarball as a drop-in for the database binary. The filename is the same. The bytes differ. Check the sha256 for the tag you downloaded.

There is no arm64 release.

## What a digest is

A **digest** is the `sha256` of the image. It names one set of bytes. `:v23.2-oss` is a moving alias from the `v23.2.15-oss` publish. `:v23.2.15-oss` and `:v23.2.15-fgdb.1` change only when that same release is rebuilt. `:sha-<git sha>` names the image built from one commit. Write the digest into a deployment. The values above match the GitHub Release notes and an anonymous pull of the public GHCR tags.

The image base is Red Hat Universal Base Image 9 minimal. **UBI** is that base operating system. The exact build is pinned by digest in `build/deploy-oss/Dockerfile`. [RELEASING.md](RELEASING.md) records the current pin: build `9.8-1791279563`, manifest list `sha256:5ed244b62bbf4095080144d9d35eb8fcd3d39a9801f94aadd63b9d10978a01ae`. On top of that base the image adds the open-source binary, GEOS libraries under `/usr/local/lib/cockroach`, and `licenses/`. The `v23.2.15-fgdb.1` image also puts `fgdb-backup` on `PATH`. The image sets `COCKROACH_CHANNEL=fgl-oss`. That channel string marks the build. It is a label, and the binary does not read it as a license key. FIPS is off unless the image is rebuilt with `--build-arg fips_enabled=1`.

The database process in the image runs as root. That matches the upstream CockroachDB image. Helm charts and existing volumes expect the files in `/cockroach/cockroach-data` to be owned by root. To run as another user, prepare the volume so that user can already write the data directory, then set `securityContext.runAsUser` and `runAsGroup`. The image does not change ownership when it starts. The example is in [RELEASING.md](RELEASING.md).

## What not to pin

| Pin | Why it is the wrong pin for this project |
| --- | --- |
| `cockroachdb/cockroach:v23.2.15` | Official image. It includes CCL code. |
| `cockroachdb/cockroach:latest-v23.2` | Floating tag. It has followed later patches that are under the CSL. |
| `binaries.cockroachdb.com/cockroach-v23.2.15.linux-amd64.tgz` | Official tarball. It includes CCL code. |
| Any v23.2.16 or newer CockroachDB image | CSL, proprietary. |

**CSL** is the CockroachDB Software License. This project does not publish CSL builds.

## Check the binary after you unpack it

```bash
./cockroach version
```

The `Distribution` line should say `OSS`. On a release build the Go line should say `go1.21.12` and the build type should say `release`.

Then follow [operations.md](operations.md) for telemetry and certificates before the process is reachable by anyone else.
