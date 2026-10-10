> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Logical backup and restore

`fgdb-backup` copies user databases with ordinary SQL. The open-source binary rejects `BACKUP` and `RESTORE` because those statements need a CCL binary. This program does not implement those statements. It reads schema and rows with `SHOW`, `COPY`, and `AS OF SYSTEM TIME`, writes files, and loads them back with `IMPORT INTO` or `COPY FROM STDIN`.

The program is a separate binary, `fgdb-backup`. It is built into the release tarball and the container image, next to `cockroach`. A Kubernetes Job or a shell script can call it. Two stable shapes are:

```bash
fgdb-backup backup  --url "$URL" --dest s3://bucket/prefix [--database name]
fgdb-backup restore --url "$URL" --src  s3://bucket/prefix/name/latest
```

`--json` prints one JSON object on stdout. Progress lines go to stderr. Exit status is 0 when the command finishes, 1 when it starts and then fails, and 2 when the command line is wrong. Exit 4 is only `restore --plan` refusing the restore. A real restore that is refused exits 1. The full list of flags, YAML keys, and exit cases is in [backup-config.md](backup-config.md). Only `backup` and `restore` read `--config`.

`--url` is a PostgreSQL connection URL. Certificates go in that URL:

```text
postgresql://root@db.example:26257/defaultdb?sslmode=verify-full&sslrootcert=/certs/ca.crt&sslcert=/certs/client.root.crt&sslkey=/certs/client.root.key
```

S3 uses the normal AWS credential chain (environment variables, a shared config file, or an IAM role). Secret keys are not written into the backup, and they are not put in the `IMPORT` statement. `--s3-import-auth` accepts `auto`, `implicit`, `specified`, and `served`. `auto` is the default. `auto`, `specified`, and `served` all do the same thing: this tool reads S3, and the database fetches the bytes from the file server. That is true even when `AWS_ACCESS_KEY_ID` is unset. Only `implicit` makes the database read `s3://` itself (`AUTH=implicit`), which is what you want when the node has an IAM role. Job records and logs then see an `http://` or `https://` URL, not the keys.

An S3 URL needs a region even with `--s3-endpoint`. Set `AWS_REGION` or pass `--s3-region`. A custom endpoint uses path-style addresses. `backup` and `unlock` also put a short-lived probe object (twice) to see if the endpoint honors `If-None-Match`, then delete it. That probe does not send server-side encryption headers. `restore`, `verify`, and `list` do not write it, so they can use read-only credentials. Details are in [backup-config.md](backup-config.md).

## What a backup contains

Format version 1. Paths look like this:

```text
<dest>/<name>/<UTC timestamp>/manifest.json
<dest>/<name>/<UTC timestamp>/objects.json
<dest>/<name>/<UTC timestamp>/schema/<database>.sql
<dest>/<name>/<UTC timestamp>/users.sql
<dest>/<name>/<UTC timestamp>/zones.sql
<dest>/<name>/<UTC timestamp>/data/<database>/<schema>/<table>.pgcopy.gz
<dest>/<name>/latest.json
```

`<name>` defaults to the cluster id. Pass `--name` to choose a folder. The timestamp is UTC, with milliseconds and 8 hex characters, like `20261015T143012.000Z-a1b2c3d4`.

`manifest.json` records the source `version()`, the cluster id, the `AS OF SYSTEM TIME` timestamp, per-table row counts, and the sha256 of each stored file. `data_format` is `pgcopy`. `objects.json` is what restore executes. The `.sql` files are the same statements in a form you can read.

Table files are PostgreSQL text `COPY` output, usually gzip-compressed. NULL is the two characters `\N`. A real string that is those two characters is stored as `\\N`, and an empty string is an empty field, so the three stay distinct. Array columns are written as SQL `ARRAY[...]::type` literals, because `IMPORT INTO ... PGCOPY` parses that form and rejects the `{a,b}` text that `COPY TO` emits for arrays. The manifest records which columns are arrays. `--load=copy` rewrites those fields to `{...}` text as it streams, because `COPY FROM STDIN` rejects the `ARRAY` literal. A string column that happens to contain that text is not rewritten. `verify` checks checksums before it trusts `objects.json`, in the same order the files were written, and recounts rows from that text. It does not restore.

