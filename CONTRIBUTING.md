# Contributing to FutureGadgetDatabases

Future Gadget Laboratories maintains this repository. Thank you for a careful change.

A short "first evening" path is in [docs/fgdb/contributing.md](docs/fgdb/contributing.md). The rules are here.

People in the project follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) (Contributor Covenant 2.1).

## What this project accepts

Docs, bug fixes, and tests for the v23.2.15 `cockroach-oss` line are welcome.

**CCL** is the CockroachDB Community License. **CSL** is the CockroachDB Software License. CSL is proprietary. CCL does not become Apache-2.0.

A change is in bounds when it:

- explains how to build, run, or operate `cockroach-oss`
- fixes a defect in the Apache-2.0 core at this pin
- keeps the published binary free of `pkg/ccl` and `pkg/ui/distccl`

A change is out of bounds when it:

- copies Cockroach Enterprise source, or source from a CSL tag (v23.2.16 and later)
- turns the default `./dev build` / `//pkg/cmd/cockroach` binary into the artifact we tell people to ship
- sets `COCKROACH_ENABLE_LICENSE_ENFORCER` as something this line requires
- replaces the imported Cockroach Labs manuals under `docs/` with a wholesale rewrite

New explanations go in `docs/fgdb/` and start with the FGL banner those pages already use. The imported design note and RFCs stay where they are.

## License of your contribution

Put Future Gadget Laboratories documentation and project files under the Apache License 2.0, the same way [NOTICE](NOTICE) describes, unless the file you are editing already has a different header.

Do not delete an upstream copyright header when you edit an imported file. Add your change under that file's existing license.

## Pull requests

Keep the pull request about one problem.

In the description, write:

1. What changed.
2. How you checked it. For a docs change, say which commands you read against the source, or that you followed the copy-paste steps. A full Bazel build is not required for a docs-only change.
3. That you did not copy CSL or Cockroach Enterprise code.

Use the pull request template. It has a license checklist.

If you change `pkg/`, say what database behavior can change: correctness, durability, or availability. Add or update a test when that area already has tests.

If you build a binary, use:

```bash
./dev build oss
./artifacts/cockroach-oss version
```

The `Distribution` line must say `OSS`. `./dev build` without `oss` links CCL code.

The release workflow in [docs/fgdb/RELEASING.md](docs/fgdb/RELEASING.md) is how maintainers publish. Leave `.github/workflows/fgdb-oss-release.yml` and `build/fgdb/` runner scripts alone unless the change is about that release path.

## Issues

Use the templates in `.github/ISSUE_TEMPLATE/`.

| Template | When |
| --- | --- |
| Bug | A query result, a crash, a stuck node, or lost availability |
| Feature | A change you want in the database or the docs |
| Security | A pointer only. Put the details in a private advisory. See [SECURITY.md](SECURITY.md). |

Label names the templates ask for:

| Label | Meaning |
| --- | --- |
| `bug` | Something is wrong |
| `enhancement` | A new or changed behavior |
| `reliability` | Correctness, durability, replication, or availability |
| `security` | Maintainers apply this on private advisories |

Create those labels in the GitHub repository if they are missing. GitHub ignores a template label that does not exist yet.

## Commit messages

Use a sentence that says what changed and why.

```text
Document the 3-node crash drill in operations.md.

A laptop lab was stopping nodes without saying that the store directory has to stay.
```

## Secrets

Do not commit tokens, `.env` files, cloud keys, or cluster certificates. The runner registration token in [docs/fgdb/RUNNER.md](docs/fgdb/RUNNER.md) expires in an hour and stays out of git.

## Code of conduct reports

Conduct reports go to the maintainers through the contact in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). A public issue is the wrong place.
