// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// RestoreOptions is the restore command.
type RestoreOptions struct {
	URL          string
	Src          string
	Location     Location
	Databases    []string
	Force        bool
	Load         string
	ImportListen string
	JSON         bool
}

// Problem is one object that could not be restored. These are never skipped silently.
type Problem struct {
	Object string `json:"object"`
	Kind   string `json:"kind"`
	SQL    string `json:"sql,omitempty"`
	Error  string `json:"error"`
}

// RestoreResult is the machine-readable restore summary.
type RestoreResult struct {
	OK            bool      `json:"ok"`
	Error         string    `json:"error,omitempty"`
	Backup        string    `json:"backup,omitempty"`
	AsOf          string    `json:"as_of,omitempty"`
	SourceVersion string    `json:"source_version,omitempty"`
	Tables        int       `json:"tables,omitempty"`
	Rows          int64     `json:"rows,omitempty"`
	Incompatible  []Problem `json:"incompatible,omitempty"`
	Warnings      []string  `json:"warnings,omitempty"`
}

type restoreBundle struct {
	root     Location
	store    Store
	base     string
	manifest Manifest
	objects  ObjectsFile
}

func runRestore(ctx context.Context, opt RestoreOptions) (RestoreResult, error) {
	var res RestoreResult
	opt, err := normalizeRestoreOptions(opt)
	if err != nil {
		return res, err
	}
	bundle, err := openRestoreBundle(ctx, opt)
	if err != nil {
		return res, err
	}
	res.Backup = bundle.root.String() + "/" + bundle.base
	res.AsOf = bundle.manifest.AsOf
	res.SourceVersion = bundle.manifest.SourceVersion

	selected, err := selectDatabases(bundle.manifest, opt.Databases)
	if err != nil {
		return res, err
	}
	db, err := connect(ctx, opt.URL)
	if err != nil {
		return res, err
	}
	defer db.Close(ctx)

	if stopped, err := restoreSchema(ctx, db, bundle.objects, selected, &res); stopped {
		return res, err
	}
	if err := prepareTargets(ctx, db, bundle.manifest, selected, opt.Force); err != nil {
		return res, err
	}
	warnings, err := loadSelected(ctx, db, bundle, selected, opt)
	if err != nil {
		return res, err
	}
	res.Warnings = append(res.Warnings, warnings...)
	return finishRestore(ctx, db, bundle, selected, &res)
}

func normalizeRestoreOptions(opt RestoreOptions) (RestoreOptions, error) {
	if opt.Load == "" {
		opt.Load = "import"
	}
	if opt.Load != "import" && opt.Load != "copy" {
		return opt, fmt.Errorf("--load must be import or copy")
	}
	if opt.ImportListen == "" {
		opt.ImportListen = "127.0.0.1:0"
	}
	return opt, nil
}

func openRestoreBundle(ctx context.Context, opt RestoreOptions) (restoreBundle, error) {
	var bundle restoreBundle
	root, rel, err := resolveBackup(ctx, opt.Location, opt.Src)
	if err != nil {
		return bundle, err
	}
	store, base, err := openBackup(ctx, root, rel)
	if err != nil {
		return bundle, err
	}
	manifest, objects, err := readBackup(ctx, store, base)
	if err != nil {
		return bundle, err
	}
	if _, _, err := verifyBackup(ctx, store, base, manifest); err != nil {
		return bundle, err
	}
	bundle.root = root
	bundle.store = store
	bundle.base = base
	bundle.manifest = manifest
	bundle.objects = objects
	return bundle, nil
}

func selectDatabases(manifest Manifest, only []string) (map[string]bool, error) {
	selected := map[string]bool{}
	if len(only) == 0 {
		for _, d := range manifest.Databases {
			selected[d] = true
		}
		return selected, nil
	}
	have := map[string]bool{}
	for _, d := range manifest.Databases {
		have[d] = true
	}
	for _, d := range only {
		if !have[d] {
			return nil, fmt.Errorf("backup does not contain database %q", d)
		}
		selected[d] = true
	}
	return selected, nil
}

func restoreSchema(ctx context.Context, db *database, objects ObjectsFile, selected map[string]bool, res *RestoreResult) (bool, error) {
	problems := createSelectedDatabases(ctx, db, objects.Databases, selected)
	if len(problems) > 0 {
		return stopIncompatible(res, problems)
	}
	pre := []string{"schema", "type", "sequence", "table", "index", "view", "other"}
	problems = applyStatements(ctx, db, objects.Statements, selected, pre, true)
	if len(problems) > 0 {
		return stopIncompatible(res, problems)
	}
	return false, nil
}