```bash
fgdb-backup verify --json --src s3://bucket/prefix/name/latest
fgdb-backup list   --json --src s3://bucket/prefix/name
```

### Compression and encryption

Data files are gzip-compressed unless you pass `--compression=none`. Gzip is the default because `IMPORT INTO` on this binary can decompress gzip. It cannot decompress zstd. zstd is a later change.

Server-side encryption is optional:

```bash
--sse=AES256
--sse=aws:kms --sse-kms-key-id <key id or ARN>
```

Those flags set S3 server-side encryption on `PutObject` and on multipart upload. Client-side encryption (encrypting the bytes before they leave the tool) is not implemented. Doing it safely means a key that restore can find, a unique nonce per file, and a way to tell a truncated file from a bad key. That is a follow-up, not a flag in this version.

### Finding the newest complete backup

`latest.json` is written only after `manifest.json` is in place, and only if this backup's timestamp is newer than the pointer already there. An older backup that finishes later does not replace a newer one. `restore --src .../latest` reads that pointer. A directory with data files but no manifest is unfinished. `list` does not return it, and it is never what `latest` points at. A backup that hits an error, including a killed backup, does not publish `latest`.

Each file is written under a temporary name. It is published only after the `COPY` succeeds and the checksum covers those bytes. If the `COPY` fails, or the process is stopped with SIGINT or SIGTERM during the upload, the tool deletes the temporary file, aborts an S3 multipart upload, and deletes any short object it already put. The abort uses a context that is still active after the signal cancels the backup, so the uploaded parts are not left in the bucket. A half-written table is not left under its final name.

### Large tables

Rows are streamed. The process does not load a whole table into memory. S3 uploads use multipart upload. The part size defaults to 8 MiB and must be at least 5 MiB (`--part-size`). S3 allows 10,000 parts per object. Before the next part would pass that limit, the tool aborts the upload and exits with an error that names the limit and the part size. Raise `--part-size`, or pass `--split-rows` so each file stays under `part size × 10000` bytes.

`--split-rows=N` splits a table into primary-key ranges of about N rows, all read at the same timestamp. The next boundary is `SELECT src.pk::STRING FROM table AS src WHERE src.pk >= previous ORDER BY src.pk OFFSET N`. The sort is on the key column. Sorting the text of the key would order 1, 10, 100 and the files would not be about N rows. Each call starts at the previous bound, so the scan stays proportional to the page. This only works for a table whose primary key is one integer column (`INT2`, `INT4`, or `INT8`). Other tables are one stream, and the tool says so on stderr. The files are named `<table>.part0001.pgcopy.gz`, and one `IMPORT` loads every part.

The backup JSON includes `peak_rss_bytes`, the high-water resident set size of the tool process (`VmHWM` on Linux).

## Snapshot age and gc.ttlseconds

Every read uses one timestamp: `AS OF SYSTEM TIME '<timestamp>'`. CockroachDB can serve that timestamp only while it is newer than the garbage-collection threshold. The zone setting `gc.ttlseconds` is that window. On a new 23.2 cluster the range default is 14400 seconds (4 hours). A database or table can set a shorter value. A backup that is still running when the window closes fails, and the files are not marked latest.

Before copying rows, the tool reads `crdb_internal.zones`, takes the smallest `gc.ttlseconds` that applies to the databases in this run, and refuses to start if `now + safety margin` is not strictly before `timestamp + gc.ttlseconds`. The default safety margin is one minute (`--safety-margin`). The error tells you the deadline and how to raise the TTL. The same check runs again before each table. If a read fails with a GC-threshold error, the tool exits non-zero with the same explanation.

