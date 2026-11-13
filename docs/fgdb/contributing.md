> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Contributing

The rules are in [CONTRIBUTING.md](../../CONTRIBUTING.md). This page is a short path for a first change.

## A useful first change

Pick a sentence in `docs/fgdb/` that confused you. Fix that sentence. A docs change does not need a Bazel build.

```bash
git checkout -b your-name/clearer-lab-step
# edit the page
git add docs/fgdb/getting-started.md
git commit -m "Explain what --store keeps on disk."
```

Open a pull request. In the body, say what you changed and that you have not copied Cockroach Enterprise or CSL code. CSL is the proprietary CockroachDB Software License. The pull request template asks for this.

## When you change database code

Code under `pkg/` is the database. A change there can lose data or stop a cluster if it is wrong. Describe the behavior you mean to change. Add or adjust a test when the tree already has a test for that area.

Build the open-source binary if your change affects what we ship:

```bash
./dev build oss
./artifacts/cockroach-oss version
```

The `Distribution` line should still say `OSS`.

The suite and how to run it are in [testing.md](testing.md). From the repository root:

```bash
fgdb/test/run.sh pr
```

That checks the claims file, the production-import baseline, and the Go tests in the pr tier. It does not start a cluster and it does not need Bazel.

Fast checks for a pull request live in `.github/workflows/fgdb-checks.yml` and run on GitHub-hosted `ubuntu-latest`. They include `fgdb/test/run.sh pr`. Every push to `main` runs that same workflow. A docs-only pull request does not need a compile.

A maintainer compiles `cockroach-oss` by starting `.github/workflows/fgdb-pr-build.yml` from the Actions tab and passing the 40-character SHA of a commit already on a branch or tag in this repository. Opening a pull request does not start that workflow, and the workflow does not accept a fork repository name. The compile runs on an ephemeral self-hosted build runner. It has no secrets. The release compile is the maintainer workflow in [RELEASING.md](RELEASING.md). The details are in [testing.md](testing.md).

Newer behavior that is only in a CSL or Enterprise build goes through [CLEANROOM.md](CLEANROOM.md). A normal fix inside the v23.2.15 tree does not.

## Where to ask

| Question | Place |
| --- | --- |
| A bug in this database | A GitHub issue, using the bug template |
| An idea | The feature template |
| A vulnerability | [security.md](security.md), in private |
| How people are expected to behave | [CODE_OF_CONDUCT.md](../../CODE_OF_CONDUCT.md) |

Imported notes from Cockroach Labs are under `docs/` outside this folder. Fixing a typo in an imported design note is fine. Replacing those notes with a rewrite of the upstream manual is out of scope. New explanations belong in `docs/fgdb/` and should carry the FGL banner at the top.
