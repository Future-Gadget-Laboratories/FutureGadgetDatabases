#!/usr/bin/env bash
#
# Prepare labcluster3 as the FutureGadgetDatabases Actions runner.
#
# The runner is a dedicated user (fgdb-runner), in a systemd slice that leaves
# CPU and RAM for slurmctld, slurmd, and Ollama. The Bazel cache, output base,
# and runner work directory stay on a local disk with at least 150 GiB free.
# When the root filesystem qualifies, they go in /var/cache/fgdb. Network
# filesystems and backup mounts are never chosen.
#
# Idempotent. Prints the plan, then applies it. Pass --dry-run to stop after
# the plan. A registration token is required only when the runner is not
# already configured (or when --replace is set). The token is never written
# into the plan, the env file, or the logs.
#
#   sudo ./build/fgdb/runner/setup-labcluster3.sh --token "$TOKEN"
#
# Token (one hour, do not commit it). Repo runner, default:
#   gh api --method POST \
#     -H "Accept: application/vnd.github+json" \
#     /repos/Future-Gadget-Laboratories/FutureGadgetDatabases/actions/runners/registration-token \
#     --jq .token
# Org runner (pass --url https://github.com/Future-Gadget-Laboratories):
#   gh api --method POST \
#     -H "Accept: application/vnd.github+json" \
#     /orgs/Future-Gadget-Laboratories/actions/runners/registration-token \
#     --jq .token

set -euo pipefail

# ---- defaults (change these, or pass the matching flags) ----
RUNNER_USER=fgdb-runner
RUNNER_NAME=labcluster3-fgdb
RUNNER_LABELS=fgdb-build
RUNNER_URL=https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases
RUNNER_VERSION=2.338.0
RUNNER_SHA256=af4b794c1bc41d73d40535e3fe092a39f9679cd8d965954c2aca25a05ca41d32
BAZELISK_VERSION=1.29.0
BAZELISK_SHA256=5a408715e932c0250d28bd84555f12edbf70117de42f9181691c736eacc4a992
RUNNER_INSTALL_DIR=/opt/fgdb-actions-runner
ENV_FILE=/etc/fgdb/runner.env
# 24 of 36 threads; 80G soft / 96G hard of 123 GB. See docs/fgdb/RUNNER.md.
CPU_QUOTA=2400%
MEMORY_HIGH=80G
MEMORY_MAX=96G
LOCAL_CPU=24
LOCAL_RAM_MB=81920
MIN_FREE_BYTES=$((150 * 1024 * 1024 * 1024))
MIN_RAM_KB=$((30 * 1024 * 1024))

DRY_RUN=0
REPLACE=0
ASSUME_YES=0
ALLOW_SMALL_HOST=0
CACHE_DIR_OVERRIDE=
TOKEN=${GH_RUNNER_REGISTRATION_TOKEN:-}

usage() {
  cat <<EOF
usage: setup-labcluster3.sh [options]

  --token TOKEN          GitHub Actions registration token (or GH_RUNNER_REGISTRATION_TOKEN)
  --url URL              Runner URL (repo default, or the org URL)
  --name NAME            Runner name (default ${RUNNER_NAME})
  --labels LABELS        Extra labels, comma-separated (default ${RUNNER_LABELS})
  --cache-dir DIR        Bazel cache parent directory (skip disk search)
  --runner-version VER   actions/runner version (default ${RUNNER_VERSION})
  --cpu-quota QUOTA      systemd CPUQuota (default ${CPU_QUOTA})
  --memory-high SIZE     systemd MemoryHigh (default ${MEMORY_HIGH})
  --memory-max SIZE      systemd MemoryMax (default ${MEMORY_MAX})
  --local-cpu N          Bazel --local_cpu_resources written to ${ENV_FILE}
  --local-ram-mb N       Bazel --local_ram_resources (MB) written to ${ENV_FILE}
  --replace              Reconfigure an existing runner (requires a token)
  --allow-small-host     Do not refuse hosts with under 30 GiB RAM
  --yes                  Do not pause before applying the plan
  --dry-run              Print the plan and exit
  -h, --help             Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --token)
      TOKEN=$2
      shift 2
      ;;
    --url)
      RUNNER_URL=$2
      shift 2
      ;;
    --name)
      RUNNER_NAME=$2
      shift 2
      ;;
    --labels)
      RUNNER_LABELS=$2
      shift 2
      ;;
    --cache-dir)
      CACHE_DIR_OVERRIDE=$2
      shift 2
      ;;
    --runner-version)
      RUNNER_VERSION=$2
      shift 2
      ;;
    --cpu-quota)
      CPU_QUOTA=$2
      shift 2
      ;;
    --memory-high)
      MEMORY_HIGH=$2
      shift 2
      ;;
    --memory-max)
      MEMORY_MAX=$2
      shift 2
      ;;
    --local-cpu)
      LOCAL_CPU=$2
      shift 2
      ;;
    --local-ram-mb)
      LOCAL_RAM_MB=$2
      shift 2
      ;;
    --replace)
      REPLACE=1
      shift
      ;;
    --allow-small-host)
      ALLOW_SMALL_HOST=1
      shift
      ;;
    --yes)
      ASSUME_YES=1
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

