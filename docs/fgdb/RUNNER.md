# labcluster3 Actions runner

The `cockroach-oss` compile runs on a self-hosted GitHub Actions runner with
the labels `self-hosted` and `fgdb-build`. That runner is labcluster3
(Xeon, 18 cores / 36 threads, 123 GB RAM, Ubuntu 26.04).

Do not register this runner on labcluster0-test, and do not point the build
at the `self-hosted-ci` label. That machine has 14 GB of RAM and is the
shared org CI runner.

The script is `build/fgdb/runner/setup-labcluster3.sh`. It is idempotent.
It prints a plan, then applies it. `--dry-run` stops after the plan.

## What it sets up

- A system user, `fgdb-runner`. The runner service runs as that user.
  `./dev` refuses to run as root.
- Docker (`docker.io` from Ubuntu) and the `docker` group. The script does
  not `dist-upgrade` the host, so Slurm and Ollama are left on the packages
  already installed.
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

## CPU, memory, and the other services

labcluster3 also runs `slurmctld`, `slurmd`, and Ollama. The slice is a hard
cap on the runner cgroup, not a nice-only hint:

| Knob | Default | Leaves for Slurm, Ollama, and the OS |
| --- | --- | --- |
| `CPUQuota` | `2400%` (24 threads) | 12 threads |
| `MemoryHigh` | `80G` | soft pressure above this |
| `MemoryMax` | `96G` | about 27 GB outside the cgroup |

The same numbers are written to `/etc/fgdb/runner.env` as
`FGDB_LOCAL_CPU` and `FGDB_LOCAL_RAM_MB` so Bazel does not schedule more
work than the slice allows. Change them together: the variables at the top
of `setup-labcluster3.sh`, or `--cpu-quota`, `--memory-high`, `--memory-max`,
`--local-cpu`, and `--local-ram-mb`. Re-run the script after changing them.

If the linker is killed by the cgroup, raise `MemoryMax` before retrying.
Do not remove the slice to get a green build.

## Bazel cache disk

The Bazel cache has to be on a local disk. Do not put it on a network share
or a backup disk. Network storage is slow for this build, and a backup disk
can fill up with files that were never meant to be kept.

If you leave out `--cache-dir`, the script picks the directory:

1. It uses `/var/cache/fgdb` when the root filesystem is a local disk and
   has at least 150 GiB free. That is the normal choice. On labcluster3 the
   cache is `/var/cache/fgdb`.
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

On labcluster3, from a checkout of this repository:

```bash
sudo ./build/fgdb/runner/setup-labcluster3.sh --token "$TOKEN"
```

Org registration:

```bash
sudo ./build/fgdb/runner/setup-labcluster3.sh \
  --token "$TOKEN" \
  --url https://github.com/Future-Gadget-Laboratories
```

A later run without `--token` is safe once
`/opt/fgdb-actions-runner/.runner` exists. Pass `--replace` to register
again; that needs a new token. `--dry-run` prints the plan only.

Confirm in GitHub that the runner is Idle and has labels `self-hosted` and
`fgdb-build`. The workflow asks for both labels, so a runner that only has
`self-hosted` will not pick up the job.

The runner needs outbound HTTPS to `github.com` (source, Actions, Bazelisk,
the `cockroachdb/bazel` release), `storage.googleapis.com` (public Bazel
toolchains and prebuilt c-deps), and the module mirrors Bazel fetches on a
cold build. It does not need the private Cockroach builder image registry.

## After it is installed

Cut the release as described in [RELEASING.md](RELEASING.md). The first
compile is a `workflow_dispatch` with `publish` left false, or a push of
tag `v23.2.15-oss` on a commit that already contains this workflow.
