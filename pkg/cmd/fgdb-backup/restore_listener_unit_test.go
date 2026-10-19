// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReviewImportListenerSkipsCopyAndImplicitS3(t *testing.T) {
	copyPlan := reviewPlan(t, &localStore{}, RestoreOptions{Load: "copy"})
	if !hasCheck(copyPlan, "import file server not required") {
		t.Fatalf("copy checks: %v", copyPlan.Checks)
	}
	implicit := reviewPlan(t, &s3Store{importAuth: "implicit"}, RestoreOptions{Load: "import"})
	if !hasCheck(implicit, "import file server not required") {
		t.Fatalf("implicit checks: %v", implicit.Checks)
	}
}

func TestReviewImportListenerWildcardRefusesPlan(t *testing.T) {
	plan := reviewPlan(t, &localStore{}, importOpt("0.0.0.0:0"))
	if plan.OK {
		t.Fatal("wildcard plan was accepted")
	}
	reason := strings.Join(plan.Reasons, "; ")
	if !strings.Contains(reason, "--serve-advertise") {
		t.Fatalf("reason = %s", reason)
	}
	if restoreStatus(&planRefusalError{message: "restore preflight refused: " + reason}) != 4 {
		t.Fatal("plan refusal should exit 4")
	}
}

func TestReviewImportListenerReportsBoundAddress(t *testing.T) {
	plan := reviewPlan(t, &localStore{}, RestoreOptions{Load: "import", ImportListen: "127.0.0.1:0"})
	if !hasCheckPrefix(plan, "import listener 127.0.0.1:") {
		t.Fatalf("checks: %v", plan.Checks)
	}
}

