# Self-hosted Actions runner

The `cockroach-oss` compile runs on a self-hosted GitHub Actions runner with
the labels `self-hosted` and `fgdb-build`.

The script is `build/fgdb/runner/setup-runner.sh`. It is idempotent.
It prints a plan, then applies it. `--dry-run` stops after the plan.
Optional settings live in a small config file. See
[runner-config.md](runner-config.md). The installed runner is allowed to
update itself. That stays on unless the config file sets `auto_update: false`.

Register that runner on a machine with at least 30 GiB of RAM and 150 GiB
free on a local disk. The script refuses a smaller machine unless you pass
`--allow-small-host` to test the script itself. Do not point the compile at
`ubuntu-latest`. The workflow asks for both `self-hosted` and `fgdb-build`,
so a runner that only has `self-hosted` will not pick up the job.

## What it sets up

- A system user, `fgdb-runner`. The runner service runs as that user.
  `./dev` refuses to run as root.
- Docker (`docker.io` from Ubuntu) and the `docker` group. The script does
  not `dist-upgrade` the host, so packages already installed on the machine
  stay as they are.
- Bazelisk 1.29.0 at `/usr/local/bin/bazel`. The tree's `.bazelversion` is
  `cockroachdb/6.2.1`. Bazelisk 1.10.1 from `build/bootstrap/bootstrap-debian.sh`
  cannot resolve that fork version; 1.29.0 can. Bazel then downloads the
  `cockroachdb/bazel` 6.2.1 binary. Go 1.21.12 comes from the Bazel SDK in
  `WORKSPACE`, not from a host Go toolchain.
- Build packages used by a native Bazel build: compilers, cmake, patchelf,
  python3, autoconf, bison, flex, ncurses. It does not install `ccache` or
  a host Node. Bazel fetches its own Node for the OSS UI.
- GitHub Actions runner 2.338.0, registered with a token you pass in.
  The token is not written to disk by this script and is not printed.
- A systemd slice, `fgdb-runner.slice`, and a drop-in on the runner service
  (`Nice=10`, `CPUWeight=50`, `IOWeight=50`, `LimitNOFILE=1048576`).

## CPU and memory

The slice is a hard cap on the runner cgroup, so the operating system and
other local services still have CPU and RAM. Defaults:

| Knob | Default | Meaning |
| --- | --- | --- |
| `CPUQuota` | `2400%` | 24 CPUs |
| `MemoryHigh` | `80G` | soft pressure above this |
| `MemoryMax` | `96G` | hard cap |

The same budget is written to `/etc/fgdb/runner.env` as `FGDB_LOCAL_CPU`
(default `24`) and `FGDB_LOCAL_RAM_MB` (default `81920`) so Bazel does not
schedule more work than the slice allows. Change them together.

Environment variables, read before the flags:

| Variable | Flag |
| --- | --- |
| `FGDB_RUNNER_NAME` | `--name` (default is `<short hostname>-fgdb`) |
| `FGDB_RUNNER_LABELS` | `--labels` (default `fgdb-build`) |
| `FGDB_CPU_QUOTA` | `--cpu-quota` |
| `FGDB_MEMORY_HIGH` | `--memory-high` |
| `FGDB_MEMORY_MAX` | `--memory-max` |
| `FGDB_LOCAL_CPU` | `--local-cpu` |
| `FGDB_LOCAL_RAM_MB` | `--local-ram-mb` |

Re-run the script after changing them. If the linker is killed by the
cgroup, raise `MemoryMax` before retrying. Do not remove the slice to get
a green build.

## Bazel cache disk

The Bazel cache has to be on a local disk. Do not put it on a network share
or a backup disk. Network storage is slow for this build, and a backup disk
can fill up with files that were never meant to be kept.

If you leave out `--cache-dir`, the script picks the directory:

1. It uses `/var/cache/fgdb` when the root filesystem is a local disk and
   has at least 150 GiB free. That is the normal choice.
2. If the root disk is smaller than that, it uses the local filesystem with
   the most free space, under `<that mount>/fgdb`.
3. It never picks a network filesystem, even when that disk has more free
   space. Skipped types include `nfs`, `nfs4`, `cifs`, `smb3`, `sshfs`
   (`fuse.sshfs`), `glusterfs`, and `ceph`, plus the same kind of remote
   disk (`lustre`, `gpfs`, `afs`, `s3fs`, and similar). It also skips
   `tmpfs`, `overlay`, and `/boot`.
4. It skips a mount under `/mnt` whose path contains `backup`, even when
   that filesystem looks local.

These three directories are created inside the chosen path:

- `bazel-disk-cache`
- `bazel-output-base`
- `runner-work` (where Actions checks out the code)

