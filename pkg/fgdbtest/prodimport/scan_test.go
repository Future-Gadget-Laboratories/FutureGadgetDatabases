// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package prodimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddedImportFails(t *testing.T) {
	dir := t.TempDir()
	prod := filepath.Join(dir, "pkg", "cli")
	if err := os.MkdirAll(prod, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "package cli\nimport \"github.com/cockroachdb/cockroach/pkg/testutils/serverutils\"\n"
	if err := os.WriteFile(filepath.Join(prod, "demo.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "baseline.txt")
	if err := os.WriteFile(base, []byte("# frozen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Check(base, dir)
	if err == nil || !strings.Contains(err.Error(), "serverutils") {
		t.Fatalf("got %v", err)
	}
}

func TestBaselineMatchesTree(t *testing.T) {
	root := repoRoot()
	sentinel := filepath.Join(root, "pkg", "cli", "testutils.go")
	if _, err := os.Stat(sentinel); err != nil {
		t.Skip("full tree is not available")
	}
	base := filepath.Join(root, "fgdb", "test", "test-code-in-prod-baseline.txt")
	if err := Check(base, root); err != nil {
		t.Fatal(err)
	}
}

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
