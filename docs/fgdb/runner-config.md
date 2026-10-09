# Runner setup configuration

`setup-runner.sh` can read a small settings file with `--config`. Copy
`build/fgdb/runner/config.example.yaml` and change the lines you need.

The file is checked before the plan is printed. A bad line stops setup.
Nothing is installed until the file parses.

## Which value wins

The script looks at settings in this order. The last one that sets a value
wins:

1. Built-in defaults.
2. `FGDB_*` environment variables, for the settings that have one.
3. The config file.
4. The matching command-line flag.

So a flag beats the file, and the file beats the environment variable.
`auto_update` and `allowed_filesystems` have no flag and no environment
variable. Set those in the file, or leave them at the defaults.
`auto_update` stays on unless the file says `false`.

## What you can write

The file is a short list of settings, not a general YAML document. Use
spaces, not tabs. The colon sits right after the key.

```yaml
# A comment line.
cache_path: /var/cache/fgdb
local_cpu: 8
auto_update: false
allowed_filesystems:
  - ext4
  - xfs
```

Rules that are accepted:

- Blank lines, and comments that start with `#`.
- One `key: value` on a line. Quotes are optional. `"false"` and `'false'`
  are the same as `false`.
- A `#` comment after the value, when the value is not quoted. Inside
  quotes, `#` is part of the value.
- A colon inside the value. `cache_path: /mnt/disk:1/fgdb` stays whole.
- Windows line endings. A trailing carriage return is removed.
- `allowed_filesystems` written as a list. Each item is indented, then a
  dash, then a space, then the name. That is the only list.

Anything else is an error. That includes a misspelled key, a repeated key,
a tab, a nested block, a flow list such as `[ext4, xfs]`, a `|` or `>`
block, a YAML tag, or a list item that is not under `allowed_filesystems`.

## Settings

| Key | What it accepts | Flag that overrides it |
| --- | --- | --- |
| `auto_update` | `true` or `false`. Default `true`, so the runner may update itself. | none |
| `cache_path` | An absolute path, with no spaces. | `--cache-dir` |
| `allowed_filesystems` | A list of names such as `ext4`. Default `ext4`, `xfs`, `btrfs`, `zfs`. | none |
| `local_cpu` | A whole number from 1 to 1024. | `--local-cpu` (`FGDB_LOCAL_CPU`) |
| `local_ram_mb` | A whole number from 1 to 4194304. | `--local-ram-mb` (`FGDB_LOCAL_RAM_MB`) |
| `cpu_quota` | A whole number and a percent sign, such as `2400%`. | `--cpu-quota` (`FGDB_CPU_QUOTA`) |
| `memory_high` | A size such as `80G`, `512M`, `80Gi`, or a plain byte count. | `--memory-high` (`FGDB_MEMORY_HIGH`) |
| `memory_max` | Same kind of size as `memory_high`. | `--memory-max` (`FGDB_MEMORY_MAX`) |
| `runner_version` | Three numbers, such as `2.338.0`. The version must also be listed in `checksums.txt`. | `--runner-version` |
| `bazelisk_version` | Three numbers, such as `1.29.0`. It must be listed in `checksums.txt`. | none |

The cache check runs against `cache_path` when you set it. It allows only
the filesystem types in `allowed_filesystems`. The systemd service runs
that same list again before each start. See [RUNNER.md](RUNNER.md) for the
disk rules, including the inode check that btrfs skips.
