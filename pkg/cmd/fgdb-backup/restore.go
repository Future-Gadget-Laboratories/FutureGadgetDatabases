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

func runRestore(ctx context.Context, opt RestoreOptions) (RestoreResult, error) {
	var res RestoreResult
	if opt.Load == "" {
		opt.Load = "import"
	}
	if opt.Load != "import" && opt.Load != "copy" {
		return res, fmt.Errorf("--load must be import or copy")
	}
	if opt.ImportListen == "" {
		opt.ImportListen = "127.0.0.1:0"
	}

	root, rel, err := resolveBackup(ctx, opt.Location, opt.Src)
	if err != nil {
		return res, err
	}
	store, base, err := openBackup(ctx, root, rel)
	if err != nil {
		return res, err
	}
	manifest, objects, err := readBackup(ctx, store, base)
	if err != nil {
		return res, err
	}
	if _, _, err := verifyBackup(ctx, store, base, manifest); err != nil {
		return res, err
	}
	res.Backup = root.String() + "/" + base
	res.AsOf = manifest.AsOf
	res.SourceVersion = manifest.SourceVersion

	selected := map[string]bool{}
	if len(opt.Databases) == 0 {
		for _, d := range manifest.Databases {
			selected[d] = true
		}
	} else {
		have := map[string]bool{}
		for _, d := range manifest.Databases {
			have[d] = true
		}
		for _, d := range opt.Databases {
			if !have[d] {
				return res, fmt.Errorf("backup does not contain database %q", d)
			}
			selected[d] = true
		}
	}

	db, err := connect(ctx, opt.URL)
	if err != nil {
		return res, err
	}
	defer db.Close(ctx)

	var problems []Problem
	for _, database := range objects.Databases {
		if !selected[database.Name] {
			continue
		}
		if _, err := db.exec(ctx, database.SQL); err != nil && !isDuplicate(err) {
			problems = append(problems, Problem{
				Object: database.Name,
				Kind:   "database",
				SQL:    database.SQL,
				Error:  err.Error(),
			})
		}
	}
	if len(problems) > 0 {
		res.Incompatible = problems
		return res, incompatibleError(problems)
	}

	pre := []string{"schema", "type", "sequence", "table", "index", "view", "other"}
	for _, kind := range pre {
		for _, st := range objects.Statements {
			if st.Kind != kind || !selected[st.Database] {
				continue
			}
			if err := db.use(ctx, st.Database); err != nil {
				problems = append(problems, Problem{Object: st.Database, Kind: "database", Error: err.Error()})
				continue
			}
			if _, err := db.exec(ctx, st.SQL); err != nil && !isDuplicate(err) {
				obj := st.Object
				if obj == "" {
					obj = st.Kind
				}
				problems = append(problems, Problem{Object: obj, Kind: st.Kind, SQL: st.SQL, Error: err.Error()})
			}
		}
	}
	if len(problems) > 0 {
		res.Incompatible = problems
		return res, incompatibleError(problems)
	}

	var nonEmpty []string
	for _, table := range manifest.Tables {
		if !selected[table.Database] {
			continue
		}
		empty, err := tableIsEmpty(ctx, db, table)
		if err != nil {
			return res, err
		}
		if !empty {
			nonEmpty = append(nonEmpty, table.qualified())
		}
	}
	if len(nonEmpty) > 0 && !opt.Force {
		return res, fmt.Errorf("refusing to overwrite non-empty tables: %s\npass --force to truncate them and load the backup", strings.Join(nonEmpty, ", "))
	}
	if opt.Force {
		for _, name := range nonEmpty {
			parts := strings.Split(name, ".")
			if len(parts) != 3 {
				return res, fmt.Errorf("bad table name %q", name)
			}
			if _, err := db.exec(ctx, "TRUNCATE TABLE "+qualified(parts[0], parts[1], parts[2])+" CASCADE"); err != nil {
				return res, fmt.Errorf("truncate %s: %w", name, err)
			}
		}
	}

	var warnings []string
	switch opt.Load {
	case "copy":
		warnings, err = loadCopy(ctx, db, store, base, manifest, selected)
	default:
		warnings, err = loadImport(ctx, db, store, root, base, manifest, selected, opt)
	}
	if err != nil {
		return res, err
	}
	res.Warnings = append(res.Warnings, warnings...)

	for _, kind := range []string{"foreign_key", "alter"} {
		for _, st := range objects.Statements {
			if st.Kind != kind || !selected[st.Database] {
				continue
			}
			if err := db.use(ctx, st.Database); err != nil {
				problems = append(problems, Problem{Object: st.Database, Kind: "database", Error: err.Error()})
				continue
			}
			if _, err := db.exec(ctx, st.SQL); err != nil {
				obj := st.Object
				if obj == "" {
					obj = st.Kind
				}
				problems = append(problems, Problem{Object: obj, Kind: st.Kind, SQL: st.SQL, Error: err.Error()})
			}
		}
	}

	for _, seq := range objects.SequenceValues {
		if !selected[seq.Database] {
			continue
		}
		if err := db.use(ctx, seq.Database); err != nil {
			problems = append(problems, Problem{Object: seq.Object, Kind: "sequence_value", SQL: seq.SQL, Error: err.Error()})
			continue
		}
		if _, err := db.exec(ctx, seq.SQL); err != nil {
			problems = append(problems, Problem{Object: seq.Object, Kind: "sequence_value", SQL: seq.SQL, Error: err.Error()})
		}
	}

	for _, g := range objects.Grants {
		if _, err := db.exec(ctx, g); err != nil && !isDuplicate(err) {
			res.Warnings = append(res.Warnings, "grant skipped: "+oneLine(err.Error())+" sql: "+oneLine(g))
		}
	}
	for _, z := range objects.Zones {
		if z.SQL == "" {
			continue
		}
		if !zoneInScope(z, selected, manifest) {
			continue
		}
		if _, err := db.exec(ctx, z.SQL); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("zone config skipped for %s: %s", z.Object, oneLine(err.Error())))
		}
	}

	var rows int64
	for _, table := range manifest.Tables {
		if !selected[table.Database] {
			continue
		}
		var n int64
		q := "SELECT count(*) FROM " + qualified(table.Database, table.Schema, table.Name)
		if err := db.queryRow(ctx, q).Scan(&n); err != nil {
			problems = append(problems, Problem{Object: table.qualified(), Kind: "row_count", Error: err.Error()})
			continue
		}
		if n != table.RowCount {
			problems = append(problems, Problem{
				Object: table.qualified(),
				Kind:   "row_count",
				Error:  fmt.Sprintf("target has %d rows, backup has %d", n, table.RowCount),
			})
			continue
		}
		rows += n
		res.Tables++
	}
	res.Rows = rows
	res.Incompatible = problems
	if len(problems) > 0 {
		res.OK = false
		return res, incompatibleError(problems)
	}
	res.OK = true
	return res, nil
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

