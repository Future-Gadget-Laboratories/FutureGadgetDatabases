> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Getting started

This page gets a CCL-free binary running on one computer, then on three local processes. **CCL** is the CockroachDB Community License. The binary that includes CCL code is not the one this project ships.

You will finish with a table called `lab.sensors` and three processes that copy data to each other.

A production install needs certificates, disk planning, and monitoring. Those topics start in [operations.md](operations.md). This page uses `--insecure` on purpose. That flag turns off encryption and login. Use it only on your own computer. Do not publish port 26257 or port 8080 to a network you do not control.

## 1. Get a binary

The published file, once the GitHub Release exists, is Linux amd64:

```text
https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/cockroach-oss-v23.2.15.linux-amd64.tgz
```

Check whether the release is there:

```bash
gh release view v23.2.15-oss \
  --repo Future-Gadget-Laboratories/FutureGadgetDatabases
```

If that command fails, the release is not published yet. Build from source in the next section. The URL above is the name the release workflow will use. It is not a claim that the file is already uploaded.

### From the release tarball

```bash
mkdir -p "$HOME/fgdb-lab" && cd "$HOME/fgdb-lab"
curl -fL -O https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/cockroach-oss-v23.2.15.linux-amd64.tgz
curl -fL -O https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/releases/download/v23.2.15-oss/SHA256SUMS
sha256sum -c SHA256SUMS
tar -xzf cockroach-oss-v23.2.15.linux-amd64.tgz
export BIN="$PWD/cockroach-oss-v23.2.15.linux-amd64/cockroach"
"$BIN" version
```

`sha256sum -c` checks the file against `SHA256SUMS`. The sums file uses the two-space format that `sha256sum` expects. The file inside the tarball is named `cockroach`. It is the `cockroach-oss` binary. `lib/libgeos.so` and `lib/libgeos_c.so` sit next to it. GEOS is the library used for spatial SQL. The binary looks for those files under a `lib/` directory beside itself.

The `Distribution` line of `version` should say `OSS`.

### From source

A cold build needs a lot of machine. Plan on about 32 GB of RAM, well over 80 GB of free disk, and a few hours the first time. Later builds are faster if the Bazel cache is still there. **Bazel** is the build tool this tree uses. You do not install the Go compiler yourself. Bazel downloads Go 1.21.12.

Install Bazelisk 1.29.0 or newer and put it on your `PATH` as `bazel`. The file `.bazelversion` says `cockroachdb/6.2.1`. Bazelisk 1.10.1, which the old `build/bootstrap/bootstrap-debian.sh` script installs, cannot resolve that version. Bazelisk 1.29.0 can.

`./dev` refuses to run as root. Do not install `ccache`. `./dev` refuses that too.

From the repository root:

```bash
./dev doctor
./dev build oss geos
./artifacts/cockroach-oss version
export BIN="$PWD/artifacts/cockroach-oss"
```

`./dev doctor` checks the machine and writes a local Bazel config. `oss` selects `//pkg/cmd/cockroach-oss:cockroach-oss`. `geos` builds the spatial libraries. `make buildoss` is only `./dev build oss`. It does not build GEOS. Use `oss geos` if you want the same pieces the release has.

`./dev build` with no `oss` argument builds `//pkg/cmd/cockroach`, which links CCL code. Do not ship that binary.

The libraries from a local `geos` build land in `artifacts/`, not in `artifacts/lib/`. Normal tables work without them. If you want spatial SQL from a local build, start the node with `--spatial-libs` pointed at the directory that contains `libgeos.so` and `libgeos_c.so`.

Publishing the Linux tarball and the container image is a separate procedure. Maintainers follow [RELEASING.md](RELEASING.md). You do not need that path for a laptop lab.

## 2. Start one node

A **single-node** cluster stores one copy of each range. If that process stops, the database stops. That is the right shape for a first lab. It is the wrong shape for data you need to keep available.

Keep using the `BIN` path from the step above. If you opened a new terminal, set `BIN` again.

```bash
mkdir -p "$HOME/fgdb-lab"
"$BIN" start-single-node \
  --insecure \
  --store="$HOME/fgdb-lab/lab-single" \
  --listen-addr=localhost:26257 \
  --http-addr=localhost:8080 \
  --pid-file="$HOME/fgdb-lab/lab-single.pid"
```

What those flags mean:

| Flag | Meaning |
| --- | --- |
| `--insecure` | No TLS and no password. Laptop only. |
| `--store` | Directory for this node's data and logs. Logs are in `<store>/logs/`. |
| `--listen-addr` | SQL and internal RPC address. The default port is 26257. |
| `--http-addr` | DB Console, the admin web UI. Open <http://localhost:8080>. |
| `--pid-file` | Writes the process id so you can stop the node later. |

`start-single-node` initializes the cluster for you and sets the replication factor to 1. **Replication factor** is how many copies of a range the cluster tries to keep. One copy means no failover.

Leave that terminal running. In a second terminal, set `BIN` to the same binary and open the SQL shell:

```bash
"$BIN" sql --insecure --host=localhost:26257
```

## 3. Create a small schema

Paste this into the SQL shell. `STRING` is this database's name for text. `FLOAT` is a double-precision number. `gen_random_uuid()` creates an id. `TIMESTAMPTZ` is a timestamp with a time zone.

```sql
CREATE DATABASE lab;
CREATE TABLE lab.sensors (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name STRING NOT NULL,
  reading FLOAT,
  seen_at TIMESTAMPTZ DEFAULT now()
);

INSERT INTO lab.sensors (name, reading) VALUES ('bench-temp', 21.5);
INSERT INTO lab.sensors (name, reading) VALUES ('bench-temp', 21.8);

SELECT name, reading, seen_at
FROM lab.sensors
ORDER BY seen_at;

UPDATE lab.sensors
SET reading = 22.0
WHERE name = 'bench-temp'
  AND reading = 21.5;

SELECT id, name, reading FROM lab.sensors ORDER BY reading;
```

