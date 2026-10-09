// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// dataFormatPGCopy is the PostgreSQL text COPY format. NULL is the two
// characters \N. A stored string that is those two characters is written
// as \\N, so restore can tell them apart. CSV with nullif '\N' cannot.
const dataFormatPGCopy = "pgcopy"

// lineCounter counts PGCOPY records. Each record ends with a newline;
// embedded newlines are escaped, so a newline always ends a row.
type lineCounter struct {
	dst  io.Writer
	rows int64
}

func newLineCounter(dst io.Writer) *lineCounter {
	return &lineCounter{dst: dst}
}

func (c *lineCounter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := c.dst.Write(p); err != nil {
		return 0, err
	}
	for _, b := range p {
		if b == '\n' {
			c.rows++
		}
	}
	return len(p), nil
}

func (c *lineCounter) Rows() int64 { return c.rows }

func countPGCopyRows(r io.Reader) (int64, error) {
	var n int64
	buf := make([]byte, 32*1024)
	for {
		nr, err := r.Read(buf)
		for i := 0; i < nr; i++ {
			if buf[i] == '\n' {
				n++
			}
		}
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
	}
}

func countDataRows(r io.Reader, format string) (int64, error) {
	if format == dataFormatPGCopy {
		return countPGCopyRows(r)
	}
	return countCSVRecords(r)
}

// sequenceRestoreSQL is the setval that puts a sequence back. An unused
// sequence has a NULL last_value in pg_sequences (the raw counter is
// start-increment, often 0, and setval of that value is out of range).
func sequenceRestoreSQL(schema, name string, start int64, last *int64) (string, bool) {
	target := quoteLiteral(qualified(schema, name))
	if last == nil {
		return fmt.Sprintf("SELECT setval(%s::REGCLASS, %d, false);", target, start), false
	}
	return fmt.Sprintf("SELECT setval(%s::REGCLASS, %d, true);", target, *last), true
}

// splitBoundQuery reads one split point. Each call skips splitRows keys
// after the previous bound, so the scan stays proportional to the page
// instead of growing with OFFSET. The key is qualified with the table
// alias. ORDER BY the output column would sort id::STRING, so the bounds
// would be 1, 10, 100 and the files would not be about splitRows rows.
func splitBoundQuery(table, pk, prev string, splitRows int) string {
	col := "src." + quoteIdent(pk)
	q := fmt.Sprintf("SELECT %s::STRING FROM %s AS src", col, table)
	if prev != "" {
		q += fmt.Sprintf(" WHERE %s >= %s", col, sqlBound(prev))
	}
	return q + fmt.Sprintf(" ORDER BY %s OFFSET %d LIMIT 1", col, splitRows)
}

// newerBackup reports whether candidate should replace the latest pointer.
// Equal timestamps are left alone so a replay cannot clobber the pointer.
func newerBackup(candidate, existing string) bool {
	if existing == "" {
		return true
	}
	candidateTime, candidateOK := backupTime(candidate)
	existingTime, existingOK := backupTime(existing)
	if candidateOK && existingOK && !candidateTime.Equal(existingTime) {
		return candidateTime.After(existingTime)
	}
	return candidate > existing
}

func backupTime(name string) (time.Time, bool) {
	for _, layout := range []string{"20060102T150405.000Z", "20060102T150405Z"} {
		if t, err := time.Parse(layout, name); err == nil {
			return t, true
		}
	}
	if i := strings.IndexByte(name, '-'); i > 0 {
		return backupTime(name[:i])
	}
	return time.Time{}, false
}

// zoneObjectNames matches a zone target to one database. "shop" does not
// match "shopping" or "myshop".
func zoneObjectNames(object, db string) bool {
	if db == "" || object == "" {
		return false
	}
	if object == db || object == "DATABASE "+db {
		return true
	}
	if strings.HasPrefix(object, db+".") {
		return true
	}
	return strings.Contains(object, " "+db+".")
}

func scrubSecrets(s string) string {
	for _, key := range []string{"AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_ACCESS_KEY_ID"} {
		s = scrubQueryParam(s, key)
	}
	return s
}

