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

Every completed backup has a unique directory. Files are created only once,
so a retry cannot silently replace a completed file.

Restore first supports a read-only plan:

```
fgdb-backup restore --config backup.yaml --plan --url URL --src ./backups/daily/latest
```

The plan checks the target before a forced restore. `--force` is still the
last-resort drop-in-place operation and never uses `CASCADE`. S3 `auto` mode
serves files through the tool. Direct reading by the database node requires
`--s3-import-auth=implicit`. The testing-only mode is not enabled by default.
