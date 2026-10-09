// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// arrayStreamReader converts marked array fields without retaining a whole
// COPY row. The input reader and output queue are bounded; the parser keeps
// only its small state and the current token.
type arrayStreamReader struct {
	in        *bufio.Reader
	kinds     []string
	out       []byte
	err       error
	done      bool
	field     int
	escaped   bool
	octal     byte
	octalN    int
	array     bool
	prefix    string
	arrayMode streamArrayMode
	sqlEsc    bool
	quote     bool
	backslash bool
	token     strings.Builder
}

type streamArrayMode uint8

const (
	streamPrefix streamArrayMode = iota
	streamArrayStart
	streamArrayQuoted
	streamArrayAfter
	streamArrayTail
)

func newArrayStreamReader(r io.Reader, kinds []string) io.Reader {
	return &arrayStreamReader{
		in:    bufio.NewReaderSize(r, 32<<10),
		kinds: kinds,
	}
}

func (r *arrayStreamReader) Read(p []byte) (int, error) {
	for len(r.out) == 0 && r.err == nil && !r.done {
		b, err := r.in.ReadByte()
		if err != nil {
			r.finish(err)
			break
		}
		if err := r.raw(b); err != nil {
			r.err = err
		}
	}
	n := copy(p, r.out)
	r.out = r.out[n:]
	if n > 0 {
		return n, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	return 0, io.EOF
}

func (r *arrayStreamReader) finish(readErr error) {
	if readErr != io.EOF {
		r.err = readErr
		return
	}
	if r.escaped {
		r.err = fmt.Errorf("truncated pgcopy escape")
		return
	}
	if r.octalN != 0 {
		r.err = fmt.Errorf("truncated pgcopy octal escape")
		return
	}
	if r.field != 0 || len(r.out) != 0 {
		if err := r.endField(); err != nil {
			r.err = err
			return
		}
		if r.field+1 != len(r.kinds) {
			r.err = fmt.Errorf("pgcopy row has %d fields, backup lists %d columns", r.field+1, len(r.kinds))
			return
		}
	}
	r.done = true
}

func (r *arrayStreamReader) raw(b byte) error {
	if r.array {
		return r.rawArray(b)
	}
	if r.escaped {
		r.escaped = false
		r.emit(b)
		return nil
	}
	if b == '\\' {
		r.escaped = true
		r.emit(b)
		return nil
	}
	if b == '\t' || b == '\n' {
		return r.endBoundary(b)
	}
	r.emit(b)
	return nil
}

func (r *arrayStreamReader) rawArray(b byte) error {
	if r.escaped {
		return r.decodeEscape(b)
	}
	if b == '\\' {
		r.escaped = true
		return nil
	}
	if b == '\t' || b == '\n' {
		return r.endBoundary(b)
	}
	return r.arrayByte(b)
}

func (r *arrayStreamReader) decodeEscape(b byte) error {
	r.escaped = false
	if b == 'N' && r.arrayMode == streamPrefix && r.prefix == "" {
		r.array = false
		r.emit('\\')
		r.emit('N')
		return nil
	}
	if mapped, ok := pgCopySimpleEscape(b); ok {
		return r.arrayByte(mapped)
	}
	if b < '0' || b > '7' {
		return r.arrayByte(b)
	}
	r.octal = b - '0'
	r.octalN = 1
	for r.octalN < 3 {
		next, err := r.in.ReadByte()
		if err != nil {
			return fmt.Errorf("truncated pgcopy octal escape")
		}
		if next < '0' || next > '7' {
			if err := r.arrayByte(r.octal); err != nil {
				return err
			}
			return r.rawArray(next)
		}
		r.octal = r.octal*8 + next - '0'
		r.octalN++
	}
	r.octalN = 0
	return r.arrayByte(r.octal)
}

func (r *arrayStreamReader) endBoundary(boundary byte) error {
	if err := r.endField(); err != nil {
		return err
	}
	if boundary == '\t' {
		r.field++
	}
	r.array = r.isArrayField()
	if r.array {
		r.resetArray()
	}
	r.emit(boundary)
	if boundary == '\n' {
		if r.field+1 != len(r.kinds) {
			return fmt.Errorf("pgcopy row has %d fields, backup lists %d columns", r.field+1, len(r.kinds))
		}
		r.field = 0
		r.array = r.isArrayField()
		if r.array {
			r.resetArray()
		}
	}
	return nil
}

func (r *arrayStreamReader) endField() error {
	if !r.array {
		return nil
	}
	if r.arrayMode == streamArrayTail {
		return nil
	}
	return fmt.Errorf("unterminated array field")
}

func (r *arrayStreamReader) isArrayField() bool {
	return r.field < len(r.kinds) && r.kinds[r.field] != ""
}

func (r *arrayStreamReader) resetArray() {
	r.arrayMode = streamPrefix
	r.prefix = ""
	r.sqlEsc = false
	r.quote = false
	r.backslash = false
	r.token.Reset()
}

func (r *arrayStreamReader) arrayByte(b byte) error {
	switch r.arrayMode {
	case streamPrefix:
		return r.readPrefix(b)
	case streamArrayStart:
		return r.readArrayStart(b)
	case streamArrayQuoted:
		return r.readQuoted(b)
	case streamArrayAfter:
		return r.readAfter(b)
	case streamArrayTail:
		return nil
	default:
		return fmt.Errorf("invalid array parser state")
	}
}

func (r *arrayStreamReader) readPrefix(b byte) error {
	if len(r.prefix) < len(sqlArrayPrefix) && len(r.prefix) == 0 && (b == ' ' || b == '\t') {
		return nil
	}
	r.prefix += string(b)
	if len(r.prefix) < len(sqlArrayPrefix) {
		return nil
	}
	if !strings.EqualFold(r.prefix, sqlArrayPrefix) {
		return fmt.Errorf("array value is not an ARRAY literal")
	}
	r.emitArray('{')
	r.arrayMode = streamArrayStart
	return nil
}

func (r *arrayStreamReader) readArrayStart(b byte) error {
	if b == ' ' || b == '\t' {
		return nil
	}
	if r.token.Len() > 0 {
		return r.readArrayStartToken(b)
	}
	return r.startArrayElement(b)
}

func (r *arrayStreamReader) readArrayStartToken(b byte) error {
	token := r.token.String()
	if strings.EqualFold(token, "e") && b == '\'' {
		r.token.Reset()
		r.openQuote(true)
		return nil
	}
	if b != ',' && b != ']' {
		r.token.WriteByte(b)
		return nil
	}
	if !strings.EqualFold(token, "NULL") {
		return fmt.Errorf("array element starts with %q", token[0])
	}
	r.emitArrayString("NULL")
	r.token.Reset()
	if b == ',' {
		r.emitArray(',')
		return nil
	}
	r.emitArray('}')
	r.arrayMode = streamArrayTail
	return nil
}

func (r *arrayStreamReader) startArrayElement(b byte) error {
	if b == ']' {
		r.emitArray('}')
		r.arrayMode = streamArrayTail
		return nil
	}
	if b == '\'' {
		r.openQuote(false)
		return nil
	}
	if b == 'e' || b == 'E' {
		r.token.WriteByte(b)
		return nil
	}
	if b == 'N' || b == 'n' {
		r.token.WriteByte(b)
		return nil
	}
	return fmt.Errorf("array element starts with %q", b)
}

func (r *arrayStreamReader) openQuote(escaped bool) {
	r.sqlEsc = escaped
	r.quote = false
	r.backslash = false
	r.arrayMode = streamArrayQuoted
	r.emitArray('"')
}

func (r *arrayStreamReader) readQuoted(b byte) error {
	if r.backslash {
		r.backslash = false
		if plain, ok := simpleSQLEscape(b); ok {
			r.emitArrayByte(plain)
		} else {
			r.emitArrayByte(b)
		}
		return nil
	}
	if r.quote {
		r.quote = false
		if b == '\'' {
			r.emitArrayByte('\'')
			return nil
		}
		r.emitArray('"')
		r.arrayMode = streamArrayAfter
		return r.readAfter(b)
	}
	if r.sqlEsc && b == '\\' {
		r.backslash = true
		return nil
	}
	if b == '\'' {
		r.quote = true
		return nil
	}
	r.emitArrayByte(b)
	return nil
}

func (r *arrayStreamReader) readAfter(b byte) error {
	if r.token.Len() > 0 {
		return r.readAfterToken(b)
	}
	return r.readAfterDelimiter(b)
}

func (r *arrayStreamReader) readAfterToken(b byte) error {
	if b != ',' && b != ']' && b != ' ' && b != '\t' {
		r.token.WriteByte(b)
		return nil
	}
	if !strings.EqualFold(r.token.String(), "NULL") {
		return fmt.Errorf("array element starts with %q", r.token.String()[0])
	}
	r.emitArrayString("NULL")
	r.token.Reset()
	if b == ',' {
		r.emitArray(',')
		r.arrayMode = streamArrayStart
		return nil
	}
	if b == ']' {
		r.emitArray('}')
		r.arrayMode = streamArrayTail
		return nil
	}
	return nil
}

func (r *arrayStreamReader) readAfterDelimiter(b byte) error {
	if b == ' ' || b == '\t' {
		return nil
	}
	if b == ',' {
		r.emitArray(',')
		r.arrayMode = streamArrayStart
		return nil
	}
	if b == ']' {
		r.emitArray('}')
		r.arrayMode = streamArrayTail
		return nil
	}
	return fmt.Errorf("array literal has %q where a comma or ] was expected", b)
}

func (r *arrayStreamReader) emitArray(b byte) {
	r.emitArrayByte(b)
}

func (r *arrayStreamReader) emitArrayString(s string) {
	for i := 0; i < len(s); i++ {
		r.emitArrayByte(s[i])
	}
}

func (r *arrayStreamReader) emitArrayByte(b byte) {
	switch b {
	case '\\':
		r.emitString(`\\\\`)
	case '\n':
		r.emitString(`\n`)
	case '\r':
		r.emitString(`\r`)
	case '\t':
		r.emitString(`\t`)
	default:
		r.emit(b)
	}
}

func (r *arrayStreamReader) emitString(s string) {
	r.out = append(r.out, s...)
}

func (r *arrayStreamReader) emit(b byte) {
	r.out = append(r.out, b)
}