func stopIncompatible(res *RestoreResult, problems []Problem) (bool, error) {
	res.Incompatible = problems
	return true, incompatibleError(problems)
}

func createSelectedDatabases(ctx context.Context, db *database, databases []NamedSQL, selected map[string]bool) []Problem {
	var problems []Problem
	for _, database := range databases {
		if !selected[database.Name] {
			continue
		}
		if p, ok := createDatabase(ctx, db, database); !ok {
			problems = append(problems, p)
		}
	}
	return problems
}

func createDatabase(ctx context.Context, db *database, database NamedSQL) (Problem, bool) {
	if _, err := db.exec(ctx, database.SQL); err != nil && !isDuplicate(err) {
		return Problem{Object: database.Name, Kind: "database", SQL: database.SQL, Error: err.Error()}, false
	}
	return Problem{}, true
}

func applyStatements(ctx context.Context, db *database, stmts []Statement, selected map[string]bool, kinds []string, ignoreDup bool) []Problem {
	var problems []Problem
	for _, kind := range kinds {
		problems = append(problems, applyKind(ctx, db, stmts, selected, kind, ignoreDup)...)
	}
	return problems
}

func applyKind(ctx context.Context, db *database, stmts []Statement, selected map[string]bool, kind string, ignoreDup bool) []Problem {
	var problems []Problem
	for _, st := range stmts {
		if st.Kind != kind || !selected[st.Database] {
			continue
		}
		if p, ok := execStatement(ctx, db, st, ignoreDup); !ok {
			problems = append(problems, p)
		}
	}
	return problems
}

func execStatement(ctx context.Context, db *database, st Statement, ignoreDup bool) (Problem, bool) {
	if err := db.use(ctx, st.Database); err != nil {
		return Problem{Object: st.Database, Kind: "database", Error: err.Error()}, false
	}
	if err := runStatement(ctx, db, st.SQL, ignoreDup); err != nil {
		return Problem{Object: statementObject(st), Kind: st.Kind, SQL: st.SQL, Error: err.Error()}, false
	}
	return Problem{}, true
}

func runStatement(ctx context.Context, db *database, sql string, ignoreDup bool) error {
	_, err := db.exec(ctx, sql)
	if err != nil && ignoreDup && isDuplicate(err) {
		return nil
	}
	return err
}

func statementObject(st Statement) string {
	if st.Object == "" {
		return st.Kind
	}
	return st.Object
}

func prepareTargets(ctx context.Context, db *database, manifest Manifest, selected map[string]bool, force bool) error {
	nonEmpty, err := nonEmptyTables(ctx, db, manifest, selected)
	if err != nil {
		return err
	}
	if len(nonEmpty) > 0 && !force {
		return fmt.Errorf("refusing to overwrite non-empty tables: %s\npass --force to truncate them and load the backup", strings.Join(nonEmpty, ", "))
	}
	if force {
		return truncateTables(ctx, db, nonEmpty)
	}
	return nil
}

func nonEmptyTables(ctx context.Context, db *database, manifest Manifest, selected map[string]bool) ([]string, error) {
	var nonEmpty []string
	for _, table := range manifest.Tables {
		if !selected[table.Database] {
			continue
		}
		empty, err := tableIsEmpty(ctx, db, table)
		if err != nil {
			return nil, err
		}
		if !empty {
			nonEmpty = append(nonEmpty, table.qualified())
		}
	}
	return nonEmpty, nil
}

func truncateTables(ctx context.Context, db *database, names []string) error {
	for _, name := range names {
		if err := truncateOne(ctx, db, name); err != nil {
			return err
		}
	}
	return nil
}

func truncateOne(ctx context.Context, db *database, name string) error {
	parts := strings.Split(name, ".")
	if len(parts) != 3 {
		return fmt.Errorf("bad table name %q", name)
	}
	_, err := db.exec(ctx, "TRUNCATE TABLE "+qualified(parts[0], parts[1], parts[2])+" CASCADE")
	if err != nil {
		return fmt.Errorf("truncate %s: %w", name, err)
	}
	return nil
}

func loadSelected(ctx context.Context, db *database, bundle restoreBundle, selected map[string]bool, opt RestoreOptions) ([]string, error) {
	if opt.Load == "copy" {
		return loadCopy(ctx, db, bundle.store, bundle.base, bundle.manifest, selected)
	}
	return loadImport(ctx, importRequest{
		db: db, store: bundle.store, root: bundle.root, base: bundle.base,
		manifest: bundle.manifest, selected: selected, opt: opt,
	})
}

