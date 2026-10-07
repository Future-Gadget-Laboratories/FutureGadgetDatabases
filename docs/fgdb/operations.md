> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Operations notes

This page is the first set of operator habits for the v23.2.15 `cockroach-oss` line. It is not a full runbook. Backup schedules, multi-region layouts, and production monitoring are follow-up work.

The examples use `"$BIN"` for the binary. Set it to your local build or to the `cockroach` file from the release tarball. See [getting-started.md](getting-started.md).

## Telemetry

**Telemetry** here means diagnostics this binary can send to Cockroach Labs: cluster metrics, and crash reports.

In this version, `diagnostics.reporting.enabled` is stored as `false` at first. When a cluster is created, a migration usually sets it to `true`. You can block that migration with an environment variable. Set it on **every** node before the first start:

```bash
export COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=true
```

Save this as `$HOME/fgdb-lab/fgdb-node.env`:

```bash
# Blocks the cluster-creation migration that turns diagnostics on.
COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=true
```

The license enforcer switch is not in that file. Setting `COCKROACH_ENABLE_LICENSE_ENFORCER=true` turns the enforcer on. This line leaves it unset.

```bash
set -a
source "$HOME/fgdb-lab/fgdb-node.env"
set +a
"$BIN" start-single-node --insecure --store="$HOME/fgdb-lab/lab-single" \
  --listen-addr=localhost:26257 --http-addr=localhost:8080
```

`set -a` exports every assignment in the file. If the cluster already exists, the env var does not rewrite the saved setting. Turn the settings off in SQL as well:

```bash
"$BIN" sql --insecure --host=localhost:26257 <<'SQL'
SET CLUSTER SETTING diagnostics.reporting.enabled = false;
SET CLUSTER SETTING diagnostics.reporting.send_crash_reports.enabled = false;
SHOW CLUSTER SETTING diagnostics.reporting.enabled;
SHOW CLUSTER SETTING diagnostics.reporting.send_crash_reports.enabled;
SQL
```

Both `SHOW` results should say `false`.

Crash reports are a separate setting. The env var above covers the migration for `diagnostics.reporting.enabled`. Set the crash-report setting in SQL even when you used the env var.

The release image sets `COCKROACH_CHANNEL=fgl-oss`. That string identifies the build channel. It does not turn telemetry off. You still set the variables and cluster settings above.

## License enforcer

The **license enforcer** is code that can limit a cluster when a license check fails. In this tree it is opt-in. It stays off unless `COCKROACH_ENABLE_LICENSE_ENFORCER=true`. The comment in `pkg/server/license/enforcer.go` says the enforcer is currently opt-in.

This line does not use a Cockroach Labs license key. Do not set `COCKROACH_ENABLE_LICENSE_ENFORCER`. Later CockroachDB patches turn an enforcer on by default. Those patches are under the CSL, and this project does not ship them.

## Certificates

`--insecure` is for the laptop lab. On any other network, create a small CA (certificate authority) and node certificates.

A **CA** is the key that signs the certificates the nodes and clients show each other. Keep the CA key off the machines that only need to run the database. The example below puts it in `my-safe-directory/`. The nodes receive the `certs/` directory, which holds certificates and the node key, not the CA key.

Run this once, on the machine where you will keep the CA key. The binary can be `cockroach-oss`:

```bash
mkdir -p "$HOME/fgdb-lab/certs" "$HOME/fgdb-lab/my-safe-directory"

"$BIN" cert create-ca \
  --certs-dir="$HOME/fgdb-lab/certs" \
  --ca-key="$HOME/fgdb-lab/my-safe-directory/ca.key"

"$BIN" cert create-node localhost \
  --certs-dir="$HOME/fgdb-lab/certs" \
  --ca-key="$HOME/fgdb-lab/my-safe-directory/ca.key"

"$BIN" cert create-client root \
  --certs-dir="$HOME/fgdb-lab/certs" \
  --ca-key="$HOME/fgdb-lab/my-safe-directory/ca.key"
```

`create-node` needs every name a client will use to reach the node. `localhost` matches the lab. A second machine needs its own DNS name or IP added to that node's certificate. One shared `node.crt` only works when every name on it is listed.

Start without `--insecure`:

```bash
"$BIN" start-single-node \
  --certs-dir="$HOME/fgdb-lab/certs" \
  --store="$HOME/fgdb-lab/lab-secure" \
  --listen-addr=localhost:26257 \
  --http-addr=localhost:8080
```

Use a new `--store`. The laptop lab above was started with `--insecure`. This example is a separate cluster, so it gets its own directory. Point the SQL client at this store's process, with the same `--certs-dir`:

