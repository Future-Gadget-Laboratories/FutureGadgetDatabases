<!-- Copyright 2026 Future Gadget Laboratories.

Licensed under the Apache License, Version 2.0. See licenses/APL.txt. -->

# pgcrypto cipher conformance vectors

**Status: RECORDED and APPROVED (D41, Vincent, 2026-11-18 8:43 PM MT); default stays OFF.**

`vectors.tsv` records deterministic AES-CBC known-answer inputs and outputs for
this implementation. Regenerate it from this directory with
`./gen-vectors.sh`; the script uses fixed inputs and the OpenSSL CLI only for
cipher operations. Recorded oracle version: OpenSSL 3.0.13 30 Jan 2024
(`OpenSSL 3.0.13 30 Jan 2024 (Library: OpenSSL 3.0.13 30 Jan 2024)`).

| Source | Version | Date | Use / regeneration |
| --- | --- | --- | --- |
| NIST SP 800-38A, F.2 | SP 800-38A | 2001 | Existing NIST known-answer tests in `cipher_test.go`; AES-CBC behavior reference. |
| OpenSSL CLI | 3.0.13 | 2024-01-30 | Independent AES-CBC cipher oracle; `./gen-vectors.sh` regenerates `vectors.tsv`. |
| PostgreSQL pgcrypto documentation | Current online docs | accessed 2026-11-18 | Semantics reference: [Raw Encryption Functions](https://www.postgresql.org/docs/current/pgcrypto.html#PGCRYPTO-RAW-ENCRYPTION-FUNCTIONS). |
| Vincent D41 decision | approval | 2026-11-18 8:43 PM MT | Vector set approved; default remains off. |

The TSV covers AES-128/192/256 with PKCS padding and `pad:none`, supported type
spellings, PKCS plaintext lengths 0/1/15/16/17/32, aligned unpadded lengths
16/32, encrypt/decrypt round trips, normalized 5/20/30-byte keys, short/exact/
long IVs, zero-IV equivalence, invalid type, key over 32 bytes, and unaligned
`pad:none` failure. Error text is taken from `cipher.go`, not an error oracle.

PostgreSQL documents IV clipping/zero-padding, the type grammar with default
CBC/PKCS behavior, and zero IV for calls without an IV. The implementation's
key zero-padding is an implementation-specific behavior not established by the
cited PostgreSQL documentation.

Still **Proposed, pending**: parity of cipher failure SQLSTATE/messages against
a stock PostgreSQL server, and any default-setting flip. No PostgreSQL server
was used as an oracle. The setting remains `false` by default.
