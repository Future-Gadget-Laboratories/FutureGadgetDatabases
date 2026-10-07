# Releasing cockroach-oss

This publishes a CCL-free build of CockroachDB **v23.2.15** (the BSL line
whose Change Date was 2026-10-01, now Apache-2.0). Official
`cockroachdb/cockroach` images and `binaries.cockroachdb.com` tarballs also
contain `pkg/ccl` and `pkg/ui/distccl`, which never convert. Those targets
are not built here.

The workflow is `.github/workflows/fgdb-oss-release.yml`. It does not run on
ordinary pushes or pull requests.

## Before the first build

1. Merge the workflow onto `main`.
2. On labcluster3, install the runner in [RUNNER.md](RUNNER.md). Confirm it
   is Idle with labels `self-hosted` and `fgdb-build`.
3. Do not send the compile to `ubuntu-latest` or to `self-hosted-ci`.

A cold build needs on the order of 32 GB of RAM and can take a few hours.
The workflow does not start that build by itself; someone has to dispatch it
or push the tag below.

## Cut v23.2.15-oss

Recommended: one tag push, one build.

`main` after this change is the v23.2.15 tree plus FGL docs and CI. It does
not change `pkg/`. Tag that commit:

```bash
git fetch origin main
git tag -a v23.2.15-oss origin/main -m "CCL-free cockroach-oss v23.2.15"
git push origin v23.2.15-oss
```

The tag must point at a commit that contains
`.github/workflows/fgdb-oss-release.yml` and the guarded upstream workflows.
Tagging the raw `v23.2.15` commit would run the old `bincheck` workflow
(`macos-latest` and `macos-latest-xlarge`) because that commit does not have
the fork guard. The publish job refuses to create the tag in that case.

The tag push builds `//pkg/cmd/cockroach-oss:cockroach-oss` and
`//c-deps:libgeos` on the `fgdb-build` runner, checks that the binary is
CCL-free, then uploads the Release and the image.

### Smoke build without publishing

From the Actions tab, run **FGDB OSS release**:

| Input | Smoke | Publish an existing release again |
| --- | --- | --- |
| `ref` | `v23.2.15` or `main` | the same ref you want rebuilt |
| `publish` | false | true |

`publish=false` compiles and uploads a workflow artifact only. It does not
create a Release or push an image.

`publish=true` against the raw `v23.2.15` tag fails before the compile if
release `v23.2.15-oss` does not exist yet. Dispatch with `ref` set to `main`
(or push the tag) for the first publish.

Pushing the tag after a `publish=true` dispatch of the same commit does not
start a second compile. The extra tag-triggered run is skipped while that
dispatch is in progress or after it has succeeded.

## Artifacts

Release tag: `v23.2.15-oss`

Binary tarball (amd64 only):

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/cockroach-oss-v23.2.15.linux-amd64.tgz
```

Checksums, two-space `sha256sum` format:

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/SHA256SUMS
```

Inside the tarball:

```text
cockroach-oss-v23.2.15.linux-amd64/cockroach
cockroach-oss-v23.2.15.linux-amd64/lib/libgeos.so
cockroach-oss-v23.2.15.linux-amd64/lib/libgeos_c.so
cockroach-oss-v23.2.15.linux-amd64/LICENSE
cockroach-oss-v23.2.15.linux-amd64/licenses/
```

`cockroach` is the `cockroach-oss` binary under the upstream filename.
`licenses/CCL.txt` is license text from the source tree, not CCL code.
`OSS-BUILD.txt` in the archive says the same thing.

## Container image

Registry (public package; the workflow tries to set visibility and will say
so if the token cannot):

```text
ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2.15-oss
ghcr.io/future-gadget-laboratories/futuregadgetdatabases:sha-<git sha>
ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2-oss
```

`v23.2-oss` is a moving alias. `v23.2.15-oss` moves only when that release
is rebuilt. `sha-<git sha>` is the immutable tag for one commit.

The image is UBI 9 minimal build `9.8-1791279563` (manifest list
`sha256:5ed244b62bbf4095080144d9d35eb8fcd3d39a9801f94aadd63b9d10978a01ae`,
pinned in `build/deploy-oss/Dockerfile`) plus the OSS binary, `libgeos` in
`/usr/local/lib/cockroach`, and `licenses/`. `COCKROACH_CHANNEL=fgl-oss`.
FIPS is off unless the image is rebuilt with `--build-arg fips_enabled=1`.
The comment at the top of the Dockerfile says how to bump the base image.

### The process runs as root

The image does not set a `USER`. The process is root, which is what the
upstream CockroachDB image does. Helm charts and existing volumes expect
the database files in `/cockroach/cockroach-data` to be owned by root.

To run as another user, prepare the volume so that user can already write
the data directory, then set `securityContext.runAsUser` and `runAsGroup`
on the pod. The image does not change ownership when it starts. If the
directory is still owned by root, the other user cannot open it.

```yaml
securityContext:
  runAsUser: 1000
  runAsGroup: 1000
```

### CipherBank pin

Pin the digest from the release notes, not a floating tag:

```text
ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:<digest>
```

Example (the digest is filled in when the image is pushed; do not invent one):