```bash
"$BIN" sql --certs-dir="$HOME/fgdb-lab/certs" --host=localhost:26257
```

The DB Console will ask for a client certificate. The `root` client certificate created above is the one the built-in SQL shell uses. Creating extra SQL users and their client certificates is the next step when more than one person connects. Cockroach Labs' v23.2 page for that flow is [Secure a Cluster](https://www.cockroachlabs.com/docs/v23.2/secure-a-cluster.html).

## Crash drill on the three-node lab

Do this on the cluster from [getting-started.md](getting-started.md), after the zone updates that set internal ranges to 3 copies. Use `--insecure` only because that lab did.

Write a row you can recognize:

```bash
"$BIN" sql --insecure --host=localhost:26257 -e \
  "INSERT INTO lab.sensors (name, reading) VALUES ('before-crash', 1);"
```

Stop node 3 the hard way. `SIGKILL` does not let the process shut down cleanly. That is the point of the drill.

```bash
kill -9 "$(cat "$HOME/fgdb-lab/node3.pid")"
"$BIN" node status --insecure --host=localhost:26257
```

One node should show `is_live` false. Then write and read through a node that is still up:

```bash
"$BIN" sql --insecure --host=localhost:26257 -e \
  "INSERT INTO lab.sensors (name, reading) VALUES ('during-outage', 2);"

"$BIN" sql --insecure --host=localhost:26258 -e \
  "SELECT name, reading FROM lab.sensors ORDER BY name;"
```

You should see `before-crash` and `during-outage`. With 3 copies, 2 live nodes are a majority. The range can still commit.

Start node 3 with the **same** store path and the **same** flags. Do not delete `$HOME/fgdb-lab/node3`.

```bash
"$BIN" start \
  --insecure \
  --store="$HOME/fgdb-lab/node3" \
  --listen-addr=localhost:26259 \
  --http-addr=localhost:8082 \
  --join=localhost:26257,localhost:26258,localhost:26259 \
  --background \
  --pid-file="$HOME/fgdb-lab/node3.pid"

"$BIN" node status --insecure --host=localhost:26257
"$BIN" sql --insecure --host=localhost:26259 -e \
  "SELECT name, reading FROM lab.sensors WHERE name IN ('before-crash', 'during-outage') ORDER BY name;"
```

All three nodes should be live again. Node 3 should return both rows. It caught up from the other copies.

If you `kill -9` two nodes, one copy is not a majority. Writes to ranges that lost quorum fail or wait until a second node is back. Start one of the dead nodes with its original store. Do not format the disk.

### Graceful stop

`kill` without `-9` sends `SIGTERM`. This process treats that as a shutdown. Use it when you mean to stop a node on purpose:

```bash
kill "$(cat "$HOME/fgdb-lab/node3.pid")"
```

Start it again with the same command as above when you want it back.

## Decommission is a different action

**Decommission** means "remove this node for good and move its copies somewhere else." It is not the crash drill.

On a 3-node cluster with 3 copies, the cluster cannot place 3 copies on the 2 nodes that would remain. `node decommission` will not finish in that state.

Practice decommission only after you have a place for the copies to go. That means a fourth node already in the cluster, or a conscious choice to lower `num_replicas` first. Lowering the replication factor removes the failure protection the copies were giving you. A full decommission runbook is later work.

The command, once that precondition is true, is:

```bash
"$BIN" node decommission 3 --insecure --host=localhost:26257
```

The `3` is the node id from `node status`, which may or may not be the process you called node 3. Read the id column. The command moves leases and replicas off that node. When it finishes, stop the process. Keep the store directory until you have confirmed the cluster is healthy, then archive or delete it as your own policy says.

## Logs and the admin UI

Each `--store` directory has a `logs/` folder. Start there when a node will not boot.

DB Console ports in the three-node lab:

| Node | UI |
| --- | --- |
| node 1 | <http://localhost:8080> |
| node 2 | <http://localhost:8081> |
| node 3 | <http://localhost:8082> |

With `--insecure`, the UI has no login. Do not expose it.

## A small config you can keep next to the lab

Save this as `$HOME/fgdb-lab/three-node.md` only if you want a reminder. The commands themselves are above. The values that matter when you come back tomorrow:

```text
binary: cockroach-oss (Distribution: OSS)
listen: localhost:26257, localhost:26258, localhost:26259
http:   localhost:8080, localhost:8081, localhost:8082
join:   localhost:26257,localhost:26258,localhost:26259
stores: $HOME/fgdb-lab/node1  node2  node3
env:    COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=true
off:    COCKROACH_ENABLE_LICENSE_ENFORCER  (do not set)
sql:    diagnostics.reporting.enabled = false
        diagnostics.reporting.send_crash_reports.enabled = false
```