To give a long backup more time, raise the TTL for the window and put it back when you are done:

```sql
ALTER DATABASE app CONFIGURE ZONE USING gc.ttlseconds = 86400;
```

`--extend-gc-ttl=12h` does that for the databases in this run, prints the statements that put the old values back, and runs those statements when the process exits, including when it is stopped with SIGINT or SIGTERM. The exit status is non-zero, and `latest` is not updated. Use a Go duration (`12h`, `30m`). `1d` is not accepted. If the process is killed with `kill -9`, the higher value stays until you run the printed statements. If the backup files are written but the old value cannot be put back, the process exits non-zero and prints the statement to run. The backup itself is still valid when `manifest.json` was written. The files record the zone configs from before the temporary raise, not the raised value.

## What is restored, and in what order

1. Databases.
2. Schemas, types, and sequences. A sequence that only exists to back an identity column is not created twice; the `CREATE TABLE` creates it.
3. Tables and indexes. Foreign keys are not created yet.
4. Functions and procedures. If one cannot be replayed, restore stops and names that object.
5. Row data (`IMPORT INTO` of PGCOPY by default, or `--load=copy`).
6. Views and materialized views, in the order they were created. A plain view that reads a materialized view is created after that materialized view, and both are created after the tables have their rows. Cockroach's `SHOW CREATE` adds a hidden `rowid` column to a materialized view that cannot be replayed; the tool removes that column when the query does not select `rowid`.
7. Foreign keys and `VALIDATE CONSTRAINT`.
8. Sequence values. A sequence that has never been called is restored with `setval(start, false)`. A sequence that has been called is restored with `setval(last_value, true)`.
9. Users, role memberships, and grants. Failures here are warnings.
10. Zone configurations. Failures here are warnings. A zone is applied only when its database is one of the databases being restored, matched by name, not by a substring.

`system` and `postgres` are skipped. Other databases, including `defaultdb`, are included unless you pass `--database`. Schemas `pg_catalog`, `information_schema`, `crdb_internal`, and `pg_extension` are skipped.

Users and grants are best effort. Passwords are not in `SHOW USERS`, and this tool does not copy password hashes. `admin`, `root`, `node`, and `public` are not replayed. The target keeps its own privileges for those names. Create passwords again on the target if you need them.

Zone configs are read from `crdb_internal.zones` for the databases in the backup, including table zones and index or partition zones when the source has them. `RANGE default` is not rewritten on the target. On restore, each saved zone statement is executed. If 23.2 OSS rejects it, the tool prints a warning and continues. Index and partition zones are the usual case: this binary returns an error that enterprise features are required. The restore still succeeds. Database and table zones such as `gc.ttlseconds` and `num_replicas` are applied when the target accepts them.

Virtual computed columns and stored computed columns are not in the data files. The `CREATE TABLE` statement still has the expression, and the database recomputes stored values when the other columns are loaded. Ordinary columns are copied. `GENERATED BY DEFAULT AS IDENTITY` columns are copied, so the values stay the same. `GENERATED ALWAYS AS IDENTITY` cannot be loaded by `COPY` or `IMPORT` on this binary; backup stops before writing files and names the column.

Restore refuses when a target database already exists and has user objects (tables, views, sequences, types, functions, or procedures). It does not merge the backup into that database. That refusal exits 1. `--plan` reports the same refusal and exits 4, without changing the cluster.

`--force` does not drop the database in place. `--force`, `--force=true`, and `--force=swap` restore beside the existing database and swap names when the preflight allows it. The old database is renamed to `<name>__fgdb_old_` plus the UTC timestamp, a hyphen, and 8 hex characters. The dots are removed and the name is lower case. An example is `app__fgdb_old_20261015t143012000z-a1b2c3d4`. One old copy is kept by default (`--retention=1`). `--retention=0` keeps every old copy and does not prune. `prune --keep 0` is different: it drops every old copy. `prune --dry-run` prints those names and does not drop them.

