// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package faults

import (
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

func TestPartitionAndDiskStayNamed(t *testing.T) {
	if !strings.Contains(Partition().Detail, labels.PartitionNotTested) {
		t.Fatal(Partition().Detail)
	}
	if !strings.Contains(Disk().Detail, labels.DiskFaultsNotTested) {
		t.Fatal(Disk().Detail)
	}
}
