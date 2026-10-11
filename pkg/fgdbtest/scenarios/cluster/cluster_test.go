// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/harness"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/scenario"
)

// TestThreeNodeSlice starts a 3-node cockroach-oss cluster and runs the
// interim fault steps. It fails closed when FGDB_REQUIRE_CLUSTER=1 and the
// binary is missing. Otherwise it skips, and the skip is not a pass in CI
// because the interim tier sets that variable.
func TestThreeNodeSlice(t *testing.T) {
	bin := os.Getenv("FGDB_COCKROACH")
	if bin == "" {
		if os.Getenv("FGDB_REQUIRE_CLUSTER") == "1" {
			t.Fatal("FGDB_COCKROACH is required and was not set")
		}
		t.Skip("FGDB_COCKROACH is unset, so the 3-node slice was not run")
	}
	root, err := harness.FindRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("FGDB_OUTPUT")
	if out == "" {
		out = t.TempDir()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	rep, err := scenario.Run(ctx, scenario.Config{
		Candidate: bin,
		Previous:  os.Getenv("FGDB_PREVIOUS"),
		WorkDir:   filepath.Join(out, "work"),
		OutputDir: out,
		RepoRoot:  root,
		ExpectGo:  os.Getenv("FGDB_EXPECT_GO"),
		BackupBin: os.Getenv("FGDB_BACKUP_BIN"),
		Meta: report.Meta{
			RunID:           os.Getenv("FGDB_RUN_ID"),
			SourceSHA:       os.Getenv("FGDB_SOURCE_SHA"),
			CandidateSHA256: os.Getenv("FGDB_CANDIDATE_SHA256"),
			PreviousSHA256:  os.Getenv("FGDB_PREVIOUS_SHA256"),
			Tier:            "interim",
		},
		OnlySteps: scenario.ParseOnly(os.Getenv("FGDB_ONLY_STEPS")),
	})
	if rep != nil {
		t.Log(rep.Markdown())
	}
	if err != nil {
		t.Fatal(err)
	}
}
