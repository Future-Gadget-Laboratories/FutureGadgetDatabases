#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# FGL docs-side validator for PAR-031 conformance case schema v1. Not a runner.
"""Usage: validate_cases.py [case.yaml ...]  (default: ../cases/*.yaml)

Every check collects errors; malformed input never raises.
"""
import datetime
import glob
import os
import re
import sys

import yaml

NAME = re.compile(r"^[a-z][a-z0-9_]*$")
REF = re.compile(r"\$\{([^}]*)\}")
SQLSTATE = re.compile(r"^[0-9A-Z]{5}$")
MASK_KINDS = ("job_id", "timestamp", "size", "path")
SIDES = ("source", "target")
SCALAR = (str, int, float, bool, type(None))
DOC_PREFIX = "https://www.cockroachlabs.com/docs/v23.2/"
TOP = ("schema_version", "id", "slice", "description", "fixtures", "variables",
       "setup", "statement", "expect", "post", "oracle")
ORACLE = ("stock_version", "doc_urls", "verification", "recorded_at",
          "license_key_state", "evidence_note")


def iso8601(v):
    """True for YAML date/datetime values or ISO-8601 text (trailing Z accepted)."""
    if isinstance(v, (datetime.date, datetime.datetime)):
        return True
    if not isinstance(v, str) or not v.strip():
        return False
    t = v[:-1] + "+00:00" if v.endswith(("Z", "z")) else v
    for parse in (datetime.datetime.fromisoformat, datetime.date.fromisoformat):
        try:
            parse(t)
            return True
        except ValueError:
            pass
    return False


def is_text(v):
    return isinstance(v, str) and bool(v.strip())


