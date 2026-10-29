// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package pgcrypto

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/eval"
	"github.com/stretchr/testify/require"
)

// The NIST SP 800-38A F.2 CBC vectors use the public example key, IV and
// plaintext from the standard. They are not secrets.
const (
	nistCBCPlaintextHex = "6bc1bee22e409f96e93d7e117393172a" +
		"ae2d8a571e03ac9c9eb76fac45af8e51" +
		"30c81c46a35ce411e5fbc1191a0a52ef" +
		"f69f2445df4f9b17ad2b417be66c3710"
	nistCBCIVHex = "000102030405060708090a0b0c0d0e0f"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// TestCipherNISTVectors checks AES-CBC without padding against NIST SP 800-38A
// F.2.1 (AES-128), F.2.3 (AES-192) and F.2.5 (AES-256).
func TestCipherNISTVectors(t *testing.T) {
	testCases := []struct {
		name          string
		keyHex        string
		ciphertextHex string
	}{
		{
			name:   "AES-128",
			keyHex: "2b7e151628aed2a6abf7158809cf4f3c",
			ciphertextHex: "7649abac8119b246cee98e9b12e9197d" +
				"5086cb9b507219ee95db113a917678b2" +
				"73bed6b8e3c1743b7116e69e22229516" +
				"3ff1caa1681fac09120eca307586e1a7",
		},
		{
			name:   "AES-192",
			keyHex: "8e73b0f7da0e6452c810f32b809079e562f8ead2522c6b7b",
			ciphertextHex: "4f021db243bc633d7178183a9fa071e8" +
				"b4d9ada9ad7dedf4e5e738763f69145a" +
				"571b242012fb7ae07fa9baac3df102e0" +
				"08b0e27988598881d920a9e64f5615cd",
		},
		{
			name:   "AES-256",
			keyHex: "603deb1015ca71be2b73aef0857d77811f352c073b6108d72d9810a30914dff4",
			ciphertextHex: "f58c4c04d6e5f1ba779eabfb5f7bfbd6" +
				"9cfc4e967edb808d679f777bc6702c7d" +
				"39f23369a9d9bacfa530e26304231461" +
				"b2eb05e2c39be9fcda6c19078c6a9d1b",
		},
	}
	plaintext := mustHex(t, nistCBCPlaintextHex)
	iv := mustHex(t, nistCBCIVHex)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := mustHex(t, tc.keyHex)
			ciphertext := mustHex(t, tc.ciphertextHex)

			got, err := encryptData(plaintext, key, iv, "aes-cbc/pad:none")
			require.NoError(t, err)
			require.Equal(t, ciphertext, got)

			decrypted, err := decryptData(ciphertext, key, iv, "aes-cbc/pad:none")
			require.NoError(t, err)
			require.Equal(t, plaintext, decrypted)
		})
	}
}

// TestCipherZeroIVDefault checks that encrypt and decrypt without an IV use
// the all-zero IV.
func TestCipherZeroIVDefault(t *testing.T) {
	key := mustHex(t, "2b7e151628aed2a6abf7158809cf4f3c")
	plaintext := mustHex(t, nistCBCPlaintextHex)
	zeroIV := make([]byte, 16)

	withNil, err := encryptData(plaintext, key, nil, "aes/pad:none")
	require.NoError(t, err)
	withZeroIV, err := encryptData(plaintext, key, zeroIV, "aes/pad:none")
	require.NoError(t, err)
	require.Equal(t, withZeroIV, withNil)

	// A short IV is zero-padded, so a 3-byte IV matches the same IV with
	// trailing zeros.
	shortIV, err := encryptData(plaintext, key, []byte{1, 2, 3}, "aes/pad:none")
	require.NoError(t, err)
	paddedIV, err := encryptData(plaintext, key, append([]byte{1, 2, 3}, make([]byte, 13)...), "aes/pad:none")
	require.NoError(t, err)
	require.Equal(t, paddedIV, shortIV)
}

