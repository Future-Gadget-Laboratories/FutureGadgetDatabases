// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package record

import (
	"errors"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyCommitKeepsAmbiguousResults(t *testing.T) {
	refused := errors.New("dial tcp 127.0.0.1:1: connect: connection refused")
	if got := classify(refused); got != labels.ResultFail {
		t.Fatalf("a refused start was %s", got)
	}
	if got := classifyCommit(refused); got != labels.ResultUnknown {
		t.Fatalf("a refused commit was %s", got)
	}
	ambiguous := &pgconn.PgError{Code: "40001", Message: "result is ambiguous"}
	if got := classifyCommit(ambiguous); got != labels.ResultUnknown {
		t.Fatalf("ambiguous commit was %s", got)
	}
	aborted := &pgconn.PgError{Code: "40001", Message: "restart transaction"}
	if got := classifyCommit(aborted); got != labels.ResultFail {
		t.Fatalf("aborted commit was %s", got)
	}
}
