// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestClassifyAndOrder(t *testing.T) {
	stmts := []string{
		"CREATE SCHEMA public;",
		"CREATE TYPE public.mood AS ENUM ('ok', 'bad');",
		"CREATE SEQUENCE public.orders_seq MINVALUE 1 MAXVALUE 9223372036854775807 INCREMENT 1 START 1;",
		"CREATE TABLE public.customers (\n\tid INT8 NOT NULL,\n\tCONSTRAINT customers_pkey PRIMARY KEY (id ASC)\n);",
		"CREATE VIEW public.order_names (id, name) AS SELECT o.id, c.name FROM orders o JOIN customers c ON c.id = o.customer_id;",
		"ALTER TABLE public.orders ADD CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id);",
		"-- Validate foreign key constraints. These can fail if there was unvalidated data during the SHOW CREATE ALL TABLES",
		"ALTER TABLE public.orders VALIDATE CONSTRAINT orders_customer_id_fkey;",
		"CREATE TABLE public.vectors (id INT PRIMARY KEY, v VECTOR(3));",
	}
	want := []string{"schema", "type", "sequence", "table", "view", "foreign_key", "comment", "foreign_key", "table"}
	for i, sql := range stmts {
		got := classifyStatement(sql)
		if got != want[i] {
			t.Fatalf("statement %d classified %s, want %s\n%s", i, got, want[i], sql)
		}
	}
	if got := objectName("table", stmts[3]); got != "public.customers" {
		t.Fatalf("object name %q", got)
	}
}

func TestCSVCounterQuotesAndNewlines(t *testing.T) {
	var buf bytes.Buffer
	c := newCSVCounter(&buf)
	in := "1,ada,\"{a,b}\",\"{\"\"n\"\": 1}\"\n2,bea,\\N,\\N\n3,\"line\nbreak\",,\n"
	if _, err := c.Write([]byte(in[:10])); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte(in[10:])); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.Rows() != 3 {
		t.Fatalf("rows = %d", c.Rows())
	}
	if buf.String() != in {
		t.Fatal("counter changed the bytes")
	}
}

func TestParseAsOf(t *testing.T) {
	got, err := parseAsOf("2026-10-08 18:57:35.287307+00")
	if err != nil {
		t.Fatal(err)
	}
	if got.UTC().Format(time.RFC3339) != "2026-10-08T18:57:35Z" {
		t.Fatalf("parsed %s", got)
	}
}

func TestGCBudget(t *testing.T) {
	asOf := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	b := newBudget(asOf, 1, "TABLE shop.public.events", time.Minute)
	if !b.exceeded(asOf.Add(2 * time.Second)) {
		t.Fatal("expected a 1 second ttl to be inside a 1 minute margin")
	}
	long := newBudget(asOf, 14400, rangeDefaultZone, time.Minute)
	if long.exceeded(asOf.Add(time.Minute)) {
		t.Fatal("4 hour ttl should still be inside the window")
	}
	if !strings.Contains(b.message(), "gc.ttlseconds") || !strings.Contains(b.message(), "--extend-gc-ttl") {
		t.Fatal(b.message())
	}
}

func TestPlanGCTTLRaises(t *testing.T) {
	zones := []zoneRow{
		{Level: "range", Object: rangeDefaultZone, Effective: 14400, FullSQL: "gc.ttlseconds = 14400"},
		{Level: "database", Database: "shop", Object: "DATABASE shop", RawSQL: "ALTER DATABASE shop CONFIGURE ZONE USING gc.ttlseconds = 30", FullSQL: "gc.ttlseconds = 30", Effective: 30},
		{Level: "table", Database: "shop", Schema: "public", Table: "events", Object: "TABLE shop.public.events", RawSQL: "ALTER TABLE shop.public.events CONFIGURE ZONE USING gc.ttlseconds = 10", FullSQL: "gc.ttlseconds = 10", Effective: 10},
	}
	changes := planGCTTLRaises(3600, []string{"shop"}, zones)
	if len(changes) != 2 {
		t.Fatalf("changes = %#v", changes)
	}
	if !strings.Contains(changes[0].Apply, "shop.public.events") || !strings.Contains(changes[0].Revert, "gc.ttlseconds = 10") {
		t.Fatalf("table change %#v", changes[0])
	}
	if !strings.Contains(changes[1].Revert, "gc.ttlseconds = 30") {
		t.Fatalf("database change %#v", changes[1])
	}
}

func TestPartWriterBound(t *testing.T) {
	const partSize = 1024
	var parts []int
	w := newPartWriter(partSize, func(part []byte) error {
		if len(part) > partSize {
			t.Fatalf("part %d exceeds %d", len(part), partSize)
		}
		parts = append(parts, len(part))
		return nil
	})
	payload := bytes.Repeat([]byte("x"), partSize*3+10)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.maxBuf > partSize {
		t.Fatalf("max buffer %d", w.maxBuf)
	}
	if len(parts) != 4 || parts[0] != partSize || parts[3] != 10 {
		t.Fatalf("parts %#v", parts)
	}
}

func TestWhereRanges(t *testing.T) {
	got := whereRanges("id", []string{"10", "20"})
	if len(got) != 3 {
		t.Fatal(got)
	}
	if got[0] != `"id" < 10` || got[2] != `"id" >= 20` {
		t.Fatal(got)
	}
}

func TestLatestNotReadFromIncomplete(t *testing.T) {
	// A timestamp directory without manifest.json is not a complete backup.
	if looksLikeTimestamp("not-a-timestamp") {
		t.Fatal("accepted a bad timestamp")
	}
	if !looksLikeTimestamp("20261008T190201Z") {
		t.Fatal("rejected a timestamp folder")
	}
}