func loadImport(ctx context.Context, db *database, store Store, root Location, base string, manifest Manifest, selected map[string]bool, opt RestoreOptions) ([]string, error) {
	var warnings []string
	var closer func()
	var httpBase string
	if root.Kind == "file" {
		ln, err := net.Listen("tcp", opt.ImportListen)
		if err != nil {
			return nil, fmt.Errorf("listen for IMPORT: %w", err)
		}
		dir := root.join(base)
		srv := &http.Server{Handler: http.FileServer(http.Dir(dir))}
		go func() { _ = srv.Serve(ln) }()
		closer = func() {
			shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(shut)
		}
		defer closer()
		httpBase = "http://" + ln.Addr().String()
		logf("serving the backup at %s for IMPORT", httpBase)
	}
	s3s, _ := store.(*s3Store)
	for _, table := range manifest.Tables {
		if !selected[table.Database] || table.RowCount == 0 {
			continue
		}
		var uris []string
		for _, f := range table.Files {
			switch root.Kind {
			case "file":
				uris = append(uris, httpBase+"/"+f.Path)
			case "s3":
				u, err := s3s.importURL(ctx, base+"/"+f.Path)
				if err != nil {
					return warnings, err
				}
				uris = append(uris, u)
			default:
				return warnings, fmt.Errorf("cannot IMPORT from %s", root.Kind)
			}
		}
		quoted := make([]string, len(uris))
		for i, u := range uris {
			quoted[i] = quoteLiteral(u)
		}
		cols := make([]string, len(table.Columns))
		for i, c := range table.Columns {
			cols[i] = quoteIdent(c)
		}
		stmt := fmt.Sprintf("IMPORT INTO %s (%s) CSV DATA (%s) WITH nullif = '\\N'",
			qualified(table.Database, table.Schema, table.Name),
			strings.Join(cols, ", "),
			strings.Join(quoted, ", "),
		)
		if manifest.Compression == "gzip" {
			stmt = fmt.Sprintf("IMPORT INTO %s (%s) CSV DATA (%s) WITH decompress = 'gzip', nullif = '\\N'",
				qualified(table.Database, table.Schema, table.Name),
				strings.Join(cols, ", "),
				strings.Join(quoted, ", "),
			)
		}
		logf("importing %s", table.qualified())
		if _, err := db.exec(ctx, stmt); err != nil {
			return warnings, fmt.Errorf("IMPORT INTO %s failed. The database node must be able to read the backup URL. For a backup that lives only on this machine, the node must reach --import-listen (%s), or rerun with --load=copy. Error: %w", table.qualified(), opt.ImportListen, err)
		}
	}
	return warnings, nil
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
