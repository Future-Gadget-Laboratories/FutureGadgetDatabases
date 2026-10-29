// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPollTTLQueryTimeoutRetry(t *testing.T) {
	calls := 0
	err := pollTTL(context.Background(), func(ctx context.Context) ([][]string, error) {
		calls++
		if calls == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return [][]string{{"2", "live"}}, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("pollTTL: calls=%d err=%v, want 2 calls and success", calls, err)
	}
}

func TestPollTTLParentCancellation(t *testing.T) {
	for _, tc := range []struct {
		when      string
		wantCalls int
	}{
		{"before query", 0},
		{"during query", 1},
		{"between queries", 1},
	} {
		t.Run(tc.when, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.when == "before query" {
				cancel()
			}
			calls := 0
			start := time.Now()
			err := pollTTL(ctx, ttlCancellationQuery(ctx, cancel, tc.when, &calls))
			if !errors.Is(err, context.Canceled) || calls != tc.wantCalls {
				t.Fatalf("pollTTL: calls=%d err=%v, want %d calls and cancellation", calls, err, tc.wantCalls)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("cancellation took %s", elapsed)
			}
		})
	}
}

func ttlCancellationQuery(ctx context.Context, cancel context.CancelFunc, when string, calls *int) func(context.Context) ([][]string, error) {
	return func(context.Context) ([][]string, error) {
		*calls++
		if *calls > 1 {
			return [][]string{{"2", "live"}}, nil
		}
		if when == "between queries" {
			time.AfterFunc(10*time.Millisecond, cancel)
			return nil, nil
		}
		cancel()
		return nil, ctx.Err()
	}
}

func TestEqualRows(t *testing.T) {
	for name, tc := range map[string]struct {
		got, want [][]string
		ok        bool
	}{
		"same":          {[][]string{{"1", "alpha"}}, [][]string{{"1", "alpha"}}, true},
		"wrong value":   {[][]string{{"1", "beta"}}, [][]string{{"1", "alpha"}}, false},
		"wrong count":   {[][]string{{"1"}}, [][]string{{"1"}, {"2"}}, false},
		"nil and empty": {nil, [][]string{}, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := equalRows(tc.got, tc.want); got != tc.ok {
				t.Fatalf("equalRows=%v, want %v", got, tc.ok)
			}
		})
	}
}

func TestBesideBinaryLib(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "cockroach")
	if _, err := os.Create(bin); err != nil {
		t.Fatal(err)
	}
	if _, err := besideBinaryLib(bin); err == nil {
		t.Fatal("missing lib passed")
	}
	lib := filepath.Join(root, "lib")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libgeos.so", "libgeos_c.so"} {
		if _, err := os.Create(filepath.Join(lib, name)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := besideBinaryLib(bin)
	if err != nil || got != lib {
		t.Fatalf("besideBinaryLib=%q, %v; want %q", got, err, lib)
	}
}

func TestImportFixtureHandler(t *testing.T) {
	w := httptest.NewRecorder()
	fixtureHandler([]importFixture{{format: "CSV", path: "/rows", body: "fixture"}})(w, httptest.NewRequest(http.MethodGet, "/rows", nil))
	if w.Code != http.StatusOK || w.Body.String() != "fixture" {
		t.Fatalf("fixture response status=%d body=%q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	fixtureHandler(nil)(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing fixture status=%d", w.Code)
	}
}

func TestMapsGEOSOnlyFrom(t *testing.T) {
	const dir = "/opt/fgdb/lib"
	line := func(p string) string { return "7f00-7f01 r-xp 00000000 08:01 42 " + p + "\n" }
	good := line("/usr/lib/libc.so.6") + line(dir+"/libgeos.so") + line(dir+"/libgeos_c.so")
	if err := mapsGEOSOnlyFrom(good, dir); err != nil {
		t.Fatalf("beside-binary mapping rejected: %v", err)
	}
	for name, maps := range map[string]string{
		"system only":  line("/usr/lib/x86_64-linux-gnu/libgeos_c.so.1"),
		"extra system": good + line("/usr/lib/x86_64-linux-gnu/libgeos_c.so.1"),
		"missing core": line(dir + "/libgeos_c.so"),
		"none":         line("/usr/lib/libc.so.6"),
	} {
		if err := mapsGEOSOnlyFrom(maps, dir); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestBesideBinaryLibRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "system-libgeos_c.so")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "libgeos.so"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(lib, "libgeos_c.so")); err != nil {
		t.Fatal(err)
	}
	if _, err := besideBinaryLib(filepath.Join(root, "cockroach")); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked library accepted: %v", err)
	}
}

func TestFixtureHandlerContentLength(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler([]importFixture{{"CSV", "/f.csv", "1,a\n"}}))
	defer srv.Close()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req, _ := http.NewRequest(method, srv.URL+"/f.csv", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.Header.Get("Content-Length") != "4" {
			t.Errorf("%s Content-Length = %q, want 4", method, resp.Header.Get("Content-Length"))
		}
	}
}

