// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type database struct {
	conn *pgx.Conn
}

func connect(ctx context.Context, url string) (*database, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse --url: %w", err)
	}
	cfg.OnNotice = func(*pgconn.PgConn, *pgconn.Notice) {
		// Notices are deliberately discarded.
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	db := &database{conn: conn}
	if _, err := db.exec(ctx, "SET statement_timeout = '0'"); err != nil {
		_ = conn.Close(ctx)
		return nil, err
	}
	return db, nil
}

func (d *database) Close(ctx context.Context) { _ = d.conn.Close(ctx) }

func (d *database) exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tag, err := d.conn.Exec(ctx, sql, args...)
	if err != nil {
		return tag, err
	}
	return tag, nil
}

func (d *database) query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return d.conn.Query(ctx, sql, args...)
}

func (d *database) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return d.conn.QueryRow(ctx, sql, args...)
}

func (d *database) version(ctx context.Context) (string, error) {
	var v string
	err := d.queryRow(ctx, "SELECT version()").Scan(&v)
	return v, err
}

func (d *database) clusterID(ctx context.Context) (string, error) {
	var id string
	err := d.queryRow(ctx, "SELECT crdb_internal.cluster_id()::STRING").Scan(&id)
	return id, err
}

func (d *database) captureAsOf(ctx context.Context) (string, error) {
	var ts string
	if err := d.queryRow(ctx, "SELECT now()::STRING").Scan(&ts); err != nil {
		return "", err
	}
	var last error
	for i := 0; i < 40; i++ {
		last = d.withSnapshot(ctx, ts, func(ctx context.Context) error {
			var one int
			return d.queryRow(ctx, "SELECT 1").Scan(&one)
		})
		if last == nil {
			return ts, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", fmt.Errorf("timestamp %s is not readable yet: %w", ts, last)
}

func (d *database) withSnapshot(ctx context.Context, asOf string, fn func(ctx context.Context) error) error {
	if _, err := d.exec(ctx, "BEGIN"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = d.exec(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := d.exec(ctx, "SET TRANSACTION AS OF SYSTEM TIME "+quoteLiteral(asOf)); err != nil {
		return err
	}
	if err := fn(ctx); err != nil {
		return err
	}
	if _, err := d.exec(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (d *database) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if _, err := d.exec(ctx, "BEGIN"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = d.exec(context.Background(), "ROLLBACK")
		}
	}()
	if err := fn(ctx); err != nil {
		return err
	}
	if _, err := d.exec(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (d *database) use(ctx context.Context, name string) error {
	_, err := d.exec(ctx, "USE "+quoteIdent(name))
	return err
}

func (d *database) copyTo(ctx context.Context, w io.Writer, sql string) error {
	_, err := d.conn.PgConn().CopyTo(ctx, w, sql)
	return err
}

func (d *database) copyFrom(ctx context.Context, r io.Reader, sql string) error {
	_, err := d.conn.PgConn().CopyFrom(ctx, r, sql)
	return err
}

func sqlState(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

func isDuplicate(err error) bool {
	switch sqlState(err) {
	case "42P04", "42P06", "42P07", "42710":
		return true
	default:
		return false
	}
}

func isGCError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "gc threshold") || strings.Contains(msg, "must be after replica gc")
}

type columnInfo struct {
	Name       string
	Generated  string // "", s, v
	TypeName   string
	FormatType string
	// TypeMod is format_type with the column's modifier, such as
	// character(5)[]. Only its "(n)" or "(p,s)" suffix is used.
	TypeMod string
}

func (d *database) columns(ctx context.Context, schema, table string) ([]columnInfo, error) {
	rows, err := d.query(ctx, `
SELECT a.attname, a.attgenerated::STRING, t.typname, format_type(t.oid, NULL), format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_type t ON t.oid = a.atttypid
WHERE n.nspname = $1 AND c.relname = $2 AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []columnInfo
	for rows.Next() {
		var c columnInfo
		if err := rows.Scan(&c.Name, &c.Generated, &c.TypeName, &c.FormatType, &c.TypeMod); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type pkInfo struct {
	Name     string
	TypeName string
}

func (d *database) primaryKey(ctx context.Context, schema, table string) ([]pkInfo, error) {
	rows, err := d.query(ctx, `
SELECT a.attname, t.typname
FROM pg_index i
JOIN pg_class c ON c.oid = i.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY (i.indkey)
JOIN pg_type t ON t.oid = a.atttypid
WHERE i.indisprimary AND n.nspname = $1 AND c.relname = $2
ORDER BY array_position(i.indkey, a.attnum)`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pkInfo
	for rows.Next() {
		var p pkInfo
		if err := rows.Scan(&p.Name, &p.TypeName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func dataColumns(cols []columnInfo) []columnInfo {
	var out []columnInfo
	for _, c := range cols {
		// Virtual and stored computed columns are not loaded. The CREATE
		// TABLE expression recomputes stored values. Identity columns are
		// ordinary data columns except GENERATED ALWAYS, which backup rejects.
		if c.Generated == "v" || c.Generated == "s" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func columnNames(cols []columnInfo) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
	}
	return out
}

// arraySQLColumns records which copied columns are arrays. The cast is the
// same one arrayLiteralExpr appends, so restore can tell an array field from
// a string that happens to look like ARRAY[...].
func arraySQLColumns(cols []columnInfo) ([]string, error) {
	out := make([]string, len(cols))
	any := false
	for i, c := range cols {
		if !strings.HasPrefix(c.TypeName, "_") {
			continue
		}
		cast, err := arrayCastSQL(c.TypeName, c.FormatType, c.TypeMod)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.Name, err)
		}
		out[i] = cast
		any = true
	}
	if !any {
		return nil, nil
	}
	return out, nil
}

func selectList(cols []columnInfo) (string, error) {
	parts := make([]string, len(cols))
	for i, c := range cols {
		if strings.HasPrefix(c.TypeName, "_") {
			cast, err := arrayCastSQL(c.TypeName, c.FormatType, c.TypeMod)
			if err != nil {
				return "", fmt.Errorf("column %s: %w", c.Name, err)
			}
			parts[i] = arrayLiteralExpr(quoteIdent(c.Name), cast)
			continue
		}
		parts[i] = quoteIdent(c.Name)
	}
	return strings.Join(parts, ", "), nil
}

// arrayLiteralExpr turns an array into text that IMPORT PGCOPY can parse.
// COPY TO writes {a,b}, and that text is not a SQL array literal, so the
// importer rejects it. ARRAY['a','b']::type is.
func arrayLiteralExpr(col, cast string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s IS NULL THEN NULL ELSE (SELECT 'ARRAY[' || IFNULL(string_agg(CASE WHEN x IS NULL THEN 'NULL' ELSE quote_literal(x::STRING) END, ',' ORDER BY ord), '') || ']::%[2]s' FROM unnest(%[1]s) WITH ORDINALITY AS u(x, ord)) END`, col, cast)
}

// arrayCastSQL is the cast written after an ARRAY[...] literal. It keeps the
// column's type modifier: a bare CHAR[] is CHAR(1)[], so IMPORT truncated
// every CHAR(n) element to one character, and a bare BIT[] rejected BIT(n).
func arrayCastSQL(typname, formatted, withMod string) (string, error) {
	mod := typeModifier(withMod)
	if base, ok := builtinArrayBase[typname]; ok {
		return base + mod + "[]", nil
	}
	if simpleArrayCast(formatted) {
		return strings.TrimSuffix(formatted, "[]") + mod + "[]", nil
	}
	return "", fmt.Errorf("cannot write array type %s in a form IMPORT PGCOPY accepts", formatted)
}

// builtinArrayBase names element types whose format_type has spaces or a
// name IMPORT does not parse.
var builtinArrayBase = map[string]string{
	"_float8":      "FLOAT8",
	"_bpchar":      "CHAR",
	"_bytea":       "BYTES",
	"_time":        "TIME",
	"_timestamp":   "TIMESTAMP",
	"_timestamptz": "TIMESTAMPTZ",
	"_timetz":      "TIMETZ",
	"_varchar":     "VARCHAR",
	"_varbit":      "VARBIT",
	"_numeric":     "DECIMAL",
}

// typeModifier returns the first "(n)" or "(p,s)" in a format_type string,
// or "" when there is none or it holds anything but digits and one comma.
func typeModifier(formatted string) string {
	open := strings.IndexByte(formatted, '(')
	if open < 0 {
		return ""
	}
	close := strings.IndexByte(formatted[open:], ')')
	if close < 0 {
		return ""
	}
	inner := formatted[open+1 : open+close]
	parts := strings.Split(inner, ",")
	if len(parts) > 2 {
		return ""
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return ""
		}
	}
	return "(" + inner + ")"
}

func simpleArrayCast(formatted string) bool {
	body := strings.TrimSuffix(formatted, "[]")
	if body == formatted || strings.ContainsAny(body, " \t\"'") {
		return false
	}
	parts := strings.Split(body, ".")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if !isSimpleIdent(part) {
			return false
		}
	}
	return true
}
