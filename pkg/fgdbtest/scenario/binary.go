// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"crypto/sha256"
	"io"
	"os"
)

func sameBinary(a, b string) (bool, error) {
	if a == "" || b == "" {
		return false, nil
	}
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if os.SameFile(ia, ib) {
		return true, nil
	}
	ha, err := fileSHA(a)
	if err != nil {
		return false, err
	}
	hb, err := fileSHA(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func fileSHA(path string) ([32]byte, error) {
	var sum [32]byte
	file, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
