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
	for _, when := range []string{"before query", "during query", "between queries"} {
		t.Run(when, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if when == "before query" {
				cancel()
			}
			calls := 0
			start := time.Now()
			err := pollTTL(ctx, func(context.Context) ([][]string, error) {
				calls++
				if calls > 1 {
					return [][]string{{"2", "live"}}, nil
				}
				if when == "between queries" {
					time.AfterFunc(10*time.Millisecond, cancel)
					return nil, nil
				}
				cancel()
				return nil, ctx.Err()
			})
			wantCalls := 1
			if when == "before query" {
				wantCalls = 0
			}
			if !errors.Is(err, context.Canceled) || calls != wantCalls {
				t.Fatalf("pollTTL: calls=%d err=%v, want %d calls and cancellation", calls, err, wantCalls)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("cancellation took %s", elapsed)
			}
		})
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

func TestCleanupReadCommitted(t *testing.T) {
	rollbackErr := errors.New("rollback failed")
	dropErr := errors.New("drop failed")
	restoreErr := errors.New("restore failed")
	for _, tc := range []struct {
		name     string
		txStatus byte
		failures map[string]error
	}{
		{name: "idle before begin or after commit", txStatus: 'I'},
		{name: "open transaction", txStatus: 'T'},
		{name: "aborted transaction", txStatus: 'E'},
		{name: "rollback error", txStatus: 'T', failures: map[string]error{"ROLLBACK": rollbackErr}},
		{name: "drop error while idle", txStatus: 'I', failures: map[string]error{"DROP TABLE IF EXISTS rc_guard": dropErr}},
		{name: "restore error while idle", txStatus: 'I', failures: map[string]error{"SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = 'false'": restoreErr}},
		{name: "all cleanup errors", txStatus: 'T', failures: map[string]error{
			"ROLLBACK":                      rollbackErr,
			"DROP TABLE IF EXISTS rc_guard": dropErr,
			"SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = 'false'": restoreErr,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			status := tc.txStatus
			exec := func(stmt string) error {
				calls = append(calls, stmt)
				if stmt == "ROLLBACK" {
					status = 'I'
				}
				return tc.failures[stmt]
			}
			err := cleanupReadCommitted(cleanupConnection{
				closed:   func() bool { return false },
				txStatus: func() byte { return status },
				exec:     exec,
			}, "false", func(context.Context) (cleanupConnection, error) {
				t.Fatal("unexpected fresh connection")
				return cleanupConnection{}, nil
			})
			want := []string{"DROP TABLE IF EXISTS rc_guard", "SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = 'false'"}
			if tc.txStatus != 'I' {
				want = append([]string{"ROLLBACK"}, want...)
			}
			if strings.Join(calls, ";") != strings.Join(want, ";") {
				t.Fatalf("cleanup statements=%v, want %v", calls, want)
			}
			if len(tc.failures) == 0 && err != nil {
				t.Fatalf("cleanup: %v", err)
			}
			for stmt, failure := range tc.failures {
				if !errors.Is(err, failure) {
					t.Errorf("cleanup error %v does not include %s error %v", err, stmt, failure)
				}
			}
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			closed := false
			primary := cleanupConnection{
				closed:   func() bool { return true },
				txStatus: func() byte { t.Fatal("status on closed primary"); return 'T' },
				exec:     func(string) error { t.Fatal("exec on closed primary"); return nil },
			}
			err := cleanupReadCommitted(primary, "false", func(ctx context.Context) (cleanupConnection, error) {
				if ctx.Err() != nil {
					t.Fatalf("fresh context: %v", ctx.Err())
				}
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > cleanupTimeout {
					t.Fatal("fresh connect context must have cleanup deadline")
				}
				if tc.connectErr != nil {
					return cleanupConnection{}, tc.connectErr
				}
				return cleanupConnection{
					closed:   func() bool { return false },
					txStatus: func() byte { return 'I' },
					exec: func(stmt string) error {
						calls = append(calls, stmt)
						if strings.HasPrefix(stmt, "SET CLUSTER SETTING") {
							return tc.restoreErr
						}
						return nil
					},
					close: func() error { closed = true; return tc.closeErr },
				}, nil
			})
			if tc.connectErr == nil {
				want := "DROP TABLE IF EXISTS rc_guard;SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = 'false'"
				if strings.Join(calls, ";") != want || !closed {
					t.Fatalf("calls=%v closed=%v", calls, closed)
				}
			}
			for _, failure := range []error{tc.connectErr, tc.restoreErr, tc.closeErr} {
				if failure != nil && !errors.Is(err, failure) {
					t.Errorf("cleanup error %v missing %v", err, failure)
				}
			}
			if tc.connectErr == nil && tc.restoreErr == nil && tc.closeErr == nil && err != nil {
				t.Fatal(err)
			}
		})
	}
}
