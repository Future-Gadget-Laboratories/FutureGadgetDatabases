// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

const (
	formatVersion = 1
	toolName      = "fgdb-backup"
	toolVersion   = "1"
)

// Manifest is the last file written into a backup directory. A directory
// without this file is incomplete and is never treated as a backup.
type Manifest struct {
	FormatVersion int          `json:"format_version"`
	Tool          string       `json:"tool"`
	ToolVersion   string       `json:"tool_version"`
	CreatedAt     string       `json:"created_at"`
	SourceVersion string       `json:"source_version"`
	ClusterID     string       `json:"cluster_id"`
	Name          string       `json:"name"`
	BackupID      string       `json:"backup_id,omitempty"`
	AsOf          string       `json:"as_of"`
	GCTTLSeconds  int          `json:"gc_ttl_seconds"`
	Compression   string       `json:"compression"`
	DataFormat    string       `json:"data_format,omitempty"`
	Databases     []string     `json:"databases"`
	ObjectsFile   FileDigest   `json:"objects_file"`
	SchemaFiles   []FileDigest `json:"schema_files,omitempty"`
	UsersFile     *FileDigest  `json:"users_file,omitempty"`
	ZonesFile     *FileDigest  `json:"zones_file,omitempty"`
	Tables        []TableEntry `json:"tables"`
	Warnings      []string     `json:"warnings,omitempty"`
	PeakRSSBytes  int64        `json:"peak_rss_bytes,omitempty"`
}

// FileDigest is a checksum of the stored bytes (compressed, when compression is on).
type FileDigest struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Bytes    int64  `json:"bytes"`
	RowCount int64  `json:"row_count,omitempty"`
}

// TableEntry is one user table. Files are in primary-key order when the
// table was split, and row_count is the sum of those files.
type TableEntry struct {
	Database string   `json:"database"`
	Schema   string   `json:"schema"`
	Name     string   `json:"name"`
	Columns  []string `json:"columns"`
	// ArraySQL is parallel to Columns. An entry is the SQL cast written into
	// that column's ARRAY[...] literal, or empty when the column is not an
	// array. COPY FROM STDIN cannot parse the literal, so --load=copy rewrites
	// only these columns. Omitted when the table has no array columns.
	ArraySQL []string     `json:"array_sql,omitempty"`
	RowCount int64        `json:"row_count"`
	Files    []FileDigest `json:"files"`
}

// ObjectsFile is what restore executes. The .sql files next to it are the
// same statements, written so a person can read them.
type ObjectsFile struct {
	FormatVersion  int             `json:"format_version"`
	Databases      []NamedSQL      `json:"databases"`
	Statements     []Statement     `json:"statements"`
	SequenceValues []SequenceValue `json:"sequence_values,omitempty"`
	Grants         []string        `json:"grants,omitempty"`
	Zones          []ZoneStatement `json:"zones,omitempty"`
}

// NamedSQL is one CREATE DATABASE statement.
type NamedSQL struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

// Statement is one schema statement with a kind restore uses for ordering.
type Statement struct {
	Database string `json:"database"`
	Kind     string `json:"kind"`
	Object   string `json:"object"`
	SQL      string `json:"sql"`
}

// SequenceValue restores a sequence counter after the table rows are loaded.
type SequenceValue struct {
	Database  string `json:"database"`
	Object    string `json:"object"`
	LastValue int64  `json:"last_value"`
	IsCalled  bool   `json:"is_called"`
	SQL       string `json:"sql"`
}

// ZoneStatement is replayed as best effort. A rejection is reported and skipped.
type ZoneStatement struct {
	Object   string `json:"object"`
	Database string `json:"database,omitempty"`
	Level    string `json:"level"`
	SQL      string `json:"sql"`
}

// LatestPointer is written only after manifest.json is in place.
type LatestPointer struct {
	FormatVersion int    `json:"format_version"`
	Name          string `json:"name"`
	Timestamp     string `json:"timestamp"`
	BackupID      string `json:"backup_id,omitempty"`
	Complete      bool   `json:"complete"`
}

func (t TableEntry) qualified() string {
	return t.Database + "." + t.Schema + "." + t.Name
}
