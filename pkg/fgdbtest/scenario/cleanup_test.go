// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
)

func TestCleanupErrorFailsRun(t *testing.T) {
	r := &run{
		rep: report.New(report.Meta{}),
		halt: func(context.Context) error {
			return errors.New("stop failed")
		},
	}
	err := joinCleanup(nil, r.finish())
	if err == nil || !strings.Contains(err.Error(), "stop failed") {
		t.Fatalf("cleanup error was dropped: %v", err)
	}
	if !strings.Contains(r.rep.Markdown(), "result: fail") {
		t.Fatalf("summary still passed:\n%s", r.rep.Markdown())
	}
	if !strings.Contains(r.rep.Markdown(), "partition NOT tested") {
		t.Fatalf("summary:\n%s", r.rep.Markdown())
	}
}

func TestCleanupErrorJoinsScenarioError(t *testing.T) {
	r := &run{
		rep: report.New(report.Meta{}),
		halt: func(context.Context) error {
			return errors.New("stop failed")
		},
	}
	err := joinCleanup(errors.New("bank failed"), r.finish())
	if err == nil || !strings.Contains(err.Error(), "bank failed") || !strings.Contains(err.Error(), "stop failed") {
		t.Fatalf("joined result %v", err)
	}
}
