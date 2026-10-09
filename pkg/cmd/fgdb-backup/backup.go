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
	"github.com/jackc/pgx/v5/pgconn"
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

type backupSource struct {
	version   string
	clusterID string
	name      string
	dbs       []string
	store     Store
	zones     []zoneRow
}

func runBackup(ctx context.Context, opt BackupOptions) (res BackupResult, err error) {
	opt, err = normalizeBackupOptions(opt)
	if err != nil {
		return res, err
	}
	db, err := connect(ctx, opt.URL)
	if err != nil {
		return res, err
	}
	defer db.Close(ctx)

	src, err := readBackupSource(ctx, db, opt)
	if err != nil {
		return res, err
	}
	// The backup replays the zones that were set before this run. A temporary
	// gc.ttlseconds raise is not part of the artifact.
	artifactZones := append([]zoneRow(nil), src.zones...)
	var revertSQL []string
	defer revertRaisedTTL(opt.URL, &revertSQL, &res, &err)

	src.zones, err = raiseGCTTL(ctx, db, opt, src.dbs, src.zones, &revertSQL, &res)
	if err != nil {
		return res, err
	}
	asOfText, budget, err := snapshotBudget(ctx, db, src.dbs, src.zones, opt.SafetyMargin)
	if err != nil {
		return res, err
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	base := src.name + "/" + ts
	setBackupHeader(&res, opt, src, asOfText, ts, base, budget.minTTL)

	objects := ObjectsFile{FormatVersion: formatVersion}
	var warnings []string
	var tables []TableEntry
	var totalRows int64
	work := &backupWork{
		db: db, opt: opt, src: src, asOfText: asOfText, base: base, budget: budget,
		objects: &objects, tables: &tables, warnings: &warnings, totalRows: &totalRows,
	}
	err = backupDatabases(ctx, work)
	if err != nil {
		return res, err
	}
	attachGrantsAndZones(ctx, db, src.dbs, artifactZones, &objects, &warnings)

	peak, err := writeBackupFiles(ctx, backupArtifact{
		src: src, opt: opt, base: base, ts: ts, asOfText: asOfText, minTTL: budget.minTTL,
		objects: objects, tables: tables, warnings: warnings,
	})
	if err != nil {
		return res, err
	}
	res.OK = true
	res.Latest = opt.Dest.String() + "/" + src.name + "/latest"
	res.Tables = len(tables)
	res.Rows = totalRows
	res.PeakRSSBytes = peak
	res.Warnings = warnings
	return res, nil
}

func normalizeBackupOptions(opt BackupOptions) (BackupOptions, error) {
	if opt.Compression == "" {
		opt.Compression = "gzip"
	}
	if opt.Compression != "gzip" && opt.Compression != "none" {
		return opt, fmt.Errorf("--compression must be gzip or none")
	}
	if opt.PartSize == 0 {
		opt.PartSize = 8 << 20
	}
	if opt.Dest.Kind == "s3" && opt.PartSize < minPartSize {
		return opt, fmt.Errorf("--part-size must be at least %d bytes for S3 multipart upload", minPartSize)
	}
	if opt.SafetyMargin <= 0 {
		opt.SafetyMargin = time.Minute
	}
	return opt, nil
}

func readBackupSource(ctx context.Context, db *database, opt BackupOptions) (backupSource, error) {
	var src backupSource
	var err error
	src.version, err = db.version(ctx)
	if err != nil {
		return src, err
	}
	src.clusterID, err = db.clusterID(ctx)
	if err != nil {
		return src, fmt.Errorf("read cluster id: %w", err)
	}
	src.name = opt.Name
	if src.name == "" {
		src.name = src.clusterID
	}
	if err = safeSegment(src.name); err != nil {
		return src, err
	}
	src.dbs, err = listDatabases(ctx, db, opt.Databases)
	if err != nil {
		return src, err
	}
	if len(src.dbs) == 0 {
		return src, fmt.Errorf("no user databases to back up")
	}
	src.store, err = openStore(ctx, opt.Dest)
	if err != nil {
		return src, err
	}
	if s3s, ok := src.store.(*s3Store); ok {
		s3s.partSize = opt.PartSize
	}
	src.zones, err = readZones(ctx, db, src.dbs)
	return src, err
}

func revertRaisedTTL(url string, revertSQL *[]string, res *BackupResult, err *error) {
	if len(*revertSQL) == 0 {
		return
	}
	// The backup connection is often already closed: cancelling COPY on
	// SIGINT/SIGTERM drops it. A new session still sees the raised zone
	// config and can put the old value back.
	ctx := context.Background()
	db, e := connect(ctx, url)
	if e != nil {
		noteTTLRevertFailure(e, strings.Join(*revertSQL, "\n"), res, err)
		return
	}
	defer db.Close(ctx)
	logf("restoring gc.ttlseconds")
	for _, sql := range *revertSQL {
		revertOneTTL(db, sql, res, err)
	}
}

func revertOneTTL(db *database, sql string, res *BackupResult, err *error) {
	if _, e := db.exec(context.Background(), sql); e != nil {
		logf("warning: could not restore gc.ttlseconds: %s", e)
		noteTTLRevertFailure(e, sql, res, err)
	}
}

func noteTTLRevertFailure(e error, sql string, res *BackupResult, err *error) {
	msg := fmt.Errorf("could not restore gc.ttlseconds: %w\nRun: %s", e, sql)
	if *err != nil {
		*err = fmt.Errorf("%w; %v", *err, msg)
	} else {
		*err = fmt.Errorf("backup files were written, but %w", msg)
	}
	res.OK = false
	res.Error = (*err).Error()
}

func raiseGCTTL(ctx context.Context, db *database, opt BackupOptions, dbs []string, zones []zoneRow, revertSQL *[]string, res *BackupResult) ([]zoneRow, error) {
	extends := planGCTTLRaises(int(opt.ExtendGCTTL/time.Second), dbs, zones)
	if len(extends) == 0 {
		return zones, nil
	}
	logf("raising gc.ttlseconds for this run")
	for _, ch := range extends {
		if err := applyTTLChange(ctx, db, ch, revertSQL, res); err != nil {
			return nil, err
		}
	}
	logRevertSQL(*revertSQL)
	return readZones(ctx, db, dbs)
}

func applyTTLChange(ctx context.Context, db *database, ch gcChange, revertSQL *[]string, res *BackupResult) error {
	logf("  %s", ch.Apply)
	if _, err := db.exec(ctx, ch.Apply); err != nil {
		return fmt.Errorf("raise gc.ttlseconds on %s: %w", ch.Object, err)
	}
	stmt := ensureSemicolon(ch.Revert)
	*revertSQL = append(*revertSQL, stmt)
	res.RevertSQL = append(res.RevertSQL, stmt)
	return nil
}

func logRevertSQL(revertSQL []string) {
	logf("if this process is killed, put the old values back with:")
	for _, sql := range revertSQL {
		logf("  %s", sql)
	}
}

func snapshotBudget(ctx context.Context, db *database, databases []string, zones []zoneRow, margin time.Duration) (string, gcBudget, error) {
	asOfText, err := db.captureAsOf(ctx)
	if err != nil {
		return "", gcBudget{}, err
	}
	asOf, err := parseAsOf(asOfText)
	if err != nil {
		return "", gcBudget{}, fmt.Errorf("timestamp %q from the source: %w", asOfText, err)
	}
	minTTL, minObj := minEffectiveTTL(databases, zones)
	if minTTL <= 0 {
		return "", gcBudget{}, fmt.Errorf("could not read gc.ttlseconds for the databases being backed up")
	}
	budget := newBudget(asOf, minTTL, minObj, margin)
	if budget.exceeded(time.Now()) {
		return "", gcBudget{}, fmt.Errorf("%s", budget.message())
	}
	logf("snapshot %s is readable until %s (gc.ttlseconds=%d on %s)", asOfText, budget.deadline.Format(time.RFC3339), minTTL, minObj)
	return asOfText, budget, nil
}

func setBackupHeader(res *BackupResult, opt BackupOptions, src backupSource, asOfText, ts, base string, minTTL int) {
	res.Backup = opt.Dest.String() + "/" + base
	res.Name = src.name
	res.Timestamp = ts
	res.AsOf = asOfText
	res.SourceVersion = src.version
	res.ClusterID = src.clusterID
	res.Databases = src.dbs
	res.GCTTLSeconds = minTTL
}

// backupWork is the mutable state for copying one snapshot.
type backupWork struct {
	db        *database
	opt       BackupOptions
	src       backupSource
	asOfText  string
	base      string
	budget    gcBudget
	objects   *ObjectsFile
	tables    *[]TableEntry
	warnings  *[]string
	totalRows *int64
}

func backupDatabases(ctx context.Context, work *backupWork) error {
	for _, database := range work.src.dbs {
		err := backupOneDatabase(ctx, work, database)
		if err != nil {
			return err
		}
	}
	return nil
}

func backupOneDatabase(ctx context.Context, work *backupWork, database string) error {
	if err := safeSegment(database); err != nil {
		return err
	}
	if err := appendDatabaseDDL(ctx, work.db, database, work.asOfText, work.budget, work.objects); err != nil {
		return err
	}
	idents, err := readIdentity(ctx, work.db, database, work.asOfText)
	if err != nil {
		return err
	}
	if err := rejectAlwaysIdentity(idents); err != nil {
		return err
	}
	omitOwnedSequences(work.objects, database, ownedSequences(idents))
	rels, err := relationsAt(ctx, work.db, database, work.asOfText)
	if err != nil {
		return fmt.Errorf("list tables in %s: %w", database, err)
	}
	seqVals, seqWarn, err := readSequenceValues(ctx, work.db, work.asOfText, database, rels)
	if err != nil {
		return err
	}
	*work.warnings = append(*work.warnings, seqWarn...)
	work.objects.SequenceValues = append(work.objects.SequenceValues, seqVals...)
	return dumpDatabaseTables(ctx, work, database, rels)
}

func appendDatabaseDDL(ctx context.Context, db *database, database, asOfText string, budget gcBudget, objects *ObjectsFile) error {
	createDB, err := showCreateDatabase(ctx, db, database)
	if err != nil {
		return err
	}
	objects.Databases = append(objects.Databases, NamedSQL{Name: database, SQL: ensureSemicolon(createDB)})
	stmts, err := schemaAt(ctx, db, database, asOfText)
	if err != nil {
		return schemaReadError(database, budget, err)
	}
	appendClassified(objects, database, stmts)
	return nil
}

func schemaReadError(database string, budget gcBudget, err error) error {
	if isGCError(err) {
		return fmt.Errorf("%s\n\nsource error: %w", budget.message(), err)
	}
	return fmt.Errorf("read schema for %s: %w", database, err)
}

func schemaAt(ctx context.Context, db *database, database, asOfText string) ([]string, error) {
	var stmts []string
	err := db.withSnapshot(ctx, asOfText, func(ctx context.Context) error {
		var qerr error
		stmts, qerr = showCreateBundle(ctx, db, database)
		return qerr
	})
	return stmts, err
}

func appendClassified(objects *ObjectsFile, database string, stmts []string) {
	for _, sql := range stmts {
		sql = stripMaterializedRowid(sql)
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
}

func relationsAt(ctx context.Context, db *database, database, asOfText string) ([]tableRef, error) {
	var rels []tableRef
	err := db.withSnapshot(ctx, asOfText, func(ctx context.Context) error {
		if err := db.use(ctx, database); err != nil {
			return err
		}
		var qerr error
		rels, qerr = listRelations(ctx, db)
		return qerr
	})
	return rels, err
}

func dumpDatabaseTables(ctx context.Context, work *backupWork, database string, rels []tableRef) error {
	for _, rel := range rels {
		if rel.Type != "table" {
			continue
		}
		entry, err := copyOneTable(ctx, work, database, rel)
		if err != nil {
			return err
		}
		*work.tables = append(*work.tables, entry)
		*work.totalRows += entry.RowCount
		logf("copied %s.%s.%s (%d rows)", database, rel.Schema, rel.Name, entry.RowCount)
	}
	return nil
}

func copyOneTable(ctx context.Context, work *backupWork, database string, rel tableRef) (TableEntry, error) {
	if err := budgetCheck(work.budget); err != nil {
		return TableEntry{}, err
	}
	entry, err := dumpTable(ctx, work.db, work.src.store, dumpSpec{
		asOf:        work.asOfText,
		base:        work.base,
		database:    database,
		schema:      rel.Schema,
		table:       rel.Name,
		compression: work.opt.Compression,
		splitRows:   work.opt.SplitRows,
		exceeded:    func() error { return budgetStillOpen(work.budget) },
	})
	if err != nil {
		return TableEntry{}, copyTableError(work.budget, err)
	}
	return entry, nil
}

func budgetStillOpen(budget gcBudget) error {
	if budget.exceeded(time.Now()) || isPast(budget) {
		return fmt.Errorf("%s", budget.message())
	}
	return nil
}

func copyTableError(budget gcBudget, err error) error {
	if isGCError(err) {
		return fmt.Errorf("%s\n\nsource error: %w", budget.message(), err)
	}
	return err
}

func attachGrantsAndZones(ctx context.Context, db *database, dbs []string, artifactZones []zoneRow, objects *ObjectsFile, warnings *[]string) {
	grantSQL, grantWarn := readGrants(ctx, db, dbs)
	*warnings = append(*warnings, grantWarn...)
	objects.Grants = grantSQL
	for _, z := range artifactZones {
		if z.Level == "range" || strings.TrimSpace(z.RawSQL) == "" {
			continue
		}
		objects.Zones = append(objects.Zones, ZoneStatement{
			Object:   z.Object,
			Database: z.Database,
			Level:    z.Level,
			SQL:      ensureSemicolon(z.RawSQL),
		})
	}
}

// backupArtifact is one finished snapshot ready to write.
type backupArtifact struct {
	src      backupSource
	opt      BackupOptions
	base     string
	ts       string
	asOfText string
	minTTL   int
	objects  ObjectsFile
	tables   []TableEntry
	warnings []string
}

// manifestParts is the files and checksums stored in manifest.json.
type manifestParts struct {
	objects FileDigest
	schema  []FileDigest
	users   FileDigest
	zones   FileDigest
	peak    int64
}

func writeBackupFiles(ctx context.Context, art backupArtifact) (int64, error) {
	schemaDigests, err := writeSchemaFiles(ctx, art.src.store, art.base, art.src.dbs, art.objects.Statements)
	if err != nil {
		return 0, err
	}
	usersDig, err := writeSQLLines(ctx, art.src.store, art.base+"/users.sql", art.objects.Grants, "%s\n")
	if err != nil {
		return 0, err
	}
	zonesDig, err := writeZoneFile(ctx, art.src.store, art.base+"/zones.sql", art.objects.Zones)
	if err != nil {
		return 0, err
	}
	objDig, err := writeJSONFile(ctx, art.src.store, art.base+"/objects.json", art.objects)
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	parts := manifestParts{objects: objDig, schema: schemaDigests, users: usersDig, zones: zonesDig, peak: peakRSSBytes()}
	manifest := newManifest(art, parts)
	if _, err = writeJSONFile(ctx, art.src.store, art.base+"/manifest.json", manifest); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	pointer := LatestPointer{FormatVersion: formatVersion, Name: art.src.name, Timestamp: art.ts, Complete: true}
	if err = publishLatest(ctx, art.src.store, art.src.name+"/latest.json", pointer); err != nil {
		return 0, err
	}
	return parts.peak, nil
}

func writeSchemaFiles(ctx context.Context, store Store, base string, dbs []string, statements []Statement) ([]FileDigest, error) {
	files := schemaBuffers(statements)
	var digests []FileDigest
	for _, database := range dbs {
		buf := files[database]
		if buf == nil {
			continue
		}
		dig, err := writeBytes(ctx, store, base+"/schema/"+database+".sql", buf.Bytes())
		if err != nil {
			return nil, err
		}
		digests = append(digests, dig)
	}
	return digests, nil
}

func schemaBuffers(statements []Statement) map[string]*bytes.Buffer {
	files := map[string]*bytes.Buffer{}
	for _, st := range statements {
		buf := files[st.Database]
		if buf == nil {
			buf = &bytes.Buffer{}
			files[st.Database] = buf
		}
		fmt.Fprintf(buf, "-- kind: %s object: %s\n%s\n\n", st.Kind, st.Object, st.SQL)
	}
	return files
}

func writeSQLLines(ctx context.Context, store Store, rel string, lines []string, format string) (FileDigest, error) {
	buf := &bytes.Buffer{}
	for _, line := range lines {
		fmt.Fprintf(buf, format, ensureSemicolon(line))
	}
	return writeBytes(ctx, store, rel, buf.Bytes())
}

func writeZoneFile(ctx context.Context, store Store, rel string, zones []ZoneStatement) (FileDigest, error) {
	buf := &bytes.Buffer{}
	for _, z := range zones {
		fmt.Fprintf(buf, "-- level: %s object: %s\n%s\n\n", z.Level, z.Object, ensureSemicolon(z.SQL))
	}
	return writeBytes(ctx, store, rel, buf.Bytes())
}

func writeJSONFile(ctx context.Context, store Store, rel string, v any) (FileDigest, error) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return FileDigest{}, err
	}
	body = append(body, '\n')
	return writeBytes(ctx, store, rel, body)
}

