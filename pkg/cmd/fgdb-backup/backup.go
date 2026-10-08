// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// BackupOptions is the backup command.
type BackupOptions struct {
	URL          string
	Dest         Location
	Databases    []string
	Name         string
	Compression  string
	SplitRows    int
	PartSize     int
	SafetyMargin time.Duration
	ExtendGCTTL  time.Duration
	JSON         bool
}

// BackupResult is the machine-readable backup summary.
type BackupResult struct {
	OK            bool     `json:"ok"`
	Error         string   `json:"error,omitempty"`
	Backup        string   `json:"backup,omitempty"`
	Latest        string   `json:"latest,omitempty"`
	Name          string   `json:"name,omitempty"`
	Timestamp     string   `json:"timestamp,omitempty"`
	AsOf          string   `json:"as_of,omitempty"`
	SourceVersion string   `json:"source_version,omitempty"`
	ClusterID     string   `json:"cluster_id,omitempty"`
	Databases     []string `json:"databases,omitempty"`
	Tables        int      `json:"tables,omitempty"`
	Rows          int64    `json:"rows,omitempty"`
	GCTTLSeconds  int      `json:"gc_ttl_seconds,omitempty"`
	PeakRSSBytes  int64    `json:"peak_rss_bytes,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	RevertSQL     []string `json:"revert_sql,omitempty"`
}

func runBackup(ctx context.Context, opt BackupOptions) (res BackupResult, err error) {
	if opt.Compression == "" {
		opt.Compression = "gzip"
	}
	if opt.Compression != "gzip" && opt.Compression != "none" {
		return res, fmt.Errorf("--compression must be gzip or none")
	}
	if opt.PartSize == 0 {
		opt.PartSize = 8 << 20
	}
	if opt.Dest.Kind == "s3" && opt.PartSize < minPartSize {
		return res, fmt.Errorf("--part-size must be at least %d bytes for S3 multipart upload", minPartSize)
	}
	if opt.SafetyMargin <= 0 {
		opt.SafetyMargin = time.Minute
	}

	db, err := connect(ctx, opt.URL)
	if err != nil {
		return res, err
	}
	defer db.Close(ctx)

	version, err := db.version(ctx)
	if err != nil {
		return res, err
	}
	clusterID, err := db.clusterID(ctx)
	if err != nil {
		return res, fmt.Errorf("read cluster id: %w", err)
	}
	name := opt.Name
	if name == "" {
		name = clusterID
	}
	if err := safeSegment(name); err != nil {
		return res, err
	}

	dbs, err := listDatabases(ctx, db, opt.Databases)
	if err != nil {
		return res, err
	}
	if len(dbs) == 0 {
		return res, fmt.Errorf("no user databases to back up")
	}

	store, err := openStore(ctx, opt.Dest)
	if err != nil {
		return res, err
	}
	if s3s, ok := store.(*s3Store); ok {
		s3s.partSize = opt.PartSize
	}

	zones, err := readZones(ctx, db, dbs)
	if err != nil {
		return res, err
	}
	// The backup replays the zones that were set before this run. A temporary
	// gc.ttlseconds raise is not part of the artifact.
	artifactZones := append([]zoneRow(nil), zones...)
	revertSQL := []string{}
	defer func() {
		if len(revertSQL) == 0 {
			return
		}
		logf("restoring gc.ttlseconds")
		for _, sql := range revertSQL {
			if _, e := db.exec(context.Background(), sql); e != nil {
				logf("warning: could not restore gc.ttlseconds: %s", e)
				if err == nil {
					err = fmt.Errorf("backup files were written, but gc.ttlseconds could not be restored: %w\nRun: %s", e, sql)
					res.OK = false
					res.Error = err.Error()
				}
			}
		}
	}()
	extends := planGCTTLRaises(int(opt.ExtendGCTTL/time.Second), dbs, zones)
	if len(extends) > 0 {
		logf("raising gc.ttlseconds for this run")
		for _, ch := range extends {
			logf("  %s", ch.Apply)
			if _, err := db.exec(ctx, ch.Apply); err != nil {
				return res, fmt.Errorf("raise gc.ttlseconds on %s: %w", ch.Object, err)
			}
			stmt := ensureSemicolon(ch.Revert)
			revertSQL = append(revertSQL, stmt)
			res.RevertSQL = append(res.RevertSQL, stmt)
		}
		logf("if this process is killed, put the old values back with:")
		for _, sql := range revertSQL {
			logf("  %s", sql)
		}
		zones, err = readZones(ctx, db, dbs)
		if err != nil {
			return res, err
		}
	}

	asOfText, err := db.captureAsOf(ctx)
	if err != nil {
		return res, err
	}
	asOf, err := parseAsOf(asOfText)
	if err != nil {
		return res, fmt.Errorf("timestamp %q from the source: %w", asOfText, err)
	}

	minTTL, minObj := minEffectiveTTL(zones)
	if minTTL <= 0 {
		return res, fmt.Errorf("could not read gc.ttlseconds for the databases being backed up")
	}
	budget := newBudget(asOf, minTTL, minObj, opt.SafetyMargin)
	if budget.exceeded(time.Now()) {
		return res, fmt.Errorf("%s", budget.message())
	}
	logf("snapshot %s is readable until %s (gc.ttlseconds=%d on %s)", asOfText, budget.deadline.Format(time.RFC3339), minTTL, minObj)

	ts := time.Now().UTC().Format("20060102T150405Z")
	base := name + "/" + ts
	res.Backup = opt.Dest.String() + "/" + base
	res.Name = name
	res.Timestamp = ts
	res.AsOf = asOfText
	res.SourceVersion = version
	res.ClusterID = clusterID
	res.Databases = dbs
	res.GCTTLSeconds = minTTL

	objects := ObjectsFile{FormatVersion: formatVersion}
	var warnings []string
	var tables []TableEntry
	var totalRows int64

	for _, database := range dbs {
		if err := safeSegment(database); err != nil {
			return res, err
		}
		createDB, err := showCreateDatabase(ctx, db, database)
		if err != nil {
			return res, err
		}
		objects.Databases = append(objects.Databases, NamedSQL{Name: database, SQL: ensureSemicolon(createDB)})

		var stmts []string
		err = db.withSnapshot(ctx, asOfText, func(ctx context.Context) error {
			var qerr error
			stmts, qerr = showCreateBundle(ctx, db, database)
			return qerr
		})
		if err != nil {
			if isGCError(err) {
				return res, fmt.Errorf("%s\n\nsource error: %w", budget.message(), err)
			}
			return res, fmt.Errorf("read schema for %s: %w", database, err)
		}
		for _, sql := range stmts {
			kind := classifyStatement(sql)
			if kind == "comment" {
				continue
			}
			objects.Statements = append(objects.Statements, Statement{
				Database: database,
				Kind:     kind,
				Object:   objectName(kind, sql),
				SQL:      ensureSemicolon(sql),
			})
		}

		var rels []tableRef
		err = db.withSnapshot(ctx, asOfText, func(ctx context.Context) error {
			if err := db.use(ctx, database); err != nil {
				return err
			}
			var qerr error
			rels, qerr = listRelations(ctx, db)
			return qerr
		})
		if err != nil {
			return res, fmt.Errorf("list tables in %s: %w", database, err)
		}

		seqVals, seqWarn, err := readSequenceValues(ctx, db, asOfText, database, rels)
		if err != nil {
			return res, err
		}
		warnings = append(warnings, seqWarn...)
		objects.SequenceValues = append(objects.SequenceValues, seqVals...)

		for _, rel := range rels {
			if rel.Type != "table" {
				continue
			}
			if err := budgetCheck(budget); err != nil {
				return res, err
			}
			entry, err := dumpTable(ctx, db, store, dumpSpec{
				asOf:        asOfText,
				base:        base,
				database:    database,
				schema:      rel.Schema,
				table:       rel.Name,
				compression: opt.Compression,
				splitRows:   opt.SplitRows,
				exceeded: func() error {
					if budget.exceeded(time.Now()) || isPast(budget) {
						return fmt.Errorf("%s", budget.message())
					}
					return nil
				},
			})
			if err != nil {
				if isGCError(err) {
					return res, fmt.Errorf("%s\n\nsource error: %w", budget.message(), err)
				}
				return res, err
			}
			tables = append(tables, entry)
			totalRows += entry.RowCount
			logf("copied %s.%s.%s (%d rows)", database, rel.Schema, rel.Name, entry.RowCount)
		}
	}

	grantSQL, grantWarn := readGrants(ctx, db, dbs)
	warnings = append(warnings, grantWarn...)
	objects.Grants = grantSQL

	for _, z := range artifactZones {
		if z.Level == "range" || strings.TrimSpace(z.RawSQL) == "" {
			continue
		}
		objects.Zones = append(objects.Zones, ZoneStatement{
			Object: z.Object,
			Level:  z.Level,
			SQL:    ensureSemicolon(z.RawSQL),
		})
	}

	schemaFiles := map[string]*bytes.Buffer{}
	for _, st := range objects.Statements {
		buf := schemaFiles[st.Database]
		if buf == nil {
			buf = &bytes.Buffer{}
			schemaFiles[st.Database] = buf
		}
		fmt.Fprintf(buf, "-- kind: %s object: %s\n%s\n\n", st.Kind, st.Object, st.SQL)
	}
	var schemaDigests []FileDigest
	for _, database := range dbs {
		buf := schemaFiles[database]
		if buf == nil {
			continue
		}
		rel := base + "/schema/" + database + ".sql"
		dig, err := writeBytes(ctx, store, rel, buf.Bytes())
		if err != nil {
			return res, err
		}
		schemaDigests = append(schemaDigests, dig)
	}

	usersBuf := &bytes.Buffer{}
	for _, g := range objects.Grants {
		fmt.Fprintf(usersBuf, "%s\n", ensureSemicolon(g))
	}
	usersDig, err := writeBytes(ctx, store, base+"/users.sql", usersBuf.Bytes())
	if err != nil {
		return res, err
	}
	zonesBuf := &bytes.Buffer{}
	for _, z := range objects.Zones {
		fmt.Fprintf(zonesBuf, "-- level: %s object: %s\n%s\n\n", z.Level, z.Object, ensureSemicolon(z.SQL))
	}
	zonesDig, err := writeBytes(ctx, store, base+"/zones.sql", zonesBuf.Bytes())
	if err != nil {
		return res, err
	}
	objBody, err := json.MarshalIndent(objects, "", "  ")
	if err != nil {
		return res, err
	}
	objBody = append(objBody, '\n')
	objDig, err := writeBytes(ctx, store, base+"/objects.json", objBody)
	if err != nil {
		return res, err
	}

	manifest := Manifest{
		FormatVersion: formatVersion,
		Tool:          toolName,
		ToolVersion:   toolVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		SourceVersion: version,
		ClusterID:     clusterID,
		Name:          name,
		AsOf:          asOfText,
		GCTTLSeconds:  minTTL,
		Compression:   opt.Compression,
		Databases:     dbs,
		ObjectsFile:   objDig,
		SchemaFiles:   schemaDigests,
		UsersFile:     &usersDig,
		ZonesFile:     &zonesDig,
		Tables:        tables,
		Warnings:      warnings,
		PeakRSSBytes:  peakRSSBytes(),
	}
	manBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return res, err
	}
	manBody = append(manBody, '\n')
	if _, err := writeBytes(ctx, store, base+"/manifest.json", manBody); err != nil {
		return res, err
	}

	pointer := LatestPointer{
		FormatVersion: formatVersion,
		Name:          name,
		Timestamp:     ts,
		Complete:      true,
	}
	ptrBody, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return res, err
	}
	ptrBody = append(ptrBody, '\n')
	if _, err := writeBytes(ctx, store, name+"/latest.json", ptrBody); err != nil {
		return res, err
	}

	res.OK = true
	res.Latest = opt.Dest.String() + "/" + name + "/latest"
	res.Tables = len(tables)
	res.Rows = totalRows
	res.PeakRSSBytes = manifest.PeakRSSBytes
	res.Warnings = warnings
	return res, nil
}

func isPast(b gcBudget) bool { return b.exceeded(time.Now()) }

func budgetCheck(b gcBudget) error {
	if b.exceeded(time.Now()) {
		return fmt.Errorf("%s", b.message())
	}
	return nil
}

type tableRef struct {
	Schema string
	Name   string
	Type   string
}

type dumpSpec struct {
	asOf        string
	base        string
	database    string
	schema      string
	table       string
	compression string
	splitRows   int
	exceeded    func() error
}

func dumpTable(ctx context.Context, db *database, store Store, spec dumpSpec) (TableEntry, error) {
	var entry TableEntry
	if err := safeSegment(spec.schema); err != nil {
		return entry, err
	}
	if err := safeSegment(spec.table); err != nil {
		return entry, err
	}
	var cols []columnInfo
	var pk []pkInfo
	err := db.withSnapshot(ctx, spec.asOf, func(ctx context.Context) error {
		if err := db.use(ctx, spec.database); err != nil {
			return err
		}
		var qerr error
		cols, qerr = db.columns(ctx, spec.schema, spec.table)
		if qerr != nil {
			return qerr
		}
		pk, qerr = db.primaryKey(ctx, spec.schema, spec.table)
		return qerr
	})
	if err != nil {
		return entry, fmt.Errorf("describe %s.%s.%s: %w", spec.database, spec.schema, spec.table, err)
	}
	cols = dataColumns(cols)
	if len(cols) == 0 {
		return entry, fmt.Errorf("%s.%s.%s has no columns to copy", spec.database, spec.schema, spec.table)
	}
	entry = TableEntry{
		Database: spec.database,
		Schema:   spec.schema,
		Name:     spec.table,
		Columns:  columnNames(cols),
	}

	var wheres []string
	if spec.splitRows > 0 && len(pk) == 1 && isIntType(pk[0].TypeName) {
		bounds, err := splitBounds(ctx, db, spec, pk[0].Name)
		if err != nil {
			return entry, err
		}
		wheres = whereRanges(pk[0].Name, bounds)
		if len(wheres) > 1 {
			logf("splitting %s.%s.%s into %d primary-key ranges", spec.database, spec.schema, spec.table, len(wheres))
		}
	} else if spec.splitRows > 0 {
		logf("not splitting %s.%s.%s (needs a single integer primary key)", spec.database, spec.schema, spec.table)
	}
	if len(wheres) == 0 {
		wheres = []string{""}
	}

	ext := ".csv"
	if spec.compression == "gzip" {
		ext = ".csv.gz"
	}
	order := ""
	if len(pk) == 1 {
		order = " ORDER BY " + quoteIdent(pk[0].Name)
	}
	for i, where := range wheres {
		rel := fmt.Sprintf("%s/data/%s/%s/%s%s", spec.base, spec.database, spec.schema, spec.table, ext)
		if len(wheres) > 1 {
			rel = fmt.Sprintf("%s/data/%s/%s/%s.part%04d%s", spec.base, spec.database, spec.schema, spec.table, i+1, ext)
		}
		dig, rows, err := copyRelation(ctx, db, store, spec, rel, selectList(cols), where, order)
		if err != nil {
			return entry, fmt.Errorf("copy %s.%s.%s: %w", spec.database, spec.schema, spec.table, err)
		}
		dig.RowCount = rows
		entry.Files = append(entry.Files, dig)
		entry.RowCount += rows
	}
	return entry, nil
}

func isIntType(typ string) bool {
	switch typ {
	case "int2", "int4", "int8":
		return true
	default:
		return false
	}
}

func splitBounds(ctx context.Context, db *database, spec dumpSpec, pk string) ([]string, error) {
	var bounds []string
	for off := spec.splitRows; ; off += spec.splitRows {
		var value *string
		err := db.withSnapshot(ctx, spec.asOf, func(ctx context.Context) error {
			q := fmt.Sprintf(
				"SELECT %s::STRING FROM %s ORDER BY %s OFFSET %d LIMIT 1",
				quoteIdent(pk), qualified(spec.database, spec.schema, spec.table), quoteIdent(pk), off,
			)
			return db.queryRow(ctx, q).Scan(&value)
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			return nil, err
		}
		if value == nil {
			break
		}
		bounds = append(bounds, *value)
		if len(bounds) > 100000 {
			return nil, fmt.Errorf("too many split points for %s", spec.table)
		}
	}
	return bounds, nil
}

func whereRanges(pk string, bounds []string) []string {
	if len(bounds) == 0 {
		return nil
	}
	col := quoteIdent(pk)
	var out []string
	out = append(out, fmt.Sprintf("%s < %s", col, sqlBound(bounds[0])))
	for i := 0; i < len(bounds)-1; i++ {
		out = append(out, fmt.Sprintf("%s >= %s AND %s < %s", col, sqlBound(bounds[i]), col, sqlBound(bounds[i+1])))
	}
	out = append(out, fmt.Sprintf("%s >= %s", col, sqlBound(bounds[len(bounds)-1])))
	return out
}

func sqlBound(v string) string {
	if _, err := strconv.ParseInt(v, 10, 64); err == nil {
		return v
	}
	return quoteLiteral(v)
}

func copyRelation(ctx context.Context, db *database, store Store, spec dumpSpec, rel, cols, where, order string) (FileDigest, int64, error) {
	var dig FileDigest
	var rows int64
	err := db.withSnapshot(ctx, spec.asOf, func(ctx context.Context) error {
		wc, err := store.Create(ctx, rel)
		if err != nil {
			return err
		}
		hw := newHashWriteCloser(wc)
		var sink io.Writer = hw
		var gz *gzip.Writer
		if spec.compression == "gzip" {
			gz = gzip.NewWriter(hw)
			sink = gz
		}
		counter := newCSVCounter(deadlineWriter{w: sink, exceeded: spec.exceeded})
		q := fmt.Sprintf("SELECT %s FROM %s", cols, qualified(spec.database, spec.schema, spec.table))
		if where != "" {
			q += " WHERE " + where
		}
		q += order
		copySQL := "COPY (" + q + ") TO STDOUT WITH CSV NULL E'\\\\N'"
		cerr := db.copyTo(ctx, counter, copySQL)
		rows = counter.Rows()
		var cerr2 error
		if gz != nil {
			cerr2 = gz.Close()
		}
		cerr3 := hw.Close()
		if cerr != nil {
			return cerr
		}
		if cerr2 != nil {
			return cerr2
		}
		if cerr3 != nil {
			return cerr3
		}
		dig = hw.digest()
		dig.Path = rel[strings.Index(rel, "/data/")+1:]
		// Path in the manifest is relative to the timestamp directory.
		if i := strings.Index(rel, "/data/"); i >= 0 {
			dig.Path = rel[i+1:]
		}
		return nil
	})
	return dig, rows, err
}

func writeBytes(ctx context.Context, store Store, rel string, body []byte) (FileDigest, error) {
	wc, err := store.Create(ctx, rel)
	if err != nil {
		return FileDigest{}, err
	}
	hw := newHashWriteCloser(wc)
	if _, err := hw.Write(body); err != nil {
		_ = hw.Close()
		return FileDigest{}, err
	}
	if err := hw.Close(); err != nil {
		return FileDigest{}, err
	}
	dig := hw.digest()
	// Store the path relative to the timestamp directory when the key contains it.
	dig.Path = relativeToTimestamp(rel)
	return dig, nil
}

func relativeToTimestamp(rel string) string {
	// name/timestamp/rest -> rest. latest.json stays as given when it does not match.
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 && parts[len(parts)-1] != "latest.json" {
		return strings.Join(parts[2:], "/")
	}
	return rel
}

func listDatabases(ctx context.Context, db *database, only []string) ([]string, error) {
	rows, err := db.query(ctx, "SELECT database_name FROM [SHOW DATABASES]")
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	defer rows.Close()
	have := map[string]bool{}
	var all []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		have[name] = true
		if skippedDatabase(name) {
			continue
		}
		all = append(all, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(only) == 0 {
		sort.Strings(all)
		return all, nil
	}
	var out []string
	for _, name := range only {
		if !have[name] {
			return nil, fmt.Errorf("database %q does not exist on the source", name)
		}
		if skippedDatabase(name) {
			return nil, fmt.Errorf("database %q is a system database and is not backed up", name)
		}
		out = append(out, name)
	}
	return out, nil
}

func showCreateDatabase(ctx context.Context, db *database, name string) (string, error) {
	var create string
	err := db.queryRow(ctx, "SELECT create_statement FROM [SHOW CREATE DATABASE "+quoteIdent(name)+"]").Scan(&create)
	if err != nil {
		return "", fmt.Errorf("SHOW CREATE DATABASE %s: %w", name, err)
	}
	return create, nil
}

func showCreateBundle(ctx context.Context, db *database, name string) ([]string, error) {
	lit := quoteLiteral(name)
	var out []string
	for _, q := range []string{
		"SELECT crdb_internal.show_create_all_schemas(" + lit + ")",
		"SELECT crdb_internal.show_create_all_types(" + lit + ")",
		"SELECT crdb_internal.show_create_all_tables(" + lit + ")",
	} {
		rows, err := db.query(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var stmt string
			if err := rows.Scan(&stmt); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, stmt)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

func listRelations(ctx context.Context, db *database) ([]tableRef, error) {
	rows, err := db.query(ctx, "SELECT schema_name, table_name, type FROM [SHOW TABLES]")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tableRef
	for rows.Next() {
		var r tableRef
		if err := rows.Scan(&r.Schema, &r.Name, &r.Type); err != nil {
			return nil, err
		}
		if skippedSchema(r.Schema) {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func readSequenceValues(ctx context.Context, db *database, asOf, database string, rels []tableRef) ([]SequenceValue, []string, error) {
	var out []SequenceValue
	var warnings []string
	for _, rel := range rels {
		if rel.Type != "sequence" {
			continue
		}
		var last *int64
		err := db.withSnapshot(ctx, asOf, func(ctx context.Context) error {
			q := "SELECT last_value FROM " + qualified(database, rel.Schema, rel.Name)
			return db.queryRow(ctx, q).Scan(&last)
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("sequence %s.%s.%s: could not read last_value: %s", database, rel.Schema, rel.Name, err.Error()))
			continue
		}
		if last == nil {
			continue
		}
		obj := qualified(database, rel.Schema, rel.Name)
		sql := fmt.Sprintf("SELECT setval(%s::REGCLASS, %d, true);", quoteLiteral(qualified(rel.Schema, rel.Name)), *last)
		out = append(out, SequenceValue{
			Database:  database,
			Object:    obj,
			LastValue: *last,
			IsCalled:  true,
			SQL:       sql,
		})
	}
	return out, warnings, nil
}

func readGrants(ctx context.Context, db *database, dbs []string) ([]string, []string) {
	var grants []string
	var warnings []string
	users, roles, memberships, err := readPrincipals(ctx, db)
	if err != nil {
		warnings = append(warnings, "users and roles were not read: "+err.Error())
	} else {
		for _, role := range roles {
			grants = append(grants, "CREATE ROLE IF NOT EXISTS "+quoteIdent(role)+";")
		}
		for _, user := range users {
			grants = append(grants, "CREATE USER IF NOT EXISTS "+quoteIdent(user)+";")
		}
		grants = append(grants, memberships...)
	}
	for _, database := range dbs {
		stmts, err := grantsForDatabase(ctx, db, database)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("grants for %s were not read: %s", database, err.Error()))
			continue
		}
		grants = append(grants, stmts...)
	}
	return grants, warnings
}

func readPrincipals(ctx context.Context, db *database) (users, roles, memberships []string, err error) {
	rows, err := db.query(ctx, `SELECT username, "isRole" FROM system.users ORDER BY username`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var isRole bool
		if err := rows.Scan(&name, &isRole); err != nil {
			return nil, nil, nil, err
		}
		if skippedPrincipal(name) {
			continue
		}
		if isRole {
			roles = append(roles, name)
		} else {
			users = append(users, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	mrows, err := db.query(ctx, `SELECT role, member, "isAdmin" FROM system.role_members`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var role, member string
		var isAdmin bool
		if err := mrows.Scan(&role, &member, &isAdmin); err != nil {
			return nil, nil, nil, err
		}
		if skippedPrincipal(role) && skippedPrincipal(member) {
			continue
		}
		if skippedPrincipal(member) && role == "admin" {
			continue
		}
		stmt := "GRANT " + quoteIdent(role) + " TO " + quoteIdent(member)
		if isAdmin {
			stmt += " WITH ADMIN OPTION"
		}
		memberships = append(memberships, stmt+";")
	}
	return users, roles, memberships, mrows.Err()
}

func grantsForDatabase(ctx context.Context, db *database, database string) ([]string, error) {
	var out []string
	appendGrants := func(q, object string) error {
		rows, err := db.query(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		fds := rows.FieldDescriptions()
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return err
			}
			rec := map[string]string{}
			for i, fd := range fds {
				if vals[i] == nil {
					continue
				}
				rec[string(fd.Name)] = fmt.Sprint(vals[i])
			}
			grantee := rec["grantee"]
			priv := rec["privilege_type"]
			if grantee == "" || priv == "" || skippedPrincipal(grantee) {
				continue
			}
			target := object
			if rec["schema_name"] != "" && rec["table_name"] != "" {
				target = "TABLE " + qualified(rec["database_name"], rec["schema_name"], rec["table_name"])
			} else if rec["schema_name"] != "" && rec["type_name"] != "" {
				target = "TYPE " + qualified(rec["database_name"], rec["schema_name"], rec["type_name"])
			} else if rec["schema_name"] != "" && rec["table_name"] == "" && rec["type_name"] == "" && object == "" {
				target = "SCHEMA " + qualified(rec["database_name"], rec["schema_name"])
			}
			if target == "" {
				continue
			}
			stmt := fmt.Sprintf("GRANT %s ON %s TO %s", priv, target, quoteIdent(grantee))
			switch rec["is_grantable"] {
			case "true", "t", "1":
				stmt += " WITH GRANT OPTION"
			}
			out = append(out, stmt+";")
		}
		return rows.Err()
	}
	if err := appendGrants("SHOW GRANTS ON DATABASE "+quoteIdent(database), "DATABASE "+quoteIdent(database)); err != nil {
		return nil, err
	}
	// Table, sequence, and view grants. SHOW GRANTS ON TABLE covers each relation we can see.
	if err := db.use(ctx, database); err != nil {
		return nil, err
	}
	rows, err := db.query(ctx, "SELECT schema_name, table_name, type FROM [SHOW TABLES]")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type rel struct{ schema, name, typ string }
	var rels []rel
	for rows.Next() {
		var r rel
		if err := rows.Scan(&r.schema, &r.name, &r.typ); err != nil {
			return nil, err
		}
		if skippedSchema(r.schema) {
			continue
		}
		rels = append(rels, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range rels {
		q := fmt.Sprintf("SHOW GRANTS ON TABLE %s", qualified(database, r.schema, r.name))
		if err := appendGrants(q, ""); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readZones(ctx context.Context, db *database, dbs []string) ([]zoneRow, error) {
	rows, err := db.query(ctx, `
SELECT target, database_name, schema_name, table_name, index_name, partition_name, raw_config_sql, full_config_sql
FROM crdb_internal.zones`)
	if err != nil {
		return nil, fmt.Errorf("read zone configurations: %w", err)
	}
	defer rows.Close()
	want := map[string]bool{}
	for _, d := range dbs {
		want[d] = true
	}
	var out []zoneRow
	for rows.Next() {
		var target, raw, full *string
		var database, schema, table, index, partition *string
		if err := rows.Scan(&target, &database, &schema, &table, &index, &partition, &raw, &full); err != nil {
			return nil, err
		}
		z := zoneRow{
			Object:    deref(target),
			Database:  deref(database),
			Schema:    deref(schema),
			Table:     deref(table),
			Index:     deref(index),
			Partition: deref(partition),
			RawSQL:    deref(raw),
			FullSQL:   deref(full),
		}
		if n, ok := parseGCTTL(z.FullSQL); ok {
			z.Effective = n
		}
		switch {
		case z.Database == "" && strings.EqualFold(z.Object, "RANGE default"):
			z.Level = "range"
			z.Object = "RANGE default"
		case z.Database != "" && !want[z.Database]:
			continue
		case z.Table == "" && z.Database != "":
			z.Level = "database"
			if z.Object == "" {
				z.Object = "DATABASE " + z.Database
			}
		case z.Index != "" || z.Partition != "":
			z.Level = "index"
			if z.Partition != "" {
				z.Level = "partition"
			}
		case z.Table != "":
			z.Level = "table"
		default:
			continue
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

func minEffectiveTTL(zones []zoneRow) (int, string) {
	min := 0
	obj := ""
	// Prefer table and database effective configs over the bare default,
	// but if a target has no zone, the default still applies.
	hasTarget := false
	for _, z := range zones {
		if z.Level == "table" || z.Level == "database" || z.Level == "index" || z.Level == "partition" {
			hasTarget = true
			break
		}
	}
	for _, z := range zones {
		if z.Effective <= 0 {
			continue
		}
		if hasTarget && z.Level == "range" {
			continue
		}
		if min == 0 || z.Effective < min {
			min = z.Effective
			obj = z.Object
			if obj == "" {
				obj = z.Level
			}
		}
	}
	if min == 0 {
		for _, z := range zones {
			if z.Level == "range" && z.Effective > 0 {
				return z.Effective, "RANGE default"
			}
		}
	}
	return min, obj
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
