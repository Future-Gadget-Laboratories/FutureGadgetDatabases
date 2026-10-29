> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Local Go setup and test boundaries

Use **Go 1.21.12** for the current FGDb baseline. `go.mod` declares Go 1.21; `WORKSPACE` and release packaging pin 1.21.12. Installing a newer Go does not establish compatibility with this pin. Toolchain modernization is a separate change with separate evidence.

## Install an isolated toolchain

The following example is for Linux amd64. For other systems, choose the matching archive and checksum from [Go's official download metadata](https://go.dev/dl/?mode=json&include=all). Keep the toolchain and caches outside the source checkout. Do not overwrite an existing installation.

```bash
export FGDB_TOOLS_DIR="$(cd .. && pwd)/.tools"
mkdir -p "$FGDB_TOOLS_DIR/downloads"
curl --fail --location \
  --output "$FGDB_TOOLS_DIR/downloads/go1.21.12.linux-amd64.tar.gz" \
  https://go.dev/dl/go1.21.12.linux-amd64.tar.gz
```

Verify before extraction. The official archive checksum is different from the mirrored/repacked archive checksum recorded in `WORKSPACE`; do not mix the two download sources.

```bash
printf '%s  %s\n' \
  121ab58632787e18ae0caa8ae285b581f9470d0f6b3defde9e1600e211f583c5 \
  "$FGDB_TOOLS_DIR/downloads/go1.21.12.linux-amd64.tar.gz" | sha256sum --check -
mkdir "$FGDB_TOOLS_DIR/go1.21.12"
tar --extract --gzip --strip-components=1 \
  --file "$FGDB_TOOLS_DIR/downloads/go1.21.12.linux-amd64.tar.gz" \
  --directory "$FGDB_TOOLS_DIR/go1.21.12"
```

Only extract if checksum validation succeeded. The separate `mkdir` deliberately fails if the installation already exists; do not continue after that failure. These commands are individual steps, not a script that ignores errors.

## Activate for a shell

Run from the repository root, preserving any existing `FGDB_TOOLS_DIR` override:

```bash
export FGDB_TOOLS_DIR="${FGDB_TOOLS_DIR:-$(cd .. && pwd)/.tools}"
export PATH="$FGDB_TOOLS_DIR/go1.21.12/bin:$PATH"
export GOPATH="$FGDB_TOOLS_DIR/gopath"
export GOCACHE="$FGDB_TOOLS_DIR/go-build-cache"
export GOTOOLCHAIN=local
go version
go env GOVERSION GOTOOLCHAIN GOPATH GOCACHE
```

Expected version: `go1.21.12 linux/amd64`. `GOTOOLCHAIN=local` prevents an implicit toolchain download from changing the compiler being tested. Save these exports in a local activation file if useful; do not commit workstation-specific paths.

## Test in layers

From the repository root:

```bash
go test -count=1 -timeout 5m ./pkg/fgdbtest/...
go vet ./pkg/fgdbtest/... ./pkg/cmd/fgdb-test/...
bash fgdb/test/run.sh pr
```

The standalone backup command has a nested module. Test it separately:

```bash
cd pkg/cmd/fgdb-backup
go test -count=1 -timeout 5m ./...
go vet ./...
```

These commands require dependency access on a cold cache. Some tests bind loopback ports. A sandbox refusing `127.0.0.1:0` is an environment failure, not a database defect. Record it and use the host's approved local-test execution path.

Without `FGDB_COCKROACH`, cluster-dependent tests can skip. A package reporting `ok` is therefore not evidence that its integration cases executed. Use `go test -json` or verbose output to preserve skips.

For integration, download the exact release from [releases.md](../releases.md), verify its recorded archive checksum, and inspect `cockroach version`. The distribution must be `OSS`. Do not use an arbitrary `cockroach` on PATH. Set `FGDB_COCKROACH` to its absolute executable path and set `LD_LIBRARY_PATH` to that archive's `lib` directory when needed. Use disposable stores and loopback addresses only.

```bash
export FGDB_COCKROACH="$FGDB_TOOLS_DIR/fgdb-oss-v23.2.15-fgdb.1/cockroach-oss-v23.2.15.linux-amd64/cockroach"
export LD_LIBRARY_PATH="$(dirname "$FGDB_COCKROACH")/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
"$FGDB_COCKROACH" version
cd pkg/cmd/fgdb-backup
go test -count=1 -timeout 20m -v ./...
```

The example assumes that release was extracted into the named tools directory. Adjust the extraction directory without changing the release pin. The `cd` in this block starts from the repository root.

## What Go alone does not provide

Core server packages depend on generated protobufs and other generated code. A failure such as missing `pkg/inspectz/inspectzpb` is not repaired by deleting imports or inventing generated stubs. Use the existing Bazel build graph.

For PR #24's hook change, the required production-package target is `//pkg/repstream:repstream_test` at that PR's exact head. It is absent from main before that PR lands. Do not call a helper-suite pass a hook pass.

The OSS server build is `./dev build oss`, or the documented release target `//pkg/cmd/cockroach-oss:cockroach-oss`. Do not use the default CCL-linked binary. A full Bazel build, disallowed-import test, binary inspection, packaging, integration and publication are separate gates. Local Go installation completes only the first prerequisite.

Record source SHA, Go version, release checksum, commands, exit codes, skips and artifact locations. Never promote a public claim merely because a test exists.