150 GiB is the minimum. The first build, including the web UI and the disk
cache, is more comfortable with extra room. To free that space later, run
`bazel clean --expunge` against that output base.

`--cache-dir` still has to point at a local directory with 150 GiB free.
The script refuses the path when it sits on a network filesystem or on a
`/mnt/*backup*` mount.

## Checking the cache disk

Before it prints the plan, the script checks the directory that will hold
the cache. `--dry-run` runs that check too, then stops. On a normal ext4
disk the check succeeds.

The check asks `findmnt` which filesystem that directory is on. `findmnt`
answers `ext4` for an ext4 disk. The script does not use `stat -f`, because
that command reports ext4 as `ext2/ext3` and would reject a healthy disk.
If `findmnt` is missing or cannot answer, the check reads
`/proc/self/mountinfo` instead.

The filesystem has to be `ext4`, `xfs`, `btrfs`, or `zfs`, unless your
config file lists a different set. The disk has to be writable and have at
least 150 GiB free. It also needs free inodes, with one exception: btrfs,
and any filesystem that reports zero inodes, does not use a fixed inode
table. The check writes a note and skips the inode test on those disks.

After install, the runner service runs the same check before every start.
That recheck uses the filesystem list from the config file (the same
default list when you did not change it) and a 50 GiB minimum, so a later
job is not started on a full disk. Setup calls the check with `bash`. The
script does not have to be marked executable for that to work. If the check
script is missing or `bash` cannot read it, setup stops with an error.

## Register the runner

Create a registration token when you are ready to run the script. It expires
after one hour. Do not commit it.

Repo runner (default URL):

```bash
gh api --method POST \
  -H "Accept: application/vnd.github+json" \
  /repos/Future-Gadget-Laboratories/FutureGadgetDatabases/actions/runners/registration-token \
  --jq .token
```

Org runner (every repository in Future-Gadget-Laboratories can use it unless
a runner group says otherwise):

```bash
gh api --method POST \
  -H "Accept: application/vnd.github+json" \
  /orgs/Future-Gadget-Laboratories/actions/runners/registration-token \
  --jq .token
```

On the build machine, from a checkout of this repository, put the token in
a root-owned file. Mode `0400` or `0600` is required. A file saved on
Windows, with a carriage return at the end of the line, still works.

```bash
umask 077
printf '%s\n' "$TOKEN" > /root/runner.token
sudo ./build/fgdb/runner/setup-runner.sh --token-file /root/runner.token
```

You can also pass the token on standard input. When stdin is a terminal,
the script hides what you type:

```bash
sudo ./build/fgdb/runner/setup-runner.sh --token-stdin
```

The script gives the token to the runner's registration program in the
environment variable `ACTIONS_RUNNER_INPUT_TOKEN`. It is not one of that
program's command-line arguments, so it does not show up in a process
listing.

`--token` still works, but the shell stores it in history and other people
can see it in the process list. Prefer `--token-file` or `--token-stdin`.

Org registration:

```bash
sudo ./build/fgdb/runner/setup-runner.sh \
  --token-file /root/runner.token \
  --url https://github.com/Future-Gadget-Laboratories
```

A different runner name:

```bash
sudo FGDB_RUNNER_NAME=fgdb-build ./build/fgdb/runner/setup-runner.sh \
  --token-file /root/runner.token
```

A later run without a token is safe once
`/opt/fgdb-actions-runner/.runner` exists. Pass `--replace` to register
again; that needs a new token. `--dry-run` prints the plan only.

Confirm in GitHub that the runner is Idle and has labels `self-hosted` and
`fgdb-build`. The workflow asks for both labels, so a runner that only has
`self-hosted` will not pick up the job.

The runner needs outbound HTTPS to `github.com` (source, Actions, Bazelisk,
the `cockroachdb/bazel` release), `storage.googleapis.com` (public Bazel
toolchains and prebuilt c-deps), and the module mirrors Bazel fetches on a
cold build. It does not need the private Cockroach builder image registry.

## Versions and checksums

The runner package and Bazelisk are pinned in
`build/fgdb/runner/checksums.txt`. Setup refuses a version that is not in
that file.

`build/fgdb/runner/update-checksums.sh` is how you refresh a pin. It
downloads the file and checks the checksum against the one the publisher
posted. For the Actions runner, that checksum is in the GitHub release
notes, between `BEGIN SHA linux-x64` and `END SHA linux-x64`. For Bazelisk,
it is the `bazelisk-linux-amd64.sha256` file attached to the release. If
those do not match the download, the script stops and leaves
`checksums.txt` unchanged. A download by itself is not enough.

## After it is installed

Cut the release as described in [RELEASING.md](RELEASING.md). The first
compile is a `workflow_dispatch` with `publish` left false, or a push of
tag `v23.2.15-oss` on a commit that already contains this workflow.