func TestReviewImportListenerCloseError(t *testing.T) {
	stubFixedListener(t, net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	plan := RestorePlan{OK: true}
	err := reviewImportListener(&plan, &localStore{}, importOpt("192.0.2.10:9"))
	if err == nil || !strings.Contains(err.Error(), "close broke") {
		t.Fatalf("close error = %v", err)
	}
	if !plan.OK || len(plan.Reasons) != 0 {
		t.Fatalf("close failure changed the plan: %+v", plan)
	}
}

func TestListenerHoldClose(t *testing.T) {
	hold := listenerHold{}
	if err := hold.close(); err != nil {
		t.Fatal(err)
	}
	ln := fixedListener(net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	hold.ln = ln
	err := hold.close()
	if err == nil || !strings.Contains(err.Error(), "close broke") || !ln.closed || hold.ln != nil {
		t.Fatalf("hold close = %v closed=%v held=%v", err, ln.closed, hold.ln != nil)
	}
}

func TestCloseListenerErrors(t *testing.T) {
	if err := closeListener(nil, errors.New("keep")); err == nil || err.Error() != "keep" {
		t.Fatalf("nil listener error = %v", err)
	}
	plain := mustListen(t, "127.0.0.1:0")
	if err := closeListener(plain, nil); err != nil {
		t.Fatal(err)
	}
	again := fixedListener(net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	err := closeListener(again, nil)
	if err == nil || !strings.Contains(err.Error(), "close import listener") || !strings.Contains(err.Error(), "close broke") {
		t.Fatalf("closeListener = %v", err)
	}
	err = joinCloseErr(errors.New("first"), "close import listener", errors.New("second"))
	if err == nil || !strings.Contains(err.Error(), "first") || !strings.Contains(err.Error(), "second") {
		t.Fatalf("joined = %v", err)
	}
	if err := joinCloseErr(errors.New("only"), "close import listener", nil); err == nil || err.Error() != "only" {
		t.Fatalf("nil close = %v", err)
	}
}

func TestCopyClosesHeldListener(t *testing.T) {
	ctx := context.Background()
	broken := fixedListener(net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	_, err := loadSelected(ctx, nil, restoreBundle{}, nil, RestoreOptions{Load: "copy"}, broken)
	if err == nil || !strings.Contains(err.Error(), "close broke") || !broken.closed {
		t.Fatalf("copy close = %v closed=%v", err, broken.closed)
	}
	plain, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadSelected(ctx, nil, restoreBundle{}, nil, RestoreOptions{Load: "copy"}, plain); err != nil {
		t.Fatal(err)
	}
}

func TestImplicitS3ClosesHeldListener(t *testing.T) {
	broken := fixedListener(net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	base, closer, errs, err := serveLocalBackup(importRequest{
		store:    &s3Store{importAuth: "implicit"},
		listener: broken,
	})
	if base != "" || closer != nil || errs != nil {
		t.Fatalf("implicit serve base=%q closer=%v errs=%v", base, closer != nil, errs)
	}
	if err == nil || !strings.Contains(err.Error(), "close broke") || !broken.closed {
		t.Fatalf("implicit close = %v closed=%v", err, broken.closed)
	}
	base, closer, errs, err = serveLocalBackup(importRequest{store: &s3Store{importAuth: "implicit"}})
	if err != nil || base != "" || closer != nil || errs != nil {
		t.Fatalf("implicit without listener: base=%q err=%v", base, err)
	}
}

func TestServeLocalBackupReusesListener(t *testing.T) {
	dir := t.TempDir()
	body := []byte("same-socket")
	path := filepath.Join(dir, "bk", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	ln := mustListen(t, "127.0.0.1:0")
	base, closer, _, err := serveLocalBackup(importRequest{
		store:    &localStore{root: dir},
		root:     Location{Kind: "file", Root: dir},
		base:     "bk",
		listener: ln,
		opt:      RestoreOptions{ImportListen: "127.0.0.1:0", Load: "import"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if closer == nil {
		t.Fatal("missing closer")
	}
	defer func() {
		if err := closer(); err != nil {
			t.Error(err)
		}
	}()
	if !strings.HasPrefix(base, "http://"+ln.Addr().String()+"/") {
		t.Fatalf("served %s, listener %s", base, ln.Addr())
	}
	resp, err := http.Get(base + "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(got) != string(body) {
		t.Fatalf("status %d body %q", resp.StatusCode, got)
	}
}

func TestImportServerCloseErrorIsReturned(t *testing.T) {
	ln := newBlockingListener(&net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 9}, errors.New("close broke"))
	_, err := loadImport(context.Background(), importRequest{
		store:    &localStore{root: t.TempDir()},
		root:     Location{Kind: "file", Root: t.TempDir()},
		listener: ln,
		opt:      RestoreOptions{ImportListen: "127.0.0.1:0", Load: "import"},
	})
	if err == nil || !strings.Contains(err.Error(), "close broke") {
		t.Fatalf("server close = %v", err)
	}
}

func TestImportURLCloseJoinsServerShutdown(t *testing.T) {
	ln := newBlockingListener(stringAddr("not-a-hostport"), errors.New("close broke"))
	_, closer, _, err := serveLocalBackup(importRequest{
		store:    &localStore{root: t.TempDir()},
		root:     Location{Kind: "file", Root: t.TempDir()},
		listener: ln,
		opt:      RestoreOptions{ServeAdvertise: "backup.example", ImportListen: "127.0.0.1:0", Load: "import"},
	})
	if closer != nil {
		t.Fatal("closer returned with the URL error")
	}
	if err == nil || !strings.Contains(err.Error(), "backup.example") || !strings.Contains(err.Error(), "close broke") {
		t.Fatalf("url close = %v", err)
	}
}

func reviewPlan(t *testing.T, store Store, opt RestoreOptions) RestorePlan {
	t.Helper()
	plan := RestorePlan{OK: true}
	if err := reviewImportListener(&plan, store, opt); err != nil {
		t.Fatal(err)
	}
	return plan
}

func hasCheck(plan RestorePlan, want string) bool {
	for _, check := range plan.Checks {
		if check == want {
			return true
		}
	}
	return false
}

func hasCheckPrefix(plan RestorePlan, prefix string) bool {
	for _, check := range plan.Checks {
		if strings.HasPrefix(check, prefix) {
			return true
		}
	}
	return false
}

type stringAddr string

func (a stringAddr) Network() string { return "tcp" }
func (a stringAddr) String() string  { return string(a) }

func newBlockingListener(addr net.Addr, closeErr error) *blockingListener {
	return &blockingListener{
		addr:     addr,
		closeErr: closeErr,
		started:  make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// blockingListener reports its address only after Accept has started, so
// Shutdown observes the socket the server is actually using.
type blockingListener struct {
	addr      net.Addr
	closeErr  error
	startOnce sync.Once
	closeOnce sync.Once
	started   chan struct{}
	done      chan struct{}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	l.startOnce.Do(func() { close(l.started) })
	<-l.done
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	l.closeOnce.Do(func() { close(l.done) })
	return l.closeErr
}

func (l *blockingListener) Addr() net.Addr {
	<-l.started
	return l.addr
}
