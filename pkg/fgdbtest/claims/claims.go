// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package claims loads the claims registry and rejects a claim that has no test.
package claims

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"gopkg.in/yaml.v3"
)

const (
	tierPR       = "pr"
	tierNightly  = "nightly"
	tierWeekly   = "weekly"
	tierInterim  = "interim"
	sourcePrefix = "docs/fgdb/"
)

var (
	idPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]{3}$`)
	tiers     = map[string]struct{}{
		tierPR: {}, tierNightly: {}, tierWeekly: {}, tierInterim: {},
	}
	statuses = map[string]struct{}{
		labels.StatusTested:   {},
		labels.StatusPartial:  {},
		labels.StatusUntested: {},
		labels.StatusWaived:   {},
	}
)

// Claim is one row of fgdb/test/claims.yaml.
type Claim struct {
	ID       string   `yaml:"id"`
	Claim    string   `yaml:"claim"`
	Source   string   `yaml:"source"`
	Category string   `yaml:"category"`
	Tests    []string `yaml:"tests"`
	Tier     string   `yaml:"tier"`
	Status   string   `yaml:"status"`
	Owner    string   `yaml:"owner"`
	Reason   string   `yaml:"reason"`
	Expiry   string   `yaml:"expiry"`
}

// Options controls registry checks that need the git checkout.
type Options struct {
	RepoRoot string
	Now      time.Time
}

// Load reads the registry. A cipherbank_need key is an error.
func Load(path string) ([]Claim, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := rejectForbiddenKeys(raw); err != nil {
		return nil, err
	}
	var claims []Claim
	if err := yaml.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	return claims, nil
}

// Check fails when a claim has no test, an unknown test, or status untested.
func Check(list []Claim, opt Options) error {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	seen := map[string]struct{}{}
	var problems []string
	for _, c := range list {
		problems = append(problems, validateClaim(c, opt)...)
		if c.ID == "" {
			continue
		}
		if _, ok := seen[c.ID]; ok {
			problems = append(problems, fmt.Sprintf("%s: duplicate id", c.ID))
		}
		seen[c.ID] = struct{}{}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("claims registry:\n%s", strings.Join(problems, "\n"))
}

func validateClaim(c Claim, opt Options) []string {
	var problems []string
	add := func(msg string) {
		problems = append(problems, fmt.Sprintf("%s: %s", blankID(c.ID), msg))
	}
	if !idPattern.MatchString(c.ID) {
		add("id must look like FT-001")
	}
	if strings.TrimSpace(c.Claim) == "" {
		add("claim text is empty")
	}
	if err := checkSource(opt.RepoRoot, c.Source); err != nil {
		add(err.Error())
	}
	if strings.TrimSpace(c.Category) == "" {
		add("category is empty")
	}
	if _, ok := tiers[c.Tier]; !ok {
		add("tier must be pr, nightly, weekly, or interim")
	}
	if _, ok := statuses[c.Status]; !ok {
		add("status must be tested, partial, untested, or waived")
	}
	if c.Status == labels.StatusUntested {
		add("untested claims fail the check")
	}
	problems = append(problems, waiverProblems(c, opt.Now)...)
	if len(c.Tests) == 0 {
		add("claim has no test")
	}
	for _, id := range c.Tests {
		if err := ResolveTest(opt.RepoRoot, id); err != nil {
			add(err.Error())
		}
	}
	return problems
}

func waiverProblems(c Claim, now time.Time) []string {
	if c.Status != labels.StatusWaived {
		return nil
	}
	var problems []string
	add := func(msg string) {
		problems = append(problems, fmt.Sprintf("%s: %s", blankID(c.ID), msg))
	}
	if strings.TrimSpace(c.Owner) == "" {
		add("waived claim needs an owner")
	}
	if strings.TrimSpace(c.Reason) == "" {
		add("waived claim needs a reason")
	}
	exp, err := time.Parse("2006-01-02", c.Expiry)
	if err != nil {
		add("waived claim needs an expiry date YYYY-MM-DD")
		return problems
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	expDay := time.Date(exp.Year(), exp.Month(), exp.Day(), 0, 0, 0, 0, time.UTC)
	if expDay.Before(today) {
		add("waived claim expiry is in the past")
	}
	return problems
}

func blankID(id string) string {
	if id == "" {
		return "(missing id)"
	}
	return id
}

func checkSource(root, source string) error {
	path, line, err := splitSource(source)
	if err != nil {
		return err
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return fmt.Errorf("source %s is not a file", source)
	}
	if line == 0 {
		return nil
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	n := strings.Count(string(body), "\n")
	if !strings.HasSuffix(string(body), "\n") {
		n++
	}
	if line > n {
		return fmt.Errorf("source %s points past the end of the file", source)
	}
	return nil
}

func splitSource(source string) (string, int, error) {
	if source == "" || strings.Contains(source, "..") {
		return "", 0, fmt.Errorf("source must be a docs/fgdb path")
	}
	path := source
	line := 0
	if i := strings.Index(source, "#L"); i >= 0 {
		path = source[:i]
		rest := source[i+2:]
		if _, err := fmt.Sscanf(rest, "%d", &line); err != nil || line < 1 || fmt.Sprintf("%d", line) != rest {
			return "", 0, fmt.Errorf("source line anchor is not a number")
		}
	}
	if !strings.HasPrefix(path, sourcePrefix) || strings.Contains(path, "\\") {
		return "", 0, fmt.Errorf("source must stay under docs/fgdb")
	}
	return path, line, nil
}

// ResolveTest reports whether a Go test function or a Bazel target exists.
func ResolveTest(root, id string) error {
	if strings.HasPrefix(id, "//") {
		return resolveBazel(root, id)
	}
	pkg, name, ok := strings.Cut(id, ":")
	if !ok || !strings.HasPrefix(name, "Test") || strings.Contains(pkg, "..") {
		return fmt.Errorf("test id %q is not pkg/path:TestName or //pkg/path:target", id)
	}
	dir := filepath.Join(root, filepath.FromSlash(pkg))
	matches, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		return err
	}
	needle := "func " + name + "("
	for _, file := range matches {
		body, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), needle) {
			return nil
		}
	}
	return fmt.Errorf("test id %s: function %s is not in %s", id, name, pkg)
}

func resolveBazel(root, id string) error {
	pkg, name, ok := strings.Cut(strings.TrimPrefix(id, "//"), ":")
	if !ok || name == "" || strings.Contains(pkg, "..") {
		return fmt.Errorf("bazel target %q is malformed", id)
	}
	path := filepath.Join(root, filepath.FromSlash(pkg), "BUILD.bazel")
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("bazel target %s: %w", id, err)
	}
	if strings.Contains(string(body), `name = "`+name+`"`) {
		return nil
	}
	return fmt.Errorf("bazel target %s is not in %s", id, pkg)
}

func rejectForbiddenKeys(raw []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return fmt.Errorf("parse claims: %w", err)
	}
	var found []string
	walkKeys(&node, func(key string) {
		if key == labels.CipherBankField {
			found = append(found, key)
		}
	})
	if len(found) > 0 {
		return fmt.Errorf("claims registry must not contain %s", labels.CipherBankField)
	}
	return nil
}

func walkKeys(node *yaml.Node, fn func(string)) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			fn(node.Content[i].Value)
			walkKeys(node.Content[i+1], fn)
		}
		return
	}
	for _, child := range node.Content {
		walkKeys(child, fn)
	}
}
