// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package workdir

import (
	"path/filepath"
	"testing"
)

func TestRequireStaysUnderParent(t *testing.T) {
	t.Setenv(envWork, t.TempDir())
	root := Parent()
	if err := Require(filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	if err := Require(filepath.Dir(root)); err == nil {
		t.Fatal("a parent of the work root was accepted")
	}
}
