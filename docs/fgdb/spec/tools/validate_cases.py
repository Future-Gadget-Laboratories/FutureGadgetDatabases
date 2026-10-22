#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# FGL docs-side validator for PAR-031 conformance case schema v1. Not a runner.
"""Usage: validate_cases.py [case.yaml ...]  (default: ../cases/*.yaml)"""
import glob, os, re, sys
import yaml

NAME = re.compile(r"^[a-z][a-z0-9_]*$")
REF = re.compile(r"\$\{([^}]*)\}")
SQLSTATE = re.compile(r"^[0-9A-Z]{5}$")
MASK_KINDS = {"job_id", "timestamp", "size", "path"}
SCALAR = (str, int, float, bool, type(None))


class Checker:
    def __init__(self, path):
        self.path, self.errs = path, []

    def err(self, where, msg):
        self.errs.append(f"{self.path}: {where}: {msg}")

    def keys(self, where, obj, required, optional=()):
        if not isinstance(obj, dict):
            return self.err(where, "must be a mapping") or False
        for k in required:
            if k not in obj:
                self.err(where, f"missing required key '{k}'")
        for k in obj:
            if k not in required and k not in optional:
                self.err(where, f"unknown key '{k}'")
        return True

    def text(self, where, v, nullable=False):
        if v is None and nullable:
            return
        if not isinstance(v, str) or not v.strip():
            self.err(where, "must be a non-empty string")

    def sql(self, where, s, bound):
        self.text(where, s)
        if isinstance(s, str):
            for ref in REF.findall(s):
                if ref not in bound:
                    self.err(where, f"unbound reference ${{{ref}}}")

    def on(self, where, v, sides):
        if v not in ("source", "target"):
            self.err(where, "'on' must be source or target")
        elif v not in sides:
            self.err(where, f"'on: {v}' has no matching fixture")

    def expect(self, where, e, bound, new_binds):
        """Returns True if any comparator is incomplete (needs a pending note)."""
        if not self.keys(where, e, (), ("result", "error", "masks")):
            return False
        if ("result" in e) == ("error" in e):
            self.err(where, "exactly one of result or error is required")
        incomplete = False
        cols = None
        if "result" in e:
            r, w = e["result"], where + ".result"
            if self.keys(w, r, ("columns", "rows", "row_match"), ("values", "capture")):
                cols = r["columns"]
                if cols is not None and (not isinstance(cols, list) or not cols
                                         or not all(isinstance(c, str) and c for c in cols)
                                         or len(set(cols)) != len(cols)):
                    self.err(w, "columns must be null or a non-empty list of unique strings")
                    cols = None
                if r["row_match"] not in ("exact", "contains", "pending"):
                    self.err(w, "row_match must be exact, contains or pending")
                rows = r["rows"]
                if rows is None:
                    incomplete = True
                    if r["row_match"] != "pending":
                        self.err(w, "rows null requires row_match pending")
                elif not isinstance(rows, list) or not all(isinstance(x, dict) for x in rows):
                    self.err(w, "rows must be null or a list of mappings")
                else:
                    if r["row_match"] == "pending":
                        self.err(w, "row_match pending requires rows null")
                    if cols is None:
                        self.err(w, "rows require known columns")
                    for x in rows:
                        if cols is not None and set(x) != set(cols):
                            self.err(w, "row keys must equal columns")
                        if not all(isinstance(v, SCALAR) for v in x.values()):
                            self.err(w, "row values must be scalars")
                if cols is None:
                    incomplete = True
                for opt, sub in (("values", "values"), ("capture", "capture")):
                    m = r.get(opt)
                    if m is None:
                        continue
                    if not isinstance(m, dict) or not m:
                        self.err(w, f"{sub} must be a non-empty mapping")
                        continue
                    for c, v in m.items():
                        if cols is None or c not in cols:
                            self.err(w, f"{sub} column '{c}' not in columns")
                        if opt == "values" and not isinstance(v, SCALAR):
                            self.err(w, f"values['{c}'] must be a scalar")
                        if opt == "capture":
                            if not isinstance(v, str) or not NAME.match(v):
                                self.err(w, f"capture binding '{v}' invalid")
                            elif v in bound or v in new_binds:
                                self.err(w, f"binding '{v}' already defined")
                            else:
                                new_binds.add(v)
        if "error" in e:
            er, w = e["error"], where + ".error"
            if self.keys(w, er, ("sqlstate", "message_pattern"), ("pending",)):
                s, m = er["sqlstate"], er["message_pattern"]
                if s is not None and not (isinstance(s, str) and SQLSTATE.match(s)):
                    self.err(w, "sqlstate must be null or 5 characters [0-9A-Z]")
                if m is not None:
                    self.text(w, m)
                    if isinstance(m, str) and not (m.startswith("^") and m.endswith("$")):
                        self.err(w, "message_pattern must be anchored ^...$")
                if s is None or m is None:
                    if "pending" not in er:
                        self.err(w, "null sqlstate/message_pattern requires pending")
                    else:
                        self.text(w + ".pending", er["pending"])
                elif "pending" in er:
                    self.err(w, "pending present but error is complete")
        masks = e.get("masks")
        if masks is not None:
            if not isinstance(masks, list) or not masks:
                self.err(where, "masks must be a non-empty list")
            else:
                seen = set()
                for i, mk in enumerate(masks):
                    w = f"{where}.masks[{i}]"
                    if self.keys(w, mk, ("field", "kind")):
                        if mk["kind"] not in MASK_KINDS:
                            self.err(w, "unknown mask kind")
                        if "result" not in e:
                            self.err(w, "masks apply only to result")
                        elif cols is not None and mk["field"] not in cols:
                            self.err(w, f"mask field '{mk['field']}' not in columns")
                        if mk["field"] in seen:
                            self.err(w, "duplicate mask field")
                        seen.add(mk["field"])
        return incomplete

    def run(self, d):
        top_req = ("schema_version", "id", "slice", "description", "fixtures",
                   "variables", "setup", "statement", "expect", "post", "oracle")
        if not self.keys("<top>", d, top_req):
            return
        if d["schema_version"] != 1 or isinstance(d["schema_version"], bool):
            self.err("schema_version", "must be integer 1")
        stem = os.path.splitext(os.path.basename(self.path))[0]
        if d["id"] != stem:
            self.err("id", f"must equal file name '{stem}'")
        if d["slice"] not in ("S1", "S2", "S3"):
            self.err("slice", "must be S1, S2 or S3")
        if isinstance(d["id"], str) and not d["id"].lower().startswith(str(d["slice"]).lower() + "-"):
            self.err("id", "must start with the slice prefix")
        self.text("description", d["description"])
        sides = set()
        if self.keys("fixtures", d["fixtures"], ("source",), ("target",)):
            for k, v in d["fixtures"].items():
                self.text(f"fixtures.{k}", v)
                sides.add(k)
        bound = set()
        if self.keys("variables", d["variables"], ("collection",), tuple(
                k for k in (d["variables"] or {}) if k != "collection")):
            for k, v in d["variables"].items():
                if not NAME.match(k):
                    self.err("variables", f"invalid name '{k}'")
                self.text(f"variables.{k}", v)
                if isinstance(v, str) and REF.search(v):
                    self.err(f"variables.{k}", "variables may not reference other bindings")
                bound.add(k)
        if not isinstance(d["setup"], list) or not d["setup"]:
            self.err("setup", "must be a non-empty list")
        else:
            for i, s in enumerate(d["setup"]):
                if self.keys(f"setup[{i}]", s, ("on", "sql")):
                    self.on(f"setup[{i}]", s["on"], sides)
                    self.sql(f"setup[{i}].sql", s["sql"], bound)
        st = d["statement"]
        if self.keys("statement", st, ("on", "sql")):
            self.on("statement", st["on"], sides)
            self.sql("statement.sql", st["sql"], bound)
        new = set()
        if self.expect("expect", d["expect"], bound, new) and not d["oracle"].get("pending_fields") \
                if isinstance(d["oracle"], dict) else False:
            self.err("expect", "incomplete statement expectation requires oracle.pending_fields")
        bound |= new
        if not isinstance(d["post"], list):
            self.err("post", "must be a list (may be empty)")
        else:
            for i, p in enumerate(d["post"]):
                w = f"post[{i}]"
                if not self.keys(w, p, ("on", "sql", "expect"), ("pending",)):
                    continue
                self.on(w, p["on"], sides)
                self.sql(w + ".sql", p["sql"], bound)
                new = set()
                inc = self.expect(w + ".expect", p["expect"], bound, new)
                bound |= new
                if inc and "pending" not in p:
                    self.err(w, "incomplete result requires pending")
                if "pending" in p:
                    self.text(w + ".pending", p["pending"])
        o = d["oracle"]
        if self.keys("oracle", o, ("stock_version", "doc_urls", "verification", "recorded_at",
                                   "license_key_state", "evidence_note"), ("pending_fields",)):
            if o["stock_version"] != "v23.2.15":
                self.err("oracle.stock_version", "must be v23.2.15")
            urls = o["doc_urls"]
            if not isinstance(urls, list) or not urls or not all(
                    isinstance(u, str) and u.startswith("https://www.cockroachlabs.com/docs/v23.2/")
                    for u in urls):
                self.err("oracle.doc_urls", "must list public v23.2 doc URLs")
            if o["verification"] not in ("unverified", "verified"):
                self.err("oracle.verification", "must be unverified or verified")
            if o["license_key_state"] not in ("not_checked", "not_required", "present_redacted"):
                self.err("oracle.license_key_state", "invalid value")
            self.text("oracle.evidence_note", o["evidence_note"])
            if o["verification"] == "unverified" and o["recorded_at"] is not None:
                self.err("oracle.recorded_at", "must be null while unverified")
            if o["verification"] == "verified":
                if not isinstance(o["recorded_at"], str):
                    self.err("oracle.recorded_at", "verified requires an ISO-8601 date string")
                if "pending" in yaml.safe_dump(d) or "pending_fields" in o:
                    self.err("oracle", "verified case may not contain pending comparators")
            pf = o.get("pending_fields")
            if pf is not None and (not isinstance(pf, list) or not pf
                                   or not all(isinstance(x, str) and x for x in pf)):
                self.err("oracle.pending_fields", "must be a non-empty list of strings")


def main(argv):
    here = os.path.dirname(os.path.abspath(__file__))
    files = argv or sorted(glob.glob(os.path.join(here, "..", "cases", "*.yaml")))
    errs, ids = [], {}
    for f in files:
        c = Checker(os.path.relpath(f))
        with open(f, encoding="utf-8") as fh:
            docs = list(yaml.safe_load_all(fh))
        if len(docs) != 1:
            c.err("<file>", "must contain exactly one YAML document")
        else:
            c.run(docs[0])
            i = docs[0].get("id") if isinstance(docs[0], dict) else None
            if i in ids:
                c.err("id", f"duplicate of {ids[i]}")
            ids[i] = f
        errs += c.errs
    for e in errs:
        print(e)
    print(f"{len(files)} case(s), {len(errs)} error(s)")
    return 1 if errs or not files else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
