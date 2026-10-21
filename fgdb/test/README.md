# FGDb test suite

This directory is the data and the entry point for the FGDb suite. The Go code lives in `pkg/fgdbtest/`. Everything under `pkg/fgdbtest/` is Bazel `testonly`, so the `cockroach-oss` binary cannot import it.

| Path | What it is |
| --- | --- |
| `claims.yaml` | One row per claim, and the test that checks it |
| `tiers/` | What `run.sh` runs for `pr`, `nightly`, `weekly`, and `interim` |
| `skips.yaml` | Upstream tests that are waived. The list is empty |
| `test-code-in-prod-baseline.txt` | Production files that already import test helpers. New ones fail the check |
| `run.sh` | `fgdb/test/run.sh <tier>` |
| `scripts/run-local.sh` | The same runner, with `--tier=` |

`docs/fgdb/testing.md` explains how to run a tier and what is not tested yet.

Partition and disk faults are not in this slice. A run says `partition NOT tested` and `disk faults NOT tested` in the first lines of `summary.md`.
