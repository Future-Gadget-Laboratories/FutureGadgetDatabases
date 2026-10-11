// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
)

func TestOnlyStepsRejectedForRelease(t *testing.T) {
	cfg := Config{OnlySteps: ParseOnly(stepKill), Meta: report.Meta{Tier: tierInterim}}
	if err := rejectOnlySteps(cfg); err == nil {
		t.Fatal("interim accepted a filter")
	}
	cfg.Meta.Tier = tierRelease
	if err := rejectOnlySteps(cfg); err == nil {
		t.Fatal("release accepted a filter")
	}
	cfg.Meta.Tier = "pr"
	cfg.RequireCluster = true
	if err := rejectOnlySteps(cfg); err == nil {
		t.Fatal("required cluster accepted a filter")
	}
	cfg.RequireCluster = false
	if err := rejectOnlySteps(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyStepsAreNotAPass(t *testing.T) {
	r := &run{
		cfg: Config{OnlySteps: ParseOnly(stepKill)},
		rep: report.New(report.Meta{Tier: "pr"}),
	}
	r.note(ClaimKill, nil)
	r.failUnrecorded("step did not run")
	if r.rep.Outcome(ClaimKill) != labels.Pass {
		t.Fatalf("kill outcome %s", r.rep.Outcome(ClaimKill))
	}
	if r.rep.Outcome(ClaimPause) != labels.NotTested {
		t.Fatalf("pause outcome %s", r.rep.Outcome(ClaimPause))
	}
	if strings.Contains(r.rep.Markdown(), "result: pass") {
		t.Fatalf("filtered run passed:\n%s", r.rep.Markdown())
	}
}