class Checker:
    def __init__(self, path):
        self.path, self.errs = path, []

    def err(self, where, msg):
        self.errs.append(f"{self.path}: {where}: {msg}")

    def keys(self, where, obj, required, optional=()):
        """Report problems; True only if obj is a mapping with all required keys."""
        if not isinstance(obj, dict):
            self.err(where, "must be a mapping")
            return False
        ok = True
        for k in required:
            if k not in obj:
                self.err(where, f"missing required key '{k}'")
                ok = False
        for k in obj:
            if not isinstance(k, str) or (k not in required and k not in optional):
                self.err(where, f"unknown key {k!r}")
        return ok

    def text(self, where, v):
        if not is_text(v):
            self.err(where, "must be a non-empty string")

    def sql(self, where, s, bound):
        if not is_text(s):
            return self.err(where, "must be a non-empty string")
        for ref in REF.findall(s):
            if ref not in bound:
                self.err(where, f"unbound reference ${{{ref}}}")

    def on(self, where, v, sides):
        if not isinstance(v, str) or v not in SIDES:
            self.err(where, "'on' must be source or target")
        elif v not in sides:
            self.err(where, f"'on: {v}' has no matching fixture")

    def step(self, where, s, sides, bound, optional=()):
        if not self.keys(where, s, ("on", "sql"), optional):
            return False
        self.on(where, s["on"], sides)
        self.sql(where + ".sql", s["sql"], bound)
        return True

    def result(self, w, r, bound, new_binds):
        """Returns (incomplete, columns or None)."""
        if not self.keys(w, r, ("columns", "rows", "row_match"), ("values", "capture")):
            return True, None
        incomplete = False
        cols = r["columns"]
        if cols is None:
            incomplete = True
        elif (not isinstance(cols, list) or not cols or not all(is_text(c) for c in cols)
              or len(set(cols)) != len(cols)):
            self.err(w, "columns must be null or a non-empty list of unique strings")
            cols, incomplete = None, True
        rm, rows = r["row_match"], r["rows"]
        if not isinstance(rm, str) or rm not in ("exact", "contains", "pending"):
            self.err(w, "row_match must be exact, contains or pending")
        if rows is None:
            incomplete = True
            if rm != "pending":
                self.err(w, "rows null requires row_match pending")
        elif not isinstance(rows, list) or not all(isinstance(x, dict) for x in rows):
            self.err(w, "rows must be null or a list of mappings")
        else:
            if rm == "pending":
                self.err(w, "row_match pending requires rows null")
            if cols is None:
                self.err(w, "rows require known columns")
            for x in rows:
                if cols is not None and set(map(repr, x)) != set(map(repr, cols)):
                    self.err(w, "row keys must equal columns")
                if not all(isinstance(v, SCALAR) for v in x.values()):
                    self.err(w, "row values must be scalars")
        for opt in ("values", "capture"):
            if opt not in r:
                continue
            m = r[opt]
            if not isinstance(m, dict) or not m:
                self.err(w, f"{opt} must be a non-empty mapping")
                continue
            for c, v in m.items():
                if not isinstance(c, str) or cols is None or c not in cols:
                    self.err(w, f"{opt} column {c!r} not in columns")
                if opt == "values" and not isinstance(v, SCALAR):
                    self.err(w, f"values[{c!r}] must be a scalar")
                if opt == "capture":
                    if not isinstance(v, str) or not NAME.match(v):
                        self.err(w, f"capture binding {v!r} invalid")
                    elif v in bound or v in new_binds:
                        self.err(w, f"binding '{v}' already defined")
                    else:
                        new_binds.add(v)
        return incomplete, cols

    def error(self, w, er):
        if not self.keys(w, er, ("sqlstate", "message_pattern"), ("pending",)):
            return True
        s, m = er["sqlstate"], er["message_pattern"]
        if s is not None and not (isinstance(s, str) and SQLSTATE.match(s)):
            self.err(w, "sqlstate must be null or 5 characters [0-9A-Z]")
        if m is not None:
            if not is_text(m):
                self.err(w, "message_pattern must be null or a non-empty string")
            else:
                if not (m.startswith("^") and m.endswith("$")):
                    self.err(w, "message_pattern must be anchored ^...$")
                try:
                    re.compile(m)
                except re.error as ex:
                    self.err(w, f"message_pattern does not compile: {ex}")
        incomplete = s is None or m is None
        if incomplete:
            if "pending" not in er:
                self.err(w, "null sqlstate/message_pattern requires pending")
            else:
                self.text(w + ".pending", er["pending"])
        elif "pending" in er:
            self.err(w, "pending present but error is complete")
        return incomplete

    def expect(self, where, e, bound, new_binds):
        """Returns True if any comparator is incomplete (needs a pending note)."""
        if not self.keys(where, e, (), ("result", "error", "masks")):
            return True
        if ("result" in e) == ("error" in e):
            self.err(where, "exactly one of result or error is required")
        incomplete, cols = False, None
        if "result" in e:
            incomplete, cols = self.result(where + ".result", e["result"], bound, new_binds)
        if "error" in e:
            incomplete = self.error(where + ".error", e["error"]) or incomplete
        if "masks" in e:
            masks = e["masks"]
            if not isinstance(masks, list) or not masks:
                self.err(where, "masks must be a non-empty list")
                return incomplete
            if "result" not in e:
                self.err(where, "masks apply only to result")
            seen = set()
            for i, mk in enumerate(masks):
                w = f"{where}.masks[{i}]"
                if not self.keys(w, mk, ("field", "kind")):
                    continue
                f, k = mk["field"], mk["kind"]
                if not is_text(f) or not isinstance(k, str):
                    self.err(w, "field and kind must be strings")
                    continue
                if k not in MASK_KINDS:
                    self.err(w, f"unknown mask kind '{k}'")
                if cols is not None and f not in cols:
                    self.err(w, f"mask field '{f}' not in columns")
                if f in seen:
                    self.err(w, "duplicate mask field")
                seen.add(f)
        return incomplete

    def run(self, d):
        if not self.keys("<top>", d, TOP):
            return
        sv = d["schema_version"]
        if type(sv) is not int or sv != 1:
            self.err("schema_version", "must be integer 1")
        stem = os.path.splitext(os.path.basename(self.path))[0]
        cid, sl = d["id"], d["slice"]
        if not isinstance(sl, str) or sl not in ("S1", "S2", "S3"):
            self.err("slice", "must be S1, S2 or S3")
            sl = None
        if not isinstance(cid, str):
            self.err("id", "must be a string")
        else:
            if cid != stem:
                self.err("id", f"must equal file name '{stem}'")
            if sl and not cid.startswith(sl.lower() + "-"):
                self.err("id", "must start with the slice prefix")
        self.text("description", d["description"])

        sides = set()
        if self.keys("fixtures", d["fixtures"], ("source",), ("target",)):
            for k, v in d["fixtures"].items():
                self.text(f"fixtures.{k}", v)
                if k in SIDES:
                    sides.add(k)

        bound = set()
        vs = d["variables"]
        extra = tuple(k for k in vs if k != "collection") if isinstance(vs, dict) else ()
        if self.keys("variables", vs, ("collection",), extra):
            for k, v in vs.items():
                if not isinstance(k, str) or not NAME.match(k):
                    self.err("variables", f"invalid name {k!r}")
                    continue
                self.text(f"variables.{k}", v)
                if isinstance(v, str) and REF.search(v):
                    self.err(f"variables.{k}", "variables may not reference other bindings")
                bound.add(k)

        setup = d["setup"]
        if not isinstance(setup, list) or not setup:
            self.err("setup", "must be a non-empty list")
        else:
            for i, s in enumerate(setup):
                self.step(f"setup[{i}]", s, sides, bound)
        self.step("statement", d["statement"], sides, bound)

        new = set()
        incomplete = self.expect("expect", d["expect"], bound, new)
        bound |= new
        o = d["oracle"]
        if incomplete and not (isinstance(o, dict) and o.get("pending_fields")):
            self.err("expect", "incomplete statement expectation requires oracle.pending_fields")

        post = d["post"]
        if not isinstance(post, list):
            self.err("post", "must be a list (may be empty)")
        else:
            for i, p in enumerate(post):
                w = f"post[{i}]"
                if not self.step(w, p, sides, bound, ("expect", "pending")):
                    continue
                if "expect" not in p:
                    self.err(w, "missing required key 'expect'")
                    continue
                new = set()
                inc = self.expect(w + ".expect", p["expect"], bound, new)
                bound |= new
                if "pending" in p:
                    self.text(w + ".pending", p["pending"])
                elif inc:
                    self.err(w, "incomplete result requires pending")

        if not self.keys("oracle", o, ORACLE, ("pending_fields",)):
            return
        if o["stock_version"] != "v23.2.15":
            self.err("oracle.stock_version", "must be v23.2.15")
        urls = o["doc_urls"]
        if not isinstance(urls, list) or not urls or not all(
                isinstance(u, str) and u.startswith(DOC_PREFIX) for u in urls):
            self.err("oracle.doc_urls", "must list public v23.2 doc URLs")
        ver = o["verification"]
        if not isinstance(ver, str) or ver not in ("unverified", "verified"):
            self.err("oracle.verification", "must be unverified or verified")
        lk = o["license_key_state"]
        if not isinstance(lk, str) or lk not in ("not_checked", "not_required", "present_redacted"):
            self.err("oracle.license_key_state", "invalid value")
        self.text("oracle.evidence_note", o["evidence_note"])
        ra = o["recorded_at"]
        if ra is not None and not iso8601(ra):
            self.err("oracle.recorded_at", "must be null or a valid ISO-8601 date/time")
        if ver == "unverified" and ra is not None:
            self.err("oracle.recorded_at", "must be null while unverified")
        if ver == "verified":
            if ra is None:
                self.err("oracle.recorded_at", "verified requires an ISO-8601 date/time")
            if incomplete or "pending_fields" in o or "pending" in yaml.safe_dump(d):
                self.err("oracle", "verified case may not contain pending comparators")
        if "pending_fields" in o:
            pf = o["pending_fields"]
            if not isinstance(pf, list) or not pf or not all(is_text(x) for x in pf):
                self.err("oracle.pending_fields", "must be a non-empty list of strings")


