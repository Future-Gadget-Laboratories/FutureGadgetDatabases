// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package pgcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
)

// cipherSpec is the parsed form of a cipher type string of the form
// <algorithm>[-<mode>][/pad:<padding>]. Only aes-cbc is implemented.
type cipherSpec struct {
	// pkcs is true when PKCS#7 padding is applied; false means pad:none.
	pkcs bool
}

// parseCipherType parses a cipher type string. "aes" alone means aes-cbc with
// PKCS#7 padding.
func parseCipherType(cipherType string) (cipherSpec, error) {
	base, padding, hasPadding := strings.Cut(cipherType, "/")
	algorithm, mode, hasMode := strings.Cut(base, "-")
	if algorithm != "aes" || (hasMode && mode != "cbc") {
		return cipherSpec{}, unsupportedCipherTypeError(cipherType)
	}
	spec := cipherSpec{pkcs: true}
	if hasPadding {
		switch padding {
		case "pad:pkcs":
			spec.pkcs = true
		case "pad:none":
			spec.pkcs = false
		default:
			return cipherSpec{}, unsupportedCipherTypeError(cipherType)
		}
	}
	return spec, nil
}

// normalizeAESKey zero-pads a key of 1 to 32 bytes up to the next AES key
// size (16, 24 or 32 bytes). Keys that are empty or longer than 32 bytes are
// rejected.
func normalizeAESKey(key []byte) ([]byte, error) {
	var size int
	switch {
	case len(key) == 0 || len(key) > 32:
		return nil, pgerror.Newf(
			pgcode.InvalidParameterValue,
			"invalid AES key length %d: key must be 1 to 32 bytes",
			len(key),
		)
	case len(key) <= 16:
		size = 16
	case len(key) <= 24:
		size = 24
	default:
		size = 32
	}
	padded := make([]byte, size)
	copy(padded, key)
	return padded, nil
}

// normalizeIV clips or zero-pads an IV to one AES block. A nil or empty IV
// becomes the all-zero IV used by encrypt and decrypt.
func normalizeIV(iv []byte) ([]byte, error) {
	padded := make([]byte, aes.BlockSize)
	copy(padded, iv)
	return padded, nil
}

// newAESCBC returns the block mode for the given key and IV. The key and IV
// are normalized first, so the only remaining failure is an internal bug.
func newAESCBC(key []byte, iv []byte, decrypt bool) (cipher.BlockMode, error) {
	normKey, err := normalizeAESKey(key)
	if err != nil {
		return nil, err
	}
	normIV, err := normalizeIV(iv)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(normKey)
	if err != nil {
		return nil, err
	}
	if decrypt {
		return cipher.NewCBCDecrypter(block, normIV), nil
	}
	return cipher.NewCBCEncrypter(block, normIV), nil
}

// pkcs7Pad appends PKCS#7 padding. A full block of padding is added when the
// input is already block-aligned.
func pkcs7Pad(data []byte) []byte {
	n := aes.BlockSize - len(data)%aes.BlockSize
	padded := make([]byte, len(data)+n)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(n)
	}
	return padded
}

// pkcs7Unpad removes PKCS#7 padding. The padding check does not short-circuit
// on the first mismatched byte.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errCorruptOrWrongKey()
	}
	n := int(data[len(data)-1])
	if n == 0 || n > aes.BlockSize || n > len(data) {
		return nil, errCorruptOrWrongKey()
	}
	var diff byte
	for _, b := range data[len(data)-n:] {
		diff |= b ^ byte(n)
	}
	if diff != 0 {
		return nil, errCorruptOrWrongKey()
	}
	return data[:len(data)-n], nil
}

// encryptData encrypts data with AES-CBC. A nil iv means the all-zero IV.
func encryptData(data, key, iv []byte, cipherType string) ([]byte, error) {
	spec, err := parseCipherType(cipherType)
	if err != nil {
		return nil, err
	}
	plaintext := data
	if spec.pkcs {
		plaintext = pkcs7Pad(data)
	} else if len(data)%aes.BlockSize != 0 {
		return nil, unalignedDataError(len(data))
	}
	mode, err := newAESCBC(key, iv, false /* decrypt */)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(plaintext))
	mode.CryptBlocks(out, plaintext)
	return out, nil
}

// decryptData decrypts data with AES-CBC. A nil iv means the all-zero IV.
// Note that CBC without a MAC is malleable; this matches the PostgreSQL raw
// cipher functions and does not authenticate the ciphertext.
func decryptData(data, key, iv []byte, cipherType string) ([]byte, error) {
	spec, err := parseCipherType(cipherType)
	if err != nil {
		return nil, err
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, unalignedDataError(len(data))
	}
	mode, err := newAESCBC(key, iv, true /* decrypt */)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	mode.CryptBlocks(out, data)
	if spec.pkcs {
		return pkcs7Unpad(out)
	}
	return out, nil
}

// cipherCCLRequiredError is the error returned by the cipher builtins when
// feature.pgcrypto_ciphers.enabled is false.
func cipherCCLRequiredError(builtin string) error {
	return pgerror.Newf(
		pgcode.CCLRequired,
		"%s can only be used with a CCL distribution",
		builtin,
	)
}

// unsupportedCipherTypeError is returned for any cipher type outside the
// supported grammar.
func unsupportedCipherTypeError(cipherType string) error {
	return pgerror.Newf(
		pgcode.InvalidParameterValue,
		"cipher type %q is not supported",
		cipherType,
	)
}

// unalignedDataError is returned when data is not a multiple of the AES block
// size and no padding is applied.
func unalignedDataError(n int) error {
	return pgerror.Newf(
		pgcode.InvalidParameterValue,
		"data length %d is not a multiple of the AES block size 16",
		n,
	)
}

// errCorruptOrWrongKey is returned when PKCS#7 padding is invalid after
// decryption, which usually means the key or IV is wrong or the data is
// corrupt.
func errCorruptOrWrongKey() error {
	return pgerror.New(
		pgcode.InvalidParameterValue,
		"wrong key or corrupt data",
	)
}
