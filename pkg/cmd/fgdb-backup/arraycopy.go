// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// sqlArrayPrefix is the text backup writes at the start of an array field.
const sqlArrayPrefix = "ARRAY["

// arrayCopyReader rewrites ARRAY[...] literals to PostgreSQL array text
// while streaming PGCOPY. IMPORT parses the SQL literal. COPY FROM STDIN
// parses { ... } and rejects ARRAY[...]. Only columns marked in the manifest
// are rewritten, so a string that looks like an array literal is left alone.
type arrayCopyReader struct {
	in    *bufio.Reader
	kinds []string
	out   []byte
	err   error
}

func newArrayCopyReader(r io.Reader, kinds []string) io.Reader {
	return newArrayStreamReader(r, kinds)
}

func hasArrayColumn(kinds []string) bool {
	for _, kind := range kinds {
		if kind != "" {
			return true
		}
	}
	return false
}

func (a *arrayCopyReader) Read(p []byte) (int, error) {
	for len(a.out) == 0 && a.err == nil {
		line, err := a.in.ReadBytes('\n')
		if len(line) > 0 {
			rewritten, rerr := rewriteArrayLine(line, a.kinds)
			if rerr != nil {
				a.err = rerr
				return 0, rerr
			}
			a.out = rewritten
		}
		if err != nil {
			if a.err == nil {
				a.err = err
			}
			break
		}
	}
	n := copy(p, a.out)
	a.out = a.out[n:]
	if n > 0 {
		return n, nil
	}
	if a.err == nil {
		return 0, io.EOF
	}
	return 0, a.err
}

func rewriteArrayLine(line []byte, kinds []string) ([]byte, error) {
	nl := false
	if len(line) > 0 && line[len(line)-1] == '\n' {
		nl = true
		line = line[:len(line)-1]
	}
	fields := splitPGCopyFields(string(line))
	if len(fields) != len(kinds) {
		return nil, fmt.Errorf("pgcopy row has %d fields, backup lists %d columns", len(fields), len(kinds))
	}
	for i, kind := range kinds {
		if kind == "" || fields[i] == `\N` {
			continue
		}
		raw, err := unescapePGCopy(fields[i])
		if err != nil {
			return nil, err
		}
		pg, err := sqlArrayToPostgres(raw)
		if err != nil {
			return nil, err
		}
		fields[i] = escapePGCopy(pg)
	}
	out := strings.Join(fields, "\t")
	if nl {
		out += "\n"
	}
	return []byte(out), nil
}

func splitPGCopyFields(line string) []string {
	var fields []string
	start := 0
	escaped := false
	for i := 0; i < len(line); i++ {
		if escaped {
			escaped = false
			continue
		}
		if line[i] == '\\' {
			escaped = true
			continue
		}
		if line[i] == '\t' {
			fields = append(fields, line[start:i])
			start = i + 1
		}
	}
	return append(fields, line[start:])
}

