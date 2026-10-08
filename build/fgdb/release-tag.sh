#!/usr/bin/env bash
#
# Choose the GitHub Release tag and the GHCR tags for one FGDB publish.
#
# v<version>-oss is the existing release. The tag and the image tags
# :v<version>-oss and :v<minor>-oss stay on that path.
#
# v<version>-fgdb.<N> (N is an integer) publishes that exact git tag.
# Its image tags are :<release-tag> and :sha-<source sha> only.
# The database version string is still the one in pkg/build/version.txt.

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: release-tag.sh select --event EVENT --ref-name NAME --dispatch-ref REF
       release-tag.sh check --version VERSION --tag TAG
       release-tag.sh package-tag --version VERSION
       release-tag.sh image-tags --repository REPO --version VERSION --release-tag TAG --sha SHA --push-minor-alias VALUE
       release-tag.sh title --version VERSION --release-tag TAG
       release-tag.sh pin --version VERSION --release-tag TAG --pin PIN
       release-tag.sh notes --version VERSION --release-tag TAG --source-sha SHA --workflow-sha SHA --workflow-run URL --repository REPO --push-minor-alias VALUE --verify-log PATH --checksums PATH --out PATH
EOF
  exit 2
}

version_ok() {
  local version=$1 body major minor patch extra
  [[ "$version" == v[0-9]* ]] || return 1
  body=${version#v}
  extra=""
  IFS=. read -r major minor patch extra <<<"$body"
  [[ "$major" =~ ^[0-9]+$ ]] || return 1
  [[ "$minor" =~ ^[0-9]+$ ]] || return 1
  [[ "$patch" =~ ^[0-9]+$ ]] || return 1
  [[ -z "$extra" ]] || return 1
  [[ "$body" == "${major}.${minor}.${patch}" ]]
}

# A tag vX.Y.Z-fgdb.N whose N is an integer. The version part is not
# compared to version.txt here.
fgdb_shape_ok() {
  local tag=$1 version_part suffix
  [[ "$tag" == *-fgdb.* ]] || return 1
  version_part=${tag%-fgdb.*}
  suffix=${tag#"${version_part}-fgdb."}
  [[ "$suffix" =~ ^[0-9]+$ ]] || return 1
  [[ "$tag" == "${version_part}-fgdb.${suffix}" ]] || return 1
  version_ok "$version_part"
}

require_version() {
  local version=$1
  if ! version_ok "$version"; then
    echo "release-tag: unexpected database version (${version})" >&2
    exit 1
  fi
}

# default, ${version}-oss, or ${version}-fgdb.N.
accept_tag() {
  local version=$1 tag=$2 version_part
  require_version "$version"
  if [[ "$tag" == "default" || "$tag" == "${version}-oss" ]]; then
    return 0
  fi
  if fgdb_shape_ok "$tag"; then
    version_part=${tag%-fgdb.*}
    if [[ "$version_part" != "$version" ]]; then
      echo "release-tag: ${tag} does not match database version ${version}" >&2
      exit 1
    fi
    return 0
  fi
  echo "release-tag: refusing release tag ${tag}" >&2
  exit 1
}

resolved_tag() {
  local version=$1 tag=$2
  accept_tag "$version" "$tag"
  if [[ "$tag" == "default" ]]; then
    printf '%s\n' "${version}-oss"
  else
    printf '%s\n' "$tag"
  fi
}

is_fgdb_release() {
  local tag=$1
  [[ "$tag" == *-fgdb.* ]]
}

require_sha() {
  local sha=$1
  if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo "release-tag: refusing source sha" >&2
    exit 1
  fi
}

require_token() {
  local label=$1 value=$2
  if [[ -z "$value" || "$value" == *" "* || "$value" == *$'\n'* || "$value" == *$'\t'* ]]; then
    echo "release-tag: refusing ${label}" >&2
    exit 1
  fi
}

# Prints IMAGE_VERSION= and TAG= lines.
# Oss: :<version>-oss, :sha-<sha>, and :<minor>-oss when the alias knob is true.
# fgdb: :<release-tag> and :sha-<sha> only.
emit_image_plan() {
  local repository=$1 version=$2 release_tag=$3 sha=$4 push_minor=$5
  local minor primary sha_tag alias image_version
  require_token repository "$repository"
  require_sha "$sha"
  if [[ "$release_tag" == "default" ]]; then
    echo "release-tag: image tags need the resolved release tag" >&2
    exit 1
  fi
  accept_tag "$version" "$release_tag"
  minor=${version%.*}
  sha_tag="${repository}:sha-${sha}"
  if is_fgdb_release "$release_tag"; then
    primary="${repository}:${release_tag}"
    image_version=$release_tag
    if [[ "$primary" == "${repository}:${version}-oss" || "$primary" == "${repository}:${minor}-oss" ]]; then
      echo "release-tag: refusing image tag ${primary} for ${release_tag}" >&2
      exit 1
    fi
    if [[ "$sha_tag" == "${repository}:${version}-oss" || "$sha_tag" == "${repository}:${minor}-oss" ]]; then
      echo "release-tag: refusing image tag ${sha_tag} for ${release_tag}" >&2
      exit 1
    fi
  else
    primary="${repository}:${version}-oss"
    image_version="${version}-oss"
    if [[ "$release_tag" != "${version}-oss" ]]; then
      echo "release-tag: oss image tags require release tag ${version}-oss" >&2
      exit 1
    fi
  fi
  printf 'IMAGE_VERSION=%s\n' "$image_version"
  printf 'TAG=%s\n' "$primary"
  printf 'TAG=%s\n' "$sha_tag"
  if ! is_fgdb_release "$release_tag" && [[ "$push_minor" == "true" ]]; then
    alias="${repository}:${minor}-oss"
    printf 'TAG=%s\n' "$alias"
  fi
}

select_cmd() {
  local event="" ref_name="" dispatch_ref="" candidate
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --event)
        event=${2-}
        shift 2
        ;;
      --ref-name)
        ref_name=${2-}
        shift 2
        ;;
      --dispatch-ref)
        dispatch_ref=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  case "$event" in
    push)
      candidate=$ref_name
      ;;
    workflow_dispatch)
      candidate=$dispatch_ref
      candidate=${candidate#refs/tags/}
      ;;
    *)
      echo "release-tag: unexpected event ${event}" >&2
      exit 1
      ;;
  esac
  if [[ "$candidate" == *-fgdb.* ]]; then
    if ! fgdb_shape_ok "$candidate"; then
      echo "release-tag: refusing ${candidate}" >&2
      echo "A v*-fgdb.* publish tag must look like v23.2.15-fgdb.1 (the last part is an integer)." >&2
      exit 1
    fi
    printf '%s\n' "$candidate"
    return 0
  fi
  printf '%s\n' default
}

