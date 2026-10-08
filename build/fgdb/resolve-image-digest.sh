#!/usr/bin/env bash
#
# Print the registry digest shared by one or more image tags.
# docker push on current Docker prints
#   <tag>: digest: sha256:<64 hex> size: N
# which does not match a line that must start with "digest:".
# Ask the registry instead, then the local image store.
#
#   1. docker buildx imagetools inspect --format '{{.Digest}}'
#   2. the same command's Manifest.Digest field
#   3. docker inspect --format '{{index .RepoDigests 0}}'
#
# Every tag must resolve to the same sha256 digest. Prints that digest.

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: resolve-image-digest.sh IMAGE [IMAGE...]" >&2
  exit 2
fi

normalize_digest() {
  local raw=$1
  local hex
  raw=${raw//$'\r'/}
  raw=${raw//\"/}
  raw=${raw//\'/}
  if [[ "$raw" =~ (sha256:[0-9a-f]{64}) ]]; then
    hex=${BASH_REMATCH[1]#sha256:}
    if [[ ${#hex} -eq 64 ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  fi
  return 1
}

# First Digest: line is the index. Later ones are platform manifests.
first_digest_line() {
  local text=$1
  local line
  while IFS= read -r line; do
    case "$line" in
      Digest:* | *"digest:"*)
        normalize_digest "$line" && return 0
        ;;
    esac
  done <<<"$text"
  return 1
}

digest_of() {
  local ref=$1
  local out normalized
  local fmt
  for fmt in '{{.Digest}}' '{{.Manifest.Digest}}'; do
    if out=$(docker buildx imagetools inspect "$ref" --format "$fmt" 2>/dev/null); then
      if normalized=$(normalize_digest "$out"); then
        printf '%s\n' "$normalized"
        return 0
      fi
    fi
  done
  if out=$(docker buildx imagetools inspect "$ref" 2>/dev/null); then
    if normalized=$(first_digest_line "$out"); then
      printf '%s\n' "$normalized"
      return 0
    fi
  fi
  if out=$(docker inspect --format '{{index .RepoDigests 0}}' "$ref" 2>/dev/null); then
    if normalized=$(normalize_digest "$out"); then
      printf '%s\n' "$normalized"
      return 0
    fi
  fi
  echo "resolve-image-digest: no digest for ${ref}" >&2
  return 1
}

shared=
for ref in "$@"; do
  one=$(digest_of "$ref")
  if [[ -z "$shared" ]]; then
    shared=$one
  elif [[ "$one" != "$shared" ]]; then
    echo "resolve-image-digest: ${ref} is ${one}, expected ${shared}" >&2
    exit 1
  fi
done

printf '%s\n' "$shared"
