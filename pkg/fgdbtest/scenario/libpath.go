// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"os"
	"path/filepath"
	"strings"
)

// useSiblingLib puts <binary>/../lib on LD_LIBRARY_PATH when that directory
// exists. Release tarballs keep libgeos next to the cockroach binary.
func useSiblingLib(binary string) {
	if binary == "" {
		return
	}
	lib := filepath.Join(filepath.Dir(binary), "lib")
	info, err := os.Stat(lib)
	if err != nil || !info.IsDir() {
		return
	}
	cur := os.Getenv("LD_LIBRARY_PATH")
	if cur == lib || strings.HasPrefix(cur, lib+string(os.PathListSeparator)) {
		return
	}
	if cur == "" {
		_ = os.Setenv("LD_LIBRARY_PATH", lib)
		return
	}
	_ = os.Setenv("LD_LIBRARY_PATH", lib+string(os.PathListSeparator)+cur)
}
