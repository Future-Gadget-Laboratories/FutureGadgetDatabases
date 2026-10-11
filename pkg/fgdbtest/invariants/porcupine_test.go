// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package invariants

import (
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

func TestPorcupineRejectsEmptyHistory(t *testing.T) {
	err := CheckHistory(nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "cannot run") {
		t.Fatalf("got %v", err)
	}
}

func TestPorcupineAcceptsOrderedHistory(t *testing.T) {
	ops := []Op{
		{Client: 0, Kind: labels.KindWrite, Value: 1, CallNS: 1, ReturnNS: 2, Result: labels.ResultOK},
		{Client: 1, Kind: labels.KindRead, Value: 1, CallNS: 3, ReturnNS: 4, Result: labels.ResultOK},
	}
	if err := CheckHistory(ops, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPorcupineRejectsStaleRead(t *testing.T) {
	ops := []Op{
		{Client: 0, Kind: labels.KindWrite, Value: 1, CallNS: 1, ReturnNS: 2, Result: labels.ResultOK},
		{Client: 1, Kind: labels.KindRead, Value: 0, CallNS: 3, ReturnNS: 4, Result: labels.ResultOK},
	}
	if err := CheckHistory(ops, time.Second); err == nil {
		t.Fatal("expected a linearizability failure")
	}
}

func TestOKWritesInWindow(t *testing.T) {
	ops := []Op{
		{Kind: labels.KindWrite, Value: 1, CallNS: 10, ReturnNS: 11, Result: labels.ResultOK},
		{Kind: labels.KindWrite, Value: 2, CallNS: 20, ReturnNS: 21, Result: labels.ResultFail},
		{Kind: labels.KindRead, Value: 1, CallNS: 12, ReturnNS: 13, Result: labels.ResultOK},
	}
	if got := OKWritesInWindow(ops, 10, 20); got != 1 {
		t.Fatalf("window count %d", got)
	}
	late := []Op{{Kind: labels.KindWrite, Value: 3, CallNS: 12, ReturnNS: 30, Result: labels.ResultOK}}
	if got := OKWritesInWindow(late, 10, 20); got != 0 {
		t.Fatalf("late ack counted as %d", got)
	}
	if got := len(OKWriteValues(ops)); got != 1 {
		t.Fatalf("values %d", got)
	}
}