You should see two rows, then one of them updated to `22`. Quit the shell with `\q`.

Turn telemetry off before you treat even a lab cluster as something you will keep. The cluster setting changes are in [operations.md](operations.md). For this first session you can run them now:

```bash
"$BIN" sql --insecure --host=localhost:26257 -e \
  "SET CLUSTER SETTING diagnostics.reporting.enabled = false; SET CLUSTER SETTING diagnostics.reporting.send_crash_reports.enabled = false;"
```

`SET CLUSTER SETTING` changes a value stored by the cluster, not only by your session.

## 4. Run three nodes on one computer

Three processes on one laptop teach the commands. They do not protect you from a dead disk or a power loss. All three stores share the same machine.

Stop the single-node process first. It is using port 26257.

```bash
kill "$(cat "$HOME/fgdb-lab/lab-single.pid")"
```

`kill` with no signal sends `SIGTERM`. This database treats that as a request to shut down. Wait until the process is gone (`ps` no longer shows it). Leave `lab-single/` on disk if you want that data later. The three-node lab below uses different directories.

Open three terminals, or use `--background`. `--background` waits until the node is ready, then returns you to the shell. It is available on Linux and macOS. On Windows, start each node in its own terminal and leave `--background` off. Set `BIN` in each terminal.

```bash
mkdir -p "$HOME/fgdb-lab"

"$BIN" start \
  --insecure \
  --store="$HOME/fgdb-lab/node1" \
  --listen-addr=localhost:26257 \
  --http-addr=localhost:8080 \
  --join=localhost:26257,localhost:26258,localhost:26259 \
  --background \
  --pid-file="$HOME/fgdb-lab/node1.pid"

"$BIN" start \
  --insecure \
  --store="$HOME/fgdb-lab/node2" \
  --listen-addr=localhost:26258 \
  --http-addr=localhost:8081 \
  --join=localhost:26257,localhost:26258,localhost:26259 \
  --background \
  --pid-file="$HOME/fgdb-lab/node2.pid"

"$BIN" start \
  --insecure \
  --store="$HOME/fgdb-lab/node3" \
  --listen-addr=localhost:26259 \
  --http-addr=localhost:8082 \
  --join=localhost:26257,localhost:26258,localhost:26259 \
  --background \
  --pid-file="$HOME/fgdb-lab/node3.pid"
```

`--join` lists the nodes that form the cluster. The other nodes do not have to be up before the first one starts. `start` does not initialize the cluster by itself. `start-single-node` does. If you pass `--join` to `start-single-node`, the command refuses. The error text says to use `start` instead.

Initialize once:

```bash
"$BIN" init --insecure --host=localhost:26257
"$BIN" node status --insecure --host=localhost:26257
```

`node status` prints `is_live` and `is_available` for each node. You want three rows, all live.

A second `init` fails because the cluster is already initialized. Keep the store directories. Do not delete them and start over unless you mean to throw away the data.

### Replication factor on a 3-node lab

User data follows the `default` zone. A **zone configuration** is the cluster's rule for how many copies to keep and where they may live. The default zone asks for 3 copies (`num_replicas = 3`).

Some internal ranges start at 5 copies. On a 3-node cluster those ranges cannot reach 5. `node status --ranges` may show under-replicated ranges. SQL still works. For this lab, set the internal ranges to 3:

```bash
"$BIN" sql --insecure --host=localhost:26257 <<'SQL'
ALTER RANGE meta CONFIGURE ZONE USING num_replicas = 3;
ALTER RANGE liveness CONFIGURE ZONE USING num_replicas = 3;
ALTER RANGE system CONFIGURE ZONE USING num_replicas = 3;
ALTER DATABASE system CONFIGURE ZONE USING num_replicas = 3;
SHOW ZONE CONFIGURATION FROM RANGE default;
SQL
```

The `default` row should show `num_replicas = 3`. Your application tables inherit that unless you set a zone on the table.

### Write through one node and read through another

```bash
"$BIN" sql --insecure --host=localhost:26257 -e \
  "CREATE DATABASE lab; CREATE TABLE lab.sensors (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name STRING NOT NULL, reading FLOAT, seen_at TIMESTAMPTZ DEFAULT now()); INSERT INTO lab.sensors (name, reading) VALUES ('bench-temp', 21.5);"

"$BIN" sql --insecure --host=localhost:26258 -e \
  "SELECT name, reading FROM lab.sensors;"
```

The second command talks to node 2. The row was written through node 1. You should still see `bench-temp`.

If `CREATE DATABASE lab` says the database already exists, you pointed `--store` at a directory from an earlier try, or you are still talking to the single-node cluster. Check `node status`. A three-node cluster has three ids.

## 5. When a command fails

| What you see | What it usually means |
| --- | --- |
| Connection refused | The process is not listening on that port. Check the pid file and `<store>/logs/`. |
| The store directory is in use | Another process still has that `--store`. Stop it. Do not delete the directory unless you intend to destroy the data. |
| `cannot use --join` with `start-single-node` | Use `start` plus `init` for more than one node. |
| Port already in use | The single-node lab is still on 26257, or you started two nodes with the same `--listen-addr`. |
| `version` does not say `Distribution: OSS` | Rebuild with `./dev build oss`. |

## 6. What to try next

Kill one of the three nodes and start it again. The walkthrough is in [operations.md](operations.md). Read that page before you put anything you care about on this cluster.

Certificates, telemetry, and the license enforcer are on that page too.

A short map of ranges and majorities is in [architecture.md](architecture.md).