func scrubQueryParam(s, key string) string {
	token := key + "="
	from := 0
	for from < len(s) {
		rel := strings.Index(s[from:], token)
		if rel < 0 {
			return s
		}
		start := from + rel + len(token)
		end := start
		for end < len(s) && s[end] != '&' && s[end] != ' ' && s[end] != '\n' && s[end] != '"' && s[end] != '\'' {
			end++
		}
		s = s[:start] + "REDACTED" + s[end:]
		from = start + len("REDACTED")
	}
	return s
}

// stripMaterializedRowid removes the hidden rowid column Cockroach adds to
// SHOW CREATE of a materialized view. Replaying that column list fails
// because the SELECT does not produce rowid.
func stripMaterializedRowid(sql string) string {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "CREATE MATERIALIZED VIEW") {
		return sql
	}
	open := strings.Index(sql, "(")
	if open < 0 {
		return sql
	}
	close, ok := matchingParen(sql, open)
	if !ok {
		return sql
	}
	body := sql[close+1:]
	if !strings.Contains(strings.ToUpper(body), " AS ") {
		return sql
	}
	if strings.Contains(strings.ToLower(body), "rowid") {
		return sql
	}
	cols := splitCommaDepth(sql[open+1 : close])
	var kept []string
	removed := false
	for _, col := range cols {
		name := strings.Trim(strings.TrimSpace(col), `"`)
		if strings.EqualFold(name, "rowid") {
			removed = true
			continue
		}
		kept = append(kept, col)
	}
	if !removed {
		return sql
	}
	if len(kept) == 0 {
		return strings.TrimRight(sql[:open], " \t\n") + body
	}
	return sql[:open+1] + strings.Join(kept, ",") + sql[close:]
}

func matchingParen(s string, open int) (int, bool) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func splitCommaDepth(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

func dropRoutineStatement(kind, createSQL string) (string, error) {
	key := "FUNCTION"
	drop := "DROP FUNCTION IF EXISTS "
	if kind == "procedure" {
		key = "PROCEDURE"
		drop = "DROP PROCEDURE IF EXISTS "
	}
	upper := strings.ToUpper(createSQL)
	i := strings.Index(upper, key)
	if i < 0 {
		return "", fmt.Errorf("cannot find %s in %s", key, oneLine(createSQL))
	}
	rest := strings.TrimSpace(createSQL[i+len(key):])
	name, args, ok := splitNameArgs(rest)
	if !ok || name == "" {
		return "", fmt.Errorf("cannot parse %s name from %s", strings.ToLower(key), oneLine(createSQL))
	}
	return drop + name + "(" + routineArgTypes(args) + ");", nil
}

func splitNameArgs(s string) (string, string, bool) {
	open := findOpenParen(s)
	if open < 0 {
		return "", "", false
	}
	close, ok := matchingParen(s, open)
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(s[:open]), s[open+1 : close], true
}

func findOpenParen(s string) int {
	quotes := sqlQuoteState{}
	for i := 0; i < len(s); i++ {
		if next, quoted := quotes.consume(s, i); quoted {
			i = next
			continue
		}
		if s[i] == '(' {
			return i
		}
	}
	return -1
}

func routineArgTypes(args string) string {
	if strings.TrimSpace(args) == "" {
		return ""
	}
	parts := splitCommaDepth(args)
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		typ := routineArgType(part)
		if typ == "" {
			continue
		}
		types = append(types, typ)
	}
	return strings.Join(types, ", ")
}

func routineArgType(part string) string {
	fields := strings.Fields(strings.TrimSpace(part))
	if len(fields) == 0 {
		return ""
	}
	switch strings.ToUpper(fields[0]) {
	case "IN", "OUT", "INOUT", "VARIADIC":
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	// "name TYPE" or just "TYPE". A name is a single identifier; the rest is the type.
	if len(fields) == 1 {
		return fields[0]
	}
	if isSimpleIdent(strings.Trim(fields[0], `"`)) {
		return strings.Join(fields[1:], " ")
	}
	return strings.Join(fields, " ")
}

func sameRoutineName(object, sequenceName string) bool {
	return normalizeObjectName(object) == normalizeObjectName(sequenceName) ||
		strings.HasSuffix(normalizeObjectName(object), "."+normalizeObjectName(sequenceName))
}

func normalizeObjectName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, `"`)
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(p), `"`)
	}
	return strings.ToLower(strings.Join(parts, "."))
}
