// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import "testing"

func TestDispatchRejectsUnknown(t *testing.T) {
	if dispatch("nope", nil) != 2 {
		t.Fatal("expected usage status")
	}
	if dispatch("help", nil) != 0 {
		t.Fatal("expected help to succeed")
	}
}
