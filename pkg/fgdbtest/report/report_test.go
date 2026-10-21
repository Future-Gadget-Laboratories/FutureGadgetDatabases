// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

func TestSummaryLeadsWithUntestedFaults(t *testing.T) {
	rep := New(Meta{Tier: "interim", RunID: "local"})
	rep.Set("FT-001", labels.Pass, "")
	rep.Set("FT-010", labels.NotTested, labels.PartitionNotTested)
	dir := t.TempDir()
	if err := rep.Write(dir); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.HasPrefix(text, labels.PartitionNotTested+"\n"+labels.DiskFaultsNotTested+"\n") {
		t.Fatalf("summary:\n%s", text)
	}
	jsonBody, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonBody), labels.PartitionNotTested) {
		t.Fatalf("result.json:\n%s", jsonBody)
	}
}

func TestFailSticks(t *testing.T) {
	rep := New(Meta{})
	rep.Set("FT-001", labels.Fail, "lost a write")
	rep.Set("FT-001", labels.Pass, "")
	rep.Set("FT-001", labels.NotTested, "")
	if rep.Outcome("FT-001") != labels.Fail {
		t.Fatalf("outcome %s", rep.Outcome("FT-001"))
	}
}
