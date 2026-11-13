# Porcupine

These files are the [Porcupine](https://github.com/anishathalye/porcupine) linearizability checker, copied at tag `v1.3.1` (`97cd067defd99df172053ba7f4ca5bbae57fe446`). `LICENSE.md` is that commit's MIT license, unchanged. `BUILD.bazel` and this README were added in this repository. The upstream copy is the baseline, with the local modification recorded below.

The suite uses this copy so the check does not depend on a network fetch and is not linked into `cockroach-oss`. The HTML view under `visualization/` is part of that upstream package. The suite reads the check result. It does not publish the HTML view.

Keep local changes minimal and record them here. When updating the upstream copy, record the new tag and commit and reevaluate the local changes and scanner exceptions.

## Local modification

In `visualization/index.js`, `render` no longer sorts a timestamp snapshot that was overwritten before use. The remaining numeric sort runs after endpoint adjustments and includes the added epsilon timestamps. Syntax was checked locally; runtime validation remains pending.

## Sonar triage

The root `sonar-project.properties` narrowly excludes cognitive complexity findings in these pinned files and rules. These algorithms and rendering closures are retained to avoid a larger behavioral fork; bug and security rules remain enabled. Reevaluate the exceptions when updating this copy.

- `go:S3776` in `checker.go`: `PartialLinearizationsOperations`, `checkSingle`, and `checkParallel` reconstruct histories, backtrack, and coordinate workers. Preserve rollback, metadata precedence, and worker-drain ordering.
- `go:S3776` in `model.go`: `NondeterministicModel.ToModel` adapts state sets and callbacks.
- `go:S3776` in `visualization.go`: `computeVisualizationData` reconstructs and annotates visualization data.
- `javascript:S3776` in `visualization/index.js`: `render` and its `handleMouseMove` closure handle layout and interaction.

In `visualization/index.js`, `render` sorts `sortedTags` with the default string ordering. Annotation tags are strings, and lexical ordering is intentional (for example, `"10"` precedes `"2"`). The `javascript:S2871` finding at that expression should be marked false positive individually; numeric timestamp sorts use a numeric comparator. No file-wide suppression of this rule is intended.

The scanner must later confirm that it loads these exact-file exceptions and that the individual false-positive disposition is applied.