mem_kb() {
  awk '/^MemTotal:/ { print $2 }' /proc/meminfo
}

avail_bytes() {
  local path=$1
  df -B1 -P "$path" | awk 'NR == 2 { print $4 }'
}

# 0 when this mount must not hold the Bazel cache.
# Network and backup disks are rejected even when they have the most free space.
mount_unsuitable() {
  local target=$1 fstype=$2
  case "$fstype" in
    tmpfs|devtmpfs|overlay|squashfs|efivarfs|proc|sysfs|cgroup2|autofs|nsfs|ramfs) return 0 ;;
    nfs|nfs3|nfs4|nfsd|fuse.nfs|fuse.nfs4) return 0 ;;
    cifs|smb|smb2|smb3|smbfs) return 0 ;;
    sshfs|fuse.sshfs) return 0 ;;
    glusterfs|fuse.glusterfs) return 0 ;;
    ceph|cephfs|fuse.ceph|fuse.ceph-fuse) return 0 ;;
    lustre|gpfs|afs|fuse.afs) return 0 ;;
    davfs|fuse.davfs|s3fs|fuse.s3fs|fuse.rclone) return 0 ;;
    moosefs|fuse.moosefs|orangefs|pvfs2|9p) return 0 ;;
  esac
  case "$target" in
    /boot|/boot/*|/snap/*) return 0 ;;
    /mnt/*backup*) return 0 ;;
  esac
  return 1
}

# Prints "cache_root free_bytes".
# Prefer /var/cache/fgdb when the root filesystem is local and has >= 150 GiB.
# Otherwise use the local filesystem with the most free space.
choose_cache_root() {
  local best_target="" best_avail=0 root_avail=-1
  local line target avail fstype
  if ! command -v findmnt >/dev/null 2>&1; then
    die "findmnt is required to choose a cache disk"
  fi
  while IFS= read -r line; do
    [[ "$line" =~ TARGET=\"([^\"]*)\"[[:space:]]AVAIL=\"([0-9]+)\"[[:space:]]FSTYPE=\"([^\"]*)\" ]] || continue
    target=${BASH_REMATCH[1]}
    avail=${BASH_REMATCH[2]}
    fstype=${BASH_REMATCH[3]}
    if mount_unsuitable "$target" "$fstype"; then
      continue
    fi
    if [[ "$target" == "/" && "$avail" -gt "$root_avail" ]]; then
      root_avail=$avail
    fi
    if (( avail > best_avail )); then
      best_avail=$avail
      best_target=$target
    fi
  done < <(findmnt -nbP -o TARGET,AVAIL,FSTYPE)

  if (( root_avail >= MIN_FREE_BYTES )); then
    printf '%s %s\n' /var/cache/fgdb "$root_avail"
    return
  fi
  if [[ -z "$best_target" ]]; then
    die "could not find a local filesystem for the Bazel cache. Network filesystems and /mnt/*backup* mounts are ignored. Pass --cache-dir to a local directory with at least 150 GiB free."
  fi
  if [[ "$best_target" == "/" ]]; then
    printf '%s %s\n' /var/cache/fgdb "$best_avail"
  else
    printf '%s %s\n' "${best_target%/}/fgdb" "$best_avail"
  fi
}

host_short=$(hostname -s 2>/dev/null || hostname)
case "$host_short" in
  labcluster0*)
    die "refusing to install the fgdb-build runner on ${host_short}. Use labcluster3, not labcluster0-test."
    ;;
esac

ram_kb=$(mem_kb)
if (( ram_kb < MIN_RAM_KB )) && (( ALLOW_SMALL_HOST == 0 )); then
  die "this host has $((ram_kb / 1024 / 1024)) GiB RAM; the OSS build needs at least 30 GiB. Refusing (labcluster0-test is 14 GiB). Pass --allow-small-host only to test the script."
fi

if [[ -n "$CACHE_DIR_OVERRIDE" ]]; then
  cache_root=$CACHE_DIR_OVERRIDE
  if [[ -d "$cache_root" ]]; then
    free_bytes=$(avail_bytes "$cache_root")
    mount_probe=$cache_root
  else
    cache_parent=$(dirname "$cache_root")
    [[ -d "$cache_parent" ]] || die "cache parent ${cache_parent} does not exist"
    free_bytes=$(avail_bytes "$cache_parent")
    mount_probe=$cache_parent
  fi
  mount_line=$(findmnt -nbP -T "$mount_probe" -o TARGET,FSTYPE) \
    || die "findmnt could not describe ${mount_probe}"
  [[ "$mount_line" =~ TARGET=\"([^\"]*)\"[[:space:]]FSTYPE=\"([^\"]*)\" ]] \
    || die "could not parse findmnt output for ${mount_probe}"
  if mount_unsuitable "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"; then
    die "refusing ${cache_root}: ${BASH_REMATCH[2]} mounted at ${BASH_REMATCH[1]} is network or backup storage. The Bazel cache must be on a local disk."
  fi
else
  read -r cache_root free_bytes < <(choose_cache_root)
fi

if (( free_bytes < MIN_FREE_BYTES )); then
  die "need at least 150 GiB free for the Bazel cache; best candidate has $((free_bytes / 1024 / 1024 / 1024)) GiB (${cache_root})"
fi

disk_cache=${cache_root}/bazel-disk-cache
output_base=${cache_root}/bazel-output-base
work_dir=${cache_root}/runner-work
home_dir=/var/lib/fgdb-runner

runner_configured=0
if [[ -f "${RUNNER_INSTALL_DIR}/.runner" ]]; then
  runner_configured=1
fi

need_token=0
if (( REPLACE == 1 )) || (( runner_configured == 0 )); then
  need_token=1
fi
if (( need_token == 1 )) && [[ -z "$TOKEN" ]] && (( DRY_RUN == 0 )); then
  die "a registration token is required (pass --token or set GH_RUNNER_REGISTRATION_TOKEN). It is not required on later runs once ${RUNNER_INSTALL_DIR}/.runner exists."
fi

token_state="not set"
if [[ -n "$TOKEN" ]]; then
  token_state="set (hidden, ${#TOKEN} characters)"
fi

log "FGDB runner setup plan"
log "  host:              ${host_short} ($(uname -srm))"
log "  ram:               $((ram_kb / 1024)) MiB"
log "  mode:              $([[ $DRY_RUN -eq 1 ]] && echo dry-run || echo apply)"
log "  user:              ${RUNNER_USER} (home ${home_dir}, system account)"
log "  runner name:       ${RUNNER_NAME}"
log "  runner labels:     self-hosted (automatic), ${RUNNER_LABELS}"
log "  runner URL:        ${RUNNER_URL}"
log "  runner version:    ${RUNNER_VERSION}"
log "  install dir:       ${RUNNER_INSTALL_DIR}"
log "  work dir:          ${work_dir}"
log "  already configured: $([[ $runner_configured -eq 1 ]] && echo yes || echo no)"
log "  replace config:    $([[ $REPLACE -eq 1 ]] && echo yes || echo no)"
log "  registration token: ${token_state}"
log "  bazelisk:          ${BAZELISK_VERSION} -> /usr/local/bin/bazel"
log "  packages:          build tools, docker.io, jq, python3 (no dist-upgrade, no ccache, no Node from apt)"
log "  bazel disk cache:  ${disk_cache}"
log "  bazel output base: ${output_base}"
log "  free space seen:   $((free_bytes / 1024 / 1024 / 1024)) GiB"
log "  systemd slice:     fgdb-runner.slice CPUQuota=${CPU_QUOTA} MemoryHigh=${MEMORY_HIGH} MemoryMax=${MEMORY_MAX}"
log "  bazel resources:   local_cpu=${LOCAL_CPU} local_ram_mb=${LOCAL_RAM_MB} (${ENV_FILE})"
log "  coexistence:       slice caps the runner so slurmctld, slurmd, and Ollama keep the remaining CPU and RAM"
log "  will not:          print the token, dist-upgrade the host, or install ccache"

if (( DRY_RUN == 1 )); then
  log "Dry run only. No changes made."
  exit 0
fi

if [[ "$(id -u)" -ne 0 ]]; then
  die "re-run as root to apply this plan (sudo)."
fi

if [[ -t 1 && "$ASSUME_YES" -eq 0 ]]; then
  log "Applying in 5 seconds. Ctrl-C to stop, or pass --yes to skip this pause."
  sleep 5
fi

export DEBIAN_FRONTEND=noninteractive

if command -v ccache >/dev/null 2>&1; then
  log "warning: ccache is already installed. Cockroach dev builds are not tested with it; this script will not use it."
fi

log "Installing packages"
apt-get update
apt-get install -y --no-install-recommends \
  build-essential \
  ca-certificates \
  curl \
  git \
  gnupg \
  python3 \
  python-is-python3 \
  unzip \
  zip \
  patch \
  pkg-config \
  autoconf \
  bison \
  flex \
  libncurses-dev \
  libssl-dev \
  zlib1g-dev \
  patchelf \
  cmake \
  rsync \
  wget \
  xz-utils \
  file \
  binutils \
  jq \
  docker.io

if ! id "$RUNNER_USER" >/dev/null 2>&1; then
  log "Creating user ${RUNNER_USER}"
  useradd \
    --system \
    --create-home \
    --home-dir "$home_dir" \
    --shell /bin/bash \
    --user-group \
    "$RUNNER_USER"
else
  log "User ${RUNNER_USER} already exists"
fi

if getent group docker >/dev/null 2>&1; then
  usermod -aG docker "$RUNNER_USER"
else
  die "docker group is missing after installing docker.io"
fi

systemctl enable --now docker

log "Installing Bazelisk ${BAZELISK_VERSION}"
install -d -m 0755 /usr/local/lib/fgdb
bazelisk_stamp=/usr/local/lib/fgdb/bazelisk.version
if [[ ! -x /usr/local/bin/bazel ]] || [[ "$(cat "$bazelisk_stamp" 2>/dev/null || true)" != "$BAZELISK_VERSION" ]]; then
  tmp_bzl=$(mktemp)
  curl -fsSL \
    "https://github.com/bazelbuild/bazelisk/releases/download/v${BAZELISK_VERSION}/bazelisk-linux-amd64" \
    -o "$tmp_bzl"
  echo "${BAZELISK_SHA256}  ${tmp_bzl}" | sha256sum -c -
  install -m 0755 "$tmp_bzl" /usr/local/bin/bazel
  rm -f "$tmp_bzl"
  printf '%s\n' "$BAZELISK_VERSION" >"$bazelisk_stamp"
else
  log "Bazelisk ${BAZELISK_VERSION} already installed"
fi

# Bazelisk 1.10.1 (the version in build/bootstrap/bootstrap-debian.sh) cannot
# resolve .bazelversion "cockroachdb/6.2.1". 1.29.0 can.

log "Preparing cache directories"
install -d -m 0755 -o "$RUNNER_USER" -g "$RUNNER_USER" \
  "$cache_root" "$disk_cache" "$output_base" "$work_dir"
install -d -m 0755 /etc/fgdb

cat >"$ENV_FILE" <<EOF
# Written by setup-labcluster3.sh. Sourced by the runner service.
FGDB_BAZEL_DISK_CACHE=${disk_cache}
FGDB_BAZEL_OUTPUT_BASE=${output_base}
FGDB_LOCAL_CPU=${LOCAL_CPU}
FGDB_LOCAL_RAM_MB=${LOCAL_RAM_MB}
FGDB_RUNNER_WORK=${work_dir}
EOF
chmod 0644 "$ENV_FILE"

log "Installing systemd slice"
cat >/etc/systemd/system/fgdb-runner.slice <<EOF
[Unit]
Description=FutureGadgetDatabases GitHub Actions runner slice
Documentation=https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/blob/main/docs/fgdb/RUNNER.md

[Slice]
CPUAccounting=yes
MemoryAccounting=yes
CPUQuota=${CPU_QUOTA}
MemoryHigh=${MEMORY_HIGH}
MemoryMax=${MEMORY_MAX}
EOF
chmod 0644 /etc/systemd/system/fgdb-runner.slice

log "Installing GitHub Actions runner ${RUNNER_VERSION}"
install -d -m 0755 -o "$RUNNER_USER" -g "$RUNNER_USER" "$RUNNER_INSTALL_DIR"
version_stamp=${RUNNER_INSTALL_DIR}/.fgdb-runner-version
if [[ "$(cat "$version_stamp" 2>/dev/null || true)" != "$RUNNER_VERSION" ]]; then
  if [[ -x ${RUNNER_INSTALL_DIR}/svc.sh ]]; then
    "${RUNNER_INSTALL_DIR}/svc.sh" stop || true
  fi
  tmp_runner=$(mktemp)
  curl -fsSL \
    "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" \
    -o "$tmp_runner"
  echo "${RUNNER_SHA256}  ${tmp_runner}" | sha256sum -c -
  tar -C "$RUNNER_INSTALL_DIR" -xzf "$tmp_runner"
  rm -f "$tmp_runner"
  printf '%s\n' "$RUNNER_VERSION" >"$version_stamp"
  chown -R "${RUNNER_USER}:${RUNNER_USER}" "$RUNNER_INSTALL_DIR"
else
  log "Runner package ${RUNNER_VERSION} already unpacked"
fi

if [[ -x ${RUNNER_INSTALL_DIR}/bin/installdependencies.sh ]]; then
  "${RUNNER_INSTALL_DIR}/bin/installdependencies.sh" || log "warning: runner installdependencies.sh returned non-zero; continuing"
fi

if (( runner_configured == 0 )) || (( REPLACE == 1 )); then
  log "Registering runner (token not printed)"
  # config.sh is relative to the runner directory. runuser does not print the token.
  # $1 and $@ are expanded by the bash -c script, not by this shell.
  # shellcheck disable=SC2016
  runuser -u "$RUNNER_USER" -- bash -c \
    'cd "$1" && shift && exec ./config.sh "$@"' \
    bash "$RUNNER_INSTALL_DIR" \
    --unattended \
    --url "$RUNNER_URL" \
    --token "$TOKEN" \
    --name "$RUNNER_NAME" \
    --labels "$RUNNER_LABELS" \
    --work "$work_dir" \
    --replace
else
  log "Runner already configured; leaving registration in place"
fi

log "Installing systemd service"
# svc.sh install is not strictly idempotent; skip when a unit for this name exists.
existing_unit=$(systemctl list-unit-files --no-legend 'actions.runner.*.service' 2>/dev/null | awk '{print $1}' | grep -F -m 1 ".${RUNNER_NAME}.service" || true)
if [[ -z "$existing_unit" ]]; then
  ( cd "$RUNNER_INSTALL_DIR" && ./svc.sh install "$RUNNER_USER" )
  existing_unit=$(systemctl list-unit-files --no-legend 'actions.runner.*.service' | awk '{print $1}' | grep -F -m 1 ".${RUNNER_NAME}.service" || true)
fi
if [[ -z "$existing_unit" ]]; then
  die "could not find the systemd unit for runner ${RUNNER_NAME}"
fi

dropin_dir="/etc/systemd/system/${existing_unit}.d"
install -d -m 0755 "$dropin_dir"
cat >"${dropin_dir}/10-fgdb-limits.conf" <<EOF
[Unit]
After=network-online.target docker.service
Wants=network-online.target

[Service]
Slice=fgdb-runner.slice
Nice=10
CPUWeight=50
IOWeight=50
LimitNOFILE=1048576
SupplementaryGroups=docker
EnvironmentFile=-${ENV_FILE}
EOF
chmod 0644 "${dropin_dir}/10-fgdb-limits.conf"

systemctl daemon-reload
systemctl enable "$existing_unit"
if systemctl is-active --quiet "$existing_unit"; then
  log "Restarting ${existing_unit} so the slice and environment apply"
  systemctl restart "$existing_unit"
else
  log "Starting ${existing_unit}"
  systemctl start "$existing_unit"
fi

log "Done."
log "  unit:   ${existing_unit}"
log "  slice:  fgdb-runner.slice (${CPU_QUOTA}, memory high ${MEMORY_HIGH}, max ${MEMORY_MAX})"
log "  cache:  ${disk_cache}"
log "  output: ${output_base}"
log "Confirm the runner is Idle in GitHub with labels self-hosted and ${RUNNER_LABELS}."
log "Do not point a build at the labcluster0-test / self-hosted-ci runner."
