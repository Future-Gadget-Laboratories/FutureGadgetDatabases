// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package claims

import "testing"

func TestPromiseWarning(t *testing.T) {
	diff := "--- a/docs/fgdb/architecture.md\n+++ b/docs/fgdb/architecture.md\n+The default is 3 copies.\n+A note.\n"
	got := AddedPromiseLines(diff)
	if len(got) != 1 {
		t.Fatalf("lines %v", got)
	}
}
