> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Logical backup and restore

`fgdb-backup` copies user databases with ordinary SQL. The open-source binary rejects `BACKUP` and `RESTORE` because those statements need a CCL binary. This program does not implement those statements. It reads schema and rows with `SHOW`, `COPY`, and `AS OF SYSTEM TIME`, writes files, and loads them back with `IMPORT INTO` or `COPY FROM STDIN`.

The program is a separate binary, `fgdb-backup`. It is built into the release tarball and the container image, next to `cockroach`. A Kubernetes Job or a shell script can call it. Two stable shapes are:

```bash
fgdb-backup backup  --url "$URL" --dest s3://bucket/prefix [--database name]
fgdb-backup restore --url "$URL" --src  s3://bucket/prefix/name/latest
```

`--json` prints one JSON object on stdout. Progress lines go to stderr. Exit status is 0 on success, 1 when the backup or restore fails, and 2 when the command line is wrong.

`--url` is a PostgreSQL connection URL. Certificates go in that URL:

```text
postgresql://root@db.example:26257/defaultdb?sslmode=verify-full&sslrootcert=/certs/ca.crt&sslcert=/certs/client.root.crt&sslkey=/certs/client.root.key
```

S3 uses the normal AWS credential chain (environment variables, a shared config file, or an IAM role). Do not put secret keys in the backup files. The tool adds keys to an `IMPORT` URL only in the database session, and only when the database itself must read S3 with `AUTH=specified`.

## What a backup contains

Format version 1. Paths look like this:

```text
<dest>/<name>/<UTC timestamp>/manifest.json
<dest>/<name>/<UTC timestamp>/objects.json
<dest>/<name>/<UTC timestamp>/schema/<database>.sql
<dest>/<name>/<UTC timestamp>/users.sql
<dest>/<name>/<UTC timestamp>/zones.sql
<dest>/<name>/<UTC timestamp>/data/<database>/<schema>/<table>.csv.gz
<dest>/<name>/latest.json
```

`<name>` defaults to the cluster id. Pass `--name` to choose a folder. The timestamp is UTC, like `20060102T150405Z`.

`manifest.json` records the source `version()`, the cluster id, the `AS OF SYSTEM TIME` timestamp, per-table row counts, and the sha256 of each stored file. `objects.json` is what restore executes. The `.sql` files are the same statements in a form you can read.

`verify` checks those checksums and recounts CSV rows. It does not restore.

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

`latest.json` is written only after `manifest.json` is in place. `restore --src .../latest` reads that pointer. A directory with data files but no manifest is unfinished. `list` does not return it, and it is never what `latest` points at.

Local files are written to a `.partial` name and renamed when the file is finished. A crash during a file write does not leave that file under its final name.

### Large tables

Rows are streamed. The process does not load a whole table into memory. S3 uploads use multipart upload. The part size defaults to 8 MiB and must be at least 5 MiB (`--part-size`).

`--split-rows=N` splits a table into primary-key ranges of about N rows, all read at the same timestamp. This only works for a table whose primary key is one integer column (`INT2`, `INT4`, or `INT8`). Other tables are one stream, and the tool says so on stderr. The files are named `<table>.part0001.csv.gz`, and one `IMPORT` loads every part.

The backup JSON includes `peak_rss_bytes`, the high-water resident set size of the tool process (`VmHWM` on Linux).

## Snapshot age and gc.ttlseconds

Every read uses one timestamp: `AS OF SYSTEM TIME '<timestamp>'`. CockroachDB can serve that timestamp only while it is newer than the garbage-collection threshold. The zone setting `gc.ttlseconds` is that window. On a new 23.2 cluster the range default is 14400 seconds (4 hours). A database or table can set a shorter value. A backup that is still running when the window closes fails, and the files are not marked latest.

Before copying rows, the tool reads `crdb_internal.zones`, takes the smallest `gc.ttlseconds` that applies to the databases in this run, and refuses to start if `now + safety margin` is not strictly before `timestamp + gc.ttlseconds`. The default safety margin is one minute (`--safety-margin`). The error tells you the deadline and how to raise the TTL. The same check runs again before each table. If a read fails with a GC-threshold error, the tool exits non-zero with the same explanation.

To give a long backup more time, raise the TTL for the window and put it back when you are done:

```sql
ALTER DATABASE app CONFIGURE ZONE USING gc.ttlseconds = 86400;
```

