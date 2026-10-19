// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportKeepsArrayTypeModifiers restores array columns whose element
// type has a width or precision through --load=import and compares every
// value with the source. A cast without the modifier (CHAR[] is CHAR(1)[])
// silently truncated values on the IMPORT path.
func TestImportKeepsArrayTypeModifiers(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	addrs := freeLoopbackAddrs(t, 4)
	srcAddr, dstAddr := addrs[0], addrs[1]
	startNode(t, bin, filepath.Join(base, "src"), srcAddr, addrs[2], true)
	startNode(t, bin, filepath.Join(base, "dst"), dstAddr, addrs[3], true)
	srcURL := "postgresql://root@" + srcAddr + "/defaultdb?sslmode=disable"
	dstURL := "postgresql://root@" + dstAddr + "/defaultdb?sslmode=disable"
	sql(t, bin, srcAddr, true, `
CREATE DATABASE arr;
CREATE TABLE arr.public.t (
  id INT PRIMARY KEY,
  c CHAR(5)[],
  c1 CHAR[],
  v VARCHAR(4)[],
  b BIT(3)[],
  vb VARBIT(4)[],
  n DECIMAL(6,2)[],
  ts TIMESTAMP(3)[],
  tz TIMESTAMPTZ(3)[],
  tm TIME(3)[],
  by BYTES[],
  f FLOAT8[]
);
INSERT INTO arr.public.t VALUES
  (1, ARRAY['abcde', 'ab', NULL], ARRAY['x'], ARRAY['wxyz'], ARRAY[B'101'], ARRAY[B'1101'],
   ARRAY[1234.56, -0.01], ARRAY['2020-01-02 03:04:05.678'::TIMESTAMP], ARRAY['2020-01-02 03:04:05.678+00'::TIMESTAMPTZ],
   ARRAY['12:34:56.789'::TIME], ARRAY[b'\x00ff', b'hi'], ARRAY[1.5, 'NaN'::FLOAT8]),
  (2, ARRAY[]::CHAR(5)[], NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL);
`)
	dest := filepath.Join(base, "backups")
	runTool(t, tool, "backup", "--json", "--url", srcURL, "--dest", dest, "--name", "arr")
	runTool(t, tool, "restore", "--json", "--load=import", "--url", dstURL, "--src", filepath.Join(dest, "arr", "latest"))
	q := `SELECT id, c::STRING, c1::STRING, v::STRING, b::STRING, vb::STRING, n::STRING, ts::STRING, tz::STRING, tm::STRING, by::STRING, f::STRING FROM arr.public.t ORDER BY id`
	want := sqlOut(t, bin, srcAddr, true, q)
	got := sqlOut(t, bin, dstAddr, true, q)
	if got != want {
		t.Fatalf("IMPORT changed array values\nsource:\n%s\nrestored:\n%s", want, got)
	}
	if !strings.Contains(got, "abcde") {
		t.Fatalf("restored output lacks the CHAR(5) value:\n%s", got)
	}
}

// freeLoopbackAddrs returns n distinct loopback addresses whose ports were
// free a moment ago. All listeners stay open until every port is picked,
// so the kernel cannot hand out the same port twice.
func freeLoopbackAddrs(t *testing.T, n int) []string {
	t.Helper()
	lns := make([]net.Listener, 0, n)
	addrs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		lns = append(lns, ln)
		addrs = append(addrs, ln.Addr().String())
	}
	for _, ln := range lns {
		if err := ln.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return addrs
}
