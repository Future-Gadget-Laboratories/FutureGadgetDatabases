<!-- Copyright 2026 Future Gadget Laboratories.

Licensed under the Apache License, Version 2.0. See licenses/APL.txt. -->

# pgcrypto cipher conformance vectors: PENDING

**Status: PENDING. Approved success oracle captures have not been recorded.**

Stock CockroachDB v23.2.15 OSS returns the CCL-required error for these
builtins, so it cannot provide the flag-on success oracle. The flag-off
behavior is the behavior checked against stock. Success vectors currently use
NIST SP 800-38A; captures from an approved success oracle are pending. PAR-005
requires those captures and maintainer approval before handoff. The cipher
setting defaults to off pending those captures and approval.

The implementation and spec use an all-zero IV for `encrypt` and `decrypt`
without `_iv`; this behavior awaits confirmation by an approved success oracle.
Encryption is deterministic and not semantically secure; use `encrypt_iv` with
a random IV instead.

What is in place instead:

- `cipher_test.go` checks AES-CBC without padding against the public NIST
  SP 800-38A F.2 vectors (AES-128, AES-192, AES-256), plus round-trip,
  padding, key-length and IV-length checks.
- `pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins` (`cipher_builtins`
  subtest) checks one NIST vector, round-trips, zero-IV equivalence, and the
  cipher error cases. The flag-on error texts come from the implementation,
  not from an oracle capture. The `3node-tenant-default-configs` configurations
  skip the flag-on cases because their tenant settings cannot be changed with
  these SQL statements; they exercise the default flag-off checks.

Still to do with an approved success oracle:

1. Record `encrypt`/`decrypt` and `encrypt_iv`/`decrypt_iv` outputs for the
   supported type strings and key lengths.
2. Record SQLSTATE and message for bad type, bad key length, and `pad:none`
   with unaligned data, then compare them with the messages in `cipher.go`.
3. Confirm whether the oracle `encrypt` is deterministic (the implementation uses a
   zero IV, so it is deterministic and keeps `Immutable`).
4. Confirm key zero-padding and over-length key behaviour, and IV handling for
   IVs shorter than one block.

Record the provenance of each oracle capture as required by PAR-005.
