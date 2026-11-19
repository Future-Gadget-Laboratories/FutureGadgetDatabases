#!/usr/bin/env bash
# Copyright 2026 Future Gadget Laboratories.
#
# Licensed under the Apache License, Version 2.0. See licenses/APL.txt.
set -euo pipefail

cd "$(dirname "$0")"
command -v openssl >/dev/null || { echo 'openssl is required' >&2; exit 1; }
command -v xxd >/dev/null || { echo 'xxd is required' >&2; exit 1; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
out=vectors.tsv
printf 'id\tfunc\ttype\tkey_hex\tiv_hex\tdata_hex\texpected_hex\texpected_error\tnotes\n' > "$out"

hex_to_file() { if [[ -n $1 ]]; then printf '%s' "$1" | xxd -r -p > "$2"; else : > "$2"; fi; }
file_hex() { xxd -p -c 100000 "$1" | tr -d '\n'; }
normalize_key() {
  local key=$1
  local size=$(( ${#key} / 2 )) target
  if (( size <= 16 )); then target=16; elif (( size <= 24 )); then target=24; else target=32; fi
  printf '%s' "$key$(printf '%0.s0' $(seq 1 $((target * 2 - ${#key}))))"
}
add_vector() { printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$@" >> "$out"; }

# fixed deterministic plaintext and standard-size keys; OpenSSL supplies every ciphertext.
keys=(000102030405060708090a0b0c0d0e0f 000102030405060708090a0b0c0d0e0f1011121314151617 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f)
keylens=(128 192 256)
types=(aes aes-cbc aes/pad:pkcs aes-cbc/pad:none)
for ki in 0 1 2; do
  key=${keys[$ki]}; bits=${keylens[$ki]}
  for typ in "${types[@]}"; do
    padding=(); [[ $typ == */pad:none ]] && padding=(-nopad)
    lengths=(0 1 15 16 17 32); [[ ${#padding[@]} -gt 0 ]] && lengths=(16 32)
    for n in "${lengths[@]}"; do
      data=; if (( n > 0 )); then data=$(printf '61%.0s' $(seq 1 "$n")); fi
      hex_to_file "$data" "$tmp/in"
      iv=00000000000000000000000000000000
      openssl enc "-aes-${bits}-cbc" -K "$key" -iv "$iv" "${padding[@]}" -in "$tmp/in" -out "$tmp/ct" 2>/dev/null
      ct=$(file_hex "$tmp/ct")
      id="aes${bits}_${typ//[^a-zA-Z0-9]/_}_n${n}"
      add_vector "$id" encrypt "$typ" "$key" - "$data" "$ct" - 'openssl CLI; fixed input'
      add_vector "${id}_dec" decrypt "$typ" "$key" - "$ct" "$data" - 'openssl ciphertext decrypt round trip'
    done
  done
done

# Key normalization and IV clipping/zero-padding behavior.
for pair in '5:0001020304' '20:000102030405060708090a0b0c0d0e0f10111213' '30:000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d'; do
  key=${pair#*:}; norm=$(normalize_key "$key"); data=616263; hex_to_file "$data" "$tmp/in"
  openssl enc "-aes-$(( ${#norm} * 4 ))-cbc" -K "$norm" -iv 00000000000000000000000000000000 -in "$tmp/in" -out "$tmp/ct" 2>/dev/null
  add_vector "key${pair%%:*}" encrypt aes "$key" - "$data" "$(file_hex "$tmp/ct")" - 'short key is zero-padded as implemented'
done
for iv in 010203 000102030405060708090a0b0c0d0e0f 000102030405060708090a0b0c0d0e0f10111213; do
  ivnorm=$(printf '%-32s' "$iv" | tr ' ' 0 | cut -c1-32); [[ ${#iv} -gt 32 ]] && ivnorm=${iv:0:32}
  hex_to_file 616263 "$tmp/in"
  openssl enc -aes-128-cbc -K "${keys[0]}" -iv "$ivnorm" -in "$tmp/in" -out "$tmp/ct" 2>/dev/null
  ct=$(file_hex "$tmp/ct")
  add_vector "iv${#iv}" encrypt_iv aes "${keys[0]}" "$iv" 616263 "$ct" - 'IV zero-padded or clipped to 16 bytes'
  add_vector "iv${#iv}_dec" decrypt_iv aes "${keys[0]}" "$iv" "$ct" 616263 - 'OpenSSL ciphertext decrypt round trip'
done
add_vector zero_iv_equivalence encrypt aes "${keys[0]}" - 616263 "$(printf abc > "$tmp/in"; openssl enc -aes-128-cbc -K "${keys[0]}" -iv 00000000000000000000000000000000 -in "$tmp/in" 2>/dev/null | xxd -p -c 100000 | tr -d '\n')" - 'matches encrypt_iv with zero IV'
add_vector unaligned_error encrypt aes/pad:none "${keys[0]}" - 616263 - 'data length 3 is not a multiple of the AES block size 16' 'message from implementation, PG parity Proposed, pending'
add_vector bad_type encrypt des "${keys[0]}" - 616263 - 'cipher type "des" is not supported' 'message from implementation'
add_vector long_key_error encrypt aes "$(printf '%064s' '' | tr ' ' 0)00" - 616263 - 'invalid AES key length 33: key must be 1 to 32 bytes' 'message from implementation'
