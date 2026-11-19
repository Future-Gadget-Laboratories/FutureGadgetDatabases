// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// This file is an original FGDb regression for PAR-201. It is not
// copied from Cockroach Labs source.

package bulk_test

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/kv/bulk"
	"github.com/cockroachdb/cockroach/pkg/kv/kvclient/kvcoord"
	"github.com/cockroachdb/cockroach/pkg/kv/kvclient/rangecache"
	"github.com/cockroachdb/cockroach/pkg/kv/kvpb"
	"github.com/cockroachdb/cockroach/pkg/kv/kvserver"
	"github.com/cockroachdb/cockroach/pkg/kv/kvserver/kvserverbase"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/testutils/serverutils"
	"github.com/cockroachdb/cockroach/pkg/util/encoding"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/limit"
	"github.com/cockroachdb/cockroach/pkg/util/mon"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// par201TablePrefix is a synthetic index prefix used only by the PAR-201 test.
func par201TablePrefix() roachpb.Key {
	return keys.SystemSQLCodec.IndexPrefix(4001, 1)
}

func par201IndexKey(prefix roachpb.Key, i int) roachpb.Key {
	return encoding.EncodeStringAscending(append([]byte{}, prefix...), fmt.Sprintf("k%d", i))
}

// par201OverlapsFirstRange reports whether an AddSSTable spans the first
// test range [prefix, k3). addSSTable retries any non-RangeKeyMismatch
// error up to 10 times, so the test filter must fail every overlap.
func par201OverlapsFirstRange(req *kvpb.AddSSTableRequest, prefix, firstRangeEnd roachpb.Key) bool {
	overlapsPrefix := bytes.HasPrefix(req.Key, prefix) || bytes.HasPrefix(req.EndKey, prefix) ||
		(req.Key.Compare(prefix) <= 0 && req.EndKey.Compare(prefix) > 0)
	overlapsFirst := req.Key.Compare(firstRangeEnd) < 0 && req.EndKey.Compare(prefix) > 0
	return overlapsPrefix && overlapsFirst
}

func par201FailFirstRangeAddSSTable(
	armed *atomic.Bool, failed *atomic.Int32, injected error,
) func(context.Context, *kvpb.BatchRequest) *kvpb.Error {
	prefix := par201TablePrefix()
	firstRangeEnd := par201IndexKey(prefix, 3)
	return func(_ context.Context, ba *kvpb.BatchRequest) *kvpb.Error {
		if !armed.Load() {
			return nil
		}
		for _, ru := range ba.Requests {
			req, ok := ru.GetInner().(*kvpb.AddSSTableRequest)
			if !ok {
				continue
			}
			if par201OverlapsFirstRange(req, prefix, firstRangeEnd) {
				failed.Add(1)
				return kvpb.NewError(injected)
			}
		}
		return nil
	}
}

// TestPAR201RangeFlushErrorNotSwallowed reproduces the public v23.2.24 /
// v23.2.26 note "Fixed a rare corruption bug that impacts import and
// materialized views" on the shared BulkAdder ingest path (IMPORT and
// materialized-view refresh both flush through SSTBatcher).
//
// When a range-boundary flush is sent asynchronously, SSTBatcher.Reset waits
// for that work and, in the v23.2.15 pin, only logs the error. The caller
// (BulkAdder.Add) then continues, and a later Flush can report success even
// though the failed SST's keys were never ingested.
func TestPAR201RangeFlushErrorNotSwallowed(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	var armed atomic.Bool
	var failed atomic.Int32
	injected := errors.New("injected AddSSTable failure for PAR-201")

	// Fail every AddSSTable that overlaps the first range [k0, k3) so
	// retries cannot succeed, while later ranges can still ingest.
	s, _, kvDB := serverutils.StartServer(t, base.TestServerArgs{
		Knobs: base.TestingKnobs{
			Store: &kvserver.StoreTestingKnobs{
				TestingRequestFilter: par201FailFirstRangeAddSSTable(&armed, &failed, injected),
			},
		},
	})
	defer s.Stopper().Stop(ctx)

	prefix := par201TablePrefix()
	key := func(i int) roachpb.Key { return par201IndexKey(prefix, i) }
	const splitAt = 3

	require.NoError(t, kvDB.AdminSplit(ctx, key(splitAt), hlc.MaxTimestamp))

	// Populate a range cache so the batcher will flush at the known split
	// (the rangeFlush path that can run asynchronously).
	ds := s.DistSenderI().(*kvcoord.DistSender)
	mockCache := rangecache.NewRangeCache(s.ClusterSettings(), ds,
		func() int64 { return 2 << 10 }, s.Stopper())
	for _, k := range []int{0, splitAt} {
		ent, err := ds.RangeDescriptorCache().Lookup(ctx, keys.MustAddr(key(k)))
		require.NoError(t, err)
		mockCache.Insert(ctx, roachpb.RangeInfo{Desc: *ent.Desc()})
	}

	ts := hlc.Timestamp{WallTime: 100}
	mem := mon.NewUnlimitedMonitor(ctx, "par201", mon.MemoryResource, nil, nil, 0, nil)
	reqs := limit.MakeConcurrentRequestLimiter("par201-reqs", 1000)
	b, err := bulk.MakeBulkAdder(
		ctx, kvDB, mockCache, s.ClusterSettings(), ts,
		kvserverbase.BulkAdderOptions{Name: "par201", MaxBufferSize: func() int64 { return 32 << 20 }},
		mem, reqs,
	)
	require.NoError(t, err)
	defer b.Close(ctx)

	var expected []kv.KeyValue
	for i := 0; i < 6; i++ {
		k := key(i)
		v := roachpb.MakeValueFromString(fmt.Sprintf("value-%d", i))
		v.Timestamp = ts
		v.InitChecksum(k)
		expected = append(expected, kv.KeyValue{Key: k, Value: &v})
	}

	armed.Store(true)
	var addErr error
	for i := 0; i < 6; i++ {
		if err := b.Add(ctx, expected[i].Key, expected[i].Value.RawBytes); err != nil {
			addErr = err
			break
		}
	}
	flushErr := b.Flush(ctx)
	armed.Store(false)

	require.Greater(t, failed.Load(), int32(0), "expected the request filter to fail at least one AddSSTable")

	got, scanErr := kvDB.Scan(ctx, key(0), key(8), 0)
	require.NoError(t, scanErr)

	t.Logf("filter failures=%d addErr=%v flushErr=%v scanned=%d/%d keys",
		failed.Load(), addErr, flushErr, len(got), len(expected))

	// Correct behavior: either the failed ingest is reported to the caller, or
	// every added key is present. Silent success with a missing prefix is the
	// corruption the public fix-note describes.
	if addErr == nil && flushErr == nil {
		require.Equal(t, expected, got, "BulkAdder reported success but the ingested key set does not match (PAR-201)")
	}
}
