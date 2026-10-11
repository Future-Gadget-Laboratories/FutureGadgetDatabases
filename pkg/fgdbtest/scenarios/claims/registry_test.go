// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package claims_test

import (
	"path/filepath"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/claims"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/harness"
)

func TestRegistryFile(t *testing.T) {
	root, err := harness.FindRoot(".")
	if err != nil {
		t.Fatal(err)
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
