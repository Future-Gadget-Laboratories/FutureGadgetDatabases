> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-005 — pgcrypto-compatible builtins on `cockroach-oss`

| Field | Value |
| --- | --- |
| Spec id | PAR-005 (pgcrypto lane) |
| Version | v1 |
| Date | 2026-10-26 |
| Role | spec writer (cleanroom; see [CLEANROOM.md](../CLEANROOM.md)) |
| Skill bundles | `database-fgdb@1.0.1`, `impl-functional-design@1.0.1`, `impl-documentation@1.0.1` |
| Sources consulted | This tree; `pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins`; PostgreSQL pgcrypto documentation (behavior only); AES-CBC-PKCS semantics per NIST/OpenSSL-compatible docs |

## Builder cleanroom boundary (mandatory)

These are behavior-only draft specifications; references to this allowed OSS tree are context, not a substitute for behavioral requirements. Before implementer handoff, a maintainer must review for expression leakage and approve the spec under [CLEANROOM.md](../CLEANROOM.md); record the writer, reviewer, approval, spec version and eventual implementer in the audit trail. Approval is pending.

Oracle baseline provenance: stock CockroachDB OSS v23.2.15 binary, with version and distribution recorded alongside each capture. Record reproducible inputs, settings, outputs or SQLSTATE and message, and capture date in versioned conformance vectors; never copy source or upstream test files. The stock OSS baseline may expose a stub rather than the desired success behavior; it does not establish missing success vectors. Any additional permitted behavioral oracle and its provenance must be identified and approved by the maintainer. Pending oracle data and approval are gates before handoff and implementation, not assumed evidence.

Implement cipher hooks only in Apache-2.0 paths (`pkg/sql/sem/builtins/pgcrypto/`). **Never** read `pkg/ccl/pgcryptoccl` (not in OSS tree; listed only in root `pkg/BUILD.bazel` as CCL targets). No newer CRDB sources. Use **Go `crypto/*` stdlib** and existing `golang.org/x/crypto/bcrypt` already linked from `pgcrypto_builtins.go` for `crypt`/`gen_salt` — no new crypto dependencies without license review.

## 1. Goal, acceptance criteria, GATE decision

**Goal.** Enable PostgreSQL-compatible `encrypt`, `decrypt`, `encrypt_iv`, and `decrypt_iv` on `cockroach-oss` with parity to stock v23.2.15 OSS baseline and an approved success oracle behavior for supported algorithms (`aes` / `aes-cbc` / PKCS padding per builtin docs), while keeping existing OSS implementations for `digest`, `hmac`, `crypt`, `gen_salt`, and `gen_random_bytes`.

**Acceptance criteria.**

