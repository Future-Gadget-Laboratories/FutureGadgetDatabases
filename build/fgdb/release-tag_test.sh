#!/usr/bin/env bash
#
# Checks for release-tag.sh. No network and no Cockroach build.

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
plan=${root}/release-tag.sh
repo=ghcr.io/future-gadget-laboratories/futuregadgetdatabases
version=v23.2.15
sha=0bdd50bbe7fe90bfb0abf1be00d42e7f9c276178
workflow_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
fgdb_tag=v23.2.15-fgdb.1

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

expect_eq() {
  local got=$1 want=$2 label=$3
  if [[ "$got" != "$want" ]]; then
    printf 'got:\n%s\nwant:\n%s\n' "$got" "$want" >&2
    fail "$label"
  fi
}

must_fail() {
  local label=$1
  shift
  local status=0
  "$@" >/tmp/fgdb-release-tag-out.$$ 2>/tmp/fgdb-release-tag-err.$$ || status=$?
  if [[ "$status" -eq 0 ]]; then
    fail "expected failure: ${label}"
  fi
}

select_tag() {
  bash "$plan" select --event "$1" --ref-name "$2" --dispatch-ref "$3"
}

expect_eq "$(select_tag push v23.2.15-oss "")" default "oss tag push stays the oss release"
expect_eq "$(select_tag push v23.2.15-fgdb.1 "")" "$fgdb_tag" "fgdb tag push"
expect_eq "$(select_tag push v23.2.15-fgdb.2 "")" v23.2.15-fgdb.2 "second fgdb tag"
expect_eq "$(select_tag workflow_dispatch main main)" default "dispatch of main stays oss"
expect_eq "$(select_tag workflow_dispatch main v23.2.15-oss)" default "dispatch of the oss tag stays oss"
expect_eq "$(select_tag workflow_dispatch main "$sha")" default "dispatch of a sha stays oss"
expect_eq "$(select_tag workflow_dispatch main v23.2.15)" default "dispatch of v23.2.15 stays oss"
expect_eq "$(select_tag workflow_dispatch main "$fgdb_tag")" "$fgdb_tag" "dispatch ref is the fgdb tag"
expect_eq "$(select_tag workflow_dispatch main "refs/tags/${fgdb_tag}")" "$fgdb_tag" "dispatch ref may be a full tag ref"

must_fail "malformed fgdb tag push" bash "$plan" select --event push --ref-name v23.2.15-fgdb.1a --dispatch-ref ""
must_fail "empty fgdb suffix" bash "$plan" select --event push --ref-name v23.2.15-fgdb. --dispatch-ref ""
must_fail "two-part version" bash "$plan" select --event push --ref-name v23.2-fgdb.1 --dispatch-ref ""
must_fail "dispatch of a bad fgdb ref must not fall through to oss" \
  bash "$plan" select --event workflow_dispatch --ref-name main --dispatch-ref v23.2.15-fgdb.beta
must_fail "unknown event" bash "$plan" select --event pull_request --ref-name main --dispatch-ref main

bash "$plan" check --version "$version" --tag default
bash "$plan" check --version "$version" --tag "${version}-oss"
bash "$plan" check --version "$version" --tag "$fgdb_tag"
must_fail "fgdb tag for a different version" bash "$plan" check --version "$version" --tag v23.2.16-fgdb.1
must_fail "empty explicit tag" env FGDB_RELEASE_TAG= bash "$plan" package-tag --version "$version"

expect_eq "$(bash "$plan" package-tag --version "$version")" "${version}-oss" "unset env is the oss tag"
expect_eq "$(FGDB_RELEASE_TAG=default bash "$plan" package-tag --version "$version")" "${version}-oss" "default sentinel is the oss tag"
expect_eq "$(FGDB_RELEASE_TAG=${version}-oss bash "$plan" package-tag --version "$version")" "${version}-oss" "explicit oss tag"
expect_eq "$(FGDB_RELEASE_TAG=$fgdb_tag bash "$plan" package-tag --version "$version")" "$fgdb_tag" "explicit fgdb tag"
must_fail "package tag version mismatch" env FGDB_RELEASE_TAG=v23.2.16-fgdb.1 bash "$plan" package-tag --version "$version"

oss_plan=$(bash "$plan" image-tags \
  --repository "$repo" \
  --version "$version" \
  --release-tag "${version}-oss" \
  --sha "$sha" \
  --push-minor-alias true)
expect_eq "$oss_plan" "$(cat <<EOF
IMAGE_VERSION=${version}-oss
TAG=${repo}:${version}-oss
TAG=${repo}:sha-${sha}
TAG=${repo}:v23.2-oss
EOF
)" "oss image tags include the minor alias"