func finishRestore(ctx context.Context, db *database, bundle restoreBundle, selected map[string]bool, res *RestoreResult) (RestoreResult, error) {
	problems := applyStatements(ctx, db, bundle.objects.Statements, selected, []string{"foreign_key", "alter"}, false)
	problems = append(problems, restoreSequences(ctx, db, bundle.objects.SequenceValues, selected)...)
	res.Warnings = append(res.Warnings, restoreGrants(ctx, db, bundle.objects.Grants)...)
	res.Warnings = append(res.Warnings, restoreZones(ctx, db, bundle.objects.Zones, selected, bundle.manifest)...)
	rows, tables, countProblems := checkRestoredRows(ctx, db, bundle.manifest, selected)
	problems = append(problems, countProblems...)
	res.Rows = rows
	res.Tables = tables
	res.Incompatible = problems
	if len(problems) > 0 {
		res.OK = false
		return *res, incompatibleError(problems)
	}
	res.OK = true
	return *res, nil
}

func restoreSequences(ctx context.Context, db *database, seqs []SequenceValue, selected map[string]bool) []Problem {
	var problems []Problem
	for _, seq := range seqs {
		if !selected[seq.Database] {
			continue
		}
		if p, ok := restoreSequence(ctx, db, seq); !ok {
			problems = append(problems, p)
		}
	}
	return problems
}

func restoreSequence(ctx context.Context, db *database, seq SequenceValue) (Problem, bool) {
	if err := db.use(ctx, seq.Database); err != nil {
		return Problem{Object: seq.Object, Kind: "sequence_value", SQL: seq.SQL, Error: err.Error()}, false
	}
	if _, err := db.exec(ctx, seq.SQL); err != nil {
		return Problem{Object: seq.Object, Kind: "sequence_value", SQL: seq.SQL, Error: err.Error()}, false
	}
	return Problem{}, true
}

func restoreGrants(ctx context.Context, db *database, grants []string) []string {
	var warnings []string
	for _, g := range grants {
		if _, err := db.exec(ctx, g); err != nil && !isDuplicate(err) {
			warnings = append(warnings, "grant skipped: "+oneLine(err.Error())+" sql: "+oneLine(g))
		}
	}
	return warnings
}