func newManifest(art backupArtifact, parts manifestParts) Manifest {
	users := parts.users
	zones := parts.zones
	return Manifest{
		FormatVersion: formatVersion,
		Tool:          toolName,
		ToolVersion:   toolVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		SourceVersion: art.src.version,
		ClusterID:     art.src.clusterID,
		Name:          art.src.name,
		AsOf:          art.asOfText,
		GCTTLSeconds:  art.minTTL,
		Compression:   art.opt.Compression,
		DataFormat:    dataFormatPGCopy,
		Databases:     art.src.dbs,
		ObjectsFile:   parts.objects,
		SchemaFiles:   parts.schema,
		UsersFile:     &users,
		ZonesFile:     &zones,
		Tables:        art.tables,
		Warnings:      art.warnings,
		PeakRSSBytes:  parts.peak,
	}
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
	arrays, err := arraySQLColumns(cols)
	if err != nil {
		return entry, fmt.Errorf("copy %s.%s.%s: %w", spec.database, spec.schema, spec.table, err)
	}
	entry = TableEntry{
		Database: spec.database,
		Schema:   spec.schema,
		Name:     spec.table,
		Columns:  columnNames(cols),
		ArraySQL: arrays,
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

	ext := ".pgcopy"
	if spec.compression == "gzip" {
		ext = ".pgcopy.gz"
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
		list, err := selectList(cols)
		if err != nil {
			return entry, fmt.Errorf("copy %s.%s.%s: %w", spec.database, spec.schema, spec.table, err)
		}
		dig, rows, err := copyRelation(ctx, relationCopy{
			db: db, store: store, spec: spec, rel: rel, cols: list, where: where, order: order,
		})
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
	prev := ""
	table := qualified(spec.database, spec.schema, spec.table)
	for {
		var value *string
		err := db.withSnapshot(ctx, spec.asOf, func(ctx context.Context) error {
			return db.queryRow(ctx, splitBoundQuery(table, pk, prev, spec.splitRows)).Scan(&value)
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
		prev = *value
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

// relationCopy is one COPY TO STDOUT of a table or primary-key range.
type relationCopy struct {
	db    *database
	store Store
	spec  dumpSpec
	rel   string
	cols  string
	where string
	order string
}

func copyRelation(ctx context.Context, job relationCopy) (FileDigest, int64, error) {
	var dig FileDigest
	var rows int64
	err := job.db.withSnapshot(ctx, job.spec.asOf, func(ctx context.Context) error {
		var cerr error
		dig, rows, cerr = streamRelation(ctx, job)
		return cerr
	})
	return dig, rows, err
}

func streamRelation(ctx context.Context, job relationCopy) (FileDigest, int64, error) {
	wc, err := job.store.Create(ctx, job.rel)
	if err != nil {
		return FileDigest{}, 0, err
	}
	hw := newHashWriteCloser(wc)
	sink, gz := maybeGzip(hw, job.spec.compression)
	counter := newLineCounter(deadlineWriter{w: sink, exceeded: job.spec.exceeded})
	cerr := job.db.copyTo(ctx, counter, copyOutSQL(job))
	rows := counter.Rows()
	dig, err := finishCopy(gz, hw, cerr)
	if err != nil {
		return FileDigest{}, rows, err
	}
	dig.Path = dataFilePath(job.rel)
	return dig, rows, nil
}

func maybeGzip(hw io.Writer, compression string) (io.Writer, *gzip.Writer) {
	if compression != "gzip" {
		return hw, nil
	}
	gz := gzip.NewWriter(hw)
	return gz, gz
}

func copyOutSQL(job relationCopy) string {
	q := fmt.Sprintf("SELECT %s FROM %s", job.cols, qualified(job.spec.database, job.spec.schema, job.spec.table))
	if job.where != "" {
		q += " WHERE " + job.where
	}
	q += job.order
	return "COPY (" + q + ") TO STDOUT"
}

// finishCopy publishes the file only after COPY succeeds and the checksum
// covers the bytes, including the gzip trailer. A failed COPY aborts the
// temporary object instead of leaving it under the final name.
func finishCopy(gz *gzip.Writer, hw *hashWriteCloser, copyErr error) (FileDigest, error) {
	if copyErr != nil {
		_ = hw.Abort()
		return FileDigest{}, copyErr
	}
	if gz != nil {
		if err := gz.Close(); err != nil {
			_ = hw.Abort()
			return FileDigest{}, err
		}
	}
	dig := hw.digest()
	if err := hw.Close(); err != nil {
		return FileDigest{}, err
	}
	return dig, nil
}

func dataFilePath(rel string) string {
	path := rel[strings.Index(rel, "/data/")+1:]
	// Path in the manifest is relative to the timestamp directory.
	if i := strings.Index(rel, "/data/"); i >= 0 {
		path = rel[i+1:]
	}
	return path
}

func writeBytes(ctx context.Context, store Store, rel string, body []byte) (FileDigest, error) {
	wc, err := store.Create(ctx, rel)
	if err != nil {
		return FileDigest{}, err
	}
	hw := newHashWriteCloser(wc)
	if _, err := hw.Write(body); err != nil {
		_ = hw.Abort()
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
	// create_function_statements and create_procedure_statements only list the
	// current database. show_create_all_* take the name, but routines do not.
	if err := db.use(ctx, name); err != nil {
		return nil, err
	}
	lit := quoteLiteral(name)
	var out []string
	for _, q := range []string{
		"SELECT crdb_internal.show_create_all_schemas(" + lit + ")",
		"SELECT crdb_internal.show_create_all_types(" + lit + ")",
		"SELECT crdb_internal.show_create_all_tables(" + lit + ")",
		"SELECT create_statement FROM crdb_internal.create_function_statements WHERE database_name = " + lit + " AND schema_name NOT IN ('pg_catalog', 'information_schema', 'crdb_internal', 'pg_extension')",
		"SELECT create_statement FROM crdb_internal.create_procedure_statements WHERE database_name = " + lit + " AND schema_name NOT IN ('pg_catalog', 'information_schema', 'crdb_internal', 'pg_extension')",
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
	want := map[string]tableRef{}
	for _, rel := range rels {
		if rel.Type == "sequence" {
			want[rel.Schema+"."+rel.Name] = rel
		}
	}
	if len(want) == 0 {
		return nil, nil, nil
	}
	var out []SequenceValue
	err := db.withSnapshot(ctx, asOf, func(ctx context.Context) error {
		if err := db.use(ctx, database); err != nil {
			return err
		}
		rows, err := db.query(ctx, `SELECT schemaname, sequencename, start_value, last_value FROM pg_catalog.pg_sequences`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			seq, ok, err := scanSequenceValue(rows, database, want)
			if err != nil {
				return err
			}
			if ok {
				out = append(out, seq)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read sequences in %s: %w", database, err)
	}
	return out, nil, nil
}

func scanSequenceValue(rows pgx.Rows, database string, want map[string]tableRef) (SequenceValue, bool, error) {
	var schema, name string
	var start int64
	var last *int64
	if err := rows.Scan(&schema, &name, &start, &last); err != nil {
		return SequenceValue{}, false, err
	}
	if _, ok := want[schema+"."+name]; !ok {
		return SequenceValue{}, false, nil
	}
	sql, called := sequenceRestoreSQL(schema, name, start, last)
	value := start
	if last != nil {
		value = *last
	}
	return SequenceValue{
		Database:  database,
		Object:    qualified(database, schema, name),
		LastValue: value,
		IsCalled:  called,
		SQL:       sql,
	}, true, nil
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
	users, roles, err = readUserNames(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	memberships, err = readMemberships(ctx, db)
	return users, roles, memberships, err
}

func readUserNames(ctx context.Context, db *database) (users, roles []string, err error) {
	rows, err := db.query(ctx, `SELECT username, "isRole" FROM system.users ORDER BY username`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		name, isRole, keep, scanErr := scanPrincipal(rows)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		if keep {
			users, roles = appendPrincipal(users, roles, name, isRole)
		}
	}
	return users, roles, rows.Err()
}

func scanPrincipal(rows pgx.Rows) (name string, isRole, keep bool, err error) {
	if err = rows.Scan(&name, &isRole); err != nil {
		return "", false, false, err
	}
	if skippedPrincipal(name) {
		return "", false, false, nil
	}
	return name, isRole, true, nil
}

func appendPrincipal(users, roles []string, name string, isRole bool) ([]string, []string) {
	if isRole {
		return users, append(roles, name)
	}
	return append(users, name), roles
}

func readMemberships(ctx context.Context, db *database) ([]string, error) {
	rows, err := db.query(ctx, `SELECT role, member, "isAdmin" FROM system.role_members`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memberships []string
	for rows.Next() {
		stmt, ok, scanErr := scanMembership(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if ok {
			memberships = append(memberships, stmt)
		}
	}
	return memberships, rows.Err()
}

func scanMembership(rows pgx.Rows) (string, bool, error) {
	var role, member string
	var isAdmin bool
	if err := rows.Scan(&role, &member, &isAdmin); err != nil {
		return "", false, err
	}
	if skipMembership(role, member) {
		return "", false, nil
	}
	stmt := "GRANT " + quoteIdent(role) + " TO " + quoteIdent(member)
	if isAdmin {
		stmt += " WITH ADMIN OPTION"
	}
	return stmt + ";", true, nil
}

func skipMembership(role, member string) bool {
	if skippedPrincipal(role) && skippedPrincipal(member) {
		return true
	}
	return skippedPrincipal(member) && role == "admin"
}

func grantsForDatabase(ctx context.Context, db *database, database string) ([]string, error) {
	out, err := readGrantQuery(ctx, db, "SHOW GRANTS ON DATABASE "+quoteIdent(database), "DATABASE "+quoteIdent(database))
	if err != nil {
		return nil, err
	}
	rels, err := grantRelations(ctx, db, database)
	if err != nil {
		return nil, err
	}
	return appendTableGrants(ctx, db, database, rels, out)
}

func appendTableGrants(ctx context.Context, db *database, database string, rels []tableRef, out []string) ([]string, error) {
	for _, r := range rels {
		q := fmt.Sprintf("SHOW GRANTS ON TABLE %s", qualified(database, r.Schema, r.Name))
		more, err := readGrantQuery(ctx, db, q, "")
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

func readGrantQuery(ctx context.Context, db *database, q, object string) ([]string, error) {
	rows, err := db.query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		stmt, ok, rowErr := nextGrant(rows, object)
		if rowErr != nil {
			return nil, rowErr
		}
		if ok {
			out = append(out, stmt)
		}
	}
	return out, rows.Err()
}

func nextGrant(rows pgx.Rows, object string) (string, bool, error) {
	vals, err := rows.Values()
	if err != nil {
		return "", false, err
	}
	rec := grantFields(rows.FieldDescriptions(), vals)
	stmt, ok := formatGrant(rec, object)
	return stmt, ok, nil
}

func grantFields(fds []pgconn.FieldDescription, vals []any) map[string]string {
	rec := map[string]string{}
	for i, fd := range fds {
		if vals[i] == nil {
			continue
		}
		rec[string(fd.Name)] = fmt.Sprint(vals[i])
	}
	return rec
}

func formatGrant(rec map[string]string, object string) (string, bool) {
	grantee := rec["grantee"]
	priv := rec["privilege_type"]
	if grantee == "" || priv == "" || skippedPrincipal(grantee) {
		return "", false
	}
	target := grantTarget(rec, object)
	if target == "" {
		return "", false
	}
	stmt := fmt.Sprintf("GRANT %s ON %s TO %s", priv, target, quoteIdent(grantee))
	if grantable(rec["is_grantable"]) {
		stmt += " WITH GRANT OPTION"
	}
	return stmt + ";", true
}

func grantTarget(rec map[string]string, object string) string {
	if rec["schema_name"] != "" && rec["table_name"] != "" {
		return "TABLE " + qualified(rec["database_name"], rec["schema_name"], rec["table_name"])
	}
	if rec["schema_name"] != "" && rec["type_name"] != "" {
		return "TYPE " + qualified(rec["database_name"], rec["schema_name"], rec["type_name"])
	}
	if schemaOnlyGrant(rec, object) {
		return "SCHEMA " + qualified(rec["database_name"], rec["schema_name"])
	}
	return object
}

func schemaOnlyGrant(rec map[string]string, object string) bool {
	return rec["schema_name"] != "" && rec["table_name"] == "" && rec["type_name"] == "" && object == ""
}

func grantable(v string) bool {
	switch v {
	case "true", "t", "1":
		return true
	default:
		return false
	}
}

func grantRelations(ctx context.Context, db *database, database string) ([]tableRef, error) {
	if err := db.use(ctx, database); err != nil {
		return nil, err
	}
	rows, err := db.query(ctx, "SELECT schema_name, table_name, type FROM [SHOW TABLES]")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectGrantRelations(rows)
}

func collectGrantRelations(rows pgx.Rows) ([]tableRef, error) {
	var rels []tableRef
	for rows.Next() {
		var r tableRef
		if err := rows.Scan(&r.Schema, &r.Name, &r.Type); err != nil {
			return nil, err
		}
		if skippedSchema(r.Schema) {
			continue
		}
		rels = append(rels, r)
	}
	return rels, rows.Err()
}

func readZones(ctx context.Context, db *database, dbs []string) ([]zoneRow, error) {
	rows, err := db.query(ctx, `
SELECT target, database_name, schema_name, table_name, index_name, partition_name, raw_config_sql, full_config_sql
FROM crdb_internal.zones`)
	if err != nil {
		return nil, fmt.Errorf("read zone configurations: %w", err)
	}
	defer rows.Close()
	return collectZones(rows, databaseSet(dbs))
}

func databaseSet(dbs []string) map[string]bool {
	want := map[string]bool{}
	for _, d := range dbs {
		want[d] = true
	}
	return want
}

func collectZones(rows pgx.Rows, want map[string]bool) ([]zoneRow, error) {
	var out []zoneRow
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, err
		}
		if keepZone(&z, want) {
			out = append(out, z)
		}
	}
	return out, rows.Err()
}

func scanZone(rows pgx.Rows) (zoneRow, error) {
	var target, raw, full *string
	var database, schema, table, index, partition *string
	err := rows.Scan(&target, &database, &schema, &table, &index, &partition, &raw, &full)
	if err != nil {
		return zoneRow{}, err
	}
	return zoneRow{
		Object:    deref(target),
		Database:  deref(database),
		Schema:    deref(schema),
		Table:     deref(table),
		Index:     deref(index),
		Partition: deref(partition),
		RawSQL:    deref(raw),
		FullSQL:   deref(full),
	}, nil
}

func keepZone(z *zoneRow, want map[string]bool) bool {
	if n, ok := parseGCTTL(z.FullSQL); ok {
		z.Effective = n
	}
	switch zoneKind(z, want) {
	case "range":
		z.Level = "range"
		z.Object = rangeDefaultZone
		return true
	case "skip":
		return false
	case "database":
		markDatabaseZone(z)
		return true
	case "index":
		markIndexZone(z)
		return true
	case "table":
		z.Level = "table"
		return true
	default:
		return false
	}
}

func zoneKind(z *zoneRow, want map[string]bool) string {
	switch {
	case z.Database == "" && strings.EqualFold(z.Object, rangeDefaultZone):
		return "range"
	case z.Database != "" && !want[z.Database]:
		return "skip"
	case z.Table == "" && z.Database != "":
		return "database"
	case z.Index != "" || z.Partition != "":
		return "index"
	case z.Table != "":
		return "table"
	default:
		return "skip"
	}
}

func markDatabaseZone(z *zoneRow) {
	z.Level = "database"
	if z.Object == "" {
		z.Object = "DATABASE " + z.Database
	}
}

func markIndexZone(z *zoneRow) {
	z.Level = "index"
	if z.Partition != "" {
		z.Level = "partition"
	}
}

func minEffectiveTTL(databases []string, zones []zoneRow) (int, string) {
	// A database without its own zone inherits the range default. Keep that
	// default in the calculation even when another database or table has an
	// explicit zone.
	dbZones := map[string]bool{}
	for _, z := range zones {
		if z.Level == "database" {
			dbZones[z.Database] = true
		}
	}
	hasInheritedDatabase := false
	for _, database := range databases {
		if !dbZones[database] {
			hasInheritedDatabase = true
			break
		}
	}
	min, obj := lowestTTL(zones, !hasInheritedDatabase)
	if min == 0 {
		return rangeDefaultTTL(zones)
	}
	return min, obj
}

func isTargetLevel(level string) bool {
	switch level {
	case "table", "database", "index", "partition":
		return true
	default:
		return false
	}
}

func lowestTTL(zones []zoneRow, hasTarget bool) (int, string) {
	min := 0
	obj := ""
	for _, z := range zones {
		if !ttlCandidate(z, hasTarget) {
			continue
		}
		if min != 0 && z.Effective >= min {
			continue
		}
		min = z.Effective
		obj = z.Object
		if obj == "" {
			obj = z.Level
		}
	}
	return min, obj
}

func ttlCandidate(z zoneRow, hasTarget bool) bool {
	if z.Effective <= 0 {
		return false
	}
	if hasTarget && z.Level == "range" {
		return false
	}
	return true
}

func rangeDefaultTTL(zones []zoneRow) (int, string) {
	for _, z := range zones {
		if z.Level == "range" && z.Effective > 0 {
			return z.Effective, rangeDefaultZone
		}
	}
	return 0, ""
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