func TestImportJobDone(t *testing.T) {
	for _, tc := range []struct {
		status   string
		done, ok bool
	}{
		{"succeeded", true, true},
		{"failed", true, false},
		{"canceled", true, false},
		{"revert-failed", true, false},
		{"reverting", false, false},
		{"cancel-requested", false, false},
		{"running", false, false},
		{"pending", false, false},
	} {
		done, err := importJobDone(1, tc.status, "boom")
		if done != tc.done || (done && (err == nil) != tc.ok) {
			t.Errorf("%s: done=%v err=%v", tc.status, done, err)
		}
	}
}

func TestImportJobTerminal(t *testing.T) {
	for status, want := range map[string]bool{
		"succeeded": true, "failed": true, "canceled": true, "revert-failed": true,
		"running": false, "reverting": false, "cancel-requested": false, "pending": false,
	} {
		if got := importJobTerminal(status); got != want {
			t.Errorf("importJobTerminal(%q) = %v, want %v", status, got, want)
		}
	}
}

const (
	rcDrop    = "DROP TABLE IF EXISTS rc_guard"
	rcRestore = "SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = 'false'"
)

func TestCleanupReadCommitted(t *testing.T) {
	rollbackErr := errors.New("rollback failed")
	dropErr := errors.New("drop failed")
	restoreErr := errors.New("restore failed")
	for _, tc := range []struct {
		name         string
		txStatus     byte
		probeErr     error
		wantRollback bool
		wantFresh    bool
		failures     map[string]error
	}{
		{name: "idle before begin or after commit", txStatus: 'I'},
		{name: "open transaction", txStatus: 'T', wantRollback: true, wantFresh: true},
		{name: "aborted transaction", txStatus: 'E', wantRollback: true, wantFresh: true},
		{name: "unknown status", txStatus: '?', wantFresh: true},
		{name: "zero status", wantFresh: true},
		{name: "timeout then rollback", txStatus: 'T', probeErr: context.DeadlineExceeded, wantRollback: true, wantFresh: true},
		{name: "timeout while idle", txStatus: 'I', probeErr: context.DeadlineExceeded, wantFresh: true},
		{name: "rollback error", txStatus: 'T', wantRollback: true, wantFresh: true, failures: map[string]error{"ROLLBACK": rollbackErr}},
		{name: "drop error while idle", txStatus: 'I', failures: map[string]error{rcDrop: dropErr}},
		{name: "restore error while idle", txStatus: 'I', failures: map[string]error{rcRestore: restoreErr}},
		{name: "all cleanup errors", txStatus: 'T', wantRollback: true, wantFresh: true, failures: map[string]error{
			"ROLLBACK": rollbackErr, rcDrop: dropErr, rcRestore: restoreErr,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primary := cleanupTestConnection{status: tc.txStatus, failures: tc.failures}
			fresh := cleanupTestConnection{status: 'I', failures: tc.failures}
			err := cleanupReadCommitted(primary.connection(t), "false", fresh.connector(t, nil), tc.probeErr)
			wantPrimary := []string{rcDrop, rcRestore}
			var wantFresh []string
			wantConnections := 0
			if tc.wantFresh {
				wantPrimary = nil
				wantFresh = []string{rcDrop, rcRestore}
				wantConnections = 1
			}
			if tc.failures[rcDrop] != nil {
				if !tc.wantFresh {
					wantPrimary = []string{rcDrop}
					wantFresh = []string{rcRestore}
				} else {
					wantConnections++
				}
				if !tc.wantFresh {
					wantConnections++
				}
			}
			if tc.wantRollback {
				wantPrimary = append([]string{"ROLLBACK"}, wantPrimary...)
			}
			requireCleanupCalls(t, "primary", primary.calls, wantPrimary...)
			requireCleanupCalls(t, "fresh", fresh.calls, wantFresh...)
			requireFreshLifecycle(t, &fresh, wantConnections, wantConnections > 0)
			requireCleanupErrors(t, err, tc.failures)
		})
	}
}

func TestCleanupReadCommittedDropErrorForcesFreshConnection(t *testing.T) {
	dropErr := errors.New("drop timed out")
	failed := false
	primary := cleanupConnection{
		closed: func() bool { return false },
		txStatus: func() byte {
			if failed {
				t.Fatal("probed connection after cleanup error")
			}
			return 'I'
		},
		exec: func(stmt string) error {
			if stmt != rcDrop {
				t.Fatalf("unexpected primary statement: %s", stmt)
			}
			failed = true
			return dropErr
		},
	}
	fresh := cleanupTestConnection{status: 'I'}
	err := cleanupReadCommitted(primary, "false", fresh.connector(t, nil), nil)
	if !errors.Is(err, dropErr) {
		t.Fatalf("cleanup error=%v, want %v", err, dropErr)
	}
	requireCleanupCalls(t, "fresh", fresh.calls, rcRestore)
	requireFreshLifecycle(t, &fresh, 1, true)
}

