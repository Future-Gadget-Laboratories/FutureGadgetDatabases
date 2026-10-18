// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package claims

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRejectsCipherBankField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.yaml")
	body := "- id: FT-001\n  cipherbank_need: yes\n  claim: x\n  tests: [a]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "cipherbank_need") {
		t.Fatalf("got %v", err)
	}
}

func TestClaimWithoutTestFails(t *testing.T) {
	dir := fixture(t, "docs/fgdb/architecture.md", "line\n")
	writeTest(t, dir, "pkg/fgdbtest/scenarios/cluster", "TestThreeNodeSlice")
	raw := `- id: FT-001
  claim: One node can die and the database can keep serving.
  source: docs/fgdb/architecture.md#L1
  category: faults
  tests: []
  tier: interim
  status: partial
`
	err := checkRaw(t, dir, raw)
	if err == nil || !strings.Contains(err.Error(), "no test") {
		t.Fatalf("got %v", err)
	}
}

func TestKnownTestPasses(t *testing.T) {
	dir := fixture(t, "docs/fgdb/architecture.md", "one\n")
	writeTest(t, dir, "pkg/fgdbtest/scenarios/cluster", "TestThreeNodeSlice")
	raw := `- id: FT-001
  claim: One node can die and the database can keep serving.
  source: docs/fgdb/architecture.md#L1
  category: faults
  tests:
    - pkg/fgdbtest/scenarios/cluster:TestThreeNodeSlice
  tier: interim
  status: partial
`
	if err := checkRaw(t, dir, raw); err != nil {
		t.Fatal(err)
	}
}

func TestUntestedStatusFails(t *testing.T) {
	dir := fixture(t, "docs/fgdb/architecture.md", "one\n")
	writeTest(t, dir, "pkg/demo", "TestDemo")
	raw := `- id: FT-002
  claim: A paused node comes back.
  source: docs/fgdb/architecture.md#L1
  category: faults
  tests:
    - pkg/demo:TestDemo
  tier: pr
  status: untested
`
	err := checkRaw(t, dir, raw)
	if err == nil || !strings.Contains(err.Error(), "untested") {
		t.Fatalf("got %v", err)
	}
}

func TestExpiredWaiverFails(t *testing.T) {
	dir := fixture(t, "docs/fgdb/architecture.md", "one\n")
	writeTest(t, dir, "pkg/demo", "TestDemo")
	raw := `- id: FT-003
  claim: Two nodes can be down.
  source: docs/fgdb/architecture.md#L1
  category: faults
  tests:
    - pkg/demo:TestDemo
  tier: weekly
  status: waived
  owner: suite
  reason: lab units are not installed
  expiry: 2000-01-01
`
	err := checkRaw(t, dir, raw, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "expiry") {
		t.Fatalf("got %v", err)
	}
}

func TestEmptySkipListPasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skips.yaml")
	if err := os.WriteFile(path, []byte("skips: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckSkips(path); err != nil {
		t.Fatal(err)
	}
}

func checkRaw(t *testing.T, dir, raw string, now ...time.Time) error {
	t.Helper()
	path := filepath.Join(dir, "claims.yaml")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := Load(path)
	if err != nil {
		return err
	}
	opt := Options{RepoRoot: dir}
	if len(now) == 1 {
		opt.Now = now[0]
	}
	return Check(list, opt)
}

func fixture(t *testing.T, rel, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeTest(t *testing.T, dir, pkg, name string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(pkg), "x_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "package p\nfunc " + name + "() {}\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
