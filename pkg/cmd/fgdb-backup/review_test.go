// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForceDropsDependentViews(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	srcURL, dstURL, _, dstAddr := startNamedPair(t, bin, base, "127.0.0.1:26281", "127.0.0.1:26283", "127.0.0.1:18111", "127.0.0.1:18113")
	sql(t, bin, hostOf(srcURL), true, `
CREATE DATABASE shop;
CREATE TABLE shop.public.t (id INT PRIMARY KEY, n INT);
INSERT INTO shop.public.t VALUES (1, 10);
CREATE VIEW shop.public.v1 AS SELECT id, n FROM shop.public.t;
CREATE VIEW shop.public.v2 AS SELECT id, n FROM shop.public.v1;
`)
	dest := filepath.Join(base, "backups")
	runTool(t, tool, "backup", "--json", "--url", srcURL, "--dest", dest, "--database", "shop", "--name", "views")
	src := filepath.Join(dest, "views", "latest")
	runTool(t, tool, "restore", "--json", "--url", dstURL, "--src", src)
	runTool(t, tool, "restore", "--json", "--force=in-place", "--url", dstURL, "--src", src)
	got := strings.TrimSpace(sqlOut(t, bin, dstAddr, true, `SELECT id, n FROM shop.public.v2;`))
	if !strings.Contains(got, "1") || !strings.Contains(got, "10") {
		t.Fatalf("second --force did not restore the dependent view: %s", got)
	}
}

func TestPlainViewOverMaterializedView(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	srcURL, dstURL, srcAddr, dstAddr := startNamedPair(t, bin, base, "127.0.0.1:26285", "127.0.0.1:26287", "127.0.0.1:18115", "127.0.0.1:18117")
	sql(t, bin, srcAddr, true, `
CREATE DATABASE shop;
CREATE TABLE shop.public.t (id INT PRIMARY KEY, n INT);
INSERT INTO shop.public.t VALUES (1, 7);
`)
	sqlDB(t, bin, srcAddr, "shop", `
CREATE MATERIALIZED VIEW mv AS SELECT id, n FROM t;
CREATE VIEW plain AS SELECT id, n FROM mv;
`)
	dest := filepath.Join(base, "backups")
	runTool(t, tool, "backup", "--json", "--url", srcURL, "--dest", dest, "--database", "shop", "--name", "mv")
	runTool(t, tool, "restore", "--json", "--url", dstURL, "--src", filepath.Join(dest, "mv", "latest"))
	got := strings.TrimSpace(sqlOut(t, bin, dstAddr, true, `SELECT id, n FROM shop.public.plain;`))
	if !strings.Contains(got, "1") || !strings.Contains(got, "7") {
		t.Fatalf("plain view over a materialized view: %s", got)
	}
}

func TestSuccessfulSwapRestoreKeepsRollbackCopy(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	srcURL, dstURL, srcAddr, dstAddr := startNamedPair(t, bin, base, "127.0.0.1:26297", "127.0.0.1:26299", "127.0.0.1:18127", "127.0.0.1:18129")
	sql(t, bin, srcAddr, true, `
CREATE DATABASE shop;
CREATE TABLE shop.public.items (id INT PRIMARY KEY, value STRING);
INSERT INTO shop.public.items VALUES (1, 'from backup');
`)
	sql(t, bin, dstAddr, true, `
CREATE DATABASE shop;
CREATE TABLE shop.public.items (id INT PRIMARY KEY, value STRING);
INSERT INTO shop.public.items VALUES (1, 'old value');
`)
	dest := filepath.Join(base, "backups")
	runTool(t, tool, "backup", "--json", "--url", srcURL, "--dest", dest, "--database", "shop", "--name", "swap")
	runTool(t, tool, "restore", "--json", "--force", "--url", dstURL, "--src", filepath.Join(dest, "swap", "latest"))
	got := strings.TrimSpace(sqlOut(t, bin, dstAddr, true, `SELECT value FROM shop.public.items;`))
	if got != "from backup" {
		t.Fatalf("swapped database contains %q", got)
	}
	old := strings.TrimSpace(sqlOut(t, bin, dstAddr, true, `SELECT database_name FROM [SHOW DATABASES] WHERE database_name LIKE 'shop__fgdb_old_%';`))
	if old == "" {
		t.Fatal("successful swap did not retain an old database copy")
	}
}

