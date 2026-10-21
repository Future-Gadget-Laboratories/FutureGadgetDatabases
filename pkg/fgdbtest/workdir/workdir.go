// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package workdir keeps one suite run's files under a single temporary root.
package workdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envWork   = "FGDB_WORK"
	envRunner = "RUNNER_TEMP"
	envTmp    = "TMPDIR"
)

// Parent is the per-run root when FGDB_WORK is set, otherwise RUNNER_TEMP or TMPDIR.
func Parent() string {
	for _, key := range []string{envWork, envRunner, envTmp} {
		if dir := os.Getenv(key); dir != "" {
			return dir
		}
	}
	return os.TempDir()
}

// Mkdir creates a directory under Parent.
func Mkdir(pattern string) (string, error) {
	return os.MkdirTemp(Parent(), pattern)
}

// Require rejects a path that sits outside Parent.
func Require(path string) error {
	if path == "" {
		return fmt.Errorf("work directory is empty")
	}
	parent := filepath.Clean(Parent())
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(parent, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return outside(path, parent)
	}
	return nil
}

func outside(path, parent string) error {
	return fmt.Errorf("work directory %s is outside %s", path, parent)
}
