// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package faults records which fault injections this host is allowed to run.
// Kill and pause are local signals. Partition and disk faults are not.
package faults

import "github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"

// Outcome is one fault the report must mention.
type Outcome struct {
	Name   string
	Detail string
}

// Partition explains why a network partition was not injected.
func Partition() Outcome {
	return Outcome{
		Name: labels.PartitionNotTested,
		Detail: "Network partitions run only through preconfigured root units " +
			"on separate machines. Those units are not installed here. " +
			labels.PartitionNotTested,
	}
}

// Disk explains why disk faults were not injected.
func Disk() Outcome {
	return Outcome{
		Name: labels.DiskFaultsNotTested,
		Detail: "Disk-slow, disk-stall, and disk-full faults run only through " +
			"preconfigured root units on separate machines. Those units are not " +
			"installed here. " + labels.DiskFaultsNotTested,
	}
}