1. **CRYPT-001.** Cipher parity is defined by an **oracle vector table** recorded from the stock v23.2.15 OSS baseline and an approved success oracle (not the `ccl_functions` logic subtest — that subtest is four `query error` `XXC01` cases with 3-byte key/IV and non-round-trip hex literals). Vectors must include: type string; key length; IV length (for `_iv` funcs); whether `encrypt` output is deterministic across two calls with the same inputs; expected ciphertext/plaintext where applicable; and SQLSTATE + message for **cipher** failures (bad type, bad key length, `pad:none`). The proposed `pad:none` behavior is unpadded AES-CBC for block-aligned input, with an oracle-recorded error for non-aligned input; it is not established until approved vectors confirm it. Before implementation or handoff, capture and obtain maintainer approval of aligned and unaligned vectors for both encrypt and decrypt, including `_iv` variants. If the oracle rejects `pad:none` or differs from the proposal, record the observed results and revise this draft through maintainer review before implementation or handoff. Implement `crypto/aes` + `crypto/cipher` CBC with PKCS#7 and the approved `pad:none` behavior. Type strings follow `cipherSupportedCipherTypeInfo` in `pgcrypto_builtins.go` — a **grammar**, not four fixed literals: `` `<algorithm>[-<mode>][/pad:<padding>]` `` with algorithm `aes`, mode `cbc` (default), padding `pkcs` (default, PKCS#7) or `none`; **`aes` alone means `aes-cbc` with PKCS#7**. Set builtin `encrypt` to **`Volatile`** only if the approved success oracle shows two `encrypt` calls with identical args can differ; keep `encrypt_iv` **immutable** per `pgcrypto_builtins.go` unless oracle proves otherwise. If the approved success oracle shows `encrypt` is non-deterministic, logic vectors must assert **round-trip** (`decrypt(encrypt(...))`) rather than a fixed ciphertext hex. Package unit tests that invoke hooks must use `eval.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())` — a nil settings argument leaves `evalCtx.Settings` nil (`eval/context.go`).
2. **CRYPT-002 (GATE-off).** With `feature.pgcrypto_ciphers.enabled = false`, all four hooks return unchanged stub errors: SQLSTATE `XXC01` (`pgcode.CCLRequired`) and messages exactly `encrypt can only be used with a CCL distribution`, `encrypt_iv can only be used with a CCL distribution`, `decrypt can only be used with a CCL distribution`, `decrypt_iv can only be used with a CCL distribution` (`pkg/sql/sem/builtins/pgcrypto/pgcrypto.go:21–47`). PAR-001 row updates are **follow-up** when that spec exists.
3. **CRYPT-003.** Existing subtests for `digest`, `hmac`, `gen_salt`, `crypt`, `gen_random_bytes` in `pgcrypto_builtins` remain passing — no regressions.
4. **CRYPT-004.** `CREATE EXTENSION pgcrypto` still allowed (`pkg/sql/create_extension.go` lists `pgcrypto` and returns success). The shared logic file must include `statement ok` / `CREATE EXTENSION pgcrypto` so the claim’s cited test actually executes it (today’s file never runs `CREATE EXTENSION`).
5. **Conformance.** Oracle vector table (see CRYPT-001) with at least two AES key lengths and IV-length cases; FGDb diff with flag on.
6. **Claims + tests.** Full `fgdb/test/claims.yaml` rows (§5). The shared `pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins` file explicitly enables the setting for cipher success vectors, resets it afterward, and checks the four `XXC01` strings with the default setting. Tenant configurations that cannot change the setting skip the flag-on cases.

**GATE contract decision: yes — cluster setting checked inside hook implementations.**

| Item | Decision |
| --- | --- |
| Mechanism | `feature.pgcrypto_ciphers.enabled` (public `settings.BoolSetting`, default `false`, pending success-oracle vectors and maintainer approval). **`Encrypt` / `EncryptIV` / `Decrypt` / `DecryptIV` remain assigned to real implementations** (not swapped in `init`); each call reads the setting from `eval.Context` and, when false, returns the four **current** stub strings unchanged. |
| Flag **on** | Hooks run stdlib AES-CBC with PKCS#7 or `pad:none` per oracle vector table and type-string parser. |
| Flag **off** | Per-call early return with `pgcrypto.go` `XXC01` messages (CRYPT-002). |
| PAR-001 | Update parity table when that spec lands; flag-off contract is the four quoted messages above. |

**Reasons.** Preserves rollback and PAR-005 inputs GATE contract without removing builtins from the catalog.

## 2. Verified survey (this tree only)

| Path | What it implies |
| --- | --- |
| `pkg/sql/sem/builtins/pgcrypto/pgcrypto.go` | **Hook point:** `Encrypt`, `EncryptIV`, `Decrypt`, `DecryptIV` vars default to `XXC01` CCL distribution errors. |
| `pkg/sql/sem/builtins/pgcrypto_builtins.go` | SQL builtins delegate to `pgcrypto.*` hooks; `crypt`/`gen_salt` use `golang.org/x/crypto/bcrypt` via `go:linkname`; `digest`/`hmac` use `crypto/md5`, `sha*` stdlib. |
| `pkg/sql/sem/builtins/pgcrypto_builtins.go` (`gen_random_bytes`, ~228+) | Already OSS; length limits `[1,1024]` and `crypto/rand`. |
| `pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins` | `digest`/`hmac`/`gen_salt`/`crypt` expectations; `ccl_functions` (~292+) is **flag-off documentation only** (`query error` `XXC01`), not success vectors. SQLSTATE `22023` in this file applies to digest/hmac/crypt paths, **not** cipher hooks (cipher errors come from oracle-recorded messages). |
| `pkg/sql/sem/builtins/fixed_oids.go` | OIDs registered for pgcrypto builtins including encrypt and `gen_random_bytes`. |
| `pkg/sql/create_extension.go` (~49) | `pgcrypto` extension name recognized. |
| `pkg/BUILD.bazel` (ccl targets only as names) | Enterprise cipher implementation lives under `pkg/ccl/pgcryptoccl` — **out of scope to read**; OSS must reimplement from spec. |

**Builtin status on OSS today.**

