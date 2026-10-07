> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Releases and pinning

This page is for people who run the binary. Maintainers who cut the release follow [RELEASING.md](RELEASING.md). The workflow file is `.github/workflows/fgdb-oss-release.yml`. A normal push does not build it.

## Names

These are the names the workflow publishes. Until the first successful publish, downloads fail. Check first:

```bash
gh release view v23.2.15-oss \
  --repo Future-Gadget-Laboratories/FutureGadgetDatabases
```

| Artifact | Name |
| --- | --- |
| Git tag and GitHub Release | `v23.2.15-oss` |
| Linux amd64 tarball | `cockroach-oss-v23.2.15.linux-amd64.tgz` |
| Checksums | `SHA256SUMS` (two-space `sha256sum` format) |
| Image | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2.15-oss` |
| Image for one git commit | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:sha-<git sha>` |
| Moving minor tag | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2-oss` |

Tarball URL:

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/cockroach-oss-v23.2.15.linux-amd64.tgz
```

Checksum URL:

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/SHA256SUMS
```

The tarball contains a file named `cockroach`. That file is the `cockroach-oss` binary. Installers that expect the upstream layout (a file named `cockroach` inside the archive) can use this tarball as a drop-in. Also in the archive: `lib/libgeos.so`, `lib/libgeos_c.so`, `LICENSE`, `licenses/`, and `OSS-BUILD.txt`.

There is no arm64 release in this first cut.

## Pin a digest

A **digest** is the `sha256` of the image. It names one set of bytes. A tag can move. `v23.2-oss` is a moving alias. `v23.2.15-oss` changes only when that release is rebuilt. `sha-<git sha>` names the image built from one commit. The digest is the pin to write into a deployment.

After the workflow pushes the image, the release notes contain the digest. Copy it from there. This file uses a placeholder because inventing a digest would point you at the wrong bytes.

```text
ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:<digest>
```

Example for a Kubernetes container:

```yaml
image: ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:<digest>
```

Replace `<digest>` with the value from the release notes. Keep the `sha256:` prefix.

The image base is Red Hat Universal Base Image 9 minimal. **UBI** is that base operating system. The exact build is pinned by digest in `build/deploy-oss/Dockerfile`. [RELEASING.md](RELEASING.md) records the current pin: build `9.8-1791279563`, manifest list `sha256:5ed244b62bbf4095080144d9d35eb8fcd3d39a9801f94aadd63b9d10978a01ae`. On top of that base the image adds the open-source binary, GEOS libraries under `/usr/local/lib/cockroach`, and `licenses/`. The image sets `COCKROACH_CHANNEL=fgl-oss`. That channel string marks the build. It is not a license key. FIPS is off unless the image is rebuilt with `--build-arg fips_enabled=1`.

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