check_cmd() {
  local version="" tag=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        version=${2-}
        shift 2
        ;;
      --tag)
        tag=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  if [[ -z "$version" || -z "$tag" ]]; then
    usage
  fi
  accept_tag "$version" "$tag"
}

package_tag_cmd() {
  local version="" requested
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        version=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  if [[ -z "$version" ]]; then
    usage
  fi
  if [[ -z "${FGDB_RELEASE_TAG+x}" ]]; then
    requested=default
  else
    requested=$FGDB_RELEASE_TAG
    if [[ -z "$requested" ]]; then
      echo "release-tag: FGDB_RELEASE_TAG is set but empty" >&2
      exit 1
    fi
  fi
  resolved_tag "$version" "$requested"
}

image_tags_cmd() {
  local repository="" version="" release_tag="" sha="" push_minor=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repository)
        repository=${2-}
        shift 2
        ;;
      --version)
        version=${2-}
        shift 2
        ;;
      --release-tag)
        release_tag=${2-}
        shift 2
        ;;
      --sha)
        sha=${2-}
        shift 2
        ;;
      --push-minor-alias)
        push_minor=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  if [[ -z "$repository" || -z "$version" || -z "$release_tag" || -z "$sha" ]]; then
    usage
  fi
  emit_image_plan "$repository" "$version" "$release_tag" "$sha" "$push_minor"
}

title_cmd() {
  local version="" release_tag=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        version=${2-}
        shift 2
        ;;
      --release-tag)
        release_tag=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  if [[ -z "$version" || -z "$release_tag" ]]; then
    usage
  fi
  if [[ "$release_tag" == "default" ]]; then
    echo "release-tag: title needs the resolved release tag" >&2
    exit 1
  fi
  accept_tag "$version" "$release_tag"
  if is_fgdb_release "$release_tag"; then
    printf 'CockroachDB %s OSS (includes fgdb-backup)\n' "$version"
  else
    printf 'CockroachDB %s OSS\n' "$version"
  fi
}