oss_plan_no_alias=$(bash "$plan" image-tags \
  --repository "$repo" \
  --version "$version" \
  --release-tag "${version}-oss" \
  --sha "$sha" \
  --push-minor-alias false)
expect_eq "$oss_plan_no_alias" "$(cat <<EOF
IMAGE_VERSION=${version}-oss
TAG=${repo}:${version}-oss
TAG=${repo}:sha-${sha}
EOF
)" "oss alias knob off"

fgdb_plan=$(bash "$plan" image-tags \
  --repository "$repo" \
  --version "$version" \
  --release-tag "$fgdb_tag" \
  --sha "$sha" \
  --push-minor-alias true)
expect_eq "$fgdb_plan" "$(cat <<EOF
IMAGE_VERSION=${fgdb_tag}
TAG=${repo}:${fgdb_tag}
TAG=${repo}:sha-${sha}
EOF
)" "fgdb image tags ignore the minor alias knob"

fgdb_plan_false=$(bash "$plan" image-tags \
  --repository "$repo" \
  --version "$version" \
  --release-tag "$fgdb_tag" \
  --sha "$sha" \
  --push-minor-alias false)
expect_eq "$fgdb_plan_false" "$fgdb_plan" "fgdb image tags do not depend on the alias knob"

case "$fgdb_plan" in
  *":${version}-oss"*|*:v23.2-oss*)
    fail "fgdb plan named an oss image tag"
    ;;
esac

expect_eq "$(bash "$plan" title --version "$version" --release-tag "${version}-oss")" \
  "CockroachDB ${version} OSS" "oss title"
expect_eq "$(bash "$plan" title --version "$version" --release-tag "$fgdb_tag")" \
  "CockroachDB ${version} OSS (includes fgdb-backup)" "fgdb title"

pin="${repo}@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
expect_eq "$(bash "$plan" pin --version "$version" --release-tag "${version}-oss" --pin "$pin")" \
  "CipherBank should pin ${pin} (tag ${version}-oss is a moving name)." \
  "oss pin sentence"
fgdb_pin=$(bash "$plan" pin --version "$version" --release-tag "$fgdb_tag" --pin "$pin")
expect_eq "$fgdb_pin" \
  "CipherBank should pin ${pin} (tag ${fgdb_tag} is the tag to pin). This publish did not move ${version}-oss or the minor alias v23.2-oss." \
  "fgdb pin sentence"
if [[ "$fgdb_pin" == *"moving name"* ]]; then
  fail "fgdb pin sentence called a tag a moving name"
fi
fgdb_pin_2=$(bash "$plan" pin --version "$version" --release-tag v23.2.15-fgdb.2 --pin "$pin")
expect_eq "$fgdb_pin_2" \
  "CipherBank should pin ${pin} (tag v23.2.15-fgdb.2 is the tag to pin). This publish did not move ${version}-oss or the minor alias v23.2-oss." \
  "second fgdb pin sentence"
must_fail "pin sentence for the default sentinel" \
  bash "$plan" pin --version "$version" --release-tag default --pin "$pin"
must_fail "pin sentence for a mismatched fgdb tag" \
  bash "$plan" pin --version "$version" --release-tag v23.2.16-fgdb.1 --pin "$pin"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf 'verified\n' >"${tmp}/verify.log"
printf 'deadbeef  cockroach-oss-%s.linux-amd64.tgz\n' "$version" >"${tmp}/SHA256SUMS"
run_url="https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/actions/runs/9"

bash "$plan" notes \
  --version "$version" \
  --release-tag "${version}-oss" \
  --source-sha "$sha" \
  --workflow-sha "$workflow_sha" \
  --workflow-run "$run_url" \
  --repository "$repo" \
  --push-minor-alias true \
  --verify-log "${tmp}/verify.log" \
  --checksums "${tmp}/SHA256SUMS" \
  --out "${tmp}/oss.md"

cat >"${tmp}/oss-want.md" <<EOF
# CockroachDB ${version} OSS

