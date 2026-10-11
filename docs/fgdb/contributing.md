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

The suite and how to run it are in [testing.md](testing.md). `fgdb/test/run.sh pr` checks the claims file and the new Go tests.

The full release compile is the maintainer workflow in [RELEASING.md](RELEASING.md). You do not need to run it for a docs pull request.

## Where to ask

| Question | Place |
| --- | --- |
| A bug in this database | A GitHub issue, using the bug template |
| An idea | The feature template |
| A vulnerability | [security.md](security.md), in private |
| How people are expected to behave | [CODE_OF_CONDUCT.md](../../CODE_OF_CONDUCT.md) |

Imported notes from Cockroach Labs are under `docs/` outside this folder. Fixing a typo in an imported design note is fine. Replacing those notes with a rewrite of the upstream manual is out of scope. New explanations belong in `docs/fgdb/` and should carry the FGL banner at the top.
