// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package pgcrypto

import (
	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/eval"
)

// featurePgcryptoCiphersEnabled gates the encrypt, encrypt_iv, decrypt and
// decrypt_iv builtins. It is read on every call so that changing it takes
// effect without a restart.
var featurePgcryptoCiphersEnabled = settings.RegisterBoolSetting(
	settings.ApplicationLevel,
	"feature.pgcrypto_ciphers.enabled",
	"set to true to enable the pgcrypto encrypt and decrypt builtins, false to return the CCL-required error; default is false",
	false,
	settings.WithPublic)

// cipherEnabled reports whether the cipher builtins are enabled for this call.
func cipherEnabled(evalCtx *eval.Context) bool {
	return featurePgcryptoCiphersEnabled.Get(&evalCtx.Settings.SV)
}
