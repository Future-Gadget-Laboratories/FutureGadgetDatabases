// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gate

import (
	"errors"
	"testing"
)

func TestSettingName(t *testing.T) {
	got, err := SettingName("backup")
	if err != nil || got != "fgdb.backup.enabled" {
		t.Fatalf("SettingName(backup) = %q, %v", got, err)
	}
	for _, invalid := range []string{"", "Backup", "1backup", "backup.enabled", "backup-restore"} {
		if _, err := SettingName(invalid); !errors.Is(err, ErrInvalidFeatureName) {
			t.Errorf("SettingName(%q) error = %v, want ErrInvalidFeatureName", invalid, err)
		}
	}
}

func TestRunUsesSelectedPath(t *testing.T) {
	stockErr := errors.New("stock behavior")
	stockCalled, fgdbCalled := false, false
	stock := func() error { stockCalled = true; return stockErr }
	fgdb := func() error { fgdbCalled = true; return nil }
	if err := Run(false, stock, fgdb); !errors.Is(err, stockErr) {
		t.Fatalf("Run(false) error = %v, want stock error", err)
	}
	if !stockCalled || fgdbCalled {
		t.Fatalf("Run(false) called stock=%t fgdb=%t", stockCalled, fgdbCalled)
	}
	if err := Run(true, stock, fgdb); err != nil {
		t.Fatalf("Run(true) error = %v", err)
	}
	if !fgdbCalled {
		t.Fatal("Run(true) did not call FGDb path")
	}
}

func TestRunRejectsNilSelectedPath(t *testing.T) {
	if err := Run(false, nil, func() error { return nil }); err == nil {
		t.Fatal("Run(false) with nil stock path succeeded")
	}

	stockCalled := false
	stock := func() error {
		stockCalled = true
		return nil
	}
	if err := Run(true, stock, nil); err == nil {
		t.Fatal("Run(true) with nil FGDb path succeeded")
	}
	if stockCalled {
		t.Fatal("Run(true) with nil FGDb path called stock path")
	}
}
