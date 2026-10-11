# Porcupine

These files are the [Porcupine](https://github.com/anishathalye/porcupine) linearizability checker, copied at tag `v1.3.1` (`97cd067defd99df172053ba7f4ca5bbae57fe446`). `LICENSE.md` is that commit's MIT license, unchanged. The Go sources and `visualization/` match that commit with no local changes. `BUILD.bazel` and this README are the only files added in this repository.

The suite uses this copy so the check does not depend on a network fetch and is not linked into `cockroach-oss`. The HTML view under `visualization/` is part of that upstream package. The suite reads the check result. It does not publish the HTML view.

Do not edit these files in place. Update them by copying a newer upstream tag and recording the new commit here.
