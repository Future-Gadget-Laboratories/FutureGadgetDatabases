// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package record

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/invariants"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

func TestShutdownUnknownWriteIsKept(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := keepResult(ctx, labels.ResultUnknown); got != labels.ResultUnknown {
		t.Fatalf("shutdown unknown was dropped: %q", got)
	}
	if got := keepResult(ctx, labels.ResultFail); got != "" {
		t.Fatalf("a definite miss was kept at shutdown: %q", got)
	}
	if got := keepResult(ctx, labels.ResultOK); got != labels.ResultOK {
		t.Fatalf("ok result was dropped: %q", got)
	}

	// The read of 2 shows the unknown write was applied. The later read of 1
	// is then stale. Dropping that write leaves only write 1 and read 1,
	// which Porcupine accepts.
	kept := []invariants.Op{
		regOp(0, labels.KindWrite, 1, 1, 2, labels.ResultOK),
		regOp(1, labels.KindWrite, 2, 3, 4, labels.ResultUnknown),
		regOp(2, labels.KindRead, 2, 5, 6, labels.ResultOK),
		regOp(3, labels.KindRead, 1, 7, 8, labels.ResultOK),
	}
	if err := invariants.CheckHistory(kept, time.Second); err == nil {
		t.Fatal("porcupine accepted a stale read after an unknown write that was observed")
	}
	dropped := []invariants.Op{kept[0], kept[3]}
	if err := invariants.CheckHistory(dropped, time.Second); err != nil {
		t.Fatalf("the history passes once the unknown write is dropped: %v", err)
	}
}

func regOp(client int, kind string, value int, call, ret int64, result string) invariants.Op {
	return invariants.Op{
		Client: client, Kind: kind, Value: value,
		CallNS: call, ReturnNS: ret, Result: result,
	}
}