| Builtin | OSS behavior today |
| --- | --- |
| `digest`, `hmac` | Functional (logic tests) |
| `gen_random_uuid` | Functional (separate impl) |
| `gen_random_bytes` | Functional with range errors |
| `gen_salt`, `crypt` | Functional (bcrypt-linked) |
| `encrypt`, `encrypt_iv`, `decrypt`, `decrypt_iv` | **Stub → `XXC01` CCL distribution** |

## 3. Deliverables (paths and size targets)

| Deliverable | Path | Target size |
| --- | --- | --- |
| Cipher implementation | `pkg/sql/sem/builtins/pgcrypto/cipher.go` (new, Apache-2.0 header) | ~150–250 LOC; AES-CBC; PKCS#7 and `pad:none`; per-call setting check; shared stub error helpers for flag off. |
| Hook wiring | `pkg/sql/sem/builtins/pgcrypto/pgcrypto.go` | Replace default stub func bodies with implementations that delegate to `cipher.go` after setting check (do **not** reassign vars from `init` based on setting). |
| Setting | `pkg/sql/sem/builtins/pgcrypto/settings.go` or adjacent | Register bool setting; init hook from `builtins` package `init` order safe. |
| Unit tests | `pkg/sql/sem/builtins/pgcrypto/cipher_test.go` | Vectors from approved success oracle table (not `ccl_functions` literals); `eval.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())` for hook calls. |
| Logic test data | `pkg/sql/logictest/testdata/logic_test/pgcrypto_builtins` (**edit in place**) | Explicitly enable the setting for cipher success vectors, reset it afterward, and exercise `subtest ccl_functions` with the default setting; include `CREATE EXTENSION pgcrypto`. Round-trip assertions when stock `encrypt` is non-deterministic. **Do not** rely on a separate fgdb config block. |
| Gate tests | Same shared logic file + optional `pkg/fgdbtest/capabilities/pgcrypto.go` | On/off `XXC01` vs oracle success. |
| BUILD.bazel | `pkg/sql/sem/builtins/pgcrypto/BUILD.bazel` | Add sources; no ccl deps. |

## 4. Build, tests, conformance, scan gate

**Build.**

```bash
./dev build oss
```

**Package tests.**

```bash
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/sem/builtins/pgcrypto -timeout 10m
```

**Logic test (`TestLogic_pgcrypto_builtins` — all configs that run this file).**

```bash
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/local -run 'TestLogic_pgcrypto_builtins' -timeout 30m
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/local-vec-off -run 'TestLogic_pgcrypto_builtins' -timeout 30m
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/local-mixed-23.1 -run 'TestLogic_pgcrypto_builtins' -timeout 30m
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/local-legacy-schema-changer -run 'TestLogic_pgcrypto_builtins' -timeout 30m
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/fakedist -run 'TestLogic_pgcrypto_builtins' -timeout 30m
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/fakedist-vec-off -run 'TestLogic_pgcrypto_builtins' -timeout 30m
GOCACHE=/tmp/gocache-par005 GOTMPDIR=/tmp \
  go test -count=1 ./pkg/sql/logictest/tests/fakedist-disk -run 'TestLogic_pgcrypto_builtins' -timeout 30m
```

(`local-mixed-23.1` bootstraps cluster version 23.1 but still uses this OSS binary and the same `pgcrypto_builtins` file.)

**TBD oracle gate.** The additional success-oracle source and approval are pending. Before implementation or handoff, the maintainer must approve its provenance and the aligned and unaligned `pad:none` vectors for both directions and `_iv` variants. Record the source, version/distribution, approver, approval date and approval reference in a versioned `fgdb/test/conformance/par005-pgcrypto/README.md`, alongside reproducible inputs and captured results. The gate remains open until the source, approval record and vectors exist.

**Conformance (oracle on approved success oracle).**

1. Oracle-driven round-trips for `encrypt`/`decrypt` and `encrypt_iv`/`decrypt_iv`.
2. Cipher cases from the approved vector table (bad type, bad key length, aligned and unaligned `pad:none` encrypt and decrypt, including `_iv` variants) — **not** digest/hmac `22023` cases.
3. Flag off: four `XXC01` messages from CRYPT-002 (logic test config with `SET CLUSTER SETTING ... = false`).

**Public references.** PostgreSQL pgcrypto `encrypt`/`decrypt` documentation (argument types and algorithm names); do not copy C source from `contrib/pgcrypto` — use as behavioral checklist only.

