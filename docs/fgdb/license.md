> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.
> It is an explanation of the license files in this tree. It is not legal advice.

# License and compliance

Read this before you publish a binary, an image, or a fork of this repository.

## Terms

| Term | Meaning in this project |
| --- | --- |
| BSL | Business Source License 1.1. The license file says it is not an open-source license while it is in force. On the Change Date it grants the Change License. |
| Change Date | The day the BSL switches. For CockroachDB 23.2 it is **2026-10-01**. See [licenses/BSL.txt](../../licenses/BSL.txt). |
| Change License | Apache License 2.0. The text in this tree is [licenses/APL.txt](../../licenses/APL.txt). |
| CCL | CockroachDB Community License. Enterprise features. There is no Change Date. The text is [licenses/CCL.txt](../../licenses/CCL.txt). |
| CSL | CockroachDB Software License. Proprietary. Used for v23.2.16 and for later CockroachDB lines. This tree does not contain CSL code. |
| OSI | Open Source Initiative. Apache-2.0 is an OSI-approved license. CCL and CSL are not. |
| `cockroach-oss` | The entry point in `pkg/cmd/cockroach-oss`. Its comment says it excludes all CCL-licensed code. |

Future Gadget Laboratories maintains this repository. Cockroach Labs wrote the v23.2.15 code and owns the CockroachDB trademark. The two projects are separate.

## What converted on 2026-10-01

The BSL parameters for this line say:

- Licensed Work: CockroachDB 23.2
- Change Date: 2026-10-01
- Change License: Apache License, Version 2.0

The Change Date has passed. Files that were under that BSL are now under Apache-2.0. The grant is per version. v23.2.0 and v23.2.15 share this Change Date. A later patch does not inherit it when that patch was published under a different license.

The root [LICENSE](../../LICENSE) file is the notice imported with the tree. It still says the source is "variously licensed" under the BSL, the CCL, and other licenses. Read it with `licenses/BSL.txt`. The BSL text itself is what applies the Change Date. [NOTICE](../../NOTICE) is the Future Gadget Laboratories summary at the root of the repo.

Other files in the tree use MIT, BSD, and the notices under [licenses/](../../licenses/). Those stay as their own headers say. Apache-2.0 conversion of the BSL core does not relicense those components.

## What did not convert

CCL code does not become Apache-2.0. In this tree the CCL program code is under `pkg/ccl`. The CCL admin UI bundle is `pkg/ui/distccl`. The open-source UI bundle is `pkg/ui/distoss`.

The `cockroach-oss` package links `pkg/ui/distoss` and does not link `pkg/ccl`. A test on that target, `disallowed_imports_test`, rejects imports of `pkg/ccl` and `pkg/ui/distccl`.

The official binary and the official Docker Hub image `cockroachdb/cockroach:v23.2.15` include CCL code. They are the wrong artifact when you need a build made only from the converted core plus the other open-source components that binary links. Use `cockroach-oss` from this project.

`licenses/CCL.txt` inside a release tarball is the license text copied from the source tree. It is not the CCL program code. The file `OSS-BUILD.txt` in that tarball says the same thing. See [RELEASING.md](RELEASING.md).

## What this project does not ship

| Do this | Leave this out |
| --- | --- |
| `./dev build oss` or `./dev build oss geos` | `./dev build` with no target, which builds `//pkg/cmd/cockroach` |
| Bazel target `//pkg/cmd/cockroach-oss:cockroach-oss` | A binary whose `version` output does not say `Distribution: OSS` |
| Source at tag v23.2.15, plus Future Gadget Laboratories docs and release tooling | Patches from v23.2.16 or any CSL tag |
| The GHCR image built by this repo's release workflow, pinned by digest | `cockroachdb/cockroach` from Docker Hub, including tag `v23.2.15` and floating tags such as `latest-v23.2` |

`latest-v23.2` on Docker Hub has pointed at later patch releases. Later v23.2 patches are CSL. Pin an explicit FGDb digest after we publish one. Do not follow a floating tag.

v23.2.16 changed the license of the `cockroach` binary Cockroach Labs distributes to the CSL. This project stops at v23.2.15 for that reason.

Copying Cockroach Enterprise source, or source from a CSL tag, into this repository is out of bounds. A clean-room port is a later roadmap item and has to be written without that source. See [roadmap.md](roadmap.md).

## How to check a binary you built

```bash
./artifacts/cockroach-oss version
```

You want:

- `Distribution` line: `OSS`
- Build type on a release build: `release`
- Go version on a release build: `go1.21.12`

The release workflow also rejects the binary if `nm` or `strings` shows `pkg/ccl` or `pkg/ui/distccl`, and it runs `//pkg/cmd/cockroach-oss:cockroach-oss_disallowed_imports_test`. Those checks are described in [RELEASING.md](RELEASING.md).

## Source that stays in the git tree

`pkg/ccl` remains in the git history because this repository imported the v23.2.15 tree. Keeping the files makes the import traceable. Building them into the artifact we publish is a different step. The release path does not build them.

If you are unsure whether a file is CCL, read the header at the top of the file and the [LICENSE](../../LICENSE) note. CCL files identify the CockroachDB Community License.
