// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package batcheval

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/kv/kvpb"
	"github.com/cockroachdb/cockroach/pkg/kv/kvserver/kvserverbase"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/storage"
	"github.com/cockroachdb/cockroach/pkg/storage/enginepb"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// MockEvalCtx's ContainsKey always returns false. Use real descriptor membership
// here so the point-key regression cases exercise collection as well as filtering.
type gcSpanEvalContext struct{ EvalContext }

func (e gcSpanEvalContext) ContainsKey(key roachpb.Key) bool {
	return e.Desc().ContainsKey(roachpb.RKey(key))
}

type gcSpanGuardBounds struct {
	name                   string
	descStart, descEnd     string
	headerStart, headerEnd string
}

type gcSpanGuardCase struct {
	name, start, end string
	reject           bool
}

func TestGCSpanGuard(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	ctx := context.Background()
	ts := hlc.Timestamp{WallTime: 1}
	for _, bounds := range []gcSpanGuardBounds{
		{"both", "b", "y", "b", "y"},
		{"descriptor", "b", "y", "a", "z"},
		{"header", "a", "z", "b", "y"},
	} {
		t.Run(bounds.name, func(t *testing.T) {
			for _, kind := range []string{"range-keys", "partial-clear-range"} {
				t.Run(kind, func(t *testing.T) {
					for _, tc := range []gcSpanGuardCase{
						{"inside", "d", "f", false},
						{"equals-bounds", "b", "y", false},
						{"end-equals-bound", "d", "y", false},
						{"crossing-start", "a", "d", true},
						{"crossing-end", "w", "z", true},
						{"outside-left", "a", "b", true},
						{"outside-right", "y", "z", true},
					} {
						t.Run(tc.name, func(t *testing.T) {
							runGCSpanGuardCase(t, ctx, ts, bounds, kind, tc)
						})
					}
				})
			}
		})
	}
}

func runGCSpanGuardCase(
	t *testing.T, ctx context.Context, ts hlc.Timestamp, bounds gcSpanGuardBounds, kind string, tc gcSpanGuardCase,
) {
	t.Helper()
	eng := storage.NewDefaultInMemForTesting()
	defer eng.Close()
	batch := eng.NewBatch()
	defer batch.Close()
	req := &kvpb.GCRequest{RequestHeader: kvpb.RequestHeader{
		Key: roachpb.Key(bounds.headerStart), EndKey: roachpb.Key(bounds.headerEnd),
	}}
	if kind == "range-keys" {
		req.RangeKeys = []kvpb.GCRequest_GCRangeKey{{
			StartKey: roachpb.Key(tc.start), EndKey: roachpb.Key(tc.end), Timestamp: ts,
		}}
	} else {
		req.ClearRange = &kvpb.GCRequest_GCClearRange{
			StartKey: roachpb.Key(tc.start), EndKey: roachpb.Key(tc.end), StartKeyTimestamp: ts,
		}
	}
	evalCtx := (&MockEvalCtx{Desc: &roachpb.RangeDescriptor{
		RangeID: 1, StartKey: roachpb.RKey(bounds.descStart), EndKey: roachpb.RKey(bounds.descEnd),
	}, GCThreshold: ts}).EvalContext()
	var stats enginepb.MVCCStats
	_, err := GC(ctx, batch, CommandArgs{
		EvalCtx: evalCtx, Header: kvpb.Header{Timestamp: ts}, Args: req, Stats: &stats,
	}, &kvpb.GCResponse{})
	if tc.reject {
		require.Error(t, err)
		require.False(t, errors.IsAssertionFailure(err))
		span := roachpb.Span{Key: roachpb.Key(tc.start), EndKey: roachpb.Key(tc.end)}
		require.Contains(t, err.Error(), span.String())
		bound := roachpb.Span{Key: roachpb.Key("b"), EndKey: roachpb.Key("y")}
		require.Contains(t, err.Error(), bound.String())
		require.True(t, batch.Empty(), "rejection must precede all writes")
		require.Equal(t, enginepb.MVCCStats{}, stats)
	} else {
		require.NoError(t, err)
	}
}

