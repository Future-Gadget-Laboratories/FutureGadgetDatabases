> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# fgdb-backup command

This page is the command as `pkg/cmd/fgdb-backup` behaves on main. The file layout is [backup.md](backup.md). Where that page disagrees about exit codes or `--force`, use this page. It was checked against the source. The config keys are [backup-config.md](backup-config.md).

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
| 1 | `backup`, `restore`, `verify`, or `list` started the operation and it failed. A restore preflight refusal without `--plan` is also 1. |
| 2 | The command line is wrong (missing `--url`, a flag Go could not parse). `fail()` also returns 2. That path covers a bad config file, a bad value such as `--lock=maybe`, and setup errors such as `prune` failing to connect. |
| 4 | `--plan` was set and the preflight refused. The cluster is unchanged. |

The usage text the binary prints still says 0, 1, and 2. It does not mention 4. The table above is what `main.go` returns.

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

## See the plan first

`--plan` prints the preflight and does not change the cluster.

```bash
fgdb-backup restore --url "$URL" --src /backups/name/latest --plan
```

Exit 0 means the preflight accepted the restore. Exit 4 means it refused. `--plan-format` is `text` or `json`. With `--json`, the plan is JSON.

## Where the database fetches a local backup

A local restore can serve the files over HTTP so `IMPORT` can read them. `--import-listen` defaults to `127.0.0.1:0`. `--serve-addr` is the address advertised to the database. A cluster on another machine needs an explicit address that is not a loopback. When `--serve-addr` is empty, the server listens on `--import-listen`.

```bash
fgdb-backup restore --url "$URL" --src /backups/name/latest \
  --serve-addr backup-host.example:8081
```

Use a host the database nodes can dial. `--serve-tls-cert` and `--serve-tls-key` are optional.

## `--testing-mode`

The flag help says "allow test-only S3 probes." The code does something narrower. With `--testing-mode`, restore creates a scratch table, loads one row with `COPY FROM STDIN`, and drops the table. That runs before the real import. It does not send an S3 request. The YAML key is `testing_mode`.

## Prune old swapped copies

```bash
fgdb-backup prune --url "$URL" --name sensors --keep 1
```

`--keep` defaults to 1, the same default as `--retention` on restore. The command drops older swapped copies of that database name and prints how many it removed.
