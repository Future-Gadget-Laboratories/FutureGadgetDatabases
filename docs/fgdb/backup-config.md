# Backup configuration

`fgdb-backup backup` and `fgdb-backup restore` take an optional YAML file
with `--config`. The tool reads that file once, when it starts. A flag wins
over the file, and a file value wins over the built-in default. The commented
example is `pkg/cmd/fgdb-backup/config.example.yaml`.

`verify`, `list`, `unlock`, and `prune` do not read `--config`. Passing it
to those commands is a bad command line (exit 2).

Unknown YAML keys are an error. So is the single-dash spelling `-config`.
Use `--config`.

## YAML keys

Omitted keys keep the built-in default. A present key wins, including when
the value is `false` or `0`.

### `backup`

| Key | Default | What it does |
| --- | --- | --- |
| `lock` | `true` | Same-name backup lock. The flag form is `--lock=on` or `--lock=off`. |
| `lock_wait` | `0s` | How long to wait for a lock. `0s` means do not wait. |
| `lock_lease` | `10m` | Lease length. Values under 30s are raised to 30s. |
| `threads` | `0` | Go processor cap. `0` means no cap. |
| `memory_bytes` | `0` | Soft memory limit in bytes. `0` means no cap. |
| `file_mode` | `0640` | Mode for new local files, as a non-zero octal string. |
| `allow_unsafe_overwrite` | `false` | Continue when an S3 endpoint ignores `If-None-Match`. |
| `s3_credential_mode` | `auto` | `auto`, `implicit`, `specified`, or `served`. |

### `restore`

| Key | Default | What it does |
| --- | --- | --- |
| `swap_restore` | `true` | Allow a beside-and-swap restore. |
| `in_place` | `false` | See the `--force` section below. This key is used only when `--force` is not passed. |
| `retention` | `1` | How many old swapped copies to keep. `0` turns the automatic prune off. |
| `testing_mode` | `false` | Create a scratch table, copy one row, and drop the table. Not an S3 switch. |
| `threads` | `0` | Go processor cap. `0` means no cap. |
| `memory_bytes` | `0` | Soft memory limit in bytes. `0` means no cap. |
| `allow_unsafe_overwrite` | `false` | Accepted on restore. Restore does not write the S3 probe, so this does not change a normal restore. |
| `s3_credential_mode` | `auto` | `auto`, `implicit`, `specified`, or `served`. |

There is no YAML key for `--url`, `--dest`, `--src`, `--database`, `--name`,
`--load`, `--s3-endpoint`, `--s3-region`, `--serve-addr`, `--serve-advertise`,
`--import-listen`, the TLS file flags, `--plan`, or `--force`. Those are
flags only.

## Flags

Defaults below are what you get with no YAML file.

### backup

| Flag | Default |
| --- | --- |
| `--url` | required |
| `--dest` | required. `s3://bucket/prefix` or a local directory |
| `--database` | every user database. Repeat the flag or use commas |
| `--name` | the cluster id |
| `--compression` | `gzip` (`gzip` or `none`) |
| `--sse` | unset (`AES256` or `aws:kms`) |
| `--sse-kms-key-id` | unset. Required with `--sse=aws:kms` |
| `--s3-region` | `AWS_REGION`, then `AWS_DEFAULT_REGION` |
| `--s3-endpoint` | unset. A custom endpoint uses path-style S3 addresses |
| `--s3-import-auth` | `auto` |
| `--allow-unsafe-overwrite` | `false` |
| `--extend-gc-ttl` | unset. A Go duration such as `12h` |
| `--split-rows` | `0` (do not split) |
| `--part-size` | `8388608` (8 MiB). S3 minimum is `5242880` |
| `--safety-margin` | `1m` |
| `--skip-grants` | `false` |
| `--threads` | `0` |
| `--memory-bytes` | `0` |
| `--lock` | `on` |
| `--lock-wait` | `0s` |
| `--lock-lease` | `10m` (minimum 30s) |
| `--json` | `false` |
| `--config` | unset |

### restore

| Flag | Default |
| --- | --- |
| `--url` | required |
| `--src` | required. A timestamp directory, or a path ending in `/latest` |
| `--database` | every database in the backup |
| `--force` | unset. See below |
| `--load` | `import` (`import` or `copy`) |
| `--import-listen` | `127.0.0.1:0` |
| `--serve-addr` | unset. When set, the file server binds this instead of `--import-listen` |
| `--serve-advertise` | the bind address, when that address is a specific host |
| `--serve-tls-cert` | unset |
| `--serve-tls-key` | unset |
| `--s3-region` | `AWS_REGION`, then `AWS_DEFAULT_REGION` |
| `--s3-endpoint` | unset. A custom endpoint uses path-style addresses |
| `--s3-import-auth` | `auto` |
| `--allow-unsafe-overwrite` | `false` |
| `--swap-restore` | `true` |
| `--retention` | `1` |
| `--plan` | `false` |
| `--plan-format` | `text` (`text` or `json`). `--json --plan` forces `json` |
| `--testing-mode` | `false` |
| `--threads` | `0` |
| `--memory-bytes` | `0` |
| `--json` | `false` |
| `--config` | unset |

