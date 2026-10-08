// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"encoding/json"
	"io"
)

func jsonNewDecoder(r io.Reader, dest any) error {
	return json.NewDecoder(r).Decode(dest)
}