# One line, no trailing explanation. The digest job appends this after the
# image digest. A second publish strips any previous "CipherBank should pin "
# line and appends this one again.
pin_cmd() {
  local version="" release_tag="" pin="" minor sentence
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        version=${2-}
        shift 2
        ;;
      --release-tag)
        release_tag=${2-}
        shift 2
        ;;
      --pin)
        pin=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  if [[ -z "$version" || -z "$release_tag" || -z "$pin" ]]; then
    usage
  fi
  if [[ "$release_tag" == "default" ]]; then
    echo "release-tag: pin sentence needs the resolved release tag" >&2
    exit 1
  fi
  accept_tag "$version" "$release_tag"
  require_token pin "$pin"
  if is_fgdb_release "$release_tag"; then
    minor=${version%.*}
    # Name the fgdb tag as the tag to pin. Do not call ${version}-oss a moving name.
    printf -v sentence 'CipherBank should pin %s (tag %s is the tag to pin). This publish did not move %s-oss or the minor alias %s-oss.' \
      "$pin" "$release_tag" "$version" "$minor"
    if [[ "$sentence" == *"moving name"* ]]; then
      echo "release-tag: fgdb pin sentence must not call an oss tag a moving name" >&2
      exit 1
    fi
  else
    printf -v sentence 'CipherBank should pin %s (tag %s-oss is a moving name).' \
      "$pin" "$version"
  fi
  printf '%s\n' "$sentence"
}

notes_cmd() {
  local version="" release_tag="" source_sha="" workflow_sha="" workflow_run=""
  local repository="" push_minor="" verify_log="" checksums="" out=""
  local minor line tag image_plan
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        version=${2-}
        shift 2
        ;;
      --release-tag)
        release_tag=${2-}
        shift 2
        ;;
      --source-sha)
        source_sha=${2-}
        shift 2
        ;;
      --workflow-sha)
        workflow_sha=${2-}
        shift 2
        ;;
      --workflow-run)
        workflow_run=${2-}
        shift 2
        ;;
      --repository)
        repository=${2-}
        shift 2
        ;;
      --push-minor-alias)
        push_minor=${2-}
        shift 2
        ;;
      --verify-log)
        verify_log=${2-}
        shift 2
        ;;
      --checksums)
        checksums=${2-}
        shift 2
        ;;
      --out)
        out=${2-}
        shift 2
        ;;
      *)
        echo "release-tag: unknown argument ${1}" >&2
        exit 2
        ;;
    esac
  done
  if [[ -z "$version" || -z "$release_tag" || -z "$source_sha" || -z "$workflow_sha" || -z "$workflow_run" || -z "$repository" || -z "$verify_log" || -z "$checksums" || -z "$out" ]]; then
    usage
  fi
  if [[ ! -f "$verify_log" || ! -f "$checksums" ]]; then
    echo "release-tag: verify log or checksums file is missing" >&2
    exit 1
  fi
  require_sha "$workflow_sha"
  # Resolve the image list before writing the file. A failure here must
  # not leave a partial notes file that a later step could publish.
  image_plan=$(emit_image_plan "$repository" "$version" "$release_tag" "$source_sha" "$push_minor")
  minor=${version%.*}
  {
    if is_fgdb_release "$release_tag"; then
      echo "# CockroachDB ${version} OSS (includes fgdb-backup)"
      echo
      echo "The database binary is still cockroach-oss ${version}."
      echo "fgdb-backup is an extra program in the tarball and on PATH in the image."
      echo "This publish does not move git tag ${version}-oss or image tags ${version}-oss and ${minor}-oss."
      echo
    else
      echo "# CockroachDB ${version} OSS"
      echo
    fi
    echo "CCL-free build of \`//pkg/cmd/cockroach-oss:cockroach-oss\` plus libgeos."
    echo "The tarball binary is named \`cockroach\` so it matches the upstream archive layout."
    echo
    echo "- Source SHA: \`${source_sha}\`"
    echo "- Workflow SHA: \`${workflow_sha}\`"
    echo "- Workflow run: ${workflow_run}"
    while IFS= read -r line; do
      case "$line" in
        IMAGE_VERSION=*)
          ;;
        TAG=*)
          tag=${line#TAG=}
          if [[ "$tag" == "${repository}:${minor}-oss" ]]; then
            echo "- Image: \`${tag}\` (moving alias)"
          else
            echo "- Image: \`${tag}\`"
          fi
          ;;
        "")
          ;;
        *)
          echo "release-tag: unexpected image plan line" >&2
          exit 1
          ;;
      esac
    done <<<"$image_plan"
    echo "- Image digest: pending"
    echo
    echo '```'
    cat "$verify_log"
    echo '```'
    echo
    echo '```'
    cat "$checksums"
    echo '```'
  } >"$out"
}

main() {
  local cmd=${1-}
  if [[ -z "$cmd" ]]; then
    usage
  fi
  shift
  case "$cmd" in
    select) select_cmd "$@" ;;
    check) check_cmd "$@" ;;
    package-tag) package_tag_cmd "$@" ;;
    image-tags) image_tags_cmd "$@" ;;
    title) title_cmd "$@" ;;
    pin) pin_cmd "$@" ;;
    notes) notes_cmd "$@" ;;
    *)
      echo "release-tag: unknown command ${cmd}" >&2
      usage
      ;;
  esac
}

main "$@"