func unescapePGCopy(s string) (string, error) {
	if !strings.Contains(s, `\`) {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		next, err := writePGCopyEscape(&b, s, i)
		if err != nil {
			return "", err
		}
		i = next
	}
	return b.String(), nil
}

func writePGCopyEscape(b *strings.Builder, s string, i int) (int, error) {
	if i+1 >= len(s) {
		return i, fmt.Errorf("truncated pgcopy escape")
	}
	i++
	if mapped, ok := pgCopySimpleEscape(s[i]); ok {
		b.WriteByte(mapped)
		return i, nil
	}
	if s[i] < '0' || s[i] > '7' {
		b.WriteByte(s[i])
		return i, nil
	}
	return writePGCopyOctal(b, s, i), nil
}

func pgCopySimpleEscape(c byte) (byte, bool) {
	switch c {
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'v':
		return '\v', true
	case '\\':
		return '\\', true
	default:
		return 0, false
	}
}

func writePGCopyOctal(b *strings.Builder, s string, i int) int {
	val, last := readOctal(s, i)
	b.WriteByte(val)
	return last
}

func escapePGCopy(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

type sqlArrayElem struct {
	null bool
	val  string
}

func sqlArrayToPostgres(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !hasSQLArrayPrefix(s) {
		return "", fmt.Errorf("array value is not an ARRAY literal")
	}
	elems, err := parseSQLArrayBody(s, len(sqlArrayPrefix))
	if err != nil {
		return "", err
	}
	return formatPostgresArray(elems), nil
}

func hasSQLArrayPrefix(s string) bool {
	return len(s) >= len(sqlArrayPrefix) && strings.EqualFold(s[:len(sqlArrayPrefix)], sqlArrayPrefix)
}

func parseSQLArrayBody(s string, i int) ([]sqlArrayElem, error) {
	i = skipSpace(s, i)
	if i < len(s) && s[i] == ']' {
		return nil, nil
	}
	var elems []sqlArrayElem
	for {
		elem, next, err := parseSQLArrayElem(s, i)
		if err != nil {
			return nil, err
		}
		elems = append(elems, elem)
		advance, done, err := nextArrayDelim(s, next)
		if err != nil {
			return nil, err
		}
		if done {
			return elems, nil
		}
		i = advance
	}
}

func nextArrayDelim(s string, i int) (int, bool, error) {
	i = skipSpace(s, i)
	if i >= len(s) {
		return 0, false, fmt.Errorf("array literal ended early")
	}
	if s[i] == ',' {
		return i + 1, false, nil
	}
	if s[i] == ']' {
		return i + 1, true, nil
	}
	return 0, false, fmt.Errorf("array literal has %q where a comma or ] was expected", s[i])
}

func parseSQLArrayElem(s string, i int) (sqlArrayElem, int, error) {
	i = skipSpace(s, i)
	if i >= len(s) {
		return sqlArrayElem{}, i, fmt.Errorf("missing array element")
	}
	if hasSQLKeyword(s, i, "NULL") {
		return sqlArrayElem{null: true}, i + 4, nil
	}
	if s[i] == '\'' {
		val, next, err := parseSQLString(s, i, false)
		return sqlArrayElem{val: val}, next, err
	}
	if i+1 < len(s) && (s[i] == 'e' || s[i] == 'E') && s[i+1] == '\'' {
		val, next, err := parseSQLString(s, i+1, true)
		return sqlArrayElem{val: val}, next, err
	}
	return sqlArrayElem{}, i, fmt.Errorf("array element starts with %q", s[i])
}

func hasSQLKeyword(s string, i int, word string) bool {
	if i+len(word) > len(s) || !strings.EqualFold(s[i:i+len(word)], word) {
		return false
	}
	if i+len(word) == len(s) {
		return true
	}
	switch s[i+len(word)] {
	case ',', ']', ' ', '\t':
		return true
	default:
		return false
	}
}

func parseSQLString(s string, i int, escaped bool) (string, int, error) {
	if i >= len(s) || s[i] != '\'' {
		return "", i, fmt.Errorf("expected a quoted string")
	}
	return readSQLString(s, i+1, escaped)
}

func readSQLString(s string, i int, escaped bool) (string, int, error) {
	var b strings.Builder
	for i < len(s) {
		if escaped && s[i] == '\\' {
			next, err := appendSQLEscape(&b, s, i)
			if err != nil {
				return "", i, err
			}
			i = next
			continue
		}
		if s[i] == '\'' {
			next, done := takeSQLQuote(&b, s, i)
			if done {
				return b.String(), next, nil
			}
			i = next
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return "", i, fmt.Errorf("unterminated string in array literal")
}

func appendSQLEscape(b *strings.Builder, s string, i int) (int, error) {
	if i+1 >= len(s) {
		return i, fmt.Errorf("truncated string escape")
	}
	i++
	if plain, ok := simpleSQLEscape(s[i]); ok {
		b.WriteByte(plain)
		return i + 1, nil
	}
	if s[i] == 'x' {
		return appendHexEscape(b, s, i)
	}
	if s[i] == 'u' {
		return appendUnicodeEscape(b, s, i, 4)
	}
	if s[i] == 'U' {
		return appendUnicodeEscape(b, s, i, 8)
	}
	if s[i] >= '0' && s[i] <= '7' {
		return appendOctalEscape(b, s, i)
	}
	b.WriteByte(s[i])
	return i + 1, nil
}

func simpleSQLEscape(c byte) (byte, bool) {
	switch c {
	case '\\', '\'':
		return c, true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	default:
		return 0, false
	}
}

func appendHexEscape(b *strings.Builder, s string, i int) (int, error) {
	if i+2 >= len(s) {
		return i, fmt.Errorf("truncated hex escape")
	}
	v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
	if err != nil {
		return i, fmt.Errorf("bad hex escape")
	}
	b.WriteByte(byte(v))
	return i + 3, nil
}

func appendUnicodeEscape(b *strings.Builder, s string, i, digits int) (int, error) {
	if i+digits >= len(s) {
		return i, fmt.Errorf("truncated unicode escape")
	}
	value, err := strconv.ParseUint(s[i+1:i+1+digits], 16, 32)
	if err != nil || !utf8.ValidRune(rune(value)) {
		return i, fmt.Errorf("bad unicode escape")
	}
	b.WriteString(string(rune(value)))
	return i + 1 + digits, nil
}

func appendOctalEscape(b *strings.Builder, s string, i int) (int, error) {
	val, last := readOctal(s, i)
	b.WriteByte(val)
	return last + 1, nil
}

func readOctal(s string, i int) (byte, int) {
	val := int(s[i] - '0')
	for n := 0; n < 2 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7'; n++ {
		i++
		val = val*8 + int(s[i]-'0')
	}
	return byte(val), i
}

func takeSQLQuote(b *strings.Builder, s string, i int) (int, bool) {
	if i+1 < len(s) && s[i+1] == '\'' {
		b.WriteByte('\'')
		return i + 2, false
	}
	return i + 1, true
}

func formatPostgresArray(elems []sqlArrayElem) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range elems {
		if i > 0 {
			b.WriteByte(',')
		}
		if e.null {
			b.WriteString("NULL")
			continue
		}
		b.WriteByte('"')
		for j := 0; j < len(e.val); j++ {
			if e.val[j] == '"' || e.val[j] == '\\' {
				b.WriteByte('\\')
			}
			b.WriteByte(e.val[j])
		}
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}