### verify

`--src` (required), `--s3-region`, `--s3-endpoint`, `--allow-unsafe-overwrite`
(default `false`), `--json`. No `--config`. Verify does not write the S3 probe.

### list

`--src` (required), `--s3-region`, `--s3-endpoint`, `--json`. No `--config`
and no `--allow-unsafe-overwrite`. List does not write the S3 probe.

### unlock

`--dest` and `--name` are required. Also `--s3-region`, `--s3-endpoint`,
`--yes`, and `--force-unlock` (default `false`). No `--config`. A missing
`--dest` or `--name` prints `unlock requires --dest and --name` and exits 2.

### prune

`--url` and `--name` are required. `--keep` defaults to `1`. `--dry-run`
prints the old copies it would drop and does not drop them. No `--json` and
no `--config`. Prune does not take a backup lock.

```bash
fgdb-backup prune --url "$URL" --name app --keep 0 --dry-run
fgdb-backup prune --url "$URL" --name app --keep 1
```

## `--force`, swap, and `in_place`

A plain restore never replaces a database that already has user objects
(tables, views, sequences, types, functions, or procedures). It does not
merge. Exit status is 1. With `--plan`, the same refusal exits 4.

`--force` is how you ask to replace that database. It is a boolean flag:

- `--force`, `--force=true`, and `--force=swap` mean swap.
- `--force=in-place` means drop the backed-up objects in place.
- `--force in-place` (a space, not `=`) is a bad command line and exits 2.
  The word `in-place` is left over as an extra argument.

Swap restores into a new database, then renames the old database out of the
way and renames the new one into place. The old copy is kept. Its name is
the original name plus `__fgdb_old_`, the UTC timestamp with the dots
removed, and 8 hex characters, all lower case. An example is
`app__fgdb_old_20261015t143012000z-a1b2c3d4`.

Swap is refused when any of these are true:

- The backup contains a view, a materialized view, a function, or a procedure.
  Restoring one of those can block the rename.
- The database already has a view, a materialized view, a function, or a procedure.
- Another database has a view, materialized view, function, or procedure
  whose `CREATE` text names this database (a cross-database link).
- A table that is not in the backup has a foreign key to a table that is.
- `--swap-restore=false` (or `swap_restore: false`). The plan then tells you
  to rerun with `--force=in-place`.

`restore.in_place: true` is applied only when you do not pass `--force`.
A restore without `--force` still refuses a non-empty database, so the YAML
key does not by itself start an in-place drop. Passing `--force` by itself
always means swap, even if the YAML key is true. The way to drop objects in
place is `--force=in-place`.

`--force=in-place` drops only objects that are in the backup. It checks the
foreign-key case above before it drops anything, and it does not use
`CASCADE` on those object drops, so a table outside the backup is not emptied.
Views are dropped in reverse creation order.

With `--load=copy`, the in-place drops and the load are one transaction.
`IMPORT INTO` cannot run inside that transaction. If an `IMPORT` restore
fails after the drops, the database can be left with the old objects gone
and only some new objects created. The error and the plan say so.

Roll back a successful swap by renaming, after you check both names:

```sql
ALTER DATABASE app RENAME TO app_failed;
ALTER DATABASE "app__fgdb_old_20261015t143012000z-a1b2c3d4" RENAME TO app;
```

## Retention and `prune --keep 0`

After a successful swap, the tool prunes old copies immediately.
`retention: 1` (the default) keeps the newest old copy and drops older ones.
`retention: 0`, or `--retention=0`, skips that automatic prune. The old copy
stays until you prune it yourself.

`prune` is separate. `--keep 0` drops every old copy of that database.
`--keep 1` keeps the newest and drops the rest. A negative `--keep` is an
error (exit 1). `--dry-run` lists the same names and does not run `DROP`.

`prune` runs `DROP DATABASE ... CASCADE`. Restore and prune do not take the
backup lock.

## Locks

With `--lock=on` (the default), two backups of the same `--name` cannot write
the same destination at once. The lock file is `<name>/LOCK.json`.

`--lock-wait` is how long a second backup waits. The default `0s` fails
immediately. `--lock-lease` is how long one renewal stays valid. The default
is 10 minutes. Anything under 30 seconds is raised to 30 seconds. The lock
is treated as expired at the last renewal, plus the lease, plus one extra
minute. A 10 minute lease with no renewal expires about 11 minutes after it
was last renewed.

