#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf 'token-from-file\n' >"$tmp/token"
chmod 600 "$tmp/token"
mkdir "$tmp/bin"
cat >"$tmp/bin/stat" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"%u"*) printf '0\n' ;;
  *"%a"*) printf '600\n' ;;
esac
EOF
chmod +x "$tmp/bin/stat"
# shellcheck source=token-input.sh
source "$ROOT/token-input.sh"
PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/token"
[[ "$REPLY_TOKEN" == token-from-file ]]
PATH="$tmp/bin:$PATH" read_registration_token_stdin <<<'token-from-stdin'
[[ "$REPLY_TOKEN" == token-from-stdin ]]
ln -s "$tmp/token" "$tmp/link"
if PATH="$tmp/bin:$PATH" read_registration_token_file "$tmp/link"; then
  exit 1
fi
printf 'token input tests passed\n'
