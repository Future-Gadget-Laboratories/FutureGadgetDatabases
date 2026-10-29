// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package repstream

import (
	"context"
	"errors"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/clusterunique"
	"github.com/cockroachdb/cockroach/pkg/sql/isql"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/eval"
)

// Embedded interfaces supply the unused methods. These tests only dispatch
// managers; they never invoke a replication implementation.
type testProducerManager struct{ eval.ReplicationStreamManager }
type testIngestManager struct{ eval.StreamIngestManager }
type testTxn struct{ isql.Txn }

func TestManagerHooksAreIndependent(t *testing.T) {
	producerHook, ingestHook := GetReplicationStreamManagerHook, GetStreamIngestManagerHook
	t.Cleanup(func() {
		GetReplicationStreamManagerHook = producerHook
		GetStreamIngestManagerHook = ingestHook
	})
	for _, tc := range []struct {
		name     string
		producer bool
		ingest   bool
	}{
		{name: "neither"},
		{name: "producer-only", producer: true},
		{name: "ingest-only", ingest: true},
		{name: "both", producer: true, ingest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Do not parallelize: these hook variables are process globals.
			GetReplicationStreamManagerHook, GetStreamIngestManagerHook = nil, nil
			producer, ingest := &testProducerManager{}, &testIngestManager{}
			producerCalls, ingestCalls := 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			evalCtx, txn := &eval.Context{}, &testTxn{}
			sessionID := clusterunique.IDFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
			if tc.producer {
				GetReplicationStreamManagerHook = func(gotCtx context.Context, gotEval *eval.Context, gotTxn isql.Txn, gotID clusterunique.ID) (eval.ReplicationStreamManager, error) {
					producerCalls++
					checkHookArguments(t, gotCtx == ctx && gotEval == evalCtx && gotTxn == txn && gotID == sessionID)
					return producer, nil
				}
			}
			if tc.ingest {
				GetStreamIngestManagerHook = func(gotCtx context.Context, gotEval *eval.Context, gotTxn isql.Txn, gotID clusterunique.ID) (eval.StreamIngestManager, error) {
					ingestCalls++
					checkHookArguments(t, gotCtx == ctx && gotEval == evalCtx && gotTxn == txn && gotID == sessionID)
					return ingest, nil
				}
			}
			gotProducer, producerErr := GetReplicationStreamManager(ctx, evalCtx, txn, sessionID)
			checkManagerResult(t, tc.producer, gotProducer, producer, producerErr, producerCalls)
			gotIngest, ingestErr := GetStreamIngestManager(ctx, evalCtx, txn, sessionID)
			checkManagerResult(t, tc.ingest, gotIngest, ingest, ingestErr, ingestCalls)
		})
	}
}

func checkHookArguments(t *testing.T, match bool) {
	t.Helper()
	if !match {
		t.Fatal("hook arguments were not forwarded unchanged")
	}
}

func checkManagerResult(t *testing.T, enabled bool, got, want interface{}, err error, calls int) {
	t.Helper()
	if enabled {
		if err != nil || got != want || calls != 1 {
			t.Fatalf("enabled hook: manager=%v error=%v calls=%d", got, err, calls)
		}
		return
	}
	if got != nil || err == nil || err.Error() != "replication streaming requires a CCL binary" || calls != 0 {
		t.Fatalf("missing hook: manager=%v error=%v calls=%d", got, err, calls)
	}
}

func TestManagerHookErrorsPropagate(t *testing.T) {
	producerHook, ingestHook := GetReplicationStreamManagerHook, GetStreamIngestManagerHook
	t.Cleanup(func() {
		GetReplicationStreamManagerHook = producerHook
		GetStreamIngestManagerHook = ingestHook
	})
	want := errors.New("manager hook failed")
	GetReplicationStreamManagerHook = func(context.Context, *eval.Context, isql.Txn, clusterunique.ID) (eval.ReplicationStreamManager, error) {
		return nil, want
	}
	GetStreamIngestManagerHook = func(context.Context, *eval.Context, isql.Txn, clusterunique.ID) (eval.StreamIngestManager, error) {
		return nil, want
	}
	var sessionID clusterunique.ID
	if _, err := GetReplicationStreamManager(context.Background(), nil, nil, sessionID); err != want {
		t.Fatalf("producer error = %v, want original hook error", err)
	}
	if _, err := GetStreamIngestManager(context.Background(), nil, nil, sessionID); err != want {
		t.Fatalf("ingest error = %v, want original hook error", err)
	}
}
