> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Using FGDb

This chart tracks FGDb additions and proposed work alongside a practical introduction to the OSS database. Supported means an FGDb addition or check has been merged and tested on main; checks cover only the scope described below. Tentative means an open, experimental change that may change; planned means a proposal, not a delivered capability. CCL features are not included.

| Feature | Status | Where to learn more |
| --- | --- | --- |
| Interim logical backup and restore with `fgdb-backup` | supported | [backup.md](backup.md), [backup-cli.md](backup-cli.md) |
| Initial OSS capability suite and guards | supported | [oss-capabilities.md](oss-capabilities.md), [#24](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/24) |
| Independent replication ingest hook check | supported | [oss-capabilities.md](oss-capabilities.md), [#24](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/24) |
| Additional OSS guard claims OSS-004–008 | tentative | [#32](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/32) |
| Expanded OSS gate contract checks | tentative | [#31](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/31) |
| Strict CCL checks by default | tentative | [#38](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/38) |
| EXPORT capability checks | tentative | [#42](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/42) |
| pgcrypto raw encrypt/decrypt (`feature.pgcrypto_ciphers.enabled`, default on) | tentative | [#43](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/43) |
| Workload IMPORT loader | planned | [#41](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/41) |
| Version-gating and feature-flag contract | planned | [#33](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/33) |

Rows marked supported are covered by checks that run in the [FGDB checks workflow](../../.github/workflows/fgdb-checks.yml) (`go vet` and the `pr` tier of [fgdb/test/run.sh](../../fgdb/test/run.sh)). The most recent run on main at the time of writing passed: [FGDB checks run 40156037249](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/actions/runs/40156037249) at commit 0ddbad46. Rows marked tentative or planned are not covered on main.

## Install

For Linux amd64, download the current release tarball and its checksums from [releases.md](releases.md), verify it with `sha256sum -c SHA256SUMS`, and unpack it. The build target is `cockroach-oss`; the executable inside the release tarball is named `cockroach`. The archive also contains `fgdb-backup` and a neighboring `lib/` directory for spatial libraries. Set `BIN` to the unpacked `cockroach` and run `"$BIN" version`; its distribution should be OSS.

The container image is `ghcr.io/future-gadget-laboratories/futuregadgetdatabases:v23.2.15-fgdb.1`. Pin its digest from [releases.md](releases.md) for deployments. To build from source, see [getting-started.md](getting-started.md#from-source); the resulting executable is `artifacts/cockroach-oss`.

## Start a cluster

These examples use `--insecure`, which disables TLS and login. Use them only on your own computer and keep the ports private. See [operations.md](operations.md) for certificates and operational guidance.

### One node

With `BIN` set to the executable:

```bash
mkdir -p "$HOME/fgdb-lab"
"$BIN" start-single-node --insecure \
  --store="$HOME/fgdb-lab/demo-single" \
  --listen-addr=localhost:26257 --http-addr=localhost:8080 \
  --pid-file="$HOME/fgdb-lab/demo-single.pid"
```

This initializes a single-replica cluster. Leave it running and use another terminal to connect.

### Three local nodes

Stop the single node first, then start three processes with separate stores and ports:

```bash
kill "$(cat "$HOME/fgdb-lab/demo-single.pid")"
# Wait for the process to exit before reusing its ports.
"$BIN" start --insecure --store="$HOME/fgdb-lab/node1" --listen-addr=localhost:26257 --http-addr=localhost:8080 --join=localhost:26257,localhost:26258,localhost:26259 --background --pid-file="$HOME/fgdb-lab/node1.pid"
"$BIN" start --insecure --store="$HOME/fgdb-lab/node2" --listen-addr=localhost:26258 --http-addr=localhost:8081 --join=localhost:26257,localhost:26258,localhost:26259 --background --pid-file="$HOME/fgdb-lab/node2.pid"
"$BIN" start --insecure --store="$HOME/fgdb-lab/node3" --listen-addr=localhost:26259 --http-addr=localhost:8082 --join=localhost:26257,localhost:26258,localhost:26259 --background --pid-file="$HOME/fgdb-lab/node3.pid"
"$BIN" init --insecure --host=localhost:26257
"$BIN" node status --insecure --host=localhost:26257
```

Initialize only once. Three processes on one computer still share its failure risks. For replication settings and a cross-node example, see [getting-started.md](getting-started.md#4-run-three-nodes-on-one-computer).

## Connect

Open the built-in SQL shell, setting `BIN` again if needed:

```bash
"$BIN" sql --insecure --host=localhost:26257
```

A PostgreSQL client can use the wire protocol. For example, for the insecure local lab:

```bash
psql 'postgresql://root@localhost:26257/defaultdb?sslmode=disable'
```

Use certificates and a secured connection outside the lab; see [operations.md](operations.md). Exit either interactive shell with `\q`.

## Basic SQL

Run these statements in the SQL shell:

```sql
CREATE DATABASE demo;
CREATE TABLE demo.sensors (id INT PRIMARY KEY, name STRING NOT NULL, reading FLOAT);
INSERT INTO demo.sensors VALUES (1, 'temperature', 21.5);
SELECT * FROM demo.sensors;
UPDATE demo.sensors SET reading = 22.0 WHERE id = 1;
DELETE FROM demo.sensors WHERE id = 1;
```

Group related operations in a transaction:

```sql
BEGIN;
INSERT INTO demo.sensors VALUES (2, 'temperature', 22.0);
UPDATE demo.sensors SET reading = 22.5 WHERE id = 2;
COMMIT;
```

Use `ROLLBACK` to abandon a transaction. Concurrent transactions can return a serialization error (SQLSTATE `40001`): applications should retry the **entire transaction**, including its reads, with bounded retries and backoff. Keep external side effects outside the retried transaction or make them safe to repeat.

## Common tasks

### Users, roles, and grants

In a secured cluster, an administrator can create a user and role, grant the role access, and assign it to the user:

```sql
CREATE USER reader;
CREATE ROLE sensor_readers;
GRANT SELECT ON TABLE demo.sensors TO sensor_readers;
GRANT sensor_readers TO reader;
```

Configure authentication for the user before connecting; see [operations.md](operations.md).

### Indexes and query plans

```sql
CREATE INDEX sensors_name_idx ON demo.sensors (name);
EXPLAIN SELECT * FROM demo.sensors WHERE name = 'temperature';
```

Use the plan to inspect how a query accesses data, and measure the effect of an index on your workload.

### Load CSV

For a small local CSV, a PostgreSQL client can stream rows using `COPY FROM STDIN` (for example, `psql`'s `\copy` command). Match the file's column order to the table:

```text
\copy demo.sensors (id, name, reading) FROM 'sensors.csv' WITH (FORMAT csv, HEADER true)
```

The OSS capability suite also checks CSV `IMPORT` using an HTTP fixture reachable by the database node; see [oss-capabilities.md](oss-capabilities.md). For larger loads, ensure nodes can reach the source and consult the applicable import syntax for this version. The planned workload loader below is separate from these existing paths.

### Change a schema

```sql
ALTER TABLE demo.sensors ADD COLUMN unit STRING;
```

Schema changes may run as jobs; inspect them with `SHOW JOBS`. Plan changes around application compatibility and wait for completion before depending on the new schema.

### Stop nodes

For the local examples, send SIGTERM using each node's pid file and wait for the process to exit:

```bash
kill "$(cat "$HOME/fgdb-lab/node1.pid")"
```

Repeat for the other nodes when ending the three-node demo. Keep the store directories to retain data. For restart and operational considerations, see [operations.md](operations.md).

## FGDb features

`fgdb-backup` is the supported interim logical backup and restore tool. It copies schema and rows through ordinary SQL to local storage or S3. Read [backup-cli.md](backup-cli.md) for the current commands and restore behavior and [backup.md](backup.md) for format and limitations; test restores before relying on them.

The supported initial OSS capability suite checks binary identity, CSV import and an OSS guard, and the separate replication ingest hook regression. These are checks of a defined slice, not comprehensive coverage; see [oss-capabilities.md](oss-capabilities.md) and [testing.md](testing.md).

The following open PRs are **experimental and may change**:

- [#32](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/32) proposes additional guards for TTL, GEOS, READ COMMITTED, wire-protocol COPY and import formats. TTL is opt-in and its deletion timing remains an open question.
- [#31](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/31) proposes expanded OSS gate contract checks. [#38](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/38) proposes stricter default build and artifact checks; full build validation remains outstanding.
- [#42](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/42) proposes EXPORT capability checks, with parity evidence still pending.
- [#43](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/43) proposes AES-CBC raw pgcrypto encrypt/decrypt controlled by the `feature.pgcrypto_ciphers.enabled` cluster setting, which defaults to on; set it to false to return the CCL-required error. See the PR for the current scope; compatibility evidence remains pending.

[#41](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/41) plans a workload IMPORT loader; it is not an implementation. [#33](https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/pull/33) proposes a version-gating and feature-flag contract, pending approval and decisions; it does not wire settings into the server.

## Keeping this chart current

When a feature lands, verify its behavior and limitations against main and the relevant release, then update its status and link to the maintained documentation. Keep an open PR tentative until it merges; describe unresolved validation and default-off gates. Mark plans as planned until implementation lands, and add new rows as FGDb-specific capabilities are delivered or proposed. Update examples only when their commands are verified for the shipped version.
