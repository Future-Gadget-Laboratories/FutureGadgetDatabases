// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package prodimport freezes production Go files that import test helpers.
package prodimport

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	testutilsImport = "github.com/cockroachdb/cockroach/pkg/testutils"
	fgdbtestImport  = "github.com/cockroachdb/cockroach/pkg/fgdbtest"
)

// excludedRoots are trees that are allowed to import test packages.
var excludedRoots = []string{
	"pkg/testutils",
	"pkg/fgdbtest",
	"pkg/cmd/fgdb-test",
	"pkg/cmd/roachtest",
}

// Scan lists "path import" lines for production files that import test helpers.
func Scan(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if skipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !includeFile(rel) {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		for _, imp := range imports {
			if isTestHelper(imp) {
				found = append(found, rel+" "+imp)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

func skipDir(rel string) bool {
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	switch base {
	case ".git", "vendor", "node_modules", "testdata", "artifacts":
		return true
	}
	for _, root := range excludedRoots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func includeFile(rel string) bool {
	if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
		return false
	}
	return !skipDir(rel) && !strings.Contains(rel, "/testdata/")
}

func isTestHelper(imp string) bool {
	if imp == testutilsImport || strings.HasPrefix(imp, testutilsImport+"/") {
		return true
	}
	return imp == fgdbtestImport || strings.HasPrefix(imp, fgdbtestImport+"/")
}

func fileImports(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		out = append(out, strings.Trim(spec.Path.Value, `"`))
	}
	return out, nil
}

// LoadBaseline reads the frozen list. Blank lines and # comments are ignored.
func LoadBaseline(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Strings(lines)
	return lines, nil
}

// Added returns imports that are in found and not in the baseline.
// Removals are allowed. A smaller baseline is a later cleanup, not a failure.
func Added(baseline, found []string) []string {
	have := map[string]struct{}{}
	for _, line := range baseline {
		have[line] = struct{}{}
	}
	var added []string
	for _, line := range found {
		if _, ok := have[line]; !ok {
			added = append(added, line)
		}
	}
	return added
}

// Check fails when found contains an import the baseline does not list.
func Check(baselinePath, root string) error {
	base, err := LoadBaseline(baselinePath)
	if err != nil {
		return err
	}
	found, err := Scan(root)
	if err != nil {
		return err
	}
	added := Added(base, found)
	if len(added) == 0 {
		return nil
	}
	return fmt.Errorf("new production imports of test packages:\n%s", strings.Join(added, "\n"))
}
