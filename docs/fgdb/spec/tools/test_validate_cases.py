# SPDX-License-Identifier: Apache-2.0
"""Tests for validate_cases.py. Run: python3 -m unittest discover -s docs/fgdb/spec/tools"""
import copy
import glob
import os
import sys
import unittest

import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import validate_cases as vc  # noqa: E402

CASES = sorted(glob.glob(os.path.join(HERE, "..", "cases", "*.yaml")))
BASE_PATH = "s1-full-table.yaml"


def check(text, path=BASE_PATH):
    errs, _ = vc.validate_text(text, path)
    return errs


def base():
    with open(os.path.join(HERE, "..", "cases", BASE_PATH), encoding="utf-8") as fh:
        return yaml.safe_load(fh)


def dump(d):
    return yaml.safe_dump(d, sort_keys=False, allow_unicode=True)


def paths(obj, prefix=()):
    yield prefix
    if isinstance(obj, dict):
        for k, v in obj.items():
            yield from paths(v, prefix + (k,))
    elif isinstance(obj, list):
        for i, v in enumerate(obj):
            yield from paths(v, prefix + (i,))


def parent(d, path):
    for p in path[:-1]:
        d = d[p]
    return d


class RealCases(unittest.TestCase):
    def test_all_real_cases_pass(self):
        self.assertEqual(len(CASES), 19)
        for f in CASES:
            with open(f, encoding="utf-8") as fh:
                errs, cid = vc.validate_text(fh.read(), os.path.basename(f))
            self.assertEqual(errs, [], f)
            self.assertIsNotNone(cid)
        self.assertEqual(vc.main(CASES), 0)


class Malformed(unittest.TestCase):
    def assertRejected(self, text, needle, path=BASE_PATH):
        errs = check(text, path)
        self.assertTrue(errs, text)
        self.assertFalse([e for e in errs if "<internal>" in e], errs)
        self.assertTrue(any(needle in e for e in errs), (needle, errs))

    def test_only_schema_version(self):
        self.assertRejected("schema_version: 1\n", "missing required key 'id'")

    def test_non_mapping_top(self):
        for text in ("- a\n- b\n", "42\n", "just text\n"):
            self.assertRejected(text, "must be a mapping")

    def test_empty_and_multi_doc(self):
        for text in ("", "\n", "---\n", "# only a comment\n", "a: 1\n---\nb: 2\n"):
            self.assertRejected(text, "exactly one non-empty YAML document")

    def test_invalid_yaml(self):
        for text in ("a: [1, 2\n", "a: b: c\n", "\t- x\n", "? [a, b]\n: 1\n"):
            errs = check(text)
            self.assertTrue(errs, text)
            self.assertFalse([e for e in errs if "<internal>" in e], errs)

    def test_unknown_key_rejected(self):
        d = base()
        d["extra"] = 1
        self.assertRejected(dump(d), "unknown key 'extra'")
        d = base()
        d["expect"]["result"]["surprise"] = True
        self.assertRejected(dump(d), "unknown key 'surprise'")

    def test_message_pattern_must_compile(self):
        d = base()
        d["expect"] = {"error": {"sqlstate": "42P01", "message_pattern": "^bad[($"}}
        self.assertRejected(dump(d), "does not compile")
        d["expect"]["error"]["message_pattern"] = "^ok.*$"
        self.assertFalse([e for e in check(dump(d)) if "message_pattern" in e])

    def test_recorded_at(self):
        d = base()
        o = d["oracle"]
        o.update(verification="verified", recorded_at="not-a-date")
        self.assertRejected(dump(d), "valid ISO-8601")
        for good in ("2026-10-21", "2026-10-21T22:01:00Z", "2026-10-21T22:01:00-06:00"):
            o["recorded_at"] = good
            self.assertFalse([e for e in check(dump(d)) if "ISO-8601" in e], good)
        o["recorded_at"] = 12345
        self.assertRejected(dump(d), "valid ISO-8601")

    def test_unbound_reference(self):
        d = base()
        d["statement"]["sql"] = "SELECT '${nope}'"
        self.assertRejected(dump(d), "unbound reference ${nope}")

    def test_fuzz_never_raises(self):
        """Delete every node and replace it with each wrong type; never an internal error."""
        b = base()
        junk = [None, 0, True, "x", [], {}, [None], {"k": None}, [[1]], 1.5]
        for path in list(paths(b)):
            if not path:
                continue
            variants = []
            d = copy.deepcopy(b)
            par = parent(d, path)
            if isinstance(par, dict):
                del par[path[-1]]
            else:
                par.pop(path[-1])
            variants.append(d)
            for j in junk:
                d = copy.deepcopy(b)
                parent(d, path)[path[-1]] = j
                variants.append(d)
            for v in variants:
                errs = check(dump(v))
                self.assertFalse([e for e in errs if "<internal>" in e], (path, errs))


if __name__ == "__main__":
    unittest.main()
