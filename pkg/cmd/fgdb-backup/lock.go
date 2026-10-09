// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type backupLeaseRecord struct {
	FormatVersion int       `json:"format_version"`
	Holder        string    `json:"holder"`
	Host          string    `json:"host"`
	PID           int       `json:"pid"`
	AcquiredAt    time.Time `json:"acquired_at"`
	RenewedAt     time.Time `json:"renewed_at"`
	LeaseSeconds  int64     `json:"lease_seconds"`
}

type backupLease struct {
	loc    Location
	rel    string
	record backupLeaseRecord
	stop   chan struct{}
	done   chan struct{}
}

func newBackupID(now time.Time) string {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		// crypto/rand failure is exceptionally unlikely. A timestamp plus the
		// process id still gives a useful collision-resistant fallback.
		copy(random[:], []byte(strconv.FormatInt(int64(os.Getpid()), 10)))
	}
	return now.UTC().Format("20060102T150405.000Z") + "-" + hex.EncodeToString(random[:])
}

func acquireBackupLease(ctx context.Context, loc Location, name, holder string, wait, lease time.Duration) (*backupLease, error) {
	record := backupLeaseRecord{
		FormatVersion: 1,
		Holder:        holder,
		Host:          hostname(),
		PID:           os.Getpid(),
		AcquiredAt:    time.Now().UTC(),
		RenewedAt:     time.Now().UTC(),
		LeaseSeconds:  int64(lease / time.Second),
	}
	l := &backupLease{loc: loc, rel: name + "/LOCK.json", record: record, stop: make(chan struct{}), done: make(chan struct{})}
	deadline := time.Now().Add(wait)
	for {
		err := l.create(ctx)
		if err == nil {
			go l.renewLoop()
			return l, nil
		}
		current, readErr := l.read(ctx)
		if readErr != nil && !isNotFound(readErr) {
			return nil, fmt.Errorf("acquire backup lock: %w", readErr)
		}
		if current.Holder == "" {
			return nil, err
		}
		if leaseExpired(current) {
			if removeErr := l.remove(ctx); removeErr == nil {
				continue
			}
		}
		if wait <= 0 || time.Now().After(deadline) {
			return nil, fmt.Errorf("backup %q is locked by %s on %s (pid %d, renewed %s); use --lock-wait or unlock after checking the holder", name, current.Holder, current.Host, current.PID, current.RenewedAt.Format(time.RFC3339))
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *backupLease) create(ctx context.Context) error {
	body, err := json.Marshal(l.record)
	if err != nil {
		return err
	}
	if l.loc.Kind == "file" {
		path := filepath.Join(l.loc.Root, filepath.FromSlash(l.rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(body)
		return err
	}
	s, ok := l.locStore(ctx)
	if !ok {
		return errors.New("backup lock requires a local or S3 destination")
	}
	w, err := s.Create(ctx, l.rel)
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		_ = abortWriter(w)
		return err
	}
	return w.Close()
}

func (l *backupLease) read(ctx context.Context) (backupLeaseRecord, error) {
	var out backupLeaseRecord
	if l.loc.Kind == "file" {
		body, err := os.ReadFile(filepath.Join(l.loc.Root, filepath.FromSlash(l.rel)))
		if err != nil {
			return out, err
		}
		return out, json.Unmarshal(body, &out)
	}
	s, ok := l.locStore(ctx)
	if !ok {
		return out, errors.New("backup lock requires a local or S3 destination")
	}
	r, err := s.Open(ctx, l.rel)
	if err != nil {
		return out, err
	}
	defer r.Close()
	return out, json.NewDecoder(r).Decode(&out)
}

func (l *backupLease) remove(ctx context.Context) error {
	if l.loc.Kind == "file" {
		err := os.Remove(filepath.Join(l.loc.Root, filepath.FromSlash(l.rel)))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s, ok := l.locStore(ctx)
	if !ok {
		return errors.New("backup lock requires a local or S3 destination")
	}
	return s.delete(ctx, l.rel)
}

func (l *backupLease) Release() error {
	close(l.stop)
	<-l.done
	current, err := l.read(context.Background())
	if err != nil && !isNotFound(err) {
		return err
	}
	if current.Holder != l.record.Holder {
		return nil
	}
	return l.remove(context.Background())
}

func (l *backupLease) renewLoop() {
	defer close(l.done)
	interval := time.Duration(l.record.LeaseSeconds) * time.Second / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-ticker.C:
			l.record.RenewedAt = now.UTC()
			_ = l.writeRenewal()
		}
	}
}

func (l *backupLease) writeRenewal() error {
	if l.loc.Kind != "file" {
		return nil
	}
	body, err := json.Marshal(l.record)
	if err != nil {
		return err
	}
	path := filepath.Join(l.loc.Root, filepath.FromSlash(l.rel))
	tmp, err := os.CreateTemp(filepath.Dir(path), ".LOCK.json.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// The lock itself is create-only. Renewal is allowed to replace it only
	// after checking that the holder is still ours.
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record backupLeaseRecord
	if json.Unmarshal(current, &record) != nil || record.Holder != l.record.Holder {
		return errors.New("backup lock ownership changed")
	}
	return os.Rename(tmpName, path)
}

func (l *backupLease) locStore(ctx context.Context) (*s3Store, bool) {
	if l.loc.Kind != "s3" {
		return nil, false
	}
	s, err := newS3Store(ctx, l.loc)
	return s, err == nil
}

func leaseExpired(record backupLeaseRecord) bool {
	return time.Now().After(record.RenewedAt.Add(time.Duration(record.LeaseSeconds)*time.Second + time.Minute))
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return host
}

func abortWriter(w interface{}) error {
	if a, ok := w.(interface{ Abort() error }); ok {
		return a.Abort()
	}
	if c, ok := w.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

func unlockBackup(ctx context.Context, loc Location, name string, force bool) error {
	l := &backupLease{loc: loc, rel: strings.Trim(name, "/") + "/LOCK.json"}
	record, err := l.read(ctx)
	if err != nil {
		if isNotFound(err) || os.IsNotExist(err) {
			return fmt.Errorf("backup %q is not locked", name)
		}
		return err
	}
	if !force && !leaseExpired(record) {
		return fmt.Errorf("backup %q is still leased by %s (use --force-unlock only after checking it)", name, record.Holder)
	}
	return l.remove(ctx)
}
