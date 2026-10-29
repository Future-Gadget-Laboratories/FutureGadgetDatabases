<!-- Copyright 2026 Future Gadget Laboratories.

Licensed under the Apache License, Version 2.0. See licenses/APL.txt. -->

# pgcrypto cipher conformance vectors: PENDING

**Status: PENDING. Stock-binary oracle vectors have not been recorded.**

PAR-005 (CRYPT-001) calls for an oracle vector table recorded from the stock
PostgreSQL-compatible v23.2.15 full binary. That recording is not available
yet, so no oracle output is in this directory or in the tests.

What is in place instead:

- `cipher_test.go` checks AES-CBC without padding against the public NIST
  SP 800-38A F.2 vectors (AES-128, AES-192, AES-256), plus round-trip,
  padding, key-length and IV-length checks.
- `pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins` (`cipher_builtins`
  subtest) checks one NIST vector, round-trips, zero-IV equivalence, and the
  cipher error cases. All error texts in the file come from the implementation,
  not from a recording.

Still to do once the build host can run the stock oracle:

1. Record `encrypt`/`decrypt` and `encrypt_iv`/`decrypt_iv` outputs for the
   supported type strings and key lengths.
2. Record SQLSTATE and message for bad type, bad key length, and `pad:none`
   with unaligned data, then compare them with the messages in `cipher.go`.
3. Confirm whether stock `encrypt` is deterministic (the implementation uses a
   zero IV, so it is deterministic and keeps `Immutable`).
4. Confirm key zero-padding and over-length key behaviour, and IV handling for
   IVs shorter than one block.

Do not add values to this directory that were not produced by the stock binary.
