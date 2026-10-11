// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotTestedStepStaysInTheReport(t *testing.T) {
	root := t.TempDir()
	writeTier(t, root, "weekly", `
name: weekly
steps:
  - id: ELLE
    kind: not-tested
    detail: The external checker is not in this repository.
`)
	out := filepath.Join(t.TempDir(), "out")
	h := &Harness{Root: root, TierName: "weekly", Output: out}
	if err := h.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := readSummary(t, out)
	if !strings.HasPrefix(body, "partition NOT tested\ndisk faults NOT tested\n") {
		t.Fatalf("summary:\n%s", body)
	}
	if !strings.Contains(body, "ELLE: not_tested") {
		t.Fatalf("summary:\n%s", body)
	}
}

func TestMissingBazelFailsTheTier(t *testing.T) {
	root := t.TempDir()
	writeTier(t, root, "interim", `
name: interim
steps:
  - id: KV-001
    kind: bazel
    claim: KV-001
    target: //pkg/kv/kvnemesis:kvnemesis_test
`)
	out := filepath.Join(t.TempDir(), "out")
	h := &Harness{
		Root: root, TierName: "interim", Output: out,
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		Command: func(context.Context, string, ...string) error {
			t.Fatal("bazel was invoked")
			return nil
		},
	}
	if err := h.Run(context.Background()); err == nil {
		t.Fatal("expected the tier to fail")
	}
	body := readSummary(t, out)
	if !strings.Contains(body, "not run here") || !strings.Contains(body, "result: fail") {
		t.Fatalf("summary:\n%s", body)
	}
}

func TestRealTiersParse(t *testing.T) {
	root, err := FindRoot(".")
	if err != nil {
		t.Skip(err)
	}
	for _, name := range []string{"pr", "nightly", "weekly", "interim"} {
		if _, err := LoadTier(root, name); err != nil {
			t.Errorf("tier %s: %v", name, err)
		}
	}
}

func writeTier(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "fgdb", "test", "tiers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSummary(t *testing.T, dir string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