`--extend-gc-ttl=12h` does that for the databases in this run, prints the statements that put the old values back, and runs those statements when the process exits. Use a Go duration (`12h`, `30m`). `1d` is not accepted. If the process is killed with `kill -9`, the higher value stays until you run the printed statements. If the backup files are written but the old value cannot be put back, the process exits non-zero and prints the statement to run. The backup itself is still valid. The files record the zone configs from before the temporary raise, not the raised value.

## What is restored, and in what order

1. Databases.
2. Schemas, types, and sequences.
3. Tables and indexes. Foreign keys are not created yet.
4. Views.
5. Row data (`IMPORT INTO` by default, or `--load=copy`).
6. Foreign keys and `VALIDATE CONSTRAINT`.
7. Sequence values (`setval` to the backed-up `last_value`).
8. Users, role memberships, and grants. Failures here are warnings.
9. Zone configurations. Failures here are warnings.

`system` and `postgres` are skipped. Other databases, including `defaultdb`, are included unless you pass `--database`. Schemas `pg_catalog`, `information_schema`, `crdb_internal`, and `pg_extension` are skipped.

Users and grants are best effort. Passwords are not in `SHOW USERS`, and this tool does not copy password hashes. `admin`, `root`, `node`, and `public` are not replayed. The target keeps its own privileges for those names. Create passwords again on the target if you need them.

Zone configs are read from `crdb_internal.zones` for the databases in the backup, including table zones and index or partition zones when the source has them. `RANGE default` is not rewritten on the target. On restore, each saved zone statement is executed. If 23.2 OSS rejects it, the tool prints a warning and continues. Index and partition zones are the usual case: this binary returns an error that enterprise features are required. The restore still succeeds. Database and table zones such as `gc.ttlseconds` and `num_replicas` are applied when the target accepts them.

Virtual computed columns are not in the CSV. Stored computed columns and ordinary columns are.

Restore refuses to load into a table that already has a row. `--force` truncates those tables with `CASCADE` and then loads. A second restore onto a full copy fails until you pass `--force`.

If a `CREATE` statement uses syntax this binary does not accept, restore stops before loading rows. The JSON lists each rejected object under `incompatible`. The tool does not delete or rewrite that statement to hide it. Duplicate objects (`CREATE` of something that already exists) are allowed so a retry can continue.

`--load=import` is the default. For a local backup the tool listens on `--import-listen` (default `127.0.0.1:0`) and the database fetches `http://...`. The database process must be able to reach that address. For S3, the database reads `s3://` itself. `--s3-import-auth` is `auto`, `implicit`, or `specified`. `auto` uses `specified` when `AWS_ACCESS_KEY_ID` is set, and `implicit` otherwise. Pass `--s3-endpoint` for a non-AWS endpoint. If the node cannot read the URL, run again with `--load=copy`. That streams the files from this machine into `COPY ... FROM STDIN`.

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

Blue-green copy: back up the live cluster, restore into the candidate, and check the JSON row counts. The candidate should be empty, or you pass `--force` if you mean to replace its tables.

```bash
fgdb-backup backup --json --url "$LIVE_URL" --dest /backups --database app --name blue
fgdb-backup restore --json --url "$CANDIDATE_URL" --src /backups/blue/latest
```

Move data off a newer CockroachDB (24.2 or 25.1) into FGDb v23.2.15-oss. A newer data directory cannot be opened by 23.2. Point `--url` at the newer cluster and `--dest` at S3 or a directory. Point restore `--url` at a fresh FGDb node. If the newer schema uses syntax 23.2 rejects, restore exits non-zero and lists those objects. Fix or drop them on the source and take another backup. This path was checked against the 23.2 OSS binary. A live 24.2 or 25.1 server was not part of that test. The reads are `SHOW`, `COPY`, and `crdb_internal`, which those versions still provide.

```bash
fgdb-backup backup --json --url "$NEWER_URL" --dest s3://backups/fgdb --name moved
fgdb-backup restore --json --url "$FGDB_URL" --src s3://backups/fgdb/moved/latest
```

## Limits

This is slower than enterprise `BACKUP`. It copies every row every time. There is no incremental backup and no revision-history point-in-time restore. One backup is one timestamp.

Also not included:

- Password hashes
- Privileges for `admin`, `root`, `node`, and `public`
- Index and partition zone configs on an OSS target (they are saved, then skipped with a warning)
- zstd
- Client-side encryption
- Tables that 23.2 OSS cannot create

`BACKUP` and `RESTORE` in SQL still fail on `cockroach-oss` with "a CCL binary is required". A later cleanroom implementation of those statements is a different project. This tool is the SQL copy you can run until then.
