// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package claims

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skip is one waived upstream test. The list is reviewed like code.
type Skip struct {
	Test   string `yaml:"test"`
	Reason string `yaml:"reason"`
	Issue  string `yaml:"issue"`
}

// SkipFile is fgdb/test/skips.yaml.
type SkipFile struct {
	Skips []Skip `yaml:"skips"`
}

// CheckSkips fails when a skip has no test, reason, or issue link.
// An empty list is valid. This slice does not waive upstream tests.
func CheckSkips(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file SkipFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("parse skips: %w", err)
	}
	var problems []string
	for i, skip := range file.Skips {
		if strings.TrimSpace(skip.Test) == "" || strings.TrimSpace(skip.Reason) == "" || strings.TrimSpace(skip.Issue) == "" {
			problems = append(problems, fmt.Sprintf("skip %d needs a test, a reason, and an issue link", i+1))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("skip list:\n%s", strings.Join(problems, "\n"))
}
