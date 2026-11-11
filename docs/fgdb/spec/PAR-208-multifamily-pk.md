> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-208 — Primary-key column alone in a non-first family must be fully updated

| Field | Value |
| --- | --- |
| Spec id | PAR-208 |
| Version | v0.2 |
| Implementer | gpt-6.1-sol (Codex), clean implementer |
| Date | 2026-11-11 |
| Role | spec writer (cleanroom spec role; see docs/fgdb/CLEANROOM.md) — Crdb Steward |
| Covers | PAR-207 rows N-011 … N-016 (one public v23.2.16 note, six conditions) |
| Sources consulted | Public v23.2.16 release note on the Cockroach Labs docs site (quoted below); black-box runs of FGDb `cockroach-oss` (v23.2.15 Apache pin, main `0ddbad46`/`dd1b1579`); FGDb's own OSS v23.2.15 tree |
| Not consulted | Any CockroachDB source, diff, PR, issue or commit after v23.2.15; any CCL/CSL code; the upstream fix |

## Public statement (v23.2.16, Bug fixes)

> Fixed a rare bug in which an update of a primary key column that is also the only column in a
> separate column family can sometimes fail to update the primary index. […] Requirements to hit the
> bug are: 1. A table with multiple column families. 2. A column family containing a single primary
> key column. 3. That column family is not the first column family. 4. That column family existed
> before its column was in the primary key. 5. That column must be of type FLOAT4/8, DECIMAL, JSON,
> collated string type, or array. 6. An update that changes that column from a composite value to a
> non-composite value.

## Terms

- **Composite value:** a value whose primary-key encoding does not preserve its exact
  representation, so two distinct representations compare equal (DECIMAL `1.00` = `1`, FLOAT `-0` =
  `0`, JSON or arrays containing such numbers). **Non-composite:** a value whose representation is
  fully recoverable from the key (DECIMAL `1`, FLOAT `0`, `{"x": 1}`, `{1}`).
- **Primary read:** a read forced to the primary index (`t@t_pkey`) or one that needs a column only the
  primary index has.

## Required behavior

B1. After any successful `UPDATE`, `UPSERT`, or `INSERT … ON CONFLICT DO UPDATE` that sets a
primary-key column, a primary read of that row returns exactly the new value's representation
(`b::STRING` equals `(new value)::T::STRING`), regardless of family layout or how the family came to
hold that column.

B2. Primary and secondary reads of the same row return identical representations.

B3. B1/B2 hold for every transition: composite→non-composite, non-composite→composite,
composite→composite, non-composite→non-composite, and for key-changing and key-preserving updates.

B4. B1/B2 hold whether the primary key is single-column or multi-column, and whether the column joined
the primary key at CREATE TABLE or via `ALTER TABLE … ALTER PRIMARY KEY`.

B5. No change to the SQL surface, error codes, or on-disk format of rows written by unaffected
shapes. Rows previously written correctly read back unchanged. (Rows already holding a stale
representation from the old behavior are out of scope; a later write of the column repairs them.)

## Acceptance tests (observable)

Shape S: `CREATE TABLE t (k INT PRIMARY KEY, a INT, b T NOT NULL, FAMILY f0 (k, a), FAMILY f1 (b));`
`INSERT INTO t VALUES (1, 10, v1); ALTER TABLE t ALTER PRIMARY KEY USING COLUMNS (b);`
`UPDATE t SET b = v2 WHERE k = 1;` then compare `b::STRING` from `t@t_pkey` and `t@t_k_key` to `v2`.

| Id | T | v1 → v2 | Expected primary and secondary |
|---|---|---|---|
| A1 | DECIMAL | 1.00 → 1 | `1` |
| A2 | FLOAT8 | -0 → 0 | `0` |
| A3 | FLOAT4 | -0 → 0 | `0` |
| A4 | JSONB | `{"x": 1.00}` → `{"x": 1}` | `{"x": 1}` |
| A5 | DECIMAL[] | `{1.00}` → `{1}` | `{1}` |
| A6 | FLOAT8[] | `{-0}` → `{0}` | `{0}` |
| A7 | DECIMAL, PK (b, k) | 1.00 → 1 | `1` |
| A8 | DECIMAL, `UPDATE t SET b = 1, a = 11` | 1.00 → 1 | `11`, `1` |
| A9 | DECIMAL, `INSERT … ON CONFLICT (b) DO UPDATE SET b = excluded.b` | 1.00 → 1 | `1` |
| R1 | DECIMAL | 1 → 1.00 | `1.00` |
| R2 | DECIMAL | 1.00 → 1.0 | `1.0` |
| R3 | DECIMAL | 1.00 → 2 | `2` |
| R4 | collated string (`en_u_ks_level2`) | 'Ab' → 'ab' | `ab` |
| R5–R8 | DECIMAL 1.00 → 1 with one family / b shares f1 with a / f1 declared first / b PK at CREATE | | `1` |

A1–A9 fail on FGDb main `dd1b1579` (stale composite value on the primary read; A9 was first
observed passing through an unhinted read that used the secondary index, and the logic test's
primary read shows it fails too); R1–R8 pass on main and must keep passing.

## Status

Repro: see the PAR-208 REPRO notes (all six conditions confirmed; one bug). Implementation: clean
implementer from this spec + the FGDb OSS v23.2.15 tree only.
