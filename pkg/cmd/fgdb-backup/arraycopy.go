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
)

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
	return &arrayCopyReader{in: bufio.NewReader(r), kinds: kinds}
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
		if i+1 >= len(s) {
			return "", fmt.Errorf("truncated pgcopy escape")
		}
		i++
		switch s[i] {
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\':
			b.WriteByte('\\')
		default:
			if s[i] >= '0' && s[i] <= '7' {
				val := int(s[i] - '0')
				for n := 0; n < 2 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7'; n++ {
					i++
					val = val*8 + int(s[i]-'0')
				}
				b.WriteByte(byte(val))
				continue
			}
			b.WriteByte(s[i])
		}
	}
	return b.String(), nil
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
	if len(s) < len("ARRAY[") || !strings.EqualFold(s[:len("ARRAY[")], "ARRAY[") {
		return "", fmt.Errorf("array value is not an ARRAY literal")
	}
	i := len("ARRAY[")
	var elems []sqlArrayElem
	i = skipSpace(s, i)
	if i < len(s) && s[i] == ']' {
		i++
	} else {
		for {
			elem, next, err := parseSQLArrayElem(s, i)
			if err != nil {
				return "", err
			}
			elems = append(elems, elem)
			i = skipSpace(s, next)
			if i >= len(s) {
				return "", fmt.Errorf("array literal ended early")
			}
			if s[i] == ',' {
				i++
				continue
			}
			if s[i] == ']' {
				i++
				break
			}
			return "", fmt.Errorf("array literal has %q where a comma or ] was expected", s[i])
		}
	}
	return formatPostgresArray(elems), nil
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
	i++
	var b strings.Builder
	for i < len(s) {
		if escaped && s[i] == '\\' {
			if i+1 >= len(s) {
				return "", i, fmt.Errorf("truncated string escape")
			}
			i++
			switch s[i] {
			case '\\', '\'':
				b.WriteByte(s[i])
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			default:
				if s[i] == 'x' {
					if i+2 >= len(s) {
						return "", i, fmt.Errorf("truncated hex escape")
					}
					v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
					if err != nil {
						return "", i, fmt.Errorf("bad hex escape")
					}
					b.WriteByte(byte(v))
					i += 3
					continue
				}
				if s[i] >= '0' && s[i] <= '7' {
					val := int(s[i] - '0')
					digits := 1
					for digits < 3 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7' {
						i++
						digits++
						val = val*8 + int(s[i]-'0')
					}
					b.WriteByte(byte(val))
					i++
					continue
				}
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			return b.String(), i + 1, nil
		}
		b.WriteByte(s[i])
		i++
	}
	return "", i, fmt.Errorf("unterminated string in array literal")
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
