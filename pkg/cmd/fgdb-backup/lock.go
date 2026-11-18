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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	store  Store
	etag   string
	cancel context.CancelFunc
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

func acquireBackupLease(ctx context.Context, loc Location, name, holder string, wait, lease time.Duration, stores ...Store) (*backupLease, error) {
	l := newBackupLease(loc, name, holder, lease, stores)
	deadline := time.Now().Add(wait)
	for {
		acquired, current, err := tryCreateLease(ctx, l)
		if acquired {
			go l.renewLoop()
			return l, nil
		}
		if err != nil {
			return nil, err
		}
		retry, err := retryLease(ctx, l, name, current, wait, deadline)
		if err != nil {
			return nil, err
		}
		if retry {
			continue
		}
	}
}

func newBackupLease(loc Location, name, holder string, lease time.Duration, stores []Store) *backupLease {
	if lease < 30*time.Second {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	record := backupLeaseRecord{
		FormatVersion: 1,
		Holder:        holder,
		Host:          hostname(),
		PID:           os.Getpid(),
		AcquiredAt:    now,
		RenewedAt:     now,
		LeaseSeconds:  int64(lease / time.Second),
	}
	l := &backupLease{
		loc: loc, rel: name + "/LOCK.json", record: record,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	if len(stores) > 0 {
		l.store = stores[0]
	}
	return l
}

func retryLease(ctx context.Context, lease *backupLease, name string, current backupLeaseRecord, wait time.Duration, deadline time.Time) (bool, error) {
	if current.Holder == "" {
		if reclaimLease(ctx, lease) {
			return true, nil
		}
		return retryLeaseAfterWait(ctx, deadline, wait, errors.New("backup lock is unreadable and did not expire"))
	}
	if leaseExpired(current) && reclaimLease(ctx, lease) {
		return true, nil
	}
	return retryLeaseAfterWait(ctx, deadline, wait, lockedError(name, current))
}

func retryLeaseAfterWait(ctx context.Context, deadline time.Time, wait time.Duration, terminal error) (bool, error) {
	if wait <= 0 || time.Now().After(deadline) {
		return false, terminal
	}
	if err := waitForLease(ctx, deadline); err != nil {
		return false, err
	}
	return true, nil
}

func tryCreateLease(ctx context.Context, lease *backupLease) (bool, backupLeaseRecord, error) {
	if err := lease.create(ctx); err == nil {
		return true, backupLeaseRecord{}, nil
	} else {
		current, readErr := lease.read(ctx)
		if readErr != nil {
			if isNotFound(readErr) || os.IsNotExist(readErr) {
				return false, backupLeaseRecord{}, nil
			}
			if lease.loc.Kind == "file" && corruptLeaseExpired(lease) {
				return false, backupLeaseRecord{}, nil
			}
			return false, backupLeaseRecord{}, fmt.Errorf("acquire backup lock: %w", readErr)
		}
		return false, current, nil
	}
}

func corruptLeaseExpired(lease *backupLease) bool {
	info, err := os.Stat(filepath.Join(lease.loc.Root, filepath.FromSlash(lease.rel)))
	if err != nil {
		return os.IsNotExist(err)
	}
	age := time.Duration(lease.record.LeaseSeconds) * time.Second
	return time.Now().After(info.ModTime().Add(age))
}

func reclaimLease(ctx context.Context, lease *backupLease) bool {
	if lease.loc.Kind == "file" {
		path := filepath.Join(lease.loc.Root, filepath.FromSlash(lease.rel))
		stale := path + ".reclaim." + lease.record.Holder
		if err := os.Rename(path, stale); err != nil {
			return false
		}
		defer os.Remove(stale)
		return lease.create(ctx) == nil
	}
	if _, err := lease.read(ctx); err != nil {
		return false
	}
	return lease.writeRenewal() == nil
}

func waitForLease(ctx context.Context, deadline time.Time) error {
	if time.Now().After(deadline) {
		return context.DeadlineExceeded
	}
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func lockedError(name string, record backupLeaseRecord) error {
	return fmt.Errorf("backup %q is locked by %s on %s (pid %d, renewed %s); use --lock-wait or unlock after checking the holder", name, record.Holder, record.Host, record.PID, record.RenewedAt.Format(time.RFC3339))
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
		if err == nil {
			err = f.Sync()
		}
		return err
	}
	s, err := l.locStore(ctx)
	if err != nil {
		return err
	}
	w, err := s.Create(ctx, l.rel)
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		_ = abortWriter(w)
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	_, err = l.read(ctx)
	return err
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
	s, err := l.locStore(ctx)
	if err != nil {
		return out, err
	}
	if s3s := s; s3s != nil {
		object, err := s3s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(s3s.bucket),
			Key:    aws.String(s3s.key(l.rel)),
		})
		if err != nil {
			return out, err
		}
		defer object.Body.Close()
		l.etag = aws.ToString(object.ETag)
		return out, json.NewDecoder(object.Body).Decode(&out)
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
	s, err := l.locStore(ctx)
	if err != nil {
		return err
	}
	return s.delete(ctx, l.rel)
}

func (l *backupLease) Release() error {
	close(l.stop)
	<-l.done
	current, err := l.read(context.Background())
	if err != nil && !isNotFound(err) && !os.IsNotExist(err) {
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
			if err := l.writeRenewal(); err != nil && l.cancel != nil {
				l.cancel()
			}
		}
	}
}

func (l *backupLease) writeRenewal() error {
	body, err := json.Marshal(l.record)
	if err != nil {
		return err
	}
	if l.loc.Kind == "s3" {
		s, err := l.locStore(context.Background())
		if err != nil {
			return err
		}
		in := &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s.key(l.rel)),
			Body:   strings.NewReader(string(body)),
		}
		out, err := s.client.PutObject(context.Background(), in, putHeader("If-Match", l.etag))
		if err != nil {
			return err
		}
		l.etag = aws.ToString(out.ETag)
		return nil
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

const lockNeedsObjectStore = "backup lock requires a local or S3 destination"

func (l *backupLease) locStore(ctx context.Context) (*s3Store, error) {
	if l.loc.Kind != "s3" {
		return nil, errors.New(lockNeedsObjectStore)
	}
	if s, ok := l.store.(*s3Store); ok {
		return s, nil
	}
	return newS3Store(ctx, l.loc)
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
	if err := safeSegment(name); err != nil {
		return err
	}
	l := &backupLease{loc: loc, rel: strings.Trim(name, "/") + "/LOCK.json"}
	if force {
		return l.remove(ctx)
	}
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
