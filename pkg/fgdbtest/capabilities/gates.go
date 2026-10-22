// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package capabilities

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const gateTimeout = 30 * time.Second

// GateCase is one PAR-001 statement-gate contract row.
type GateCase struct {
	ID           string
	Setup        []string
	Statement    string
	WantSQLState string
	WantMessage  string
}

var gateCases = []GateCase{
	// PAR-001 statement-gate rows.
	{ID: "GATE-001", Statement: "BACKUP DATABASE defaultdb TO 'nodelocal://1/par001'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.Backup"},
	{ID: "GATE-002", Statement: "RESTORE DATABASE defaultdb FROM 'nodelocal://1/par001'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.Restore"},
	{ID: "GATE-003", Statement: "SHOW BACKUP FROM LATEST IN 'nodelocal://1/par001'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.ShowBackup"},
	{ID: "GATE-004", Setup: []string{"CREATE TABLE IF NOT EXISTS par001 (id INT PRIMARY KEY)"}, Statement: "CREATE CHANGEFEED FOR TABLE defaultdb.public.par001", WantSQLState: cclState, WantMessage: cclMessage + " *tree.CreateChangefeed"},
	{ID: "GATE-005", Statement: "ALTER CHANGEFEED 1 SET min_checkpoint_frequency = '1s'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.AlterChangefeed"},
	{ID: "GATE-007", Statement: "ALTER BACKUP 'nodelocal://1/par001' ADD NEW_KMS = 'kms://local/new' WITH OLD_KMS = 'kms://local/old'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.AlterBackup"},
	{ID: "GATE-008", Setup: []string{"CREATE TABLE IF NOT EXISTS par001 (id INT PRIMARY KEY)"}, Statement: "CREATE SCHEDULE FOR CHANGEFEED TABLE defaultdb.public.par001 INTO 'nodelocal://1/par001' RECURRING '@hourly'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.ScheduledChangefeed"},
	{ID: "GATE-009", Statement: "CREATE SCHEDULE FOR BACKUP DATABASE defaultdb INTO 'nodelocal://1/par001' RECURRING '@daily'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.ScheduledBackup"},
	{ID: "GATE-010", Statement: "ALTER BACKUP SCHEDULE 1 SET INTO 'nodelocal://1/par001'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.AlterBackupSchedule"},
	{ID: "GATE-011", Statement: "CREATE VIRTUAL CLUSTER v FROM REPLICATION OF source ON 'postgresql://127.0.0.1:26257'", WantSQLState: cclState, WantMessage: cclMessage + " *tree.CreateTenantFromReplication"},

	// GATE-012 through GATE-020: specific hook gates.
	{ID: "GATE-012", Statement: "CREATE DATABASE mr PRIMARY REGION 'us-east1' SURVIVE ZONE FAILURE", WantSQLState: cclState, WantMessage: "creating multi-region databases requires a CCL binary"},
	{ID: "GATE-013", Statement: "ALTER DATABASE defaultdb PRIMARY REGION 'us-east1'", WantSQLState: cclState, WantMessage: "creating multi-region databases requires a CCL binary"},
	{ID: "GATE-015", Statement: "CREATE TABLE p (a INT PRIMARY KEY) PARTITION BY LIST (a) (PARTITION p1 VALUES IN (1))", WantSQLState: cclState, WantMessage: "creating or manipulating partitions requires a CCL binary"},
	{ID: "GATE-016", Setup: []string{"CREATE TABLE IF NOT EXISTS p2 (a INT PRIMARY KEY)"}, Statement: "ALTER TABLE p2 PARTITION BY LIST (a) (PARTITION p1 VALUES IN (1))", WantSQLState: cclState, WantMessage: "creating or manipulating partitions requires a CCL binary"},
	{ID: "GATE-017", Statement: "CREATE FUNCTION f() RETURNS RECORD AS $$ BEGIN RETURN ROW(0); END $$ LANGUAGE PLpgSQL", WantSQLState: cclState, WantMessage: "using PL/pgSQL requires a CCL binary"},
	{ID: "GATE-018", Statement: "CREATE PROCEDURE p() AS $$ BEGIN RAISE NOTICE '1'; END $$ LANGUAGE PLpgSQL", WantSQLState: cclState, WantMessage: "using PL/pgSQL requires a CCL binary"},
	{ID: "GATE-019", Statement: "SET plan_cache_mode = force_generic_plan", WantSQLState: cclState, WantMessage: "plan_cache_mode=force_generic_plan and plan_cache_mode=auto require a CCL binary"},
	{ID: "GATE-020", Statement: "SET plan_cache_mode = auto", WantSQLState: cclState, WantMessage: "plan_cache_mode=force_generic_plan and plan_cache_mode=auto require a CCL binary"},

	// GATE-021 through GATE-026: builtin gates.
	{ID: "GATE-021", Statement: "SELECT encrypt('abc', 'key', 'aes')", WantSQLState: cclState, WantMessage: "encrypt can only be used with a CCL distribution"},
	{ID: "GATE-022", Statement: "SELECT encrypt_iv('abc', 'key', '123', 'aes')", WantSQLState: cclState, WantMessage: "encrypt_iv can only be used with a CCL distribution"},
	{ID: "GATE-023", Statement: "SELECT decrypt('\\xdb5f149a7caf0cd275ca18c203a212c9', 'key', 'aes')", WantSQLState: cclState, WantMessage: "decrypt can only be used with a CCL distribution"},
	{ID: "GATE-024", Statement: "SELECT decrypt_iv('\\x91b4ef63852013c8da53829da662b871', 'key', '123', 'aes')", WantSQLState: cclState, WantMessage: "decrypt_iv can only be used with a CCL distribution"},
	{ID: "GATE-025", Statement: "SELECT with_min_timestamp(now())", WantSQLState: cclState, WantMessage: "with_min_timestamp can only be used with a CCL distribution"},
	{ID: "GATE-026", Statement: "SELECT with_max_staleness('1s')", WantSQLState: cclState, WantMessage: "with_max_staleness can only be used with a CCL distribution"},
}