Swap is refused when the backup itself contains a view, materialized view, function, or procedure, when the live database has any of those, when another database's view or routine names this database, or when a table outside the backup has a foreign key to a table in the backup. `--swap-restore=false` also refuses swap and tells you to use `--force=in-place`. YAML `restore.in_place: true` is read only when you do not pass `--force`, and a restore without `--force` still refuses a non-empty database. The in-place form is the flag `--force=in-place`. `--force in-place` with a space is a bad command line and exits 2.

`--force=in-place` drops and recreates only the objects that are in the backup. Before it drops anything, it looks for a foreign key from a table that is not in the backup to a table that is. If it finds one, it stops and leaves the database as it was. Views and materialized views are then dropped in reverse creation order, so a view that reads another view is dropped first. Those drops do not use `CASCADE`, so a table that is not in the backup is not emptied. With `--load=copy`, the drops and the load are one transaction. `IMPORT INTO` cannot run inside that transaction, so a failed import can leave a partial database. The recovery is to rerun with `--force=in-place`. `prune` drops old swapped copies with `DROP DATABASE ... CASCADE`. Restore and prune do not take the backup lock.

If a `CREATE` statement for a database, schema, type, sequence, table, index, function, or procedure is rejected, restore stops before loading rows and names the object. Views are created after the rows are loaded, and a rejected view is named the same way. The error quotes the target's message. It does not describe every rejection as a syntax problem. The JSON lists each rejected object under `incompatible`. The tool does not delete or rewrite that statement to hide it, except for the hidden `rowid` on a materialized view described above. Duplicate objects (`CREATE` of something that already exists) are allowed so a retry can continue after `--force=in-place` has dropped them.

`--load=import` is the default. For a local backup, and for every S3 mode except `implicit`, the tool serves the files itself. It binds `--serve-addr` when you set it, otherwise `--import-listen` (default `127.0.0.1:0`). One random token is used for every file in that run, not a new token per file. `--serve-advertise` is the host the nodes dial. It defaults to the bind address when that address is a specific host such as `10.0.0.8` or `127.0.0.1`. A bind that listens on every interface is refused unless you pass `--serve-advertise`. That includes an empty host (`:8080`), `0:8080`, `0.0.0.0`, `[::]`, `[::0]`, and any other spelling of an unspecified address. Those binds would put a URL in the statement that points each node at itself.

```bash
fgdb-backup restore --url "$URL" --src /backups/app/latest \
  --serve-addr 0.0.0.0:8080 \
  --serve-advertise 192.0.2.10
```

Set `--serve-tls-cert` and `--serve-tls-key` together, or set neither. One without the other is refused before the restore starts. With both set, the URL is `https://`. The nodes must already trust that certificate. This tool has no flag that installs a CA. If `IMPORT` fails, the error names `--serve-addr` when you set it, and `--import-listen` otherwise.

`--s3-import-auth=implicit` is the mode where the database reads `s3://` itself and the URL has no keys. `auto` does not do that, whether or not `AWS_ACCESS_KEY_ID` is set. Pass `--s3-endpoint` for a non-AWS endpoint. If the node cannot read the URL, run again with `--load=copy`. That streams the files from this machine into `COPY ... FROM STDIN` and does not start the file server. Array fields are rewritten to the `{...}` form `COPY` accepts. The files on disk stay in the `ARRAY` form `IMPORT` accepts.

`--testing-mode` is not an S3 switch. During an `IMPORT` restore it creates a table named `__fgdb_backup_probe_<id>` in the database from `--url`, copies one row, and drops the table. The default is off.

## Examples

Nightly backup of one database to S3:

```bash
fgdb-backup backup --json \
  --url "$URL" \
  --dest s3://backups/fgdb \
  --database app \
  --name prod \
  --extend-gc-ttl=12h \
  --sse=AES256
```

Start a fresh cluster from the newest complete backup:

