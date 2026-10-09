// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
		"CREATE FUNCTION public.add(IN a INT8, IN b INT8) RETURNS INT8 LANGUAGE SQL AS $$ SELECT a + b; $$;",
		"CREATE PROCEDURE public.echo(IN a INT8) LANGUAGE SQL AS $$ SELECT a; $$;",
		"CREATE MATERIALIZED VIEW public.mv (id, rowid) AS SELECT id FROM public.t;",
		"CREATE VIEW public.order_names (id, name) AS SELECT o.id, c.name FROM orders o JOIN customers c ON c.id = o.customer_id;",
		"ALTER TABLE public.orders ADD CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id);",
		"-- Validate foreign key constraints. These can fail if there was unvalidated data during the SHOW CREATE ALL TABLES",
		"ALTER TABLE public.orders VALIDATE CONSTRAINT orders_customer_id_fkey;",
		"CREATE TABLE public.vectors (id INT PRIMARY KEY, v VECTOR(3));",
	}
	want := []string{"schema", "type", "sequence", "table", "function", "procedure", "materialized_view", "view", "foreign_key", "comment", "foreign_key", "table"}
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

func TestMinEffectiveTTLIncludesInheritedRangeDefault(t *testing.T) {
	zones := []zoneRow{
		{Level: "range", Object: rangeDefaultZone, Effective: 60},
		{Level: "table", Database: "shop", Object: "TABLE shop.public.explicit", Effective: 3600},
	}
	got, object := minEffectiveTTL([]string{"shop"}, zones)
	if got != 60 || object != rangeDefaultZone {
		t.Fatalf("minimum ttl = %d on %q, want 60 on %q", got, object, rangeDefaultZone)
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

func TestPlanGCTTLRaisesInheritedRangeAndIndex(t *testing.T) {
	zones := []zoneRow{
		{Level: "range", Object: rangeDefaultZone, RawSQL: "ALTER RANGE default CONFIGURE ZONE USING gc.ttlseconds = 60", Effective: 60},
		{Level: "index", Database: "shop", Schema: "public", Table: "events", Index: "events_pkey", Object: "INDEX shop.public.events@events_pkey", RawSQL: "ALTER INDEX shop.public.events@events_pkey CONFIGURE ZONE USING gc.ttlseconds = 120", Effective: 120},
	}
	changes := planGCTTLRaises(3600, []string{"shop"}, zones)
	if len(changes) != 2 {
		t.Fatalf("changes = %#v", changes)
	}
	if !strings.Contains(changes[0].Apply, "ALTER RANGE default") || !strings.Contains(changes[0].Revert, "60") {
		t.Fatalf("range change %#v", changes[0])
	}
	if !strings.Contains(changes[1].Apply, "ALTER INDEX") || !strings.Contains(changes[1].Revert, "120") {
		t.Fatalf("index change %#v", changes[1])
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

func TestNullAndSequenceHelpers(t *testing.T) {
	unused, called := sequenceRestoreSQL("public", "unused_seq", 1, nil)
	if called || !strings.Contains(unused, ", 1, false)") {
		t.Fatalf("unused sequence: %s called=%v", unused, called)
	}
	last := int64(4)
	used, called := sequenceRestoreSQL("public", "used", 1, &last)
	if !called || !strings.Contains(used, ", 4, true)") {
		t.Fatalf("used sequence: %s", used)
	}
	mv := "CREATE MATERIALIZED VIEW public.mv (\n\tid,\n\tnote,\n\trowid\n) AS SELECT id, note FROM public.t WHERE id > 0;"
	stripped := stripMaterializedRowid(mv)
	if strings.Contains(strings.ToLower(stripped), "rowid") {
		t.Fatalf("rowid remains: %s", stripped)
	}
	if classifyStatement(stripped) != "materialized_view" {
		t.Fatalf("kind %s", classifyStatement(stripped))
	}
	drop, err := dropRoutineStatement("function", "CREATE FUNCTION public.add(IN a INT8, IN b INT8) RETURNS INT8 LANGUAGE SQL AS $$ SELECT a + b; $$")
	if err != nil || drop != "DROP FUNCTION IF EXISTS public.add(INT8, INT8);" {
		t.Fatalf("drop function: %s %v", drop, err)
	}
}

func TestQuotedObjectNamesStaySingleTokens(t *testing.T) {
	sql := `CREATE TABLE "public"."table name.with.dot" ("id" INT PRIMARY KEY);`
	if got := objectName("table", sql); got != `"public"."table name.with.dot"` {
		t.Fatalf("object name %q", got)
	}
	drop, err := dropSequenceSQL(Statement{Object: `"public"."sequence.name"`})
	if err != nil || drop != `DROP SEQUENCE IF EXISTS "public"."sequence.name"` {
		t.Fatalf("drop sequence: %s %v", drop, err)
	}
	if got := quotedObjectName(`"public"."table name.with.dot"`, 2); got != `"public"."table name.with.dot"` {
		t.Fatalf("quoted object name %q", got)
	}
}

func TestRoutineNameWithParenthesisIsNotSplitEarly(t *testing.T) {
	got, err := dropRoutineStatement("function", `CREATE FUNCTION "public"."fn(name)"(IN value INT) RETURNS INT LANGUAGE SQL AS 'SELECT value'`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `DROP FUNCTION IF EXISTS "public"."fn(name)"(INT);` {
		t.Fatalf("drop routine: %s", got)
	}
}

func TestZoneMatchIsExact(t *testing.T) {
	selected := map[string]bool{"shop": true}
	manifest := Manifest{Databases: []string{"shop"}}
	if zoneInScope(ZoneStatement{Object: "shopping.public.t"}, selected, manifest) {
		t.Fatal("shop matched shopping")
	}
	if zoneInScope(ZoneStatement{Object: "DATABASE shopping"}, selected, manifest) {
		t.Fatal("shop matched DATABASE shopping")
	}
	if !zoneInScope(ZoneStatement{Database: "shop", Object: "TABLE shop.public.users"}, selected, manifest) {
		t.Fatal("database field did not match shop")
	}
	if zoneInScope(ZoneStatement{Database: "shopping", Object: "TABLE shopping.public.t"}, selected, manifest) {
		t.Fatal("database field matched the wrong database")
	}
	if !zoneInScope(ZoneStatement{Object: "shop.public.users"}, selected, manifest) {
		t.Fatal("qualified shop object was skipped")
	}
}

func TestLatestPointerDoesNotMoveBackward(t *testing.T) {
	if !newerBackup("20261008T120000Z", "") {
		t.Fatal("first pointer should publish")
	}
	if newerBackup("20261008T110000Z", "20261008T120000Z") {
		t.Fatal("older timestamp replaced a newer one")
	}
	if newerBackup("20261008T120000Z", "20261008T120000Z") {
		t.Fatal("equal timestamp should not replace the pointer")
	}
	dir := t.TempDir()
	store := &localStore{root: dir}
	ctx := context.Background()
	newer := LatestPointer{FormatVersion: 1, Name: "lab", Timestamp: "20261008T120000Z", Complete: true}
	older := LatestPointer{FormatVersion: 1, Name: "lab", Timestamp: "20261008T110000Z", Complete: true}
	if err := store.putLatest(ctx, "lab/latest.json", newer); err != nil {
		t.Fatal(err)
	}
	if err := store.putLatest(ctx, "lab/latest.json", older); err != nil {
		t.Fatal(err)
	}
	got, ok := readLatestPointer(ctx, store, "lab/latest.json")
	if !ok || got.Timestamp != newer.Timestamp {
		t.Fatalf("latest is %#v", got)
	}
}

func TestAbortDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	store := &localStore{root: dir}
	wc, err := store.Create(context.Background(), "lab/20261008T120000Z/data/t.pgcopy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wc.Write([]byte("1\t\\N\n")); err != nil {
		t.Fatal(err)
	}
	hw := newHashWriteCloser(wc)
	if _, err := finishCopy(nil, hw, context.Canceled); err == nil {
		t.Fatal("expected the copy error")
	}
	final := filepath.Join(dir, "lab", "20261008T120000Z", "data", "t.pgcopy")
	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("partial file was published: %v", err)
	}
}

func TestArraySelectList(t *testing.T) {
	list, err := selectList([]columnInfo{
		{Name: "id", TypeName: "int8", FormatType: "bigint"},
		{Name: "tags", TypeName: "_text", FormatType: "text[]"},
		{Name: "when", TypeName: "_timestamptz", FormatType: "timestamp with time zone[]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, `"id"`) || strings.Contains(list, "unnest(\"id\")") {
		t.Fatal(list)
	}
	if !strings.Contains(list, `unnest("tags")`) || !strings.Contains(list, "::text[]") {
		t.Fatal(list)
	}
	if !strings.Contains(list, "TIMESTAMPTZ[]") {
		t.Fatal(list)
	}
	if _, err := selectList([]columnInfo{{Name: "c", TypeName: "_char", FormatType: `"char"[]`}}); err == nil {
		t.Fatal("expected an unsupported array type to fail")
	}
}

func TestScrubSecrets(t *testing.T) {
	in := "s3://bucket/key?AWS_ACCESS_KEY_ID=AKIASECRET&AWS_SECRET_ACCESS_KEY=supersecret&AWS_SESSION_TOKEN=sessiontoken"
	out := scrubSecrets(in)
	for _, secret := range []string{"AKIASECRET", "supersecret", "sessiontoken"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret remains in %s", out)
		}
	}
}

func TestSplitBoundQueryUsesKeyset(t *testing.T) {
	first := splitBoundQuery("shop.public.events", "id", "", 20000)
	if strings.Contains(first, "WHERE") || !strings.Contains(first, "OFFSET 20000") {
		t.Fatal(first)
	}
	next := splitBoundQuery("shop.public.events", "id", "20000", 20000)
	if !strings.Contains(next, `WHERE src."id" >= 20000`) || !strings.Contains(next, "OFFSET 20000") {
		t.Fatal(next)
	}
	if !strings.Contains(next, `ORDER BY src."id"`) || strings.Contains(next, `ORDER BY "id"`) {
		t.Fatal(next)
	}
	if strings.Contains(next, "OFFSET 40000") {
		t.Fatal(next)
	}
}

func TestArrayCopyRewrite(t *testing.T) {
	ints, err := sqlArrayToPostgres(`ARRAY['1',NULL,'2']::bigint[]`)
	if err != nil || ints != `{"1",NULL,"2"}` {
		t.Fatalf("%s %v", ints, err)
	}
	words, err := sqlArrayToPostgres(`ARRAY['a',NULL,'b,c',e'd\'e',e'a\\b']::text[]`)
	if err != nil || words != `{"a",NULL,"b,c","d'e","a\\b"}` {
		t.Fatalf("%s %v", words, err)
	}
	empty, err := sqlArrayToPostgres(`ARRAY[]::bigint[]`)
	if err != nil || empty != `{}` {
		t.Fatalf("%s %v", empty, err)
	}
	literal := `ARRAY['a',NULL,'b,c',e'd\'e',e'a\\b']::text[]`
	line := "1\t" + escapePGCopy(literal) + "\t" + escapePGCopy(`ARRAY[1]::int`) + "\n"
	got, err := rewriteArrayLine([]byte(line), []string{"", "text[]", ""})
	if err != nil {
		t.Fatal(err)
	}
	want := "1\t" + escapePGCopy(words) + "\t" + escapePGCopy(`ARRAY[1]::int`) + "\n"
	if string(got) != want {
		t.Fatalf("rewritten row\n%s\nwant\n%s", got, want)
	}
	nullRow, err := rewriteArrayLine([]byte("3\t\\N\t\\N\n"), []string{"", "bigint[]", "text[]"})
	if err != nil || string(nullRow) != "3\t\\N\t\\N\n" {
		t.Fatalf("null row %q %v", nullRow, err)
	}
}

func TestViewDropIsReverseDependencyOrder(t *testing.T) {
	objects := ObjectsFile{Statements: []Statement{
		{Database: "shop", Kind: "view", Object: "public.v1"},
		{Database: "shop", Kind: "materialized_view", Object: "public.mv"},
		{Database: "shop", Kind: "view", Object: "public.plain"},
	}}
	matched := matchingStatements(objects, "shop", []string{"view", "materialized_view"})
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	var names []string
	for _, st := range matched {
		names = append(names, st.Object)
	}
	if strings.Join(names, ",") != "public.plain,public.mv,public.v1" {
		t.Fatal(names)
	}
}

func TestIncompatibleErrorDoesNotBlameSyntax(t *testing.T) {
	err := incompatibleError([]Problem{{
		Object: "public.plain",
		Kind:   "view",
		Error:  `ERROR: relation "mv" does not exist`,
	}})
	if strings.Contains(err.Error(), "syntax") {
		t.Fatal(err)
	}
	if !strings.Contains(err.Error(), "public.plain") || !strings.Contains(err.Error(), "does not exist") {
		t.Fatal(err)
	}
}

func TestRelationKeyUsesSchemaAndName(t *testing.T) {
	if got := relationKey("shop.public.users"); got != "public.users" {
		t.Fatal(got)
	}
	if got := relationKey(`"public"."Users"`); got != "public.Users" {
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