func TestArrayColumnsBothLoadModes(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	srcURL, dstURL, srcAddr, dstAddr := startNamedPair(t, bin, base, "127.0.0.1:26289", "127.0.0.1:26291", "127.0.0.1:18119", "127.0.0.1:18121")
	sql(t, bin, srcAddr, true, `
CREATE DATABASE shop;
CREATE TABLE shop.public.arr (
  id INT PRIMARY KEY,
  nums INT[],
  words TEXT[]
);
INSERT INTO shop.public.arr VALUES
  (1, ARRAY[1, NULL, 2], ARRAY['a', NULL, 'b,c', 'd''e', e'a\\b']),
  (2, ARRAY[]::INT[], ARRAY[]::TEXT[]),
  (3, NULL, NULL);
`)
	want := strings.TrimSpace(sqlOut(t, bin, srcAddr, true, `SELECT id, nums::STRING, words::STRING FROM shop.public.arr ORDER BY id;`))
	dest := filepath.Join(base, "backups")
	runTool(t, tool, "backup", "--json", "--url", srcURL, "--dest", dest, "--database", "shop", "--name", "arr")
	src := filepath.Join(dest, "arr", "latest")
	for _, load := range []string{"import", "copy"} {
		sql(t, bin, dstAddr, true, `DROP DATABASE IF EXISTS shop CASCADE;`)
		runTool(t, tool, "restore", "--json", "--url", dstURL, "--src", src, "--load="+load)
		got := strings.TrimSpace(sqlOut(t, bin, dstAddr, true, `SELECT id, nums::STRING, words::STRING FROM shop.public.arr ORDER BY id;`))
		if got != want {
			t.Fatalf("--load=%s\nwant:\n%s\ngot:\n%s", load, want, got)
		}
	}
}

func TestSplitRowsNearRequestedSize(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	srcURL, dstURL, _, dstAddr := startNamedPair(t, bin, base, "127.0.0.1:26293", "127.0.0.1:26295", "127.0.0.1:18123", "127.0.0.1:18125")
	sql(t, bin, hostOf(srcURL), true, `
CREATE DATABASE shop;
CREATE TABLE shop.public.events (id INT PRIMARY KEY, payload STRING);
INSERT INTO shop.public.events SELECT i, 'x' FROM generate_series(1, 200) AS i;
`)
	dest := filepath.Join(base, "backups")
	out := runTool(t, tool, "backup", "--json", "--url", srcURL, "--dest", dest, "--database", "shop", "--name", "splits", "--split-rows", "50")
	var bres BackupResult
	if err := json.Unmarshal(out, &bres); err != nil {
		t.Fatalf("backup json: %v\n%s", err, out)
	}
	manifestPath := filepath.Join(bres.Backup, "manifest.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	var files []FileDigest
	for _, table := range manifest.Tables {
		if table.Name == "events" {
			files = table.Files
		}
	}
	if len(files) != 4 {
		t.Fatalf("200 rows at 50-row splits produced %d files", len(files))
	}
	var total int64
	for _, f := range files {
		total += f.RowCount
		if f.RowCount < 40 || f.RowCount > 60 {
			t.Fatalf("split file %s has %d rows, want about 50", f.Path, f.RowCount)
		}
	}
	if total != 200 {
		t.Fatalf("split files sum to %d rows", total)
	}
	runTool(t, tool, "restore", "--json", "--url", dstURL, "--src", filepath.Join(dest, "splits", "latest"))
	got := strings.TrimSpace(sqlOut(t, bin, dstAddr, true, `SELECT count(*), min(id), max(id) FROM shop.public.events;`))
	if !strings.Contains(got, "200") || !strings.Contains(got, "1") {
		t.Fatalf("restored split table: %s", got)
	}
}

func startNamedPair(t *testing.T, bin, base, srcAddr, dstAddr, srcHTTP, dstHTTP string) (srcURL, dstURL, srcHost, dstHost string) {
	t.Helper()
	srcDir := filepath.Join(base, "src")
	dstDir := filepath.Join(base, "dst")
	startNode(t, bin, srcDir, srcAddr, srcHTTP, true)
	startNode(t, bin, dstDir, dstAddr, dstHTTP, true)
	srcURL = "postgresql://root@" + srcAddr + "/defaultdb?sslmode=disable"
	dstURL = "postgresql://root@" + dstAddr + "/defaultdb?sslmode=disable"
	return srcURL, dstURL, srcAddr, dstAddr
}

func hostOf(pgURL string) string {
	rest := strings.TrimPrefix(pgURL, "postgresql://root@")
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}
