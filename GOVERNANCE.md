# Governance

This is a short description of who accepts changes. It is not a corporate charter.

## Maintainers

Future Gadget Laboratories maintains `Future-Gadget-Laboratories/FutureGadgetDatabases`. People with write access on that GitHub repository are the maintainers. They review pull requests, apply labels, and moderate under [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Vincent (Crdb Steward) owns the direction of this database line: the version pin, the CCL-free build, and what the project ships.

**CCL** means the CockroachDB Community License. The binary this project ships is built without CCL code.

## Design partner

CipherBank is a design partner and a supported consumer. CipherBank's operators can ask for operational needs and can pin the published image. CipherBank does not own the repository, and CipherBank is not the licensor of the imported CockroachDB code.

A change to CipherBank's own installers happens in CipherBank's repositories, after a digest exists. This repository's release docs name the artifact. They do not edit CipherBank pins.

## Upstream

Cockroach Labs wrote CockroachDB v23.2.15. That commit is the source pin (`3497fb02dce0beb6fc2bbf76c1ed7ad69cc31344`). Cockroach Labs is not the maintainer of this fork. Future Gadget Laboratories does not speak for Cockroach Labs.

## What needs a maintainer review

- Anything that changes which binary we tell people to run
- Anything that copies or links CCL code into that binary
- Any proposal to import code from a CockroachDB Software License tag, or from Cockroach Enterprise
- License text, `NOTICE`, and the security policy
- The release workflow and the runner setup script

Docs fixes that keep those rules intact can be reviewed like any other small pull request.

## Decisions

Maintainers talk on the pull request. A change lands when a maintainer merges it. The Crdb Steward decides disputes about the pin, the license boundary, and the roadmap in [docs/fgdb/roadmap.md](docs/fgdb/roadmap.md).