func TestGCSpanGuardNoWrites(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	ctx := context.Background()
	ts := hlc.Timestamp{WallTime: 1}
	for _, kind := range []string{"range-keys", "partial-clear-range", "whole-clear-range"} {
		t.Run(kind, func(t *testing.T) {
			eng := storage.NewDefaultInMemForTesting()
			defer eng.Close()
			_, _, err := storage.MVCCDelete(ctx, eng, roachpb.Key("c"), ts, storage.MVCCWriteOptions{})
			require.NoError(t, err)
			require.NoError(t, storage.MVCCDeleteRangeUsingTombstone(ctx, eng, nil,
				roachpb.Key("d"), roachpb.Key("f"), ts, hlc.ClockTimestamp{}, nil, nil, false, 0, nil))
			mock := &MockEvalCtx{
				Desc:        &roachpb.RangeDescriptor{RangeID: 1, StartKey: roachpb.RKey("b"), EndKey: roachpb.RKey("y")},
				GCThreshold: ts,
				EvalKnobs:   kvserverbase.BatchEvalTestingKnobs{AllowGCWithNewThresholdAndKeys: true},
			}
			evalCtx := gcSpanEvalContext{mock.EvalContext()}
			sl := MakeStateLoader(evalCtx)
			hint := &roachpb.GCHint{LatestRangeDeleteTimestamp: ts}
			require.NoError(t, sl.SetGCHint(ctx, eng, nil, hint))
			req := &kvpb.GCRequest{
				RequestHeader: kvpb.RequestHeader{Key: roachpb.Key("b"), EndKey: roachpb.Key("y")},
				Threshold:     hlc.Timestamp{WallTime: 2},
			}
			if kind == "range-keys" {
				req.Keys = []kvpb.GCRequest_GCKey{{Key: roachpb.Key("c"), Timestamp: ts}}
				req.RangeKeys = []kvpb.GCRequest_GCRangeKey{
					{StartKey: roachpb.Key("d"), EndKey: roachpb.Key("f"), Timestamp: ts},
					{StartKey: roachpb.Key("x"), EndKey: roachpb.Key("z"), Timestamp: ts},
				}
			} else {
				req.ClearRange = &kvpb.GCRequest_GCClearRange{StartKey: roachpb.Key("x"), EndKey: roachpb.Key("z")}
				if kind == "partial-clear-range" {
					req.ClearRange.StartKeyTimestamp = ts
				}
			}
			batch := eng.NewBatch()
			defer batch.Close()
			var stats enginepb.MVCCStats
			res, err := GC(ctx, batch, CommandArgs{
				EvalCtx: evalCtx, Header: kvpb.Header{Timestamp: hlc.Timestamp{WallTime: 3}}, Args: req, Stats: &stats,
			}, &kvpb.GCResponse{})
			require.Error(t, err)
			require.False(t, errors.IsAssertionFailure(err))
			require.True(t, batch.Empty(), "no point GC, range GC, threshold, or hint writes on rejection")
			require.Equal(t, enginepb.MVCCStats{}, stats)
			require.Nil(t, res.Replicated.State)
			gotHint, err := sl.LoadGCHint(ctx, batch)
			require.NoError(t, err)
			require.Equal(t, hint, gotHint)
		})
	}
}

func TestGCSpanGuardUnchangedRequests(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	ctx := context.Background()
	ts := hlc.Timestamp{WallTime: 1}
	for _, kind := range []string{
		"point-keys", "whole-clear-range", "whole-clear-range-subset",
		"whole-clear-range-narrow-header", "threshold-only",
	} {
		t.Run(kind, func(t *testing.T) {
			runGCSpanGuardUnchangedRequest(t, ctx, ts, kind)
		})
	}
}

