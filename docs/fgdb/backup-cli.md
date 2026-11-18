> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# fgdb-backup command

This page is the command as `pkg/cmd/fgdb-backup` behaves in this tree. The file layout is [backup.md](backup.md). Where that page disagrees about exit codes or `--force`, use this page. It was checked against the source. The config keys are [backup-config.md](backup-config.md).

The binary is in the `v23.2.15-fgdb.1` tarball and image. The earlier `v23.2.15-oss` tarball has `cockroach` and the GEOS libraries, and it has no `fgdb-backup`. Checksums are in [releases.md](releases.md).

## Commands

```bash
fgdb-backup backup  --url "$URL" --dest /backups
fgdb-backup restore --url "$URL" --src /backups/name/latest
fgdb-backup verify  --src /backups/name/latest
fgdb-backup list    --src /backups/name
fgdb-backup unlock  --dest /backups --name name --yes
fgdb-backup prune   --url "$URL" --name name --keep 1
```

`--url` is a PostgreSQL connection URL. Put certificates in the URL. `--json` prints one JSON object on stdout. Progress lines go to stderr.

## Exit status

| Status | When |
| --- | --- |
| 0 | The command finished. |
| 1 | The command started, then failed. `fail()` returns 1. That includes a bad config file, a bad value such as `--s3-import-auth=maybe`, a refused unlock, a failed prune, a TLS mismatch or bad certificate, a refused or failed import bind, and a restore preflight refusal without `--plan`. |
| 2 | The command line is wrong. Missing required flags, a flag Go could not parse, and `--force in-place` with a space are exit 2. |
| 4 | `--plan` was set and the preflight refused. A served-import listener or wildcard that the plan rejects is also exit 4. The cluster is unchanged. |

`restore` returns these statuses from `restoreStatus`. Other commands return 1 from `fail()` and 2 from flag parsing. The usage text printed by the binary lists 0, 1, 2, and 4.

> **Warning: restore load mode.** `--load=import` (`IMPORT INTO`) is the default and the supported loader. `--load=copy` has known bugs and is not safe for restores: `BYTES` values are stored as their literal escaped text, `STRING COLLATE` columns are not restored correctly, and `--force=in-place` with `--load=copy` fails with SQLSTATE 25P02. A known `IMPORT` corruption bug is fixed: backup wrote array columns whose element type has a width with a cast that dropped the width, so `IMPORT` cut `CHAR(n)[]` values to their first character and rejected `BIT(n)[]` with n > 1. The cast is in the backup files, so a backup taken before the fix still restores those columns wrong. Take a new backup of tables with `CHAR(n)[]` or `BIT(n)[]` columns, and check tables restored from older backups.

## Restore when the database already exists

`--swap-restore` defaults to true. `--retention` defaults to 1. That number is how many old swapped database copies to keep.

`--force` chooses what happens when the target is not empty:

```bash
# Swap names. Same as --force=true and --force=swap.
fgdb-backup restore --url "$URL" --src /backups/name/latest --force

# Drop and recreate. Last resort.
fgdb-backup restore --url "$URL" --src /backups/name/latest --force=in-place
```

`--force` used alone is the swap path. `--force=in-place` is the drop path. Any other value is a bad flag and the process exits 2.

With `--load=import`, the import listener is opened and checked before those drops. A refused wildcard, a bind failure, or a bad certificate exits 1 and leaves the existing rows and schema in place. `--load=copy` does not open that listener. The drops and the COPY load are one transaction.

## See the plan first

`--plan` prints the preflight and does not change the cluster. For a served import it also checks the listener, including a wildcard bind, and then closes that socket. It does not start the file server.

```bash
fgdb-backup restore --url "$URL" --src /backups/name/latest --plan
```

Exit 0 means the preflight accepted the restore. Exit 4 means it refused. `--plan-format` is `text` or `json`. With `--json`, the plan is JSON.

## Where the database fetches a local backup

A local restore can serve the files over HTTP so `IMPORT` can read them. `--import-listen` defaults to `127.0.0.1:0`. `--serve-addr` replaces that bind. `--serve-advertise` is required when the bind listens on every interface, including a hostname that resolves to one. `--load=copy` and `--s3-import-auth=implicit` do not serve files.

```bash
fgdb-backup restore --url "$URL" --src /backups/name/latest \
  --serve-addr 0.0.0.0:8081 \
  --serve-advertise 192.0.2.10
```

Use a host the database nodes can dial. A cluster on another machine needs an address that is not a loopback. Set `--serve-tls-cert` and `--serve-tls-key` together, or set neither. One without the other exits 1 before the restore starts.

## `--testing-mode`

`--testing-mode` is not an S3 switch. During an `IMPORT` restore, before the real import, it creates a scratch table, copies one row with `COPY FROM STDIN`, and drops the table. It does not send an S3 request. The YAML key is `testing_mode`.

## Prune old swapped copies

```bash
fgdb-backup prune --url "$URL" --name sensors --keep 1
fgdb-backup prune --url "$URL" --name sensors --keep 0 --dry-run
```

`--keep` defaults to 1, the same default as `--retention` on restore. The command drops older swapped copies of that database name and prints how many it removed. `--dry-run` prints the names and does not drop them. A failed prune exits 1. A missing `--url` or `--name` exits 2.
