#!/usr/bin/env bash
#
# Package a CipherBank-compatible tarball.
# The file inside the archive is named "cockroach" (upstream tgz layout).
# It is the cockroach-oss binary, not //pkg/cmd/cockroach.
#
# Archive layout:
#   cockroach-oss-<version>.linux-amd64/cockroach
#   cockroach-oss-<version>.linux-amd64/fgdb-backup   (when pkg/cmd/fgdb-backup is present)
#   cockroach-oss-<version>.linux-amd64/lib/libgeos.so
#   cockroach-oss-<version>.linux-amd64/lib/libgeos_c.so
#   cockroach-oss-<version>.linux-amd64/LICENSE
#   cockroach-oss-<version>.linux-amd64/LICENSE.txt
#   cockroach-oss-<version>.linux-amd64/THIRD-PARTY-NOTICES.txt
#   cockroach-oss-<version>.linux-amd64/licenses/...
#
# Writes OUT_DIR/<tarball> and OUT_DIR/SHA256SUMS (two spaces, sha256sum format).

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: package-oss-tarball.sh SRC_DIR OUT_DIR" >&2
  exit 2
fi

src=$1
out=$2

version=$(tr -d '[:space:]' <"${src}/pkg/build/version.txt")
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "package-oss-tarball: unexpected pkg/build/version.txt (${version})" >&2
  exit 1
fi

oss_bin="${src}/cockroach-oss"
if [[ ! -f "$oss_bin" ]]; then
  echo "package-oss-tarball: missing ${oss_bin}" >&2
  exit 1
fi
# Refuse to package the CCL entrypoint if someone passed the wrong file.
if [[ -f "${src}/cockroach" && "${src}/cockroach" -ef "$oss_bin" ]]; then
  echo "package-oss-tarball: cockroach and cockroach-oss are the same file; refusing" >&2
  exit 1
fi

for lib in libgeos.so libgeos_c.so; do
  if [[ ! -f "${src}/lib/${lib}" ]]; then
    echo "package-oss-tarball: missing ${src}/lib/${lib}" >&2
    exit 1
  fi
done

for lic in LICENSE licenses/LICENSE.txt licenses/THIRD-PARTY-NOTICES.txt licenses/BSL.txt licenses/APL.txt; do
  if [[ ! -f "${src}/${lic}" ]]; then
    echo "package-oss-tarball: missing ${src}/${lic}" >&2
    exit 1
  fi
done

prefix="cockroach-oss-${version}.linux-amd64"
tarball_name="${prefix}.tgz"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

mkdir -p "${stage}/${prefix}/lib" "${stage}/${prefix}/licenses"
install -m 0755 "$oss_bin" "${stage}/${prefix}/cockroach"
if [[ -f "${src}/pkg/cmd/fgdb-backup/go.mod" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "package-oss-tarball: go is required to build pkg/cmd/fgdb-backup" >&2
    exit 1
  fi
  echo "package-oss-tarball: building fgdb-backup"
  # The Cockroach go.mod line is "go 1.21". Releases are built with the
  # toolchain in EXPECT_GO (go1.21.12). Fail if PATH has a different one.
  want_go="${FGDB_EXPECT_GO:-go1.21.12}"
  got_go=$(go env GOVERSION)
  if [[ "$got_go" != "$want_go" ]]; then
    echo "package-oss-tarball: fgdb-backup must be built with ${want_go}, found ${got_go}" >&2
    exit 1
  fi
  (
    cd "${src}/pkg/cmd/fgdb-backup"
    CGO_ENABLED=0 GOWORK=off GOTOOLCHAIN=local GOFLAGS="${GOFLAGS:--mod=readonly}" go build -trimpath -o "${stage}/${prefix}/fgdb-backup" .
  )
fi
install -m 0644 "${src}/lib/libgeos.so" "${stage}/${prefix}/lib/libgeos.so"
install -m 0644 "${src}/lib/libgeos_c.so" "${stage}/${prefix}/lib/libgeos_c.so"
cp -a "${src}/LICENSE" "${stage}/${prefix}/LICENSE"
cp -a "${src}/licenses/LICENSE.txt" "${stage}/${prefix}/LICENSE.txt"
cp -a "${src}/licenses/THIRD-PARTY-NOTICES.txt" "${stage}/${prefix}/THIRD-PARTY-NOTICES.txt"
cp -a "${src}/licenses/." "${stage}/${prefix}/licenses/"

# licenses/CCL.txt is the license text for pkg/ccl, which is not in this
# binary. Keep it in the licenses directory so the source-tree notices stay
# intact, and say so in a sidecar the tarball also ships.
cat >"${stage}/${prefix}/OSS-BUILD.txt" <<EOF
This archive contains the cockroach-oss binary (Bazel target
//pkg/cmd/cockroach-oss:cockroach-oss), renamed to "cockroach" so it matches
the upstream CockroachDB tarball layout.

It is built without pkg/ccl and pkg/ui/distccl. licenses/CCL.txt is included
only as license text from the source tree. It is not a grant to CCL code,
and no CCL object code is in the cockroach binary.

Version: ${version}

fgdb-backup, when this archive includes it, is a separate SQL client for
logical backup and restore. It is not a CockroachDB BACKUP implementation.
The database binary in this archive is still cockroach-oss.
EOF

mkdir -p "$out"
tar -C "$stage" -czf "${out}/${tarball_name}" "$prefix"
(
  cd "$out"
  sha256sum "$tarball_name" >SHA256SUMS
)

cat >"${out}/build-info.env" <<EOF
VERSION=${version}
TARBALL=${tarball_name}
RELEASE_TAG=${version}-oss
EOF

echo "Packaged ${out}/${tarball_name}"
cat "${out}/SHA256SUMS"
