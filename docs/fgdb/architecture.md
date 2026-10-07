> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Architecture

This is a working picture of FutureGadgetDatabases at the v23.2.15 pin. It is enough to understand the labs in [getting-started.md](getting-started.md) and the crash drill in [operations.md](operations.md).

Cockroach Labs wrote the long version for this code: [Architecture overview (v23.2)](https://www.cockroachlabs.com/docs/v23.2/architecture/overview.html). The historical design note in this tree is [../design.md](../design.md). That note still mentions RocksDB. This tree stores bytes with **Pebble**, an on-disk key-value engine. When the design note and this page disagree about the storage engine, trust this page.

## The pieces

```text
SQL client (psql, a driver, or `cockroach sql`)
        |
        |  PostgreSQL wire protocol, port 26257
        v
the node you connected to  (the SQL gateway)
        |
        |  turns SQL into reads and writes on keys
        v
ranges  (slices of one sorted map of keys)
        |
        |  each range has several copies
        v
Raft group for that range  (a majority of copies must agree)
        |
        v
Pebble on each node's disk  (the --store directory)
```

A **SQL gateway** is any node that accepted your connection. It plans the query and contacts the nodes that hold the rows. You can point a client at any live node. The three-node lab reads a row from node 2 that was inserted through node 1 for this reason.

A **range** is a contiguous slice of the key space. The default zone tries to keep a range between 128 MiB and 512 MiB. The cluster splits and merges ranges as tables grow and shrink. You do not assign rows to nodes by hand.

Each range is copied to several nodes. The number of copies is the **replication factor**, `num_replicas` in the zone configuration. The default for your tables is 3. `start-single-node` sets the cluster up with one copy, because there is only one place to put data.

The copies of one range use **Raft** to agree on the order of writes. Raft is a way for a small group to keep the same log. A write is committed when a **majority** of the copies have it. With 3 copies, 2 form a majority. With 1 copy, that single copy is the only vote.

One copy holds the **lease**. That copy serves reads and coordinates writes for the range. If it dies, another copy takes the lease, as long as a majority of the group is still up.

## What a crash does

Use the three-node lab, with `num_replicas = 3`.

| Event | What the range can do |
| --- | --- |
| 1 node stops | 2 copies are left. They are a majority. Reads and writes continue. The stopped node catches up when it starts again, if you keep its store directory. |
| 2 nodes stop | 1 copy is left. That is not a majority. The range stops accepting writes until a second copy is back. |
| You delete a store directory | That disk is gone. Starting an empty directory does not bring the old copy back. The remaining nodes still have the data only if they held copies. |

A one-node cluster has no second copy. Stopping that process stops the database. The data is still in `--store` when you start the same command again.

Internal ranges named `meta`, `liveness`, and `system`, and the `system` database, start at 5 copies in this version. A 3-node lab cannot satisfy 5. Those ranges show up as under-replicated until you set them to 3. The SQL in [getting-started.md](getting-started.md) does that. User tables follow the `default` zone, which is already 3.

## What you can look at

From any live node:

```bash
"$BIN" node status --insecure --host=localhost:26257
"$BIN" node status --ranges --insecure --host=localhost:26257
```

`is_live` tells you the process is up. `--ranges` adds `ranges_unavailable` and `ranges_underreplicated`.

In SQL:

```sql
SHOW ZONE CONFIGURATION FROM RANGE default;
SHOW RANGES FROM TABLE lab.sensors;
```

Run the second statement after [getting-started.md](getting-started.md) has created `lab.sensors`. It lists the ranges that hold that table and which nodes have copies. On a brand-new table you may see a single range. That is normal.

The DB Console at <http://localhost:8080> (or 8081 and 8082 for the other local nodes) shows the same cluster. The open-source binary serves the open-source UI. CCL-only screens are not in this build.

## What this page leaves out

Multi-region survival, backup files, and admission control have upstream write-ups that are still useful at v23.2:

- [CockroachDB v23.2 architecture overview](https://www.cockroachlabs.com/docs/v23.2/architecture/overview.html)
- Tech notes already in this tree, under [../tech-notes/](../tech-notes/README.md)

Those notes were written by Cockroach Labs. Read them as upstream engineering notes for this pin. The "stable" manual on cockroachlabs.com describes newer releases. This project does not ship those releases.
