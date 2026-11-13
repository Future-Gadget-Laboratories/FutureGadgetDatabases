// Copyright 2023 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package pgcrypto

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/sql/sem/eval"
)

// The hooks below are assigned to real implementations. Each call checks
// feature.pgcrypto_ciphers.enabled and returns the CCL-required error when the
// setting is false.

var Decrypt = func(_ context.Context, evalCtx *eval.Context, data []byte, key []byte, cipherType string) ([]byte, error) {
	if !cipherEnabled(evalCtx) {
		return nil, cipherCCLRequiredError("decrypt")
	}
	return decryptData(data, key, nil, cipherType)
}

var DecryptIV = func(_ context.Context, evalCtx *eval.Context, data []byte, key []byte, iv []byte, cipherType string) ([]byte, error) {
	if !cipherEnabled(evalCtx) {
		return nil, cipherCCLRequiredError("decrypt_iv")
	}
	return decryptData(data, key, iv, cipherType)
}

var Encrypt = func(_ context.Context, evalCtx *eval.Context, data []byte, key []byte, cipherType string) ([]byte, error) {
	if !cipherEnabled(evalCtx) {
		return nil, cipherCCLRequiredError("encrypt")
	}
	return encryptData(data, key, nil, cipherType)
}

var EncryptIV = func(_ context.Context, evalCtx *eval.Context, data []byte, key []byte, iv []byte, cipherType string) ([]byte, error) {
	if !cipherEnabled(evalCtx) {
		return nil, cipherCCLRequiredError("encrypt_iv")
	}
	return encryptData(data, key, iv, cipherType)
}
