// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package pgcrypto

import (
	"context"
	_ "embed"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/eval"
	"github.com/stretchr/testify/require"
)

//go:embed conformance/vectors.tsv
var cipherVectorTSV string

func TestCipherConformanceVectors(t *testing.T) {
	ctx := context.Background()
	st := cluster.MakeTestingClusterSettings()
	evalCtx := eval.MakeTestingEvalContext(st)
	featurePgcryptoCiphersEnabled.Override(ctx, &st.SV, true)

	rows := strings.Split(strings.TrimSpace(cipherVectorTSV), "\n")
	require.Greater(t, len(rows), 1)
	require.Equal(t, []string{"id", "func", "type", "key_hex", "iv_hex", "data_hex", "expected_hex", "expected_error", "notes"}, strings.Split(rows[0], "\t"))
	executedRows := 0
	for _, row := range rows[1:] {
		fields := strings.Split(row, "\t")
		require.Len(t, fields, 9, row)
		executedRows++
		t.Run(fields[0], func(t *testing.T) {
			decode := func(value string) []byte {
				if value == "-" {
					return nil
				}
				decoded, err := hex.DecodeString(value)
				require.NoError(t, err)
				return decoded
			}
			data, key, iv := decode(fields[5]), decode(fields[3]), decode(fields[4])
			var got []byte
			var err error
			switch fields[1] {
			case "encrypt":
				got, err = Encrypt(ctx, &evalCtx, data, key, fields[2])
			case "encrypt_iv":
				got, err = EncryptIV(ctx, &evalCtx, data, key, iv, fields[2])
			case "decrypt":
				got, err = Decrypt(ctx, &evalCtx, data, key, fields[2])
			case "decrypt_iv":
				got, err = DecryptIV(ctx, &evalCtx, data, key, iv, fields[2])
			default:
				t.Fatalf("unknown function %q", fields[1])
			}
			if fields[7] != "-" {
				require.EqualError(t, err, fields[7])
				return
			}
			require.NoError(t, err)
			require.Equal(t, decode(fields[6]), got)
		})
	}
	require.Equal(t, len(rows)-1, executedRows)
}