```text
image: ghcr.io/future-gadget-laboratories/futuregadgetdatabases@sha256:<digest>
```

The binary URL above is the drop-in for an installer that today fetches
`cockroach-v<version>.linux-amd64.tgz` and expects a file named `cockroach`.

## What the build actually runs

On the runner, as `fgdb-runner`, `build-oss.sh` runs:

```text
bazel build //pkg/cmd/cockroach-oss:cockroach-oss //c-deps:libgeos
```

That is the same shape as the v23.2.15 release tooling
(`pkg/cmd/publish-provisional-artifacts` and
`build/teamcity/cockroach/ci/builds/build_impl.sh`): bazel is invoked
directly, with the targets on the command line. `./dev build oss geos`
is the workstation command. On a clean checkout `./dev` exits until
`dev doctor` has written `bin/.dev-status`. Doctor on Linux asks which
config to append to `.bazelrc.user` and can add more lines
(`lintonbuild`, a test tmpdir). The release job does not run it.
`--interactive=false` does not make that autofix safe: the Linux path
refuses to edit the file unless interactive mode is on.

`.bazelrc.user` still supplies `--config=ci`, `--config=nolintonbuild`,
`--//build/toolchains:nogo_disable_flag`, `--config=crosslinuxbase`,
`-c opt`, and one quoted stamp option:

```text
build '--workspace_status_command=./build/bazelutil/stamp.sh x86_64-pc-linux-gnu fgl-oss release'
```

The quotes are required. Bazel splits unquoted spaces in `.bazelrc` into
separate arguments, and the leftover words become build targets. An
unquoted line is why the first lab build tried to build `//:fgl-oss`.
`stamp.sh` reads the channel from that argument list. It does not read
the `BUILD_CHANNEL` environment variable. The three arguments are the
target triple, the channel (`fgl-oss`), and the build type (`release`).
Upstream v23.2.15 release builds use the same shape in
`pkg/cmd/publish-provisional-artifacts`: one single-quoted
`--workspace_status_command=./build/bazelutil/stamp.sh <triple> <channel> release`,
then `-c opt`, `--config=ci`, and `--config=crosslinuxbase`. This fork
keeps the channel `fgl-oss` and does not pass `--config=force_build_cdeps`.

`crosslinuxbase` is the release config. It selects the public
linux/amd64 crosstool and does not set its own stamp command.
`--config=crosslinux` is the development form of that config: it adds a
stamp command that only passes the triple, which would leave the channel
as `unknown` and the build type as `development`.

`./dev` is not used, including `./dev build --cross`. That `--cross` path
starts the private `us-east1-docker.pkg.dev/crl-ci-images/cockroach/bazel`
image, which this fork cannot pull.

`//pkg/ui/distoss` is a Bazel dependency of `cockroach-oss`. The UI is
built with that target. There is no separate Node or `dev ui` step.
Submodules come from the workflow checkout (`submodules: recursive`),
not from `dev doctor`.

libgeos comes from the public prebuilt c-dep archive
(`storage.googleapis.com/public-bazel-artifacts/c-deps/...`), not from
`--config=force_build_cdeps`. Forcing a from-source c-dep build needs the
private builder image or a host toolchain this tree was not tested with
(Ubuntu 26.04). After the build, the script copies
`output_base/external/archived_cdep_libgeos_linux/lib/libgeos.so` and
`libgeos_c.so` to `src/lib/`. The binary is copied from
`_bazel/bin/pkg/cmd/cockroach-oss/cockroach-oss_/cockroach-oss` to
`src/cockroach-oss`. Those are the paths `verify-oss-binary.sh` and
`package-oss-tarball.sh` read. The image is built from the tarball, not
from `bazel-bin`.

After the link, the workflow:

- runs `cockroach-oss version` and requires `Distribution: OSS`, build type
  `release`, and Go `go1.21.12`
- rejects the binary if `nm` or `strings` shows `pkg/ccl` or `pkg/ui/distccl`
- runs `//pkg/cmd/cockroach-oss:cockroach-oss_disallowed_imports_test`

## Knobs

In `.github/workflows/fgdb-oss-release.yml`:

| Env | Default | Effect |
| --- | --- | --- |
| `IMAGE_REPOSITORY` | `ghcr.io/future-gadget-laboratories/futuregadgetdatabases` | image name |
| `GHCR_PACKAGE` | `futuregadgetdatabases` | package visibility API name |
| `BUILD_CHANNEL` | `fgl-oss` | stamp channel |
| `PUSH_MINOR_ALIAS` | `true` | also push `v23.2-oss` |
| `EXPECT_GO` | `go1.21.12` | version check |

In `build/fgdb/write-bazelrc-user.sh`, `FGDB_EXTRA_BAZELRC` appends extra
Bazel lines. Those lines must already be valid bazelrc: quote any value
that contains a space. CPU and RAM defaults are in the runner setup script.
The script refuses a channel, triple, cache path, or output path that
contains a space or a quote, so those values cannot split into targets.

## Out of scope

- arm64 and other platforms
- v23.2.16 and anything under the CockroachDB Software License
- `//pkg/cmd/cockroach` (that binary links CCL)
- Changing CipherBank pins in this repository