```bash
fgdb-backup restore --json \
  --url "$NEW_URL" \
  --src s3://backups/fgdb/prod/latest
```

Blue-green copy: back up the live cluster, restore into the candidate, and check the JSON row counts. The candidate databases should not already contain user objects. Pass `--force` when you mean to swap the existing database aside. Pass `--force=in-place` only when you mean to drop the backed-up objects first. A same-name lock is on by default for backup. It expires one minute after the lease (default 10 minutes) past the last renewal. `fgdb-backup unlock --dest /backups --name blue --yes` removes an expired lock.

```bash
fgdb-backup backup --json --url "$LIVE_URL" --dest /backups --database app --name blue
fgdb-backup restore --json --url "$CANDIDATE_URL" --src /backups/blue/latest
```

Move data off a newer CockroachDB (24.2 or 25.1) into FGDb v23.2.15-oss. A newer data directory cannot be opened by 23.2. Point `--url` at the newer cluster and `--dest` at S3 or a directory. Point restore `--url` at a fresh FGDb node. If the newer schema uses syntax 23.2 rejects, restore exits non-zero and lists those objects. Fix or drop them on the source and take another backup. This path was checked against the 23.2 OSS binary. A live 24.2 or 25.1 server was not part of that test. The reads are `SHOW`, `COPY`, and `crdb_internal`, which those versions still provide.

```bash
fgdb-backup backup --json --url "$NEWER_URL" --dest s3://backups/fgdb --name moved
fgdb-backup restore --json --url "$FGDB_URL" --src s3://backups/fgdb/moved/latest
```

## Running against CockroachDB on Kubernetes

`IMPORT INTO` can run on any node, so every database pod must be able to open the address where this tool is serving files. Run it as a Job that binds to the pod IP on a fixed port, and allow that port in any network policy that would otherwise block it. With TLS, the certificate needs the pod IP as a subject alternative name, and the nodes must already trust the certificate authority.

`--load=copy` skips the file server. The tool reads the files and streams them with `COPY FROM STDIN`. Use that when the pods cannot reach the Job.

The container image installs the program at `/cockroach/fgdb-backup` and runs as user id 1000. A backup written to a local disk needs a volume that user can write. Put client certificates in a Secret, mount it, and pass those paths in `--url`.

A bad command line exits 2. A failure after the command has started exits 1. A Job can use that difference when it decides whether to retry. Exit 4 is only `restore --plan` refusing the restore. Retrying that plan will refuse again until the database or the flags change.

Kubernetes stops a pod about 30 seconds after it asks the process to exit. That can be too short for `--extend-gc-ttl`, because the tool puts the old `gc.ttlseconds` back on the way out. The SQL user also needs permission to change zone configs. Raise the Job's termination grace period when you use that flag.

Stop the application while `IMPORT` is running. After a swap, restart the application pods. The swap renames the live database to an old copy and puts the restored database on the original name. Pods that still have the old database open need a new connection to see it.

After you build an image, run `fgdb-backup help` inside it. You should see this command's usage text. A placeholder program is not enough to validate a backup.

## Limits

This is slower than enterprise `BACKUP`. It copies every row every time. There is no incremental backup and no revision-history point-in-time restore. One backup is one timestamp.

Also not included:

- Password hashes
- Privileges for `admin`, `root`, `node`, and `public`
- Index and partition zone configs on an OSS target (they are saved, then skipped with a warning)
- `GENERATED ALWAYS AS IDENTITY` columns (backup stops and names the column)
- zstd
- Client-side encryption
- Tables that 23.2 OSS cannot create

The release tarball builds `fgdb-backup` with Go 1.21.12, the toolchain this repository releases with. The package script exits if `go env GOVERSION` is different, so a random Go on `PATH` is not used.

`BACKUP` and `RESTORE` in SQL still fail on `cockroach-oss` with "a CCL binary is required". A later cleanroom implementation of those statements is a different project. This tool is the SQL copy you can run until then.
