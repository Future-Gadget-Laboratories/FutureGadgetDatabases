// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/jackc/pgx/v5/pgconn"
)

type failingGateExecutor struct {
	err error
}

func (e failingGateExecutor) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, e.err
}

func TestGateSetupFailureReportsRemainingGatesNotTested(t *testing.T) {
	cases := []GateCase{
		{ID: "GATE-FAIL", Setup: []string{"SELECT 1"}, Statement: "SELECT 2"},
		{ID: "GATE-LATER-1", Statement: "SELECT 3"},
		{ID: "GATE-LATER-2", Statement: "SELECT 4"},
	}
	oldCases := gateCases
	gateCases = cases
	t.Cleanup(func() { gateCases = oldCases })

	rep := report.New(report.Meta{})
	setupErr := errors.New("controlled setup failure")
	conn := failingGateExecutor{err: setupErr}
	err := runGateContract(context.Background(), conn, rep)
	if !errors.Is(err, setupErr) {
		t.Fatalf("runGateContract error = %v, want setup failure %v", err, setupErr)
	}
	if rep.Outcome("GATE-FAIL") != labels.Fail {
		t.Fatalf("failed gate outcome = %q, want fail", rep.Outcome("GATE-FAIL"))
	}
	for _, id := range []string{"GATE-LATER-1", "GATE-LATER-2"} {
		if got := rep.Outcome(id); got != "" {
			t.Errorf("unrun gate claim outcome = %q, want no claim update", got)
		}
		found := false
		for _, step := range rep.Steps {
			if step.ID == id && step.Outcome == labels.NotTested && step.Required {
				found = true
			}
		}
		if !found {
			t.Errorf("missing required not_tested step for %s", id)
		}
	}
	if !rep.Rejected() {
		t.Fatal("report accepted a gate setup failure")
	}
}

func TestGate011UsesNonRoutablePlaceholder(t *testing.T) {
	for _, tc := range gateCases {
		if tc.ID == "GATE-011" {
			if strings.Contains(tc.Statement, "127.0.0.1") || !strings.Contains(tc.Statement, "invalid.invalid") {
				t.Fatalf("GATE-011 connection target is not an inert placeholder: %s", tc.Statement)
			}
			return
		}
	}
	t.Fatal("GATE-011 is missing")
}

func TestGateContractClassifier(t *testing.T) {
	const statement = "SELECT gate()"
	const wantMessage = cclMessage + " *tree.Gate"
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "success"},
		{name: "transport", err: errors.New("connection refused")},
		{name: "wrong SQLSTATE", err: &pgconn.PgError{Code: "42501", Message: wantMessage}},
		{name: "wrong message", err: &pgconn.PgError{Code: cclState, Message: "wrong"}},
		{name: "right", err: &pgconn.PgError{Code: cclState, Message: wantMessage}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyGateError(tc.err, statement, cclState, wantMessage)
			if (err == nil) != tc.want {
				t.Fatalf("classifyGateError = %v, want success %v", err, tc.want)
			}
		})
	}
}

func TestGateContractCaseInvariants(t *testing.T) {
	seen := make(map[string]struct{}, len(gateCases))
	for _, tc := range gateCases {
		if _, ok := seen[tc.ID]; ok {
			t.Errorf("duplicate gate id %q", tc.ID)
		}
		seen[tc.ID] = struct{}{}
		if tc.ID == "" || tc.Statement == "" || tc.WantSQLState == "" || tc.WantMessage == "" {
			t.Errorf("incomplete gate case: %+v", tc)
		}
	}
	if len(gateCases) != 24 {
		t.Fatalf("gate case count = %d, want 24 active non-contiguous gates", len(gateCases))
	}
}

// wantGateIDs is the PAR-001 v1.2 active claim set (GATE-006 retired,
// GATE-014 unreachable on cockroach-oss).
var wantGateIDs = []string{
	"GATE-001", "GATE-002", "GATE-003", "GATE-004", "GATE-005", "GATE-007",
	"GATE-008", "GATE-009", "GATE-010", "GATE-011", "GATE-012", "GATE-013",
	"GATE-015", "GATE-016", "GATE-017", "GATE-018", "GATE-019", "GATE-020",
	"GATE-021", "GATE-022", "GATE-023", "GATE-024", "GATE-025", "GATE-026",
}

func TestGateContractExactIDs(t *testing.T) {
	got := gateClaimIDs()
	if len(got) != len(wantGateIDs) {
		t.Fatalf("gate ids = %v, want %v", got, wantGateIDs)
	}
	for i := range wantGateIDs {
		if got[i] != wantGateIDs[i] {
			t.Errorf("gate id %d = %q, want %q", i, got[i], wantGateIDs[i])
		}
	}
}

func TestGateContractRegistry(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "fgdb", "test", "claims.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		// Bazel sandboxes do not stage the registry; the go test tier checks it.
		t.Skip("claims registry not present in this sandbox; registry sync not tested")
	}
	if err != nil {
		t.Fatalf("read claims registry: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*-?\s*id:\s*(GATE-\d{3})\s*$`)
	var registered []string
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		registered = append(registered, m[1])
	}
	sort.Strings(registered)
	want := append([]string(nil), wantGateIDs...)
	sort.Strings(want)
	if strings.Join(registered, ",") != strings.Join(want, ",") {
		t.Fatalf("claims.yaml GATE ids = %v, want %v", registered, want)
	}
}

func TestGateContractTypeBoundary(t *testing.T) {
	want := cclMessage + " *tree.Backup"
	for msg, ok := range map[string]bool{
		cclMessage + " *tree.Backup":         true,
		cclMessage + " *tree.Backup (hint)":  true,
		cclMessage + " *tree.BackupSchedule": false,
		cclMessage + " *tree.Backup_x":       false,
		cclMessage + " *tree.Restore":        false,
	} {
		if got := containsToken(msg, want); got != ok {
			t.Errorf("containsToken(%q) = %v, want %v", msg, got, ok)
		}
	}
}