func restoreZones(ctx context.Context, db *database, zones []ZoneStatement, selected map[string]bool, manifest Manifest) []string {
	var warnings []string
	for _, z := range zones {
		if warning, ok := restoreZone(ctx, db, z, selected, manifest); ok {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func restoreZone(ctx context.Context, db *database, z ZoneStatement, selected map[string]bool, manifest Manifest) (string, bool) {
	if z.SQL == "" || !zoneInScope(z, selected, manifest) {
		return "", false
	}
	if _, err := db.exec(ctx, z.SQL); err != nil {
		return fmt.Sprintf("zone config skipped for %s: %s", z.Object, oneLine(err.Error())), true
	}
	return "", false
}

func checkRestoredRows(ctx context.Context, db *database, manifest Manifest, selected map[string]bool) (int64, int, []Problem) {
	var rows int64
	var tables int
	var problems []Problem
	for _, table := range manifest.Tables {
		if !selected[table.Database] {
			continue
		}
		n, problem, ok := countRestoredTable(ctx, db, table)
		if !ok {
			problems = append(problems, problem)
			continue
		}
		rows += n
		tables++
	}
	return rows, tables, problems
}

func countRestoredTable(ctx context.Context, db *database, table TableEntry) (int64, Problem, bool) {
	var n int64
	q := "SELECT count(*) FROM " + qualified(table.Database, table.Schema, table.Name)
	if err := db.queryRow(ctx, q).Scan(&n); err != nil {
		return 0, Problem{Object: table.qualified(), Kind: "row_count", Error: err.Error()}, false
	}
	if n != table.RowCount {
		return 0, Problem{
			Object: table.qualified(),
			Kind:   "row_count",
			Error:  fmt.Sprintf("target has %d rows, backup has %d", n, table.RowCount),
		}, false
	}
	return n, Problem{}, true
}

func zoneInScope(z ZoneStatement, selected map[string]bool, manifest Manifest) bool {
	for _, d := range manifest.Databases {
		if selected[d] && strings.Contains(z.Object, d) {
			return true
		}
	}
	// Database-less statements are not expected. Keep index/table objects
	// whose qualified name starts with a selected database.
	return false
}

func tableIsEmpty(ctx context.Context, db *database, table TableEntry) (bool, error) {
	var n int
	q := "SELECT 1 FROM " + qualified(table.Database, table.Schema, table.Name) + " LIMIT 1"
	err := db.queryRow(ctx, q).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return true, nil
		}
		return false, fmt.Errorf("check %s: %w", table.qualified(), err)
	}
	return false, nil
}

// importRequest is one IMPORT INTO pass over a backup.
type importRequest struct {
	db       *database
	store    Store
	root     Location
	base     string
	manifest Manifest
	selected map[string]bool
	opt      RestoreOptions
}

func loadImport(ctx context.Context, req importRequest) ([]string, error) {
	var warnings []string
	httpBase, closer, err := serveLocalBackup(req)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer()
	}
	for _, table := range req.manifest.Tables {
		if err := importTable(ctx, req, httpBase, table); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func serveLocalBackup(req importRequest) (string, func(), error) {
	if req.root.Kind != "file" {
		return "", nil, nil
	}
	ln, err := net.Listen("tcp", req.opt.ImportListen)
	if err != nil {
		return "", nil, fmt.Errorf("listen for IMPORT: %w", err)
	}
	dir := req.root.join(req.base)
	srv := &http.Server{Handler: http.FileServer(http.Dir(dir))}
	go func() { _ = srv.Serve(ln) }()
	closer := func() {
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}
	httpBase := "http://" + ln.Addr().String()
	logf("serving the backup at %s for IMPORT", httpBase)
	return httpBase, closer, nil
}

func importTable(ctx context.Context, req importRequest, httpBase string, table TableEntry) error {
	if !req.selected[table.Database] || table.RowCount == 0 {
		return nil
	}
	uris, err := importURIs(ctx, req, httpBase, table)
	if err != nil {
		return err
	}
	logf("importing %s", table.qualified())
	_, err = req.db.exec(ctx, importCSVStatement(table, uris, req.manifest.Compression))
	if err != nil {
		return fmt.Errorf("IMPORT INTO %s failed. The database node must be able to read the backup URL. For a backup that lives only on this machine, the node must reach --import-listen (%s), or rerun with --load=copy. Error: %w", table.qualified(), req.opt.ImportListen, err)
	}
	return nil
}

func importURIs(ctx context.Context, req importRequest, httpBase string, table TableEntry) ([]string, error) {
	s3s, _ := req.store.(*s3Store)
	var uris []string
	for _, f := range table.Files {
		u, err := oneImportURI(ctx, req.root, s3s, httpBase, req.base, f.Path)
		if err != nil {
			return nil, err
		}
		uris = append(uris, u)
	}
	return uris, nil
}

func oneImportURI(ctx context.Context, root Location, s3s *s3Store, httpBase, base, path string) (string, error) {
	switch root.Kind {
	case "file":
		return httpBase + "/" + path, nil
	case "s3":
		return s3s.importURL(ctx, base+"/"+path)
	default:
		return "", fmt.Errorf("cannot IMPORT from %s", root.Kind)
	}
}

func importCSVStatement(table TableEntry, uris []string, compression string) string {
	quoted := make([]string, len(uris))
	for i, u := range uris {
		quoted[i] = quoteLiteral(u)
	}
	cols := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		cols[i] = quoteIdent(c)
	}
	with := "nullif = '\\N'"
	if compression == "gzip" {
		with = "decompress = 'gzip', nullif = '\\N'"
	}
	return fmt.Sprintf("IMPORT INTO %s (%s) CSV DATA (%s) WITH %s",
		qualified(table.Database, table.Schema, table.Name),
		strings.Join(cols, ", "),
		strings.Join(quoted, ", "),
		with,
	)
}

func loadCopy(ctx context.Context, db *database, store Store, base string, manifest Manifest, selected map[string]bool) ([]string, error) {
	for _, table := range manifest.Tables {
		if !selected[table.Database] || table.RowCount == 0 {
			continue
		}
		cols := make([]string, len(table.Columns))
		for i, c := range table.Columns {
			cols[i] = quoteIdent(c)
		}
		copySQL := fmt.Sprintf("COPY %s (%s) FROM STDIN WITH CSV NULL E'\\\\N'",
			qualified(table.Database, table.Schema, table.Name),
			strings.Join(cols, ", "),
		)
		for _, f := range table.Files {
			rc, err := store.Open(ctx, base+"/"+f.Path)
			if err != nil {
				return nil, err
			}
			var src io.Reader = rc
			var gz *gzip.Reader
			if manifest.Compression == "gzip" || strings.HasSuffix(f.Path, ".gz") {
				gz, err = gzip.NewReader(rc)
				if err != nil {
					rc.Close()
					return nil, err
				}
				src = gz
			}
			logf("copying %s", table.qualified())
			err = db.copyFrom(ctx, bufio.NewReader(src), copySQL)
			if gz != nil {
				_ = gz.Close()
			}
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("COPY %s: %w", table.qualified(), err)
			}
		}
	}
	return nil, nil
}

