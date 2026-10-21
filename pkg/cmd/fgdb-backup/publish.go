// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func publishLatest(ctx context.Context, store Store, rel string, ptr LatestPointer) error {
	switch s := store.(type) {
	case *s3Store:
		return s.putLatest(ctx, rel, ptr)
	case *localStore:
		return s.putLatest(ctx, rel, ptr)
	default:
		_, err := writeJSONFile(ctx, store, rel, ptr)
		return err
	}
}

func (s *localStore) putLatest(ctx context.Context, rel string, ptr LatestPointer) error {
	lockPath := s.path(rel) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()
	if cur, ok := readLatestPointer(ctx, s, rel); ok && !newerBackup(ptr.Timestamp, cur.Timestamp) {
		logf("leaving latest at %s; %s is not newer", cur.Timestamp, ptr.Timestamp)
		return nil
	}
	body, err := json.MarshalIndent(ptr, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path(rel)), ".latest.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path(rel)); err != nil {
		return fmt.Errorf("replace latest pointer: %w", err)
	}
	return nil
}

func readLatestPointer(ctx context.Context, store Store, rel string) (LatestPointer, bool) {
	var ptr LatestPointer
	rc, err := store.Open(ctx, rel)
	if err != nil {
		return ptr, false
	}
	defer rc.Close()
	if err := json.NewDecoder(rc).Decode(&ptr); err != nil {
		return ptr, false
	}
	if !ptr.Complete || ptr.Timestamp == "" {
		return ptr, false
	}
	return ptr, true
}
