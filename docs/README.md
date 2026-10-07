# Documentation

## FutureGadgetDatabases

Future Gadget Laboratories wrote the project docs in [fgdb/](fgdb/README.md). Start there for what this database is, how to build the CCL-free binary, and how to run a small cluster.

**CCL** means the CockroachDB Community License. The binary this project ships leaves that code out.

The product brief is [fgdb/PROJECT.md](fgdb/PROJECT.md).

Maintainer and process pages:

- [Release procedure](fgdb/RELEASING.md) and [build machine](fgdb/RUNNER.md)
- [Cleanroom process](fgdb/CLEANROOM.md) and [citations](fgdb/SOURCES.md)
- [Agent enforcement](agent/CLEANROOM-ENFORCEMENT.md) — refuse contaminated work and discard it

## Imported CockroachDB notes

The rest of this directory came with CockroachDB v23.2.15. Those files are upstream engineering notes and design history. Future Gadget Laboratories left them in place.

- [Design document](design.md). This note still mentions RocksDB in places. The storage engine in this tree is Pebble. See [fgdb/architecture.md](fgdb/architecture.md).
- [Tech notes](tech-notes/README.md)
- [RFCs](RFCS/README.md)
- [Imported root README and contributing note](upstream/README.md)

Client-facing Cockroach Labs manuals for this pin live on the web, not in this folder: <https://www.cockroachlabs.com/docs/v23.2/>. The "stable" manuals on that site describe newer CockroachDB releases. This project does not ship those releases.
