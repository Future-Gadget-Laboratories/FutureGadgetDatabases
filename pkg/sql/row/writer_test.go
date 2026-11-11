// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package row

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/kv/kvpb"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/tabledesc"
	"github.com/cockroachdb/cockroach/pkg/sql/rowenc/valueside"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
	"github.com/stretchr/testify/require"
)

// TestCompactPrimaryKeyFamilyOverwrite checks PAR-208 cleanup without changing
// the compact encoding or adding KV operations for unrelated column shapes.
func TestCompactPrimaryKeyFamilyOverwrite(t *testing.T) {
	composite, err := tree.ParseDDecimal("1.00")
	require.NoError(t, err)
	nonComposite, err := tree.ParseDDecimal("1")
	require.NoError(t, err)
	require.True(t, composite.IsComposite())
	require.False(t, nonComposite.IsComposite())

	for _, tc := range []struct {
		name                                   string
		value                                  tree.Datum
		primaryKey, stored, overwrite, updated bool
		wantOp                                 string
	}{
		{"remove-old-composite", nonComposite, true, false, true, true, "Delete"},
		{"keep-composite-encoding", composite, true, false, true, true, "Put"},
		{"insert-recoverable-key", nonComposite, true, false, false, true, ""},
		{"integer-key", tree.NewDInt(1), true, false, true, true, ""},
		{"integer-array-key", tree.NewDArray(types.Int), true, false, true, true, ""},
		{"stored-non-key", nonComposite, false, true, true, true, "Put"},
		{"column-outside-primary-index", nonComposite, false, false, true, true, ""},
		{"family-not-updated", nonComposite, true, false, true, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desc := compactPrimaryKeyFamilyTestDesc(tc.value, tc.primaryKey, tc.stored)
			helper := RowHelper{Codec: keys.SystemSQLCodec, TableDesc: desc}
			helper.Init()
			cols := desc.PublicColumns()
			colMap := ColIDtoRowIndexFromCols(cols)
			var updatedCols catalog.TableColMap
			if tc.updated {
				updatedCols.Set(2, 1)
			}
			var batch kv.Batch
			var key roachpb.Key
			var value roachpb.Value
			primaryKey := []byte{1, 2, 3}
			_, err := prepareInsertOrUpdateBatch(
				context.Background(), &KVBatchAdapter{Batch: &batch}, &helper,
				primaryKey, cols, []tree.Datum{tree.NewDInt(1), tc.value}, colMap,
				updatedCols, &key, &value, nil, insertPutFn, tc.overwrite, false,
			)
			require.NoError(t, err)
			requests := batch.Requests()
			if tc.wantOp == "" {
				require.Empty(t, requests)
				return
			}
			require.Len(t, requests, 1)
			request := requests[0].GetInner()
			require.Equal(t, tc.wantOp, request.Method().String())
			require.Equal(t, roachpb.Key(keys.MakeFamilyKey(primaryKey, 1)), request.Header().Key)
			if put, ok := request.(*kvpb.PutRequest); ok {
				expected, err := valueside.MarshalLegacy(tc.value.ResolvedType(), tc.value)
				require.NoError(t, err)
				require.True(t, expected.EqualTagAndData(put.Value))
			}
		})
	}
}

func compactPrimaryKeyFamilyTestDesc(
	value tree.Datum, primaryKey, stored bool,
) catalog.TableDescriptor {
	primaryKeyCol := descpb.ColumnID(1)
	if primaryKey {
		primaryKeyCol = 2
	}
	var storedCols []descpb.ColumnID
	if stored {
		storedCols = []descpb.ColumnID{2}
	}
	// Model the retained DefaultColumnID after b joins the primary key.
	return tabledesc.NewBuilder(&descpb.TableDescriptor{
		ID: 100,
		Columns: []descpb.ColumnDescriptor{
			{ID: 1, Name: "k", Type: tree.NewDInt(1).ResolvedType()},
			{ID: 2, Name: "b", Type: value.ResolvedType()},
		},
		PrimaryIndex: descpb.IndexDescriptor{
			ID: 1, KeyColumnIDs: []descpb.ColumnID{primaryKeyCol}, StoreColumnIDs: storedCols,
		},
		Families: []descpb.ColumnFamilyDescriptor{
			{ID: 0, ColumnIDs: []descpb.ColumnID{1}},
			{ID: 1, ColumnIDs: []descpb.ColumnID{2}, DefaultColumnID: 2},
		},
	}).BuildImmutableTable()
}
