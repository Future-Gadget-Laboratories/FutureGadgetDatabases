# Backup configuration

`fgdb-backup` accepts an optional YAML file with `--config`. It reads the file
once when it starts. A command-line flag has priority over the file, and a
file value has priority over the built-in default. The commented example is
at `pkg/cmd/fgdb-backup/config.example.yaml`.

The default lock prevents two backups with the same name from changing the
same destination at once. Use `--lock-wait` when a scheduled retry should
wait. A lock that has expired can be removed with:

```
fgdb-backup unlock --dest ./backups --name daily --yes
```

Every completed backup has a unique directory. Local files use create-only
writes and mode `0640` by default; set `file_mode` when a different mode is
needed. A retry cannot silently replace a completed file.

Restore first supports a read-only plan:

```
fgdb-backup restore --config backup.yaml --plan --url URL --src ./backups/daily/latest
```

The plan checks the target before changing it. A plain restore refuses a
non-empty database. `--force` restores beside the existing database and swaps
names when the full preflight allows it. The old copy is kept; one old copy is
retained by default. Roll back with `ALTER DATABASE new_name RENAME TO
failed_name; ALTER DATABASE old_copy RENAME TO new_name;`, after checking that
both names are correct. Use `fgdb-backup prune --url URL --name new_name
--keep 1` to remove older copies.

If swap is impossible, the plan refuses and asks for `--force=in-place`.
That last-resort mode drops the backed-up objects before loading them. With
`--load=copy`, the drops and load are one transaction. `IMPORT INTO` cannot
run inside that transaction; in that mode a failure can leave the database
with the old objects removed and only the newly created objects that
succeeded. The error and plan identify this state.

S3 `auto` mode serves files through a random, one-use URL path. It binds to
localhost by default, so a remote cluster needs an explicit `--serve-addr`;
use `--serve-tls-cert` and `--serve-tls-key` when the route is not otherwise
private. Direct reading by the database node requires
`--s3-import-auth=implicit`. The testing-only scratch import is not enabled by
default and runs only with `testing_mode: true`.

Exit status 4 means the read-only restore preflight refused the operation.
Unknown YAML keys and the single-dash `-config` spelling are errors. Flags
override YAML values, and YAML values override built-in defaults.
