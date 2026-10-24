// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	fixtureCSV  = "1,alpha\n2,beta\n"
	fixturePath = "/rows.csv"
	backupSQL   = "BACKUP INTO 'nodelocal://1/fgdb-capability-backup'"
	cclState    = "XXC01"
	cclMessage  = "a CCL binary is required to use this statement type:"
)

func importCSV(ctx context.Context, conn *pgx.Conn) error {
	srv := httptest.NewServer(http.HandlerFunc(serveCSV))
	defer srv.Close()
	if _, err := conn.Exec(ctx, "CREATE TABLE defaultdb.public.fgdb_capability_import (id INT PRIMARY KEY, value STRING NOT NULL)"); err != nil {
		return fmt.Errorf("create IMPORT target: %w", err)
	}
	importURL, err := url.Parse(srv.URL)
	if err != nil || importURL.Scheme != "http" || importURL.Hostname() != "127.0.0.1" && importURL.Hostname() != "::1" {
		return fmt.Errorf("unexpected httptest URL %q", srv.URL)
	}
	importURL = importURL.JoinPath(fixturePath)
	// IMPORT has no bind placeholders, so quote the validated loopback URL as a SQL literal.
	stmt := "IMPORT INTO defaultdb.public.fgdb_capability_import CSV DATA (" + quoteSQLString(importURL.String()) + ")"
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("OSS CSV IMPORT: %w", err)
	}
	var count, exact int64
	err = conn.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE (id = 1 AND value = 'alpha') OR (id = 2 AND value = 'beta')) FROM defaultdb.public.fgdb_capability_import`).Scan(&count, &exact)
	if err != nil {
		return fmt.Errorf("check imported rows: %w", err)
	}
	if count != 2 || exact != 2 {
		return fmt.Errorf("IMPORT returned count=%d exact=%d, want two exact fixture rows", count, exact)
	}
	return nil
}

func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func serveCSV(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != fixturePath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	http.ServeContent(w, r, "rows.csv", time.Time{}, strings.NewReader(fixtureCSV))
}

func rejectBackup(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, backupSQL)
	return requireCCLError(err)
}

func requireCCLError(err error) error {
	if err == nil {
		return errors.New("BACKUP unexpectedly succeeded on the OSS binary")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("BACKUP did not return a PostgreSQL error: %w", err)
	}
	if pgErr.Code != cclState || !strings.Contains(pgErr.Message, cclMessage) {
		return fmt.Errorf("BACKUP SQLSTATE=%s message=%q; want %s and the OSS statement gate", pgErr.Code, pgErr.Message, cclState)
	}
	return nil
}