func TestCleanupReadCommittedFailedBegin(t *testing.T) {
	beginErr := errors.New("begin failed")
	const begin = "BEGIN TRANSACTION ISOLATION LEVEL READ COMMITTED"
	for _, tc := range []struct {
		name   string
		status byte
	}{
		{name: "idle", status: 'I'},
		{name: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primary := cleanupTestConnection{status: tc.status, failures: map[string]error{begin: beginErr}}
			fresh := cleanupTestConnection{status: 'I'}
			conn := primary.connection(t)
			probeErr := conn.exec(begin)
			if !errors.Is(probeErr, beginErr) {
				t.Fatalf("BEGIN error=%v, want %v", probeErr, beginErr)
			}
			err := cleanupReadCommitted(conn, "false", fresh.connector(t, nil), probeErr)
			requireCleanupCalls(t, "primary", primary.calls, begin)
			requireCleanupCalls(t, "fresh", fresh.calls, rcDrop, rcRestore)
			requireFreshLifecycle(t, &fresh, 1, true)
			requireCleanupErrors(t, err, nil)
		})
	}
}

func TestCleanupReadCommittedFreshConnection(t *testing.T) {
	restoreErr := errors.New("fresh restore failed")
	connectErr := errors.New("fresh connect failed")
	closeErr := errors.New("fresh close failed")
	for _, tc := range []struct {
		name       string
		connectErr error
		restoreErr error
		closeErr   error
	}{
		{name: "closed primary"},
		{name: "restore error", restoreErr: restoreErr},
		{name: "connect error", connectErr: connectErr},
		{name: "close error", closeErr: closeErr},
		{name: "restore and close errors", restoreErr: restoreErr, closeErr: closeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primary := cleanupTestConnection{closed: true}
			fresh := cleanupTestConnection{status: 'I', failures: map[string]error{rcRestore: tc.restoreErr}, closeErr: tc.closeErr}
			err := cleanupReadCommitted(primary.connection(t), "false", fresh.connector(t, tc.connectErr), nil)
			wantConnections := 1
			wantCalls := []string{rcDrop, rcRestore}
			if tc.connectErr != nil {
				wantConnections = 2
				wantCalls = nil
			}
			requireCleanupCalls(t, "primary", primary.calls)
			requireCleanupCalls(t, "fresh", fresh.calls, wantCalls...)
			requireFreshLifecycle(t, &fresh, wantConnections, tc.connectErr == nil)
			requireCleanupErrors(t, err, map[string]error{"connect": tc.connectErr, "restore": tc.restoreErr, "close": tc.closeErr})
		})
	}
}

type cleanupTestConnection struct {
	status      byte
	closed      bool
	calls       []string
	failures    map[string]error
	closeErr    error
	connections int
}

func (c *cleanupTestConnection) connection(t *testing.T) cleanupConnection {
	t.Helper()
	return cleanupConnection{
		closed: func() bool { return c.closed },
		txStatus: func() byte {
			if c.closed {
				t.Fatal("status on closed connection")
			}
			return c.status
		},
		exec: func(stmt string) error {
			if c.closed {
				t.Fatal("exec on closed connection")
			}
			c.calls = append(c.calls, stmt)
			err := c.failures[stmt]
			if stmt == "ROLLBACK" && err == nil {
				c.status = 'I'
			}
			return err
		},
		close: func() error { c.closed = true; return c.closeErr },
	}
}

func (c *cleanupTestConnection) connector(t *testing.T, connectErr error) func(context.Context) (cleanupConnection, error) {
	t.Helper()
	return func(ctx context.Context) (cleanupConnection, error) {
		c.connections++
		if ctx.Err() != nil {
			t.Fatalf("fresh context: %v", ctx.Err())
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > cleanupTimeout {
			t.Fatal("fresh connect context must have cleanup deadline")
		}
		if connectErr != nil {
			return cleanupConnection{}, connectErr
		}
		return c.connection(t), nil
	}
}

func requireCleanupCalls(t *testing.T, name string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ";") != strings.Join(want, ";") {
		t.Errorf("%s statements=%v, want %v", name, got, want)
	}
}

func requireFreshLifecycle(t *testing.T, c *cleanupTestConnection, connections int, closed bool) {
	t.Helper()
	if c.connections != connections || c.closed != closed {
		t.Errorf("fresh connections=%d closed=%v, want %d, %v", c.connections, c.closed, connections, closed)
	}
}

func requireCleanupErrors(t *testing.T, got error, failures map[string]error) {
	t.Helper()
	wantError := false
	for stmt, failure := range failures {
		if failure == nil {
			continue
		}
		wantError = true
		if !errors.Is(got, failure) {
			t.Errorf("cleanup error %v does not include %s error %v", got, stmt, failure)
		}
	}
	if !wantError && got != nil {
		t.Fatalf("cleanup: %v", got)
	}
}