func incompatibleError(problems []Problem) error {
	var b strings.Builder
	b.WriteString("restore stopped. These objects use syntax or features the target rejected. Nothing was dropped to hide them:\n")
	for _, p := range problems {
		fmt.Fprintf(&b, "- %s (%s): %s\n", p.Object, p.Kind, oneLine(p.Error))
	}
	return fmt.Errorf("%s", strings.TrimSpace(b.String()))
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

func resolveBackup(ctx context.Context, root Location, src string) (Location, string, error) {
	// src is the original user string. root is its parsed form.
	// A path ending in /latest or /latest.json is a pointer.
	trimmed := strings.TrimRight(src, "/")
	baseName := trimmed
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		baseName = trimmed[i+1:]
	}
	if baseName == "latest" || baseName == "latest.json" {
		parent := strings.TrimSuffix(trimmed, "/"+baseName)
		parentLoc, err := parseLocation(parent, root.Region, root.Endpoint, root.SSE, root.KMSKeyID, root.ImportAuth)
		if err != nil {
			return Location{}, "", err
		}
		store, err := openStore(ctx, parentLoc)
		if err != nil {
			return Location{}, "", err
		}
		rc, err := store.Open(ctx, "latest.json")
		if err != nil {
			return Location{}, "", fmt.Errorf("read latest pointer: %w", err)
		}
		defer rc.Close()
		var ptr LatestPointer
		if err := json.NewDecoder(rc).Decode(&ptr); err != nil {
			return Location{}, "", fmt.Errorf("latest pointer: %w", err)
		}
		if !ptr.Complete || ptr.Timestamp == "" {
			return Location{}, "", fmt.Errorf("latest pointer is not a complete backup")
		}
		return parentLoc, ptr.Timestamp, nil
	}
	// The user passed the timestamp directory. The store root is the parent
	// that contains timestamp folders when the last element looks like a timestamp,
	// otherwise the path itself is the backup directory.
	if looksLikeTimestamp(baseName) {
		parent := strings.TrimSuffix(trimmed, "/"+baseName)
		parentLoc, err := parseLocation(parent, root.Region, root.Endpoint, root.SSE, root.KMSKeyID, root.ImportAuth)
		if err != nil {
			return Location{}, "", err
		}
		return parentLoc, baseName, nil
	}
	return root, "", fmt.Errorf("restore --src must be a backup timestamp directory or a path ending in /latest")
}

func looksLikeTimestamp(s string) bool {
	if len(s) != len("20060102T150405Z") {
		return false
	}
	_, err := time.Parse("20060102T150405Z", s)
	return err == nil
}

func openBackup(ctx context.Context, root Location, timestamp string) (Store, string, error) {
	store, err := openStore(ctx, root)
	if err != nil {
		return nil, "", err
	}
	ok, err := store.Exists(ctx, timestamp+"/manifest.json")
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("no manifest.json in %s/%s. An unfinished backup is not restored", root.String(), timestamp)
	}
	return store, timestamp, nil
}

func readBackup(ctx context.Context, store Store, base string) (Manifest, ObjectsFile, error) {
	var manifest Manifest
	var objects ObjectsFile
	rc, err := store.Open(ctx, base+"/manifest.json")
	if err != nil {
		return manifest, objects, err
	}
	err = json.NewDecoder(rc).Decode(&manifest)
	rc.Close()
	if err != nil {
		return manifest, objects, fmt.Errorf("manifest.json: %w", err)
	}
	if manifest.FormatVersion != formatVersion {
		return manifest, objects, fmt.Errorf("backup format version %d is not supported (this tool reads version %d)", manifest.FormatVersion, formatVersion)
	}
	body, err := store.Open(ctx, base+"/"+manifest.ObjectsFile.Path)
	if err != nil {
		return manifest, objects, err
	}
	err = json.NewDecoder(body).Decode(&objects)
	body.Close()
	if err != nil {
		return manifest, objects, fmt.Errorf("objects.json: %w", err)
	}
	return manifest, objects, nil
}
