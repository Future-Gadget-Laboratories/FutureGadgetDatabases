// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

func qualified(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, quoteIdent(p))
	}
	return strings.Join(out, ".")
}

func ensureSemicolon(sql string) string {
	s := strings.TrimSpace(sql)
	if s == "" {
		return ""
	}
	if strings.HasSuffix(s, ";") {
		return s
	}
	return s + ";"
}

func stripLeadingComments(sql string) string {
	s := strings.TrimSpace(sql)
	for strings.HasPrefix(s, "--") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = strings.TrimSpace(s[i+1:])
			continue
		}
		return ""
	}
	return s
}

func classifyStatement(sql string) string {
	s := stripLeadingComments(sql)
	if s == "" {
		return "comment"
	}
	fields := strings.Fields(s)
	n := len(fields)
	if n > 4 {
		n = 4
	}
	head := strings.ToUpper(strings.Join(fields[:n], " "))
	switch {
	case strings.HasPrefix(head, "CREATE SCHEMA"):
		return "schema"
	case strings.HasPrefix(head, "CREATE TYPE"):
		return "type"
	case strings.HasPrefix(head, "CREATE SEQUENCE"):
		return "sequence"
	case strings.HasPrefix(head, "CREATE TABLE"):
		return "table"
	case strings.HasPrefix(head, "CREATE MATERIALIZED VIEW"), strings.HasPrefix(head, "CREATE VIEW"):
		return "view"
	case strings.HasPrefix(head, "CREATE UNIQUE INDEX"), strings.HasPrefix(head, "CREATE INDEX"):
		return "index"
	case strings.HasPrefix(head, "ALTER TABLE"):
		up := strings.ToUpper(s)
		if strings.Contains(up, "FOREIGN KEY") || strings.Contains(up, "VALIDATE CONSTRAINT") {
			return "foreign_key"
		}
		return "alter"
	default:
		return "other"
	}
}

func objectName(kind, sql string) string {
	s := stripLeadingComments(sql)
	fields := strings.Fields(s)
	upper := make([]string, len(fields))
	for i, f := range fields {
		upper[i] = strings.ToUpper(f)
	}
	switch kind {
	case "schema", "type", "sequence", "table", "view", "index":
		// CREATE [UNIQUE] [MATERIALIZED] INDEX|TABLE|... [IF NOT EXISTS] name
		for i := 0; i < len(upper); i++ {
			if upper[i] == "EXISTS" && i+1 < len(fields) {
				return trimObjectToken(fields[i+1])
			}
		}
		for i := 0; i < len(upper); i++ {
			switch upper[i] {
			case "SCHEMA", "TYPE", "SEQUENCE", "TABLE", "VIEW", "INDEX":
				if i+1 < len(fields) {
					return trimObjectToken(fields[i+1])
				}
			}
		}
	case "foreign_key", "alter":
		for i := 0; i < len(upper)-1; i++ {
			if upper[i] == "TABLE" {
				return trimObjectToken(fields[i+1])
			}
		}
	}
	return ""
}

func trimObjectToken(tok string) string {
	tok = strings.TrimSpace(tok)
	tok = strings.TrimRight(tok, ",(")
	return tok
}

func safeSegment(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name %q cannot be used in a backup path", name)
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return fmt.Errorf("name %q cannot be used in a backup path", name)
		}
	}
	return nil
}

func isSimpleIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func parseAsOf(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05.999999Z07",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	var last error
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		last = err
	}
	return time.Time{}, fmt.Errorf("parse timestamp %q: %w", s, last)
}

func skippedDatabase(name string) bool {
	switch strings.ToLower(name) {
	case "system", "postgres":
		return true
	default:
		return false
	}
}

func skippedSchema(name string) bool {
	switch strings.ToLower(name) {
	case "pg_catalog", "information_schema", "crdb_internal", "pg_extension":
		return true
	default:
		return false
	}
}

func skippedPrincipal(name string) bool {
	switch strings.ToLower(name) {
	case "admin", "root", "node", "public":
		return true
	default:
		return false
	}
}
