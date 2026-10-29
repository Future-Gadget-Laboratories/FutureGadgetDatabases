// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apache/arrow/go/v11/parquet/file"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ClaimExportCSV      = "EXP-001"
	ClaimExportParquet  = "EXP-002"
	ClaimExportDisabled = "EXP-003"
	ClaimExportGate     = "EXP-004"

	activeSQLTransactionState = "25001"
)

func runExportChecks(ctx context.Context, conn *pgx.Conn, store string, rep *report.Report) error {
	var result error
	check := func(id string, err error) {
		note(rep, id, id, err)
		result = errors.Join(result, err)
	}
	_, err := conn.Exec(ctx, `EXPORT INTO CSV 'nodelocal://1/fgdb-export-csv/' FROM SELECT 1`)
	if err == nil {
		var paths []string
		paths, err = filepath.Glob(filepath.Join(store, "extern", "fgdb-export-csv", "*.csv"))
		if err == nil && len(paths) != 1 {
			err = fmt.Errorf("CSV export produced %d files, want one", len(paths))
		}
		if err == nil {
			var data []byte
			data, err = os.ReadFile(paths[0])
			if err == nil && string(data) != "1\n" {
				err = fmt.Errorf("CSV export = %q, want %q", data, "1\n")
			}
		}
	}
	check(ClaimExportCSV, err)

	_, err = conn.Exec(ctx, `EXPORT INTO PARQUET 'nodelocal://1/fgdb-export-parquet/' WITH compression = gzip FROM SELECT 42::INT AS value`)
	if err == nil {
		var paths []string
		paths, err = filepath.Glob(filepath.Join(store, "extern", "fgdb-export-parquet", "*.parquet.gz"))
		if err == nil && len(paths) != 1 {
			err = fmt.Errorf("PARQUET export produced %d files, want one", len(paths))
		}
		if err == nil {
			err = checkParquet(paths[0])
		}
	}
	check(ClaimExportParquet, err)

	_, err = conn.Exec(ctx, `EXPORT INTO CSV 'nodelocal://1/fgdb-export-gate/' FROM SELECT 1`)
	var gateErr *pgconn.PgError
	if errors.As(err, &gateErr) && gateErr.Code == cclState && strings.Contains(gateErr.Message, cclMessage) {
		err = fmt.Errorf("EXPORT hit CCL statement gate: %w", err)
	}
	if err != nil {
		check(ClaimExportGate, err)
	} else {
		detail := "CSV EXPORT succeeded without SQLSTATE XXC01 and the CCL statement-gate message"
		rep.Set(ClaimExportGate, labels.Pass, detail)
		rep.AddStep(report.StepResult{ID: ClaimExportGate, Outcome: labels.Pass, Detail: detail, Required: true})
	}

	_, err = conn.Exec(ctx, `SET CLUSTER SETTING feature.export.enabled = false`)
	if err == nil {
		_, err = conn.Exec(ctx, `EXPORT INTO CSV 'nodelocal://1/fgdb-export-disabled/' FROM SELECT 1`)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "57000" || !strings.Contains(pgErr.Message, "feature EXPORT was disabled by the database administrator") {
			err = fmt.Errorf("disabled EXPORT error = %v, want SQLSTATE 57000 and administrator disable message", err)
		} else {
			err = nil
		}
	}
	check(ClaimExportDisabled, err)

	_, err = conn.Exec(ctx, `SET CLUSTER SETTING feature.export.enabled = true`)
	if err != nil {
		err = fmt.Errorf("re-enable EXPORT: %w", err)
	} else {
		_, err = conn.Exec(ctx, `BEGIN`)
		if err == nil {
			_, exportErr := conn.Exec(ctx, `EXPORT INTO CSV 'nodelocal://1/fgdb-export-txn/' FROM SELECT 1`)
			_, rollbackErr := conn.Exec(ctx, `ROLLBACK`)
			var pgErr *pgconn.PgError
			if !errors.As(exportErr, &pgErr) || pgErr.Code != activeSQLTransactionState {
				err = fmt.Errorf("transaction EXPORT error = %v, want SQLSTATE %s", exportErr, activeSQLTransactionState)
			}
			err = errors.Join(err, rollbackErr)
		}
	}
	note(rep, "EXP-MULTI-STATEMENT", "", err)
	return errors.Join(result, err)
}

// checkParquet reads the exported value without relying on test-only metadata.
func checkParquet(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	reader, err := file.NewParquetReader(f)
	if err != nil {
		f.Close()
		return err
	}
	defer reader.Close()
	if reader.NumRowGroups() != 1 {
		return fmt.Errorf("PARQUET row groups = %d, want one", reader.NumRowGroups())
	}
	group := reader.RowGroup(0)
	if group.NumRows() != 1 || group.NumColumns() != 1 {
		return fmt.Errorf("PARQUET rows=%d columns=%d, want one of each", group.NumRows(), group.NumColumns())
	}
	col, err := group.Column(0)
	if err != nil {
		return err
	}
	values, ok := col.(interface {
		ReadBatch(int64, []int64, []int16, []int16) (int64, int, error)
	})
	if !ok {
		return fmt.Errorf("PARQUET column has unexpected type %T", col)
	}
	var value [1]int64
	var def, rep [1]int16
	rows, count, err := values.ReadBatch(1, value[:], def[:], rep[:])
	if err != nil {
		return err
	}
	if rows != 1 || count != 1 || value[0] != 42 {
		return fmt.Errorf("PARQUET rows=%d values=%d value=%d, want 42", rows, count, value[0])
	}
	return nil
}
