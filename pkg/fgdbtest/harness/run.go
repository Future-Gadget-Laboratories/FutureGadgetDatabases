// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package harness runs one suite tier and writes the report.
package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/claims"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/cluster"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/faults"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/prodimport"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/scenario"
)

// Harness runs the steps in one tier file.
type Harness struct {
	Root     string
	TierName string
	Output   string
	LookPath func(string) (string, error)
	Command  func(ctx context.Context, name string, args ...string) error
	rep      *report.Report
	failed   bool
}

// Run executes the tier and writes result.json and summary.md.
func (h *Harness) Run(ctx context.Context) error {
	if err := h.prepare(); err != nil {
		return err
	}
	tier, err := LoadTier(h.Root, h.TierName)
	if err != nil {
		return err
	}
	for _, step := range tier.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		h.record(step, h.execute(ctx, step))
	}
	h.ensureFaultLines()
	if err := h.rep.Write(h.Output); err != nil {
		return err
	}
	if h.failed {
		return errors.New("suite tier failed")
	}
	return nil
}

func (h *Harness) prepare() error {
	if h.Root == "" || h.TierName == "" || h.Output == "" {
		return errors.New("root, tier, and output are required")
	}
	if h.LookPath == nil {
		h.LookPath = exec.LookPath
	}
	if h.Command == nil {
		h.Command = runCommand
	}
	_ = os.Setenv("FGDB_REPO_ROOT", h.Root)
	h.rep = report.New(report.Meta{
		RunID:           os.Getenv("FGDB_RUN_ID"),
		SourceSHA:       os.Getenv("FGDB_SOURCE_SHA"),
		CandidateSHA256: os.Getenv("FGDB_CANDIDATE_SHA256"),
		PreviousSHA256:  os.Getenv("FGDB_PREVIOUS_SHA256"),
		Tier:            h.TierName,
	})
	return nil
}

func (h *Harness) execute(ctx context.Context, step Step) error {
	switch step.Kind {
	case kindClaims:
		return h.checkClaims()
	case kindImports:
		return h.checkImports()
	case kindGoTest:
		return h.goTest(ctx, step)
	case kindBazel:
		return h.bazel(ctx, step)
	case kindCluster:
		return h.cluster(ctx)
	case kindNotTested:
		return errNotTested
	default:
		return fmt.Errorf("unknown kind %s", step.Kind)
	}
}

func (h *Harness) record(step Step, err error) {
	outcome, detail := labels.Pass, ""
	switch {
	case err == nil:
	case errors.Is(err, errNotTested) && step.Kind == kindNotTested:
		outcome = labels.NotTested
		detail = step.Detail
	case err != nil:
		if errors.Is(err, errNotTested) {
			outcome = labels.NotTested
		} else {
			outcome = labels.Fail
		}
		detail = err.Error()
		h.failed = true
	}
	if step.Claim != "" {
		h.rep.Set(step.Claim, outcome, detail)
	}
	h.rep.AddStep(report.StepResult{
		ID: step.ID, Outcome: outcome, Detail: detail, Required: step.Kind != kindNotTested,
	})
}

var errNotTested = errors.New("not tested")

func (h *Harness) checkClaims() error {
	path := filepath.Join(h.Root, "fgdb", "test", "claims.yaml")
	list, err := claims.Load(path)
	if err != nil {
		return err
	}
	if err := claims.Check(list, claims.Options{RepoRoot: h.Root}); err != nil {
		return err
	}
	return claims.CheckSkips(filepath.Join(h.Root, "fgdb", "test", "skips.yaml"))
}

func (h *Harness) checkImports() error {
	base := filepath.Join(h.Root, "fgdb", "test", "test-code-in-prod-baseline.txt")
	return prodimport.Check(base, h.Root)
}

func (h *Harness) goTest(ctx context.Context, step Step) error {
	args := append([]string{"test", "-count=1"}, step.Packages...)
	return h.Command(ctx, "go", args...)
}

func (h *Harness) bazel(ctx context.Context, step Step) error {
	if _, err := h.LookPath("bazel"); err != nil {
		return fmt.Errorf("%w: bazel is not installed on this machine. This step was not run here", errNotTested)
	}
	args := []string{"test", step.Target, "--test_output=errors"}
	if step.Filter != "" {
		args = append(args, "--test_filter="+step.Filter)
	}
	if cpu := os.Getenv("FGDB_LOCAL_CPU"); cpu != "" {
		args = append(args, "--local_cpu_resources="+cpu)
	}
	if ram := os.Getenv("FGDB_LOCAL_RAM_MB"); ram != "" {
		args = append(args, "--local_ram_resources="+ram)
	}
	tmp := filepath.Join(h.Output, "bazel-tmp", step.ID)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	args = append(args, "--test_tmpdir="+tmp)
	if err := h.Command(ctx, "bazel", args...); err != nil {
		return err
	}
	return nil
}

func (h *Harness) cluster(ctx context.Context) error {
	if os.Getenv("FGDB_COCKROACH") == "" {
		return fmt.Errorf("%w: FGDB_COCKROACH is unset. The 3-node slice was not run here", errNotTested)
	}
	sub := filepath.Join(h.Output, "cluster")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return err
	}
	_ = os.Setenv("FGDB_OUTPUT", sub)
	_ = os.Setenv("FGDB_REQUIRE_CLUSTER", "1")
	err := h.Command(ctx, "go", "test", "-count=1", "-timeout", "45m",
		"./pkg/fgdbtest/scenarios/cluster", "-run", "^TestThreeNodeSlice$")
	if rep, readErr := report.Read(sub); readErr == nil {
		h.merge(rep)
	}
	return err
}

func (h *Harness) merge(rep *report.Report) {
	for _, claim := range rep.Claims {
		h.rep.Set(claim.ID, claim.Outcome, claim.Detail)
	}
	for _, step := range rep.Steps {
		h.rep.AddStep(step)
	}
}

func (h *Harness) ensureFaultLines() {
	if h.rep.Outcome(scenario.ClaimPartition) == "" {
		h.rep.Set(scenario.ClaimPartition, labels.NotTested, faults.Partition().Detail)
	}
	if h.rep.Outcome(scenario.ClaimDisk) == "" {
		h.rep.Set(scenario.ClaimDisk, labels.NotTested, faults.Disk().Detail)
	}
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	env, err := cluster.CommandEnv()
	if err != nil {
		return err
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = os.Getenv("FGDB_REPO_ROOT")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