```bash
fgdb-backup unlock --dest ./backups --name daily --yes
```

`--yes` is required. Without `--force-unlock`, unlock refuses a lease that
has not expired. `--force-unlock` removes a live lease. Check the holder
first. Unlock writes the S3 conditional-write probe described below, so it
needs credentials that can put and delete an object.

## S3 modes and the conditional-write probe

`--s3-import-auth` accepts four values: `auto`, `implicit`, `specified`,
and `served`.

`auto`, `specified`, and `served` all do the same thing. The tool reads S3
and the database fetches the bytes from the file server. This happens even
when `AWS_ACCESS_KEY_ID` is unset. The `IMPORT` statement gets an `http://`
or `https://` URL, not your keys.

`implicit` is the only mode where the database reads `s3://` itself, with
`AUTH=implicit`. Use it when the node has an IAM role. The statement has no
keys.

An S3 URL needs a region even when you pass `--s3-endpoint`. Set
`AWS_REGION`, `AWS_DEFAULT_REGION`, or `--s3-region`. A custom endpoint uses
path-style addressing (`http://endpoint/bucket/key`).

`backup` and `unlock` check that the endpoint honors `If-None-Match`. They
put a small object named `.fgdb-if-none-match-<id>` twice, then delete it.
The probe body is the word `probe`. The probe does not send server-side
encryption headers, so a bucket that requires SSE rejects the probe, and
credentials that cannot write also fail.

`restore`, `verify`, and `list` do not write that probe. They can use
read-only credentials.

If the second put succeeds, the endpoint is ignoring `If-None-Match`.
`backup` then fails unless `--allow-unsafe-overwrite` (or
`allow_unsafe_overwrite: true`) is set. The real backup objects still send
`If-None-Match`. The flag does not turn the header off. It only lets the
backup continue after the probe. `unlock` has no such flag. If its probe
sees an endpoint that ignores the header, unlock fails with the probe error.

A failed S3 probe during a lock reports that probe error. It does not say
that the lock needs a local or S3 destination. That sentence is only for a
destination that is neither a directory nor `s3://`.

## `testing_mode`

`testing_mode: true` or `--testing-mode` runs during an `IMPORT` restore,
before the tables are loaded. On the database named in `--url` (often
`defaultdb`) it creates a table named `__fgdb_backup_probe_<id>`, copies
one row into it, and drops the table. It does not turn the S3 probe on or
off. The default is off.

## Served files, bind address, and TLS

When the tool serves files (local backups, and S3 modes other than
`implicit`), it listens on `--serve-addr` if you set it, otherwise on
`--import-listen` (`127.0.0.1:0`). One random token is created for the run
and reused for every file. The URL looks like
`http://127.0.0.1:12345/<32 hex characters>/data/app/public/t.pgcopy.gz`.
It is not a new token per file.

`--serve-advertise` is the host the database nodes dial. When the bind
address is a specific host, the default advertise address is that host and
the port the server actually got. A bind that listens on every interface is
refused unless you pass `--serve-advertise`. That includes an empty host
(`:8080`), `0:8080`, `0.0.0.0`, `[::]`, `[::0]`, and any other spelling of an
unspecified address. A URL that contains one of those addresses points each
node at itself.

```bash
fgdb-backup restore --url "$URL" --src /backups/app/latest \
  --serve-addr 0.0.0.0:8080 \
  --serve-advertise 192.0.2.10
```

That URL uses `192.0.2.10` and the bound port. If `--serve-advertise`
already includes a port, that port is used as written.

Set both `--serve-tls-cert` and `--serve-tls-key`, or neither. One without
the other is refused before the restore starts. With both set, the URL is
`https://`. A bad certificate file is reported as a file-server error, not
ignored. There is no flag that installs a CA on the database nodes. The
nodes must already trust the certificate. If they cannot reach the server,
rerun with `--load=copy`.

If `IMPORT` fails, the error names the flag that chose the bind address:
`--serve-addr` when you set it, otherwise `--import-listen`.

## Exit status

| Code | When |
| --- | --- |
| 0 | The command finished. |
| 1 | The command started, then failed. This includes a bad config file, a bad YAML value, a bad path or S3 location, a refused unlock, a failed prune, a TLS flag mismatch, and a refused wildcard bind. A real restore that the preflight refuses also exits 1. |
| 2 | The command line is wrong: unknown command, bad flag syntax, missing required flags, or extra arguments. |
| 4 | Only `restore --plan`, when the preflight refuses. `--plan` does not change the cluster. |

```bash
fgdb-backup restore --config backup.yaml --plan --url "$URL" --src ./backups/daily/latest
```

`--json` prints one JSON object on stdout. Progress lines go to stderr.
