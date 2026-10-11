// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	kindClaims    = "check-claims"
	kindImports   = "check-imports"
	kindGoTest    = "go-test"
	kindBazel     = "bazel"
	kindCluster   = "cluster"
	kindNotTested = "not-tested"
)

// Step is one row of a tier file.
type Step struct {
	ID       string   `yaml:"id"`
	Kind     string   `yaml:"kind"`
	Claim    string   `yaml:"claim"`
	Target   string   `yaml:"target"`
	Filter   string   `yaml:"filter"`
	Detail   string   `yaml:"detail"`
	Packages []string `yaml:"packages"`
}

// Tier is fgdb/test/tiers/<name>.yaml.
type Tier struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// LoadTier reads one tier file.
func LoadTier(root, name string) (Tier, error) {
	path := filepath.Join(root, "fgdb", "test", "tiers", name+".yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Tier{}, err
	}
	var tier Tier
	if err := yaml.Unmarshal(raw, &tier); err != nil {
		return Tier{}, fmt.Errorf("parse tier %s: %w", name, err)
	}
	if tier.Name != name {
		return Tier{}, fmt.Errorf("tier file %s says name %q", name, tier.Name)
	}
	if len(tier.Steps) == 0 {
		return Tier{}, fmt.Errorf("tier %s has no steps", name)
	}
	for _, step := range tier.Steps {
		if err := checkStep(step); err != nil {
			return Tier{}, err
		}
	}
	return tier, nil
}

func checkStep(step Step) error {
	if step.ID == "" {
		return fmt.Errorf("a tier step is missing an id")
	}
	switch step.Kind {
	case kindClaims, kindImports, kindCluster, kindNotTested, kindGoTest, kindBazel:
	default:
		return fmt.Errorf("step %s has unknown kind %q", step.ID, step.Kind)
	}
	if step.Kind == kindBazel && step.Target == "" {
		return fmt.Errorf("step %s needs a bazel target", step.ID)
	}
	if step.Kind == kindGoTest && len(step.Packages) == 0 {
		return fmt.Errorf("step %s needs packages", step.ID)
	}
	if step.Kind == kindNotTested && step.Detail == "" {
		return fmt.Errorf("step %s needs a detail that says what was not tested", step.ID)
	}
	return nil
}
