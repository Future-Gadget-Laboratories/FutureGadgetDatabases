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
	"testing"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestOSSCapabilities is the pinned-binary entry point used by patch CI.
// The default unit-only PR tier skips it; required runtime jobs fail closed.
func TestOSSCapabilities(t *testing.T) {
	bin := os.Getenv("FGDB_COCKROACH")
	if bin == "" {
		if os.Getenv("FGDB_REQUIRE_CLUSTER") == "1" {
			t.Fatal("FGDB_COCKROACH is required and was not set")
		}
		t.Skip("OSS runtime capabilities not tested: FGDB_COCKROACH is unset")
	}
	out := os.Getenv("FGDB_OUTPUT")
	if out == "" {
		out = t.TempDir()
	}
	rep, runErr := Run(context.Background(), Config{
		Binary: bin, WorkDir: filepath.Join(t.TempDir(), "work"),
		ExpectGo: os.Getenv("FGDB_EXPECT_GO"),
		Meta: report.Meta{
			RunID: os.Getenv("FGDB_RUN_ID"), SourceSHA: os.Getenv("FGDB_SOURCE_SHA"),
			CandidateSHA256: os.Getenv("FGDB_CANDIDATE_SHA256"), Tier: "oss-capabilities",
		},
	})
	if rep == nil {
		t.Fatal("OSS capabilities did not return a report")
	}
	if err := rep.Write(out); err != nil {
		t.Errorf("write capability report: %v", err)
	}
	t.Log(rep.Markdown())
	if runErr != nil || rep.Rejected() {
		t.Fatalf("OSS capabilities failed: %v", runErr)
	}
	// A green run must have executed every gate; not_tested is never a pass.
	for _, id := range gateClaimIDs() {
		if got := rep.Outcome(id); got != labels.Pass {
			t.Errorf("claim %s = %s after a successful run, want pass", id, got)
		}
	}
}

func TestMissingBinaryDoesNotPass(t *testing.T) {
	rep, err := Run(context.Background(), Config{WorkDir: t.TempDir()})
	if err == nil || rep == nil || !rep.Rejected() {
		t.Fatalf("missing binary: report=%v error=%v", rep, err)
	}
	ids := []string{ClaimVersion, ClaimImport, ClaimBackup, ClaimTTL, ClaimGEOS, ClaimRC, ClaimCopy, ClaimImportMatrix}
	ids = append(ids, gateClaimIDs()...)
	for _, id := range ids {
		if got := rep.Outcome(id); got != labels.NotTested {
			t.Errorf("claim %s = %s, want not_tested", id, got)
		}
	}
}

func TestRequireCCLError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		ok   bool
	}{
		{name: "success"},
		{name: "transport", err: errors.New("connection refused")},
		{name: "parser", err: &pgconn.PgError{Code: "42601", Message: "syntax error"}},
		{name: "unrelated", err: &pgconn.PgError{Code: cclState, Message: "another error"}},
		{name: "wrong-state", err: &pgconn.PgError{Code: "42501", Message: cclMessage}},
		{name: "oss-gate", err: &pgconn.PgError{Code: cclState, Message: cclMessage + " *tree.Backup"}, ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := requireCCLError(tc.err)
			if (err == nil) != tc.ok {
				t.Fatalf("requireCCLError = %v, expected success %v", err, tc.ok)
			}
		})
	}
}

func TestCSVFixture(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		status int
		body   string
	}{
		{http.MethodGet, fixturePath, http.StatusOK, fixtureCSV},
		{http.MethodHead, fixturePath, http.StatusOK, ""},
		{http.MethodGet, "/missing", http.StatusNotFound, "404 page not found\n"},
		{http.MethodPost, fixturePath, http.StatusMethodNotAllowed, ""},
	} {
		w := httptest.NewRecorder()
		serveCSV(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || w.Body.String() != tc.body {
			t.Errorf("%s %s: status=%d body=%q", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestCSVFixtureRange(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, fixturePath, nil)
	request.Header.Set("Range", "bytes=0-2")
	w := httptest.NewRecorder()
	serveCSV(w, request)
	if w.Code != http.StatusPartialContent || w.Body.String() != fixtureCSV[:3] {
		t.Fatalf("range response status=%d body=%q", w.Code, w.Body.String())
	}
}