func TestCipherRoundTrip(t *testing.T) {
	// Keys of 1 to 32 bytes are zero-padded up to 16, 24 or 32 bytes.
	keys := []string{
		"k",
		"key",
		"0123456789abcdef",
		"0123456789abcdef01234",
		"0123456789abcdef01234567",
		"0123456789abcdef0123456789abcdef",
	}
	types := []string{"aes", "aes-cbc", "aes-cbc/pad:pkcs", "aes/pad:pkcs"}
	datas := []string{"", "a", "abc", "0123456789abcdef", "0123456789abcdef0", "0123456789abcdef0123456789abcdef01"}
	iv := []byte("0123456789abcdef")

	for _, k := range keys {
		for _, typ := range types {
			for _, d := range datas {
				t.Run(fmt.Sprintf("%s/%d/%d", typ, len(k), len(d)), func(t *testing.T) {
					ciphertext, err := encryptData([]byte(d), []byte(k), nil, typ)
					require.NoError(t, err)
					require.Equal(t, 0, len(ciphertext)%16)

					plaintext, err := decryptData(ciphertext, []byte(k), nil, typ)
					require.NoError(t, err)
					require.Equal(t, []byte(d), plaintext)

					// Encryption with the same IV is deterministic.
					again, err := encryptData([]byte(d), []byte(k), nil, typ)
					require.NoError(t, err)
					require.Equal(t, ciphertext, again)

					ivCiphertext, err := encryptData([]byte(d), []byte(k), iv, typ)
					require.NoError(t, err)
					ivPlaintext, err := decryptData(ivCiphertext, []byte(k), iv, typ)
					require.NoError(t, err)
					require.Equal(t, []byte(d), ivPlaintext)
				})
			}
		}
	}
}

func TestCipherPKCS7Padding(t *testing.T) {
	// Block-aligned input gains a full block of padding.
	padded := pkcs7Pad(make([]byte, 16))
	require.Len(t, padded, 32)
	require.Equal(t, bytes.Repeat([]byte{16}, 16), padded[16:])

	padded = pkcs7Pad([]byte("abc"))
	require.Len(t, padded, 16)
	require.Equal(t, bytes.Repeat([]byte{13}, 13), padded[3:])

	plain, err := pkcs7Unpad(padded)
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), plain)
}

func TestCipherBadPadding(t *testing.T) {
	key := []byte("key")
	// block is the raw plaintext block, decrypted with pad:pkcs.
	cases := []struct {
		name  string
		block []byte
	}{
		{name: "zero pad byte", block: append(bytes.Repeat([]byte{'a'}, 15), 0)},
		{name: "pad byte above block size", block: append(bytes.Repeat([]byte{'a'}, 15), 17)},
		{name: "mismatched pad bytes", block: append(bytes.Repeat([]byte{'a'}, 13), 1, 2, 3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt the raw block without padding, then decrypt it with
			// PKCS#7 so the unpad step sees the crafted plaintext.
			ciphertext, err := encryptData(tc.block, key, nil, "aes/pad:none")
			require.NoError(t, err)
			_, err = decryptData(ciphertext, key, nil, "aes")
			require.Error(t, err)
			require.Contains(t, err.Error(), "wrong key or corrupt data")
		})
	}

	// An empty ciphertext cannot hold PKCS#7 padding.
	_, err := decryptData(nil, key, nil, "aes")
	require.Error(t, err)

	// Ciphertext that is not block-aligned fails before decryption.
	_, err = decryptData(make([]byte, 17), key, nil, "aes")
	require.Error(t, err)
}

func TestCipherUnalignedNoPadding(t *testing.T) {
	_, err := encryptData([]byte("abc"), []byte("key"), nil, "aes/pad:none")
	require.Error(t, err)
	_, err = decryptData(make([]byte, 15), []byte("key"), nil, "aes/pad:none")
	require.Error(t, err)
}

