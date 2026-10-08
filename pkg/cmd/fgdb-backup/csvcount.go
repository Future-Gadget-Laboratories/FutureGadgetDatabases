// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import "io"

// csvCounter copies bytes and counts CSV records. Quoted quotes ("") and
// newlines inside quotes do not end a record. Memory use is constant.
type csvCounter struct {
	dst       io.Writer
	inQuote   bool
	prevQuote bool
	rows      int64
	any       bool
}

func newCSVCounter(dst io.Writer) *csvCounter {
	return &csvCounter{dst: dst}
}

func (c *csvCounter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := c.dst.Write(p); err != nil {
		return 0, err
	}
	c.scan(p)
	return len(p), nil
}

func (c *csvCounter) scan(p []byte) {
	for _, b := range p {
		c.scanByte(b)
	}
}

// consumeQuoted reports whether b was handled inside a quoted field.
// A closing quote followed by another byte falls through to plain scanning.
func (c *csvCounter) consumeQuoted(b byte) bool {
	if !c.inQuote {
		return false
	}
	if b == '"' {
		c.prevQuote = !c.prevQuote
		return true
	}
	if !c.prevQuote {
		c.any = true
		return true
	}
	c.inQuote = false
	c.prevQuote = false
	return false
}

func (c *csvCounter) scanByte(b byte) {
	if c.consumeQuoted(b) {
		return
	}
	c.scanPlain(b)
}

func (c *csvCounter) scanPlain(b byte) {
	if b == '"' {
		c.inQuote = true
		c.any = true
		return
	}
	if b == '\n' {
		c.rows++
		c.any = false
		return
	}
	if b != '\r' {
		c.any = true
	}
}

func (c *csvCounter) Close() error {
	if c.prevQuote && c.inQuote {
		c.inQuote = false
		c.prevQuote = false
	}
	if c.any {
		c.rows++
		c.any = false
	}
	if closer, ok := c.dst.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (c *csvCounter) Rows() int64 { return c.rows }

// countCSVRecords counts records in p. It is used by verify.
func countCSVRecords(r io.Reader) (int64, error) {
	c := newCSVCounter(io.Discard)
	if _, err := io.Copy(c, r); err != nil {
		return 0, err
	}
	if err := c.Close(); err != nil {
		return 0, err
	}
	return c.Rows(), nil
}
