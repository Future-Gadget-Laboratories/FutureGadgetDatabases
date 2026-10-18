// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"strings"
	"testing"
)

func TestParseBankPair(t *testing.T) {
	count, sum, err := parsePair("count,sum\n20,0\n")
	if err != nil {
		t.Fatal(err)
	}
	if count != 20 || sum != 0 {
		t.Fatalf("got (%d, %d)", count, sum)
	}
}

func TestConsistencyStatus(t *testing.T) {
	if err := judgeConsistency("status\nRANGE_CONSISTENT\n"); err != nil {
		t.Fatal(err)
	}
	if err := judgeConsistency("status\nRANGE_INCONSISTENT\n"); err == nil {
		t.Fatal("expected a consistency failure")
	}
	if err := judgeConsistency("status\n"); err == nil {
		t.Fatal("expected an empty result to fail")
	}
}

func TestBankArgsUsesAccountCount(t *testing.T) {
	args := bankArgs(20, []string{"postgresql://root@127.0.0.1:1?sslmode=disable", "postgresql://root@127.0.0.1:2?sslmode=disable"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--rows=20") || !strings.Contains(joined, "--tolerate-errors") {
		t.Fatalf("bank args %v", args)
	}
}

func TestParseOnly(t *testing.T) {
	got := ParseOnly(" node-kill, ,node-pause ")
	if len(got) != 2 {
		t.Fatalf("steps %v", got)
	}
	if _, ok := got[stepKill]; !ok {
		t.Fatal(got)
	}
}