func TestCipherBadKeyLength(t *testing.T) {
	_, err := encryptData([]byte("abc"), nil, nil, "aes")
	require.Error(t, err)
	_, err = encryptData([]byte("abc"), make([]byte, 33), nil, "aes")
	require.Error(t, err)
	_, err = decryptData(make([]byte, 16), make([]byte, 33), nil, "aes")
	require.Error(t, err)

	// A 17-byte key is padded to 24 bytes, so it succeeds.
	_, err = encryptData([]byte("abc"), make([]byte, 17), nil, "aes")
	require.NoError(t, err)
}

func TestCipherLongIVIsClipped(t *testing.T) {
	key := []byte("key")
	iv := []byte("0123456789abcdef")
	longIV := append(append([]byte(nil), iv...), 1)
	want, err := encryptData([]byte("abc"), key, iv, "aes")
	require.NoError(t, err)
	got, err := encryptData([]byte("abc"), key, longIV, "aes")
	require.NoError(t, err)
	require.Equal(t, want, got)
	plain, err := decryptData(got, key, longIV, "aes")
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), plain)
}

func TestParseCipherType(t *testing.T) {
	valid := map[string]cipherSpec{
		"aes":              {pkcs: true},
		"aes-cbc":          {pkcs: true},
		"aes-cbc/pad:pkcs": {pkcs: true},
		"aes/pad:pkcs":     {pkcs: true},
		"aes/pad:none":     {pkcs: false},
		"aes-cbc/pad:none": {pkcs: false},
	}
	for typ, want := range valid {
		got, err := parseCipherType(typ)
		require.NoError(t, err, typ)
		require.Equal(t, want, got, typ)
	}

	invalid := []string{
		"",
		"des",
		"AES",
		"aes-ecb",
		"aes-",
		"aes-cbc-x",
		"aes/pad:x",
		"aes/pad:",
		"aes/pkcs",
		"aes/pad:none/extra",
	}
	for _, typ := range invalid {
		_, err := parseCipherType(typ)
		require.Error(t, err, typ)
	}
}

// TestCipherHooksGate checks the flag-off error text and the flag-on path for
// the four hooks.
func TestCipherHooksGate(t *testing.T) {
	ctx := context.Background()
	st := cluster.MakeTestingClusterSettings()
	evalCtx := eval.MakeTestingEvalContext(st)

	// The hooks are disabled by default.
	_, err := Encrypt(ctx, &evalCtx, []byte("abc"), []byte("key"), "aes")
	require.EqualError(t, err, "encrypt can only be used with a CCL distribution")

	featurePgcryptoCiphersEnabled.Override(ctx, &st.SV, true)
	ct, err := Encrypt(ctx, &evalCtx, []byte("abc"), []byte("key"), "aes")
	require.NoError(t, err)
	pt, err := Decrypt(ctx, &evalCtx, ct, []byte("key"), "aes")
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), pt)

	iv := []byte("0123456789abcdef")
	ct, err = EncryptIV(ctx, &evalCtx, []byte("abc"), []byte("key"), iv, "aes")
	require.NoError(t, err)
	pt, err = DecryptIV(ctx, &evalCtx, ct, []byte("key"), iv, "aes")
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), pt)

	// With the setting off, all four hooks return the CCL-required error.
	featurePgcryptoCiphersEnabled.Override(ctx, &st.SV, false)
	_, err = Encrypt(ctx, &evalCtx, []byte("abc"), []byte("key"), "aes")
	require.EqualError(t, err, "encrypt can only be used with a CCL distribution")
	_, err = EncryptIV(ctx, &evalCtx, []byte("abc"), []byte("key"), iv, "aes")
	require.EqualError(t, err, "encrypt_iv can only be used with a CCL distribution")
	_, err = Decrypt(ctx, &evalCtx, []byte("0123456789abcdef"), []byte("key"), "aes")
	require.EqualError(t, err, "decrypt can only be used with a CCL distribution")
	_, err = DecryptIV(ctx, &evalCtx, []byte("0123456789abcdef"), []byte("key"), iv, "aes")
	require.EqualError(t, err, "decrypt_iv can only be used with a CCL distribution")
}