CCL-free build of \`//pkg/cmd/cockroach-oss:cockroach-oss\` plus libgeos.
The tarball binary is named \`cockroach\` so it matches the upstream archive layout.

- Source SHA: \`${sha}\`
- Workflow SHA: \`${workflow_sha}\`
- Workflow run: ${run_url}
- Image: \`${repo}:${version}-oss\`
- Image: \`${repo}:sha-${sha}\`
- Image: \`${repo}:v23.2-oss\` (moving alias)
- Image digest: pending

\`\`\`
verified
\`\`\`

\`\`\`
deadbeef  cockroach-oss-${version}.linux-amd64.tgz
\`\`\`
EOF
if ! cmp -s "${tmp}/oss.md" "${tmp}/oss-want.md"; then
  diff -u "${tmp}/oss-want.md" "${tmp}/oss.md" >&2 || true
  fail "oss notes changed"
fi
if grep -q 'fgdb-backup' "${tmp}/oss.md"; then
  fail "oss notes mentioned fgdb-backup"
fi

bash "$plan" notes \
  --version "$version" \
  --release-tag "$fgdb_tag" \
  --source-sha "$sha" \
  --workflow-sha "$workflow_sha" \
  --workflow-run "$run_url" \
  --repository "$repo" \
  --push-minor-alias true \
  --verify-log "${tmp}/verify.log" \
  --checksums "${tmp}/SHA256SUMS" \
  --out "${tmp}/fgdb.md"

grep -q "The database binary is still cockroach-oss ${version}." "${tmp}/fgdb.md" \
  || fail "fgdb notes missing the database version sentence"
grep -q "fgdb-backup is an extra program in the tarball and on PATH in the image." "${tmp}/fgdb.md" \
  || fail "fgdb notes missing the fgdb-backup sentence"
grep -q "includes fgdb-backup" "${tmp}/fgdb.md" \
  || fail "fgdb notes title does not say it includes fgdb-backup"
grep -q -- "- Image: \`${repo}:${fgdb_tag}\`" "${tmp}/fgdb.md" \
  || fail "fgdb notes missing the release image tag"
grep -q -- "- Image: \`${repo}:sha-${sha}\`" "${tmp}/fgdb.md" \
  || fail "fgdb notes missing the sha image tag"
if grep -q -- ":${version}-oss" "${tmp}/fgdb.md" || grep -q -- ':v23.2-oss' "${tmp}/fgdb.md"; then
  fail "fgdb notes listed an oss image tag"
fi
if grep -q 'moving alias' "${tmp}/fgdb.md"; then
  fail "fgdb notes listed the moving alias"
fi

# Packaging still names the tarball from version.txt. The release tag is
# only the RELEASE_TAG line in build-info.env.
src=${tmp}/src
out=${tmp}/out
mkdir -p "${src}/pkg/build" "${src}/lib" "${src}/licenses" "$out"
printf '%s\n' "$version" >"${src}/pkg/build/version.txt"
printf 'bin\n' >"${src}/cockroach-oss"
printf 'geos\n' >"${src}/lib/libgeos.so"
printf 'geosc\n' >"${src}/lib/libgeos_c.so"
printf 'license\n' >"${src}/LICENSE"
printf 'notice\n' >"${src}/licenses/LICENSE.txt"
printf 'third\n' >"${src}/licenses/THIRD-PARTY-NOTICES.txt"
printf 'bsl\n' >"${src}/licenses/BSL.txt"
printf 'apl\n' >"${src}/licenses/APL.txt"

package_once() {
  local dest=$1
  shift
  mkdir -p "$dest"
  env "$@" bash "${root}/package-oss-tarball.sh" "$src" "$dest" >/dev/null
}

package_once "${out}/default"
expect_eq "$(sed -n 's/^VERSION=//p' "${out}/default/build-info.env")" "$version" "packaged version"
expect_eq "$(sed -n 's/^RELEASE_TAG=//p' "${out}/default/build-info.env")" "${version}-oss" "default packaged tag"
expect_eq "$(sed -n 's/^TARBALL=//p' "${out}/default/build-info.env")" \
  "cockroach-oss-${version}.linux-amd64.tgz" "default tarball name"

package_once "${out}/fgdb" FGDB_RELEASE_TAG="$fgdb_tag"
expect_eq "$(sed -n 's/^VERSION=//p' "${out}/fgdb/build-info.env")" "$version" "fgdb packaged version"
expect_eq "$(sed -n 's/^RELEASE_TAG=//p' "${out}/fgdb/build-info.env")" "$fgdb_tag" "fgdb packaged tag"
expect_eq "$(sed -n 's/^TARBALL=//p' "${out}/fgdb/build-info.env")" \
  "cockroach-oss-${version}.linux-amd64.tgz" "fgdb tarball name stays on the database version"
version_line=$(tar -xOzf "${out}/fgdb/cockroach-oss-${version}.linux-amd64.tgz" \
  "cockroach-oss-${version}.linux-amd64/OSS-BUILD.txt" | sed -n 's/^Version: //p')
expect_eq "$version_line" "$version" "sidecar version"

if env FGDB_RELEASE_TAG=v23.2.16-fgdb.1 bash "${root}/package-oss-tarball.sh" "$src" "${out}/bad" >/dev/null 2>"${tmp}/bad.err"; then
  fail "package script accepted a mismatched fgdb tag"
fi
grep -q 'does not match database version' "${tmp}/bad.err" || fail "mismatch error was not specific"

echo "release-tag tests passed"