def validate_text(text, path):
    """Validate one case's text. Returns (errors, id or None). Never raises."""
    c = Checker(path)
    try:
        docs = list(yaml.safe_load_all(text))
    except yaml.YAMLError as ex:
        c.err("<file>", "invalid YAML: " + (str(ex).splitlines() or ["?"])[0])
        return c.errs, None
    except Exception as ex:  # e.g. unhashable complex keys
        c.err("<file>", f"unloadable YAML: {type(ex).__name__}: {ex}")
        return c.errs, None
    if len(docs) != 1 or docs[0] is None:
        c.err("<file>", "must contain exactly one non-empty YAML document")
        return c.errs, None
    try:
        c.run(docs[0])
    except Exception as ex:  # defensive: a validator bug must not abort the run
        c.err("<internal>", f"validator error {type(ex).__name__}: {ex}")
    cid = docs[0].get("id") if isinstance(docs[0], dict) else None
    return c.errs, cid if isinstance(cid, str) else None


def main(argv):
    here = os.path.dirname(os.path.abspath(__file__))
    files = argv or sorted(glob.glob(os.path.join(here, "..", "cases", "*.yaml")))
    errs, ids = [], {}
    for f in files:
        rel = os.path.relpath(f)
        try:
            with open(f, encoding="utf-8") as fh:
                text = fh.read()
        except (OSError, UnicodeDecodeError) as ex:
            errs.append(f"{rel}: <file>: unreadable: {ex}")
            continue
        e, cid = validate_text(text, rel)
        if cid is not None:
            if cid in ids:
                e.append(f"{rel}: id: duplicate of {ids[cid]}")
            ids[cid] = rel
        errs += e
    for e in errs:
        print(e)
    print(f"{len(files)} case(s), {len(errs)} error(s)")
    return 1 if errs or not files else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