**Scan gate.** Scope `pkg/sql/sem/builtins/pgcrypto/**`, capability tests, spec markdown; `easyscan-scan` as in workload spec.

## 5. Risks, maintainer questions, PENDING

**Risks.**

- **Crypto security:** Use `crypto/aes`, `crypto/cipher` CBC mode; IVs for `encrypt` only via mechanism confirmed by oracle (if stock uses random IV inside `encrypt`, use `crypto/rand` and **`Volatile`** on `encrypt`); no custom block ciphers; document non-constant-time CBC caveats matching PostgreSQL/Cockroach semantics.
- **License:** Any new dependency requires Apache/BSD/MIT check; prefer zero new deps.
- **Error text parity:** Flag-off must match `pgcrypto.go` strings byte-for-byte (CRYPT-002).
- **Enterprise license strings in builtin INFO:** `cipherRequiresEnterpriseLicenseInfo` in `pgcrypto_builtins.go` may need doc update when OSS enables ciphers — avoid claiming CCL-only in user-facing INFO when flag on (maintainer copy review).
- **go:linkname bcrypt:** Keep existing pattern; do not refactor crypt path in same PR unless required.

**Maintainer questions.**

1. Are additional algorithms beyond the `aes` / `cbc` / `pkcs` / `none` grammar in scope for v1?
2. *(Reviewer: shared `pgcrypto_builtins` only — **accepted**; dedicated file or fgdb config block **declined** — must edit the shared file and explicitly enable the setting for flag-on cases.)*
3. *(Reviewer default `false`, pending success-oracle vectors and maintainer approval — **accepted**; `ccl_functions` checks the default-off `XXC01` behavior after the setting is reset.)*

**Claims registry examples (`CRYPT-001`..`CRYPT-004`).**

```yaml
- id: CRYPT-001
  claim: pgcrypto cipher builtins match approved success oracle vectors when feature.pgcrypto_ciphers.enabled is true.
  source: docs/fgdb/spec/PAR-005-pgcrypto.md
  category: correctness
  tests:
    - pkg/sql/logictest/tests/local:TestLogic_pgcrypto_builtins
  tier: interim
  status: partial
- id: CRYPT-002
  claim: Disabling feature.pgcrypto_ciphers.enabled restores XXC01 stub errors for all four cipher builtins.
  source: docs/fgdb/spec/PAR-005-pgcrypto.md
  category: correctness
  tests:
    - pkg/sql/logictest/tests/local:TestLogic_pgcrypto_builtins
  tier: interim
  status: partial
- id: CRYPT-003
  claim: Non-cipher pgcrypto builtins (digest, hmac, crypt, gen_salt, gen_random_bytes) remain unchanged.
  source: docs/fgdb/spec/PAR-005-pgcrypto.md
  category: correctness
  tests:
    - pkg/sql/logictest/tests/local:TestLogic_pgcrypto_builtins
  tier: interim
  status: partial
- id: CRYPT-004
  claim: CREATE EXTENSION pgcrypto remains allowed on cockroach-oss.
  source: docs/fgdb/spec/PAR-005-pgcrypto.md
  category: correctness
  tests:
    - pkg/sql/logictest/tests/local:TestLogic_pgcrypto_builtins
  tier: interim
  status: partial
```

**PENDING.**

- Full logic test run on build host.
- Stock OSS baseline and approved success oracle ciphertext capture.
- Security review of IV handling and key length errors vs PostgreSQL.

## 6. Review checklist

- [ ] No `pkg/ccl` imports; `nogo` clean.
- [ ] Flag off restores four `pgcrypto.go` `XXC01` messages exactly; hooks read setting per call.
- [ ] Shared `pgcrypto_builtins` edited: flag-on subtests first; then cipher success + `CREATE EXTENSION`; then `SET` false; then `ccl_functions`; all seven logic configs pass `TestLogic_pgcrypto_builtins`.
- [ ] CRYPT-001 vectors from approved success oracle, not `ccl_functions` success assumptions; cipher type strings follow `cipherSupportedCipherTypeInfo` grammar.
- [ ] `cipher_test.go` uses `eval.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())`.
- [ ] `crypt`/`digest` paths untouched unless regression proves otherwise.
- [ ] License headers on new FGL files (Apache-2.0).
- [ ] No secrets or static keys in tests; use logic test literals only.
- [ ] Claims.yaml updated with full registry rows; PAR-001 flip noted as follow-up.
- [ ] easyscan on scoped files.
