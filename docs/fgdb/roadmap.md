> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Roadmap

Three stages. The first one is the project you can build today.

## 1. Stay on v23.2.15 and ship `cockroach-oss`

The source pin is CockroachDB v23.2.15, upstream commit `3497fb02dce0beb6fc2bbf76c1ed7ad69cc31344`. The BSL Change Date for that line was 2026-10-01, so the BSL core is Apache-2.0. The binary target is `//pkg/cmd/cockroach-oss:cockroach-oss`. CCL code stays out of the published binary.

The release tag is `v23.2.15-oss`. The image name is `ghcr.io/future-gadget-laboratories/futuregadgetdatabases`. Consumers pin the digest from the release notes after the image exists. The procedure is [RELEASING.md](RELEASING.md). The names are listed in [releases.md](releases.md).

This stage does not add v23.2.16 or any CSL patch. **CSL** is the CockroachDB Software License.

## 2. Newer behavior by cleanroom

Some behavior exists only in Cockroach Enterprise or in CSL releases. **Cleanroom** means one group writes down what you can observe (inputs, outputs, error codes), and a different group implements that behavior. The implementers do not look at the proprietary source.

The process is [CLEANROOM.md](CLEANROOM.md). The public sources for it are [SOURCES.md](SOURCES.md). Rules for agents are [../agent/CLEANROOM-ENFORCEMENT.md](../agent/CLEANROOM-ENFORCEMENT.md).

This repository does not copy that proprietary source. Until a cleanroom change lands here, features that exist only in newer CockroachDB releases are outside FGDb.

## 3. FGL names

After the behavior is in place, user-facing names can move toward FutureGadgetDatabases. Today `cockroach version` still prints CockroachDB. That string is part of the imported pin. Changing it is a later patch, with a note so scripts that parse the version output can adjust.

## Explicitly later

These are real gaps. They are not in this docs pass:

- A public website
- Deeper operations runbooks (backup, restore, upgrades, multi-region)
- More cleanroom feature notes, as each feature gets a behavior spec
- arm64 release artifacts
- Consumer install pins, after a published image digest exists. This repository does not edit those pins.