func gateClaimIDs() []string {
	ids := make([]string, 0, len(gateCases))
	for _, tc := range gateCases {
		ids = append(ids, tc.ID)
	}
	return ids
}

func runGateContract(ctx context.Context, conn *pgx.Conn, rep *report.Report) error {
	var failures []error
	for _, tc := range gateCases {
		caseCtx, cancel := context.WithTimeout(ctx, gateTimeout)
		setupErr := runGateSetup(caseCtx, conn, tc)
		if setupErr != nil {
			cancel()
			err := fmt.Errorf("%s setup: %w", tc.ID, setupErr)
			note(rep, tc.ID, tc.ID, err)
			return errors.Join(append(failures, err)...)
		}
		_, err := conn.Exec(caseCtx, tc.Statement)
		cancel()
		err = classifyGateError(err, tc.Statement, tc.WantSQLState, tc.WantMessage)
		note(rep, tc.ID, tc.ID, err)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", tc.ID, err))
		}
	}
	return errors.Join(failures...)
}

func runGateSetup(ctx context.Context, conn *pgx.Conn, tc GateCase) error {
	for _, statement := range tc.Setup {
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("setup statement %q: %w", statement, err)
		}
	}
	return nil
}

func classifyGateError(err error, statement, wantState, wantMessage string) error {
	if err == nil {
		return fmt.Errorf("%s unexpectedly succeeded", statement)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("%s did not return a PostgreSQL error: %w", statement, err)
	}
	if pgErr.Code != wantState || !strings.Contains(pgErr.Message, wantMessage) {
		return fmt.Errorf("%s SQLSTATE=%s message=%q; want %s and message containing %q", statement, pgErr.Code, pgErr.Message, wantState, wantMessage)
	}
	return nil
}