func runGCSpanGuardUnchangedRequest(t *testing.T, ctx context.Context, ts hlc.Timestamp, kind string) {
	t.Helper()
	eng := storage.NewDefaultInMemForTesting()
	defer eng.Close()
	mock := &MockEvalCtx{
		Desc:        &roachpb.RangeDescriptor{RangeID: 1, StartKey: roachpb.RKey("b"), EndKey: roachpb.RKey("y")},
		GCThreshold: ts,
	}
	evalCtx := gcSpanEvalContext{mock.EvalContext()}
	req := &kvpb.GCRequest{RequestHeader: kvpb.RequestHeader{Key: roachpb.Key("b"), EndKey: roachpb.Key("y")}}
	setupGCSpanGuardUnchangedRequest(t, ctx, ts, kind, eng, mock, req)
	batch := eng.NewBatch()
	defer batch.Close()
	var stats enginepb.MVCCStats
	res, err := GC(ctx, batch, CommandArgs{
		EvalCtx: evalCtx, Header: kvpb.Header{Timestamp: hlc.Timestamp{WallTime: 3}}, Args: req, Stats: &stats,
	}, &kvpb.GCResponse{})
	if kind == "whole-clear-range-subset" {
		require.EqualError(t, err, "gc with clear range operation could only be used on the full range")
		require.True(t, batch.Empty())
		return
	}
	if kind == "whole-clear-range-narrow-header" {
		require.Error(t, err)
		require.False(t, errors.IsAssertionFailure(err))
		require.Contains(t, err.Error(), (roachpb.Span{Key: req.Key, EndKey: req.EndKey}).String())
		require.True(t, batch.Empty())
		require.Equal(t, enginepb.MVCCStats{}, stats)
		return
	}
	require.NoError(t, err)
	switch kind {
	case "point-keys":
		for _, key := range []roachpb.Key{roachpb.Key("a"), roachpb.Key("c"), roachpb.Key("z")} {
			got, err := storage.MVCCGet(ctx, batch, key, ts, storage.MVCCGetOptions{Tombstones: true})
			require.NoError(t, err)
			if key.Equal(roachpb.Key("c")) {
				require.Nil(t, got.Value)
			} else {
				require.NotNil(t, got.Value, "out-of-range point tombstone must remain")
			}
		}
	case "whole-clear-range":
		iter, err := batch.NewMVCCIterator(storage.MVCCKeyIterKind, storage.IterOptions{
			LowerBound: req.Key, UpperBound: req.EndKey, KeyTypes: storage.IterKeyTypePointsAndRanges,
		})
		require.NoError(t, err)
		defer iter.Close()
		iter.SeekGE(storage.MVCCKey{Key: req.Key})
		ok, err := iter.Valid()
		require.NoError(t, err)
		require.False(t, ok, "whole-range GC must remove the range tombstone")
	case "threshold-only":
		require.NotNil(t, res.Replicated.State)
		require.Equal(t, &req.Threshold, res.Replicated.State.GCThreshold)
		threshold, err := MakeStateLoader(evalCtx).LoadGCThreshold(ctx, batch)
		require.NoError(t, err)
		require.Equal(t, &req.Threshold, threshold)
	}
}

func setupGCSpanGuardUnchangedRequest(
	t *testing.T, ctx context.Context, ts hlc.Timestamp, kind string,
	eng storage.Engine, mock *MockEvalCtx, req *kvpb.GCRequest,
) {
	t.Helper()
	switch kind {
	case "point-keys":
		for _, key := range []roachpb.Key{roachpb.Key("a"), roachpb.Key("c"), roachpb.Key("z")} {
			_, _, err := storage.MVCCDelete(ctx, eng, key, ts, storage.MVCCWriteOptions{})
			require.NoError(t, err)
			req.Keys = append(req.Keys, kvpb.GCRequest_GCKey{Key: key, Timestamp: ts})
		}
	case "whole-clear-range", "whole-clear-range-subset", "whole-clear-range-narrow-header":
		require.NoError(t, storage.MVCCDeleteRangeUsingTombstone(ctx, eng, &mock.Stats,
			roachpb.Key("d"), roachpb.Key("f"), ts, hlc.ClockTimestamp{}, nil, nil, false, 0, nil))
		req.ClearRange = &kvpb.GCRequest_GCClearRange{StartKey: roachpb.Key("b"), EndKey: roachpb.Key("y")}
		if kind == "whole-clear-range-subset" {
			req.ClearRange.EndKey = roachpb.Key("x")
		}
		if kind == "whole-clear-range-narrow-header" {
			req.Key, req.EndKey = roachpb.Key("c"), roachpb.Key("x")
		}
	case "threshold-only":
		req.Threshold = hlc.Timestamp{WallTime: 2}
	}
}
