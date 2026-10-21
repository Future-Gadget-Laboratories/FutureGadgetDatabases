// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindRoot walks parents of start until it finds fgdb/test/claims.yaml.
func FindRoot(start string) (string, error) {
	starts := candidateRoots(start)
	for _, dir := range starts {
		if root := walkRoot(dir); root != "" {
			return root, nil
		}
	}
	return "", fmt.Errorf("cannot find fgdb/test/claims.yaml")
}

func candidateRoots(start string) []string {
	var starts []string
	if start != "" {
		starts = append(starts, start)
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if src := os.Getenv("TEST_SRCDIR"); src != "" {
		ws := os.Getenv("TEST_WORKSPACE")
		if ws == "" {
			ws = "_main"
		}
		starts = append(starts, filepath.Join(src, ws))
	}
	return starts
}

func walkRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "fgdb", "test", "claims.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
