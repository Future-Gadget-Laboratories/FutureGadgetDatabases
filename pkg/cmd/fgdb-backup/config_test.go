// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupConfigValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.yaml")
	body := []byte(`backup:
  lock: false
  lock_wait: 2s
  lock_lease: 1m
  threads: 3
  memory_bytes: 4096
  s3_credential_mode: auto
restore:
  swap_restore: false
  testing_mode: true
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := readBackupConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if configBool(cfg.Backup.Lock, true) {
		t.Fatal("lock config was ignored")
	}
	wait, err := configuredDuration(cfg.Backup.LockWait, "wait", 0)
	if err != nil || wait != 2*time.Second {
		t.Fatalf("wait = %s, err = %v", wait, err)
	}
	if configBool(cfg.Restore.SwapRestore, true) {
		t.Fatal("swap config was ignored")
	}
	if !configBool(cfg.Restore.TestingMode, false) {
		t.Fatal("testing mode config was ignored")
	}
}

func TestConfigDefaultsAndInvalidValues(t *testing.T) {
	cfg, err := readBackupConfig("")
	if err != nil || !configBool(cfg.Backup.Lock, true) {
		t.Fatalf("defaults: %#v %v", cfg, err)
	}
	cfg.Backup.LockWait = "not-a-duration"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("invalid duration was accepted")
	}
}

func TestConfigRejectsUnknownKeysAndSingleDash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("backup:\n  lokc: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBackupConfig(path); err == nil {
		t.Fatal("unknown YAML key was accepted")
	}
	if got, err := configPath([]string{"--config", path}); err != nil || got != path {
		t.Fatalf("double-dash config = %q, %v", got, err)
	}
	if _, err := configPath([]string{"-config", path}); err == nil {
		t.Fatal("single-dash config was accepted")
	}
}

func TestConfigLeavesResourceCapsUnsetByDefault(t *testing.T) {
	cfg, err := readBackupConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if configInt(cfg.Backup.Threads, 0) != 0 || configInt64(cfg.Backup.MemoryBytes, 0) != 0 {
		t.Fatalf("backup caps have defaults: %#v", cfg.Backup)
	}
	if configInt(cfg.Restore.Threads, 0) != 0 || configInt64(cfg.Restore.MemoryBytes, 0) != 0 {
		t.Fatalf("restore caps have defaults: %#v", cfg.Restore)
	}
}
