// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package claims_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/build/bazel"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/claims"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/harness"
)

func TestRegistryFile(t *testing.T) {
	var root string
	if bazel.BuiltWithBazel() {
		claimsFile, err := bazel.Runfile("fgdb/test/claims.yaml")
		if err != nil {
			t.Fatalf("locate claims.yaml runfile: %v", err)
		}
		claimsFile, err = filepath.EvalSymlinks(claimsFile)
		if err != nil {
			t.Fatalf("resolve claims.yaml runfile: %v", err)
		}
		root = filepath.Dir(filepath.Dir(filepath.Dir(claimsFile)))
		if _, err := os.Stat(filepath.Join(root, "fgdb", "test", "claims.yaml")); err != nil {
			t.Fatalf("derived repository root %q does not contain claims.yaml: %v", root, err)
		}
		// Check reads the full source registry, so this requires a local or linux-sandbox run with the source tree visible, not remote execution.
	} else {
		var err error
		root, err = harness.FindRoot(".")
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := claims.Load(filepath.Join(root, "fgdb", "test", "claims.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := claims.Check(list, claims.Options{RepoRoot: root}); err != nil {
		t.Fatal(err)
	}
	if err := claims.CheckSkips(filepath.Join(root, "fgdb", "test", "skips.yaml")); err != nil {
		t.Fatal(err)
	}
}
