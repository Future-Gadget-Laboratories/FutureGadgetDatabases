// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestGateContractClassifier(t *testing.T) {
	const statement = "SELECT gate()"
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "success"},
		{name: "transport", err: errors.New("connection refused")},
		{name: "wrong SQLSTATE", err: &pgconn.PgError{Code: "42501", Message: cclMessage + " *tree.Gate"}},
		{name: "wrong message", err: &pgconn.PgError{Code: cclState, Message: "wrong"}},
		{name: "right", err: &pgconn.PgError{Code: cclState, Message: cclMessage + " *tree.Gate"}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyGateError(tc.err, statement, cclState, cclMessage+" *tree.Gate")
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
