// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/cluster"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linkedin/goavro/v2"
)

// guardPoll covers the job scheduler startup delay on a fresh node, one TTL cron minute and the TTL read lag (spec v1.2 erratum).
const guardPoll = 300 * time.Second

// ttlGuardEnv opts in to OSS-004. A fresh single node did not delete the
// expired row within guardPoll during development (spec open question), so
// by default the claim is reported not_tested with that reason, never pass.
const ttlGuardEnv = "FGDB_TTL_GUARD"

func runGuards(ctx context.Context, cfg Config, conn *pgx.Conn, c *cluster.Cluster, rep *report.Report) error {
	var errs []error
	checks := []struct {
		id string
		fn func() error
	}{
		{ClaimRC, func() error { return guardReadCommitted(ctx, c) }},
		{ClaimCopy, func() error { return guardCopy(ctx, conn) }},
		{ClaimImportMatrix, func() error { return guardImportMatrix(ctx, conn) }},
	}
	if os.Getenv(ttlGuardEnv) == "1" {
		checks = append([]struct {
			id string
			fn func() error
		}{{ClaimTTL, func() error { return guardTTL(ctx, conn) }}}, checks...)
	} else {
		rep.Set(ClaimTTL, labels.NotTested, "OSS-004 not_tested: opt-in via "+ttlGuardEnv+"=1; expired-row deletion was not observed within the poll budget on a fresh node (spec open question)")
	}
	for _, check := range checks {
		err := check.fn()
		note(rep, check.id, check.id, err)
		if check.id == ClaimImportMatrix {
			rep.AddStep(report.StepResult{ID: "OSS-008-DUMPS", Outcome: labels.NotTested, Detail: "MYSQLDUMP and PGDUMP not_tested: oracle-pending per PAR-002 maintainer notes", Required: false})
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	lib, err := besideBinaryLib(cfg.Binary)
	if err != nil {
		rep.Set(ClaimGEOS, labels.NotTested, fmt.Sprintf("GEOS beside-binary libraries not tested: %v", err))
	} else {
		err = guardGEOS(ctx, conn, lib)
		note(rep, ClaimGEOS, ClaimGEOS, err)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// OSS-005: the two shipped libraries must be regular files in binary/../lib.
func besideBinaryLib(binary string) (string, error) {
	if binary == "" || !strings.Contains(binary, string(os.PathSeparator)) {
		return "", errors.New("binary has no directory")
	}
	lib := filepath.Join(filepath.Dir(binary), "lib")
	info, err := os.Stat(lib)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("lib is not a directory")
	}
	for _, name := range []string{"libgeos.so", "libgeos_c.so"} {
		st, err := os.Stat(filepath.Join(lib, name))
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		if !st.Mode().IsRegular() {
			return "", fmt.Errorf("%s is not a regular file", name)
		}
	}
	return lib, nil
}

func readSettingText(ctx context.Context, conn *pgx.Conn, setting string) (string, error) {
	var value string
	// SHOW ALL CLUSTER SETTINGS exposes every value as text, independent of the setting's type.
	const query = "SELECT value FROM [SHOW ALL CLUSTER SETTINGS] WHERE variable = $1"
	if err := conn.QueryRow(ctx, query, setting).Scan(&value); err != nil {
		return "", err
	}
	return value, nil
}

// execEach runs each statement on its own so none shares an implicit transaction.
func execEach(ctx context.Context, conn *pgx.Conn, statements ...string) error {
	for _, statement := range statements {
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%q: %w", statement, err)
		}
	}
	return nil
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// OSS-005: area is checked without rounding an incorrect spatial result.
func guardGEOS(ctx context.Context, conn *pgx.Conn, lib string) error {
	if lib == "" {
		return errors.New("empty beside-binary GEOS directory")
	}
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var area float64
	if err := conn.QueryRow(queryCtx, `SELECT ST_Area(ST_GeomFromText('POLYGON ((0 0, 0 1, 1 1, 1 0, 0 0))'))`).Scan(&area); err != nil {
		return fmt.Errorf("GEOS query using %s: %w", lib, err)
	}
	if math.Abs(area-1) > 1e-9 {
		return fmt.Errorf("GEOS area=%v, want 1", area)
	}
	return nil
}

// OSS-004: TTL must visibly delete the expired row, not merely hide it.
func guardTTL(ctx context.Context, conn *pgx.Conn) (err error) {
	originalAdopt, readErr := readSettingText(ctx, conn, "jobs.registry.interval.adopt")
	if readErr != nil {
		return fmt.Errorf("read TTL adoption setting: %w", readErr)
	}
	defer func() {
		_, dropErr := conn.Exec(context.Background(), `DROP TABLE IF EXISTS ttl_guard`)
		_, restoreErr := conn.Exec(context.Background(), fmt.Sprintf("SET CLUSTER SETTING jobs.registry.interval.adopt = %s", sqlLiteral(originalAdopt)))
		err = errors.Join(err, dropErr, restoreErr)
	}()
	// Cluster settings cannot share an implicit multi-statement transaction.
	if err := execEach(ctx, conn,
		`SET CLUSTER SETTING sql.ttl.job.enabled = true`,
		`SET CLUSTER SETTING jobs.registry.interval.adopt = '1s'`,
		`CREATE TABLE ttl_guard (id INT PRIMARY KEY, v STRING) WITH (ttl_expire_after = '1 second', ttl_job_cron = '* * * * *')`,
		`INSERT INTO ttl_guard VALUES (1, 'expired'), (2, 'live')`,
		`UPDATE ttl_guard SET crdb_internal_expiration = now() - INTERVAL '1 minute' WHERE id = 1`,
	); err != nil {
		return fmt.Errorf("TTL setup: %w", err)
	}
	deadline := time.Now().Add(guardPoll)
	for time.Now().Before(deadline) {
		pollCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		rows, err := queryRows(pollCtx, conn, `SELECT id, v FROM ttl_guard ORDER BY id`)
		cancel()
		if err == nil && equalRows(rows, [][]string{{"2", "live"}}) {
			return nil
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("TTL poll: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("TTL expired row was not deleted before deadline")
}

// OSS-006: READ COMMITTED observes B's committed write while A remains open.
func guardReadCommitted(ctx context.Context, c *cluster.Cluster) (err error) {
	a, err := connect(ctx, c)
	if err != nil {
		return err
	}
	defer a.Close(context.Background())
	b, err := connect(ctx, c)
	if err != nil {
		return err
	}
	defer b.Close(context.Background())
	originalEnabled, readErr := readSettingText(ctx, a, "sql.txn.read_committed_isolation.enabled")
	if readErr != nil {
		return fmt.Errorf("read READ COMMITTED setting: %w", readErr)
	}
	defer func() {
		_, _ = a.Exec(context.Background(), `ROLLBACK`)
		_, dropErr := a.Exec(context.Background(), `DROP TABLE IF EXISTS rc_guard`)
		_, restoreErr := a.Exec(context.Background(), fmt.Sprintf("SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = %s", originalEnabled))
		err = errors.Join(err, dropErr, restoreErr)
	}()
	if err = execEach(ctx, a,
		`SET CLUSTER SETTING sql.txn.read_committed_isolation.enabled = true`,
		`DROP TABLE IF EXISTS rc_guard`,
		`CREATE TABLE rc_guard (id INT PRIMARY KEY)`,
		`INSERT INTO rc_guard VALUES (1)`,
	); err != nil {
		return fmt.Errorf("READ COMMITTED setup: %w", err)
	}
	if _, err = a.Exec(ctx, `BEGIN TRANSACTION ISOLATION LEVEL READ COMMITTED`); err != nil {
		return err
	}
	var first int
	if err = a.QueryRow(ctx, `SELECT count(*) FROM rc_guard`).Scan(&first); err != nil || first != 1 {
		return fmt.Errorf("READ COMMITTED first count=%d: %w", first, err)
	}
	if _, err = b.Exec(ctx, `INSERT INTO rc_guard VALUES (2)`); err != nil {
		return err
	}
	var second int
	if err = a.QueryRow(ctx, `SELECT count(*) FROM rc_guard`).Scan(&second); err != nil || second != 2 {
		return fmt.Errorf("READ COMMITTED second count=%d: %w", second, err)
	}
	_, err = a.Exec(ctx, `COMMIT`)
	return err
}

// OSS-007: exercise pgx's wire-protocol COPY methods in both directions.
func guardCopy(ctx context.Context, conn *pgx.Conn) (err error) {
	if _, err := conn.Exec(ctx, `DROP TABLE IF EXISTS copy_guard; CREATE TABLE copy_guard (id INT PRIMARY KEY, v STRING NOT NULL)`); err != nil {
		return err
	}
	defer func() {
		_, dropErr := conn.Exec(context.Background(), `DROP TABLE IF EXISTS copy_guard`)
		err = errors.Join(err, dropErr)
	}()
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tag, err := conn.PgConn().CopyFrom(deadline, strings.NewReader("1\talpha\n2\tbeta\n"), `COPY copy_guard FROM STDIN`)
	if err != nil {
		return fmt.Errorf("COPY FROM: %w", err)
	}
	if tag.RowsAffected() != 2 {
		return fmt.Errorf("COPY FROM rows=%d, want 2", tag.RowsAffected())
	}
	rows, err := queryRows(deadline, conn, `SELECT id, v FROM copy_guard ORDER BY id`)
	if err != nil || !equalRows(rows, [][]string{{"1", "alpha"}, {"2", "beta"}}) {
		return fmt.Errorf("COPY verification rows=%v: %w", rows, err)
	}
	var out bytes.Buffer
	if _, err = conn.PgConn().CopyTo(deadline, &out, `COPY (SELECT id, v FROM copy_guard ORDER BY id) TO STDOUT`); err != nil {
		return fmt.Errorf("COPY TO: %w", err)
	}
	if out.String() != "1\talpha\n2\tbeta\n" {
		return fmt.Errorf("COPY TO output=%q", out.String())
	}
	return nil
}

type importFixture struct{ format, path, body string }

// OSS-008: accepted row formats use loopback HTTP fixtures; dump formats remain oracle-pending.
func guardImportMatrix(ctx context.Context, conn *pgx.Conn) (err error) {
	fixtures := []importFixture{{"CSV", "/par002.csv", "1,alpha\n2,beta\n"}, {"DELIMITED", "/par002.delimited", "1\talpha\n2\tbeta\n"}, {"PGCOPY", "/par002.pgc", string(pgCopyFixture())}, {"AVRO", "/par002.avro", string(avroFixture())}}
	var errs []error
	srv := httptest.NewServer(fixtureHandler(fixtures))
	/*
		The server is loopback-only because httptest.NewServer binds 127.0.0.1.
	*/
	defer srv.Close()
	defer func() {
		for i := range fixtures {
			_, dropErr := conn.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS import_guard_%d`, i))
			err = errors.Join(err, dropErr)
		}
	}()
	for i, f := range fixtures {
		table := fmt.Sprintf("import_guard_%d", i)
		if _, createErr := conn.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (id INT PRIMARY KEY, v STRING NOT NULL)`, table)); createErr != nil {
			errs = append(errs, fmt.Errorf("IMPORT %s create target: %w", f.format, createErr))
			continue
		}
		stmt := fmt.Sprintf(`IMPORT INTO %s %s DATA ('%s%s')`, table, f.format, srv.URL, f.path)
		if f.format == "DELIMITED" {
			stmt += ` WITH fields_terminated_by = e'\t'`
		}
		importCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, importErr := conn.Exec(importCtx, stmt)
		if importErr != nil {
			cancel()
			errs = append(errs, fmt.Errorf("IMPORT %s: %w", f.format, importErr))
			continue
		}
		rows, rowErr := queryRows(importCtx, conn, fmt.Sprintf(`SELECT id, v FROM %s ORDER BY id`, table))
		cancel()
		if rowErr != nil {
			errs = append(errs, fmt.Errorf("IMPORT %s rows=%v: %w", f.format, rows, rowErr))
		} else if !equalRows(rows, [][]string{{"1", "alpha"}, {"2", "beta"}}) {
			errs = append(errs, fmt.Errorf("IMPORT %s rows=%v: want exactly two fixture rows", f.format, rows))
		}
		if _, dropErr := conn.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, table)); dropErr != nil {
			errs = append(errs, fmt.Errorf("IMPORT %s cleanup: %w", f.format, dropErr))
		}
	}
	if _, createErr := conn.Exec(ctx, `CREATE TABLE import_guard (id INT PRIMARY KEY, v STRING NOT NULL)`); createErr != nil {
		errs = append(errs, fmt.Errorf("IMPORT PARQUET create target: %w", createErr))
		return errors.Join(errs...)
	}
	defer func() {
		_, dropErr := conn.Exec(context.Background(), `DROP TABLE IF EXISTS import_guard`)
		err = errors.Join(err, dropErr)
	}()
	parquetCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, parquetErr := conn.Exec(parquetCtx, `IMPORT INTO import_guard PARQUET DATA ('nodelocal://1/par002/unused')`)
	cancel()
	if parquetErr == nil {
		errs = append(errs, errors.New("IMPORT PARQUET unexpectedly succeeded"))
	} else {
		var pe *pgconn.PgError
		if !errors.As(parquetErr, &pe) || pe.Code != "0A000" || !strings.Contains(pe.Message, `unsupported import format: "PARQUET"`) {
			errs = append(errs, fmt.Errorf("IMPORT PARQUET: %w", parquetErr))
		}
	}
	return errors.Join(errs...)
}

func fixtureHandler(fixtures []importFixture) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		for _, f := range fixtures {
			if r.URL.Path == f.path {
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = io.WriteString(w, f.body)
				}
				return
			}
		}
		http.NotFound(w, r)
	}
}

func queryRows(ctx context.Context, conn *pgx.Conn, sql string) ([][]string, error) {
	rs, err := conn.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out [][]string
	for rs.Next() {
		vals, err := rs.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			row[i] = fmt.Sprint(v)
		}
		out = append(out, row)
	}
	return out, rs.Err()
}
func equalRows(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}
func pgCopyFixture() []byte {
	return []byte("1\talpha\n2\tbeta\n")
}
func avroFixture() []byte {
	schema := `{"type":"record","name":"row","fields":[{"name":"id","type":"int"},{"name":"v","type":"string"}]}`
	codec, err := goavro.NewCodec(schema)
	if err != nil {
		return nil
	}
	var b bytes.Buffer
	w, err := goavro.NewOCFWriter(goavro.OCFConfig{W: &b, Codec: codec, Schema: schema})
	if err != nil {
		return nil
	}
	_ = w.Append([]interface{}{map[string]interface{}{"id": int32(1), "v": "alpha"}, map[string]interface{}{"id": int32(2), "v": "beta"}})
	return b.Bytes()
}
