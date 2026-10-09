package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupIDIsUniqueAndTimestampOrdered(t *testing.T) {
	first := newBackupID(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	second := newBackupID(time.Date(2026, 10, 9, 12, 0, 0, 1_000_000, time.UTC))
	if first == second || !newerBackup(second, first) {
		t.Fatalf("backup IDs are not unique and ordered: %s %s", first, second)
	}
	if !looksLikeTimestamp(first) {
		t.Fatalf("new backup ID is not accepted: %s", first)
	}
}

func TestLocalStoreCreateOnly(t *testing.T) {
	store := &localStore{root: t.TempDir()}
	first, err := store.Create(context.Background(), "name/run/data")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(context.Background(), "name/run/data")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = second.Write([]byte("second"))
	if err := second.Close(); err == nil {
		t.Fatal("duplicate final object was replaced")
	}
	body, err := os.ReadFile(filepath.Join(store.root, "name/run/data"))
	if err != nil || string(body) != "first" {
		t.Fatalf("stored body = %q, err = %v", body, err)
	}
}

func TestBackupLeaseRefusesConcurrentRuns(t *testing.T) {
	loc := Location{Kind: "file", Root: t.TempDir()}
	first, err := acquireBackupLease(context.Background(), loc, "daily", "first", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireBackupLease(context.Background(), loc, "daily", "second", 0, time.Minute); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("second lease error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireBackupLease(context.Background(), loc, "daily", "second", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}
