<!-- Copyright 2026 Future Gadget Laboratories.

Licensed under the Apache License, Version 2.0. See licenses/APL.txt. -->

> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.

# pgcrypto cipher functions

## Overview

FGDb adds opt-in AES-CBC support to the PostgreSQL-compatible raw cipher builtins `encrypt`, `encrypt_iv`, `decrypt`, and `decrypt_iv` on `cockroach-oss`. These functions take data and key as `BYTES`, an IV as `BYTES` for the `_iv` variants, and a cipher type as `STRING`. They return `BYTES`.

## Spec

[PAR-005 pgcrypto](../spec/PAR-005-pgcrypto.md#1-goal-acceptance-criteria-gate-decision) defines cipher conformance (CRYPT-001), the disabled error contract (CRYPT-002), regression coverage for existing pgcrypto builtins (CRYPT-003), and `CREATE EXTENSION pgcrypto` compatibility (CRYPT-004). It also calls for an oracle vector table and shared SQL logic coverage. The [D41 conformance record](../../../pkg/sql/sem/builtins/pgcrypto/conformance/README.md) records approval of the OpenSSL success vectors on 2026-11-18.

## How it works

1. `pgcryptoBuiltins` in [`pkg/sql/sem/builtins/pgcrypto_builtins.go`](../../../pkg/sql/sem/builtins/pgcrypto_builtins.go#L53) dispatches SQL calls to `Encrypt`, `EncryptIV`, `Decrypt`, and `DecryptIV` in [`pkg/sql/sem/builtins/pgcrypto/pgcrypto.go`](../../../pkg/sql/sem/builtins/pgcrypto/pgcrypto.go#L25). All four overloads are registered as `Immutable`. Identical inputs yield identical ciphertext when enabled. The non-`_iv` variants use an all-zero IV.
2. `parseCipherType` in [`pkg/sql/sem/builtins/pgcrypto/cipher.go`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L25) accepts `<algorithm>[-<mode>][/pad:<padding>]`. The algorithm is `aes`, the mode defaults to `cbc`, and padding defaults to `pkcs`. The six accepted spellings are `aes`, `aes-cbc`, `aes/pad:pkcs`, `aes-cbc/pad:pkcs`, `aes/pad:none`, and `aes-cbc/pad:none`. Matching is case-sensitive.
3. In `cipher.go`, `newAESCBC` uses Go's `crypto/aes` and `crypto/cipher`. `normalizeAESKey` accepts keys of 1–32 bytes and zero-pads them to the smallest AES key size that fits (16, 24, or 32 bytes). `normalizeIV` zero-pads short IVs and clips long IVs to the first 16 bytes. An omitted or empty IV becomes 16 zero bytes.
4. `encryptData` calls `pkcs7Pad` for `pad:pkcs`, adding a full block for already aligned input, including empty data. `decryptData` checks ciphertext alignment and calls `pkcs7Unpad` to validate and remove padding. With `pad:none`, both directions require a multiple of 16 bytes and apply no padding. Invalid type, key length, block alignment, or PKCS#7 padding returns SQLSTATE `22023` (`pgcode.InvalidParameterValue`).
5. Cipher calls return raw bytes and create no database objects. `createExtensionNode.startExec` in [`pkg/sql/create_extension.go`](../../../pkg/sql/create_extension.go#L44) accepts `CREATE EXTENSION pgcrypto` and records telemetry. The functions are already built into the SQL catalog.

## External references

- PostgreSQL documents the SQL signatures, cipher type grammar, default padding and IV handling. [pgcrypto raw encryption functions](https://www.postgresql.org/docs/current/pgcrypto.html#PGCRYPTO-RAW-ENCRYPTION-FUNCTIONS)
- CockroachDB v23.2 documents `encrypt`, `decrypt`, both `_iv` variants, and `gen_random_bytes` in its column encryption examples. [Column Level Encryption](https://www.cockroachlabs.com/docs/v23.2/column-level-encryption)
- CockroachDB v23.2 lists `gen_random_uuid` among ID generation functions. [Functions and Operators](https://www.cockroachlabs.com/docs/v23.2/functions-and-operators#id-generation-functions)
- NIST specifies CBC in §6.2 and provides AES-CBC known-answer examples in Appendix F.2. [NIST SP 800-38A](https://csrc.nist.gov/pubs/sp/800/38/a/final)
- CMS specifies the PKCS#7 padding convention used here, including the full block on aligned input. [RFC 5652 §6.3](https://www.rfc-editor.org/rfc/rfc5652.html#section-6.3)
- OpenSSL documents the `enc` options for supplying a key and IV and disabling padding. [OpenSSL 3.0 `enc`](https://docs.openssl.org/3.0/man1/openssl-enc/)

## Configuration and gating

`feature.pgcrypto_ciphers.enabled` defaults to **OFF** (`false`). `featurePgcryptoCiphersEnabled` registers it as a public `settings.ApplicationLevel` boolean in [`pkg/sql/sem/builtins/pgcrypto/settings.go`](../../../pkg/sql/sem/builtins/pgcrypto/settings.go#L16). `cipherEnabled` reads the current value from `eval.Context` on every hook call.

```sql
SET CLUSTER SETTING feature.pgcrypto_ciphers.enabled = true;
```

Use `RESET CLUSTER SETTING feature.pgcrypto_ciphers.enabled` to restore the default or set it explicitly to `false` to disable. No restart is needed. The registration has no FGDb release-admission check. The setting controls future evaluations. Existing ciphertext remains in the caller's data.

With the gate off, the hooks return SQLSTATE `XXC01` (`pgcode.CCLRequired`) before cipher processing. The message is `<function> can only be used with a CCL distribution` for each of the four function names. The other pgcrypto builtins and `CREATE EXTENSION pgcrypto` use their existing paths.

## Spec → code → test cross-reference

The SQL anchors below refer to [`pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins). The Go test links point into `pkg/sql/sem/builtins/pgcrypto/`.

| Spec requirement | Code | Test |
| --- | --- | --- |
| [CRYPT-001](../spec/PAR-005-pgcrypto.md#L31) · Type grammar, defaults and PKCS#7 | [`cipher.go` · `parseCipherType`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L25), [`pkcs7Pad` and `pkcs7Unpad`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L100) | [`TestParseCipherType`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L232), [`TestCipherPKCS7Padding`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L151), [`TestCipherRoundTrip`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L109), [`cipher_builtins` · round trips](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L329) |
| CRYPT-001 · Aligned `pad:none` in both directions | [`cipher.go` · `encryptData` and `decryptData`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L131) | [`TestCipherNISTVectors`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L38), [`TestCipherConformanceVectors`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_vectors_test.go#L22), [`cipher_builtins` · round trip](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L350) and [known ciphertext](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L395) |
| CRYPT-001 · Determinism and zero IV | [`pgcrypto_builtins.go` · `encrypt` and `encrypt_iv` registrations](../../../pkg/sql/sem/builtins/pgcrypto_builtins.go#L171), [`cipher.go` · `normalizeIV`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L71) | [`TestCipherZeroIVDefault`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L89), [`TestCipherRoundTrip`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L109), [`cipher_builtins` · zero-IV equivalence and repeated encryption](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L402) |
| CRYPT-001 · Invalid type, key length, alignment and padding | [`cipher.go` · `normalizeAESKey`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L48), [`unsupportedCipherTypeError`, `unalignedDataError`, `errCorruptOrWrongKey`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L184) | [`TestCipherBadKeyLength`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L205), [`TestCipherUnalignedNoPadding`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L198), [`TestCipherBadPadding`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L166), [`cipher_builtins` · `22023` statements](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L416) |
| [CRYPT-002](../spec/PAR-005-pgcrypto.md#L32) · Gate-off parity for four hooks | [`pgcrypto.go` · four hooks](../../../pkg/sql/sem/builtins/pgcrypto/pgcrypto.go#L25), [`cipher.go` · `cipherCCLRequiredError`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L176) | [`TestCipherHooksGate`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L267) checks hook messages and both settings. [`ccl_functions`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L450) checks all four messages and SQLSTATE `XXC01`. |
| [CRYPT-003](../spec/PAR-005-pgcrypto.md#L33) · Existing builtins | [`pgcrypto_builtins.go` · `pgcryptoBuiltins`](../../../pkg/sql/sem/builtins/pgcrypto_builtins.go#L53) | [`digest`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L1), [`hmac`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L37), [`gen_salt_invalid_algo`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L71), [`gen_salt_bf`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L152), [`crypt_invalid_algo`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L170), [`crypt_bf`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L216), and [`gen_random_bytes`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L292). [`gen_random_uuid`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L62) also has existing coverage. |
| [CRYPT-004](../spec/PAR-005-pgcrypto.md#L34) · Extension statement succeeds | [`pkg/sql/create_extension.go` · `createExtensionNode.startExec`](../../../pkg/sql/create_extension.go#L44) | [`cipher_builtins` · `statement ok` / `CREATE EXTENSION pgcrypto`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L319) |
| [Conformance](../spec/PAR-005-pgcrypto.md#L35) · Key and IV lengths against reference vectors | [`cipher.go` · `normalizeAESKey`, `normalizeIV`, `newAESCBC`](../../../pkg/sql/sem/builtins/pgcrypto/cipher.go#L48) | [`TestCipherNISTVectors`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L38), [`TestCipherConformanceVectors`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_vectors_test.go#L22), [`TestCipherLongIVIsClipped`](../../../pkg/sql/sem/builtins/pgcrypto/cipher_test.go#L218), [`cipher_builtins` · OpenSSL vector statements](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L360) |
| [Claims and tests](../spec/PAR-005-pgcrypto.md#L36) · Shared SQL fixture enables and resets the gate | [`settings.go` · `featurePgcryptoCiphersEnabled` and `cipherEnabled`](../../../pkg/sql/sem/builtins/pgcrypto/settings.go#L16) | [`TestLogic_pgcrypto_builtins`](../../../pkg/sql/logictest/tests/local/generated_test.go#L1516) runs [`cipher_builtins` · `SET`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L313), [`RESET`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L445), and then [`ccl_functions`](../../../pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins#L450). The `skipif config 3node-tenant-default-configs` directives skip the setting changes and enabled cipher assertions. |

## Test evidence

| Orchestrator check | Evidence and result |
| --- | --- |
| `bazel test //pkg/sql/sem/builtins/pgcrypto:pgcrypto_test` | [Target and fixture embedding](../../../pkg/sql/sem/builtins/pgcrypto/BUILD.bazel#L20). The tests cover NIST known answers, OpenSSL vectors, parsing, padding, key and IV handling, and both gate states. Run pending on the orchestrator. |
| `bazel test //pkg/sql/logictest/tests/local:local_test --test_filter=TestLogic_pgcrypto_builtins` | The `pgcrypto_builtins` logictest in config `local` covers enabled SQL results and errors, extension creation, existing builtins, and the four disabled SQLSTATEs and messages. Run pending on the orchestrator. |

For PR #58 (head `81ec99ca` plus this worktree), the test names and SQL anchors above were checked in source. Running [`gen-vectors.sh`](../../../pkg/sql/sem/builtins/pgcrypto/conformance/gen-vectors.sh) in a temporary copy with OpenSSL 3.0.13 reproduced all 133 [`vectors.tsv`](../../../pkg/sql/sem/builtins/pgcrypto/conformance/vectors.tsv) rows byte for byte. The generator supplies normalized keys and IVs to OpenSSL. OpenSSL supplies ciphertext for the success vectors. The fixture's error rows assert FGDb messages, and the SQL fixture asserts FGDb SQLSTATEs.

The checked-in suite covers aligned `pad:none` in both directions at the Go implementation level with NIST IVs. It checks unaligned encryption and decryption through `TestCipherUnalignedNoPadding` and checks the unaligned `encrypt` SQL error in `cipher_builtins`. The recorded oracle table lacks unaligned `decrypt` and `_iv` error cases. Stock v23.2.15 OSS baseline capture for this feature and the wider logic config run remain pending. The four claim rows appear as examples in the spec. Registration in `fgdb/test/claims.yaml` and the PAR-001 updates remain follow-up.

## Limitations and differences

- FGDb implements AES-CBC with `pad:pkcs` or `pad:none`. Other ciphers and modes in the PostgreSQL reference are outside this implementation. The key zero-padding and 1–32 byte range are FGDb behavior. The public PostgreSQL documentation establishes IV normalization and defaults.
- Raw CBC provides confidentiality without authentication. The zero-IV overload is deterministic and reveals repeated plaintext under a fixed key. Callers can supply an IV via `encrypt_iv` and must retain it for `decrypt_iv`.
- **Proposed, pending** — parity of cipher failure SQLSTATEs and messages against a stock PostgreSQL server. D41 accepted OpenSSL success vectors. A PostgreSQL server comparison is still pending.
- **Proposed, pending** — enabling `feature.pgcrypto_ciphers.enabled` by default. The current default is `false`.

## Change log

- 2026-11-18 · D41 approved the OpenSSL success vector set and kept the gate off by default.
- 2026-11-22 · PR #58 records the D41 fixture, adds SQL known-answer cases, and documents the cipher behavior and verification.
