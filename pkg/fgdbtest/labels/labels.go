// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package labels holds the short strings the suite prints in more than one place.
package labels

const (
	// Pass, Fail, and NotTested are claim outcomes in a run report.
	Pass      = "pass"
	Fail      = "fail"
	NotTested = "not_tested"

	// PartitionNotTested and DiskFaultsNotTested are exact report lines.
	// Partition and disk faults need preconfigured root units. This slice
	// never runs them, and it never hides that fact.
	PartitionNotTested  = "partition NOT tested"
	DiskFaultsNotTested = "disk faults NOT tested"

	// StatusTested, StatusPartial, StatusUntested, and StatusWaived are
	// registry statuses. Untested fails the checker.
	StatusTested   = "tested"
	StatusPartial  = "partial"
	StatusUntested = "untested"
	StatusWaived   = "waived"

	// CipherBankField is rejected wherever it appears in the registry.
	CipherBankField = "cipherbank_need"

	ResultOK      = "ok"
	ResultFail    = "fail"
	ResultUnknown = "unknown"

	KindRead  = "read"
	KindWrite = "write"
)
