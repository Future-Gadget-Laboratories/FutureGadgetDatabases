// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAlwaysIdentityIsNamed(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	dir := filepath.Join(base, "node")
	addr := "127.0.0.1:26277"
	startNode(t, bin, dir, addr, "127.0.0.1:18087", true)
	url := "postgresql://root@" + addr + "/defaultdb?sslmode=disable"
	sql(t, bin, addr, true, `
CREATE DATABASE identfail;
CREATE TABLE identfail.public.locked (
  id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name STRING
);
INSERT INTO identfail.public.locked (name) VALUES ('a');
`)
	_, err := runToolErr(tool, "backup", "--json", "--url", url, "--dest", filepath.Join(base, "out"), "--database", "identfail", "--name", "ident")
	if err == nil {
		t.Fatal("backup of a GENERATED ALWAYS identity column succeeded")
	}
	if !strings.Contains(err.Error(), "identfail.public.locked.id") {
		t.Fatalf("error did not name the column: %v", err)
	}
}

func TestSignalRevertsTTLAndSkipsLatest(t *testing.T) {
	bin := cockroachBin(t)
	tool := buildTool(t)
	base := t.TempDir()
	dir := filepath.Join(base, "node")
	addr := "127.0.0.1:26279"
	startNode(t, bin, dir, addr, "127.0.0.1:18089", true)
	url := "postgresql://root@" + addr + "/defaultdb?sslmode=disable"
	sql(t, bin, addr, true, `
CREATE DATABASE sig;
CREATE TABLE sig.public.wide (id INT PRIMARY KEY, payload STRING);
INSERT INTO sig.public.wide SELECT i, repeat('x', 200) FROM generate_series(1, 80000) AS i;
ALTER DATABASE sig CONFIGURE ZONE USING gc.ttlseconds = 30;
`)
	dest := filepath.Join(base, "backups")
	cmd := exec.Command(tool, "backup", "--json", "--url", url, "--dest", dest, "--database", "sig", "--name", "sig", "--extend-gc-ttl", "1h")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(45 * time.Second)
	raised := false
	for time.Now().Before(deadline) {
		out := sqlOut(t, bin, addr, true, `SELECT raw_config_sql FROM crdb_internal.zones WHERE database_name = 'sig' AND table_name IS NULL;`)
		if strings.Contains(out, "3600") {
			raised = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !raised {
		t.Fatalf("gc.ttlseconds was not raised\nstderr:\n%s", stderr.String())
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("interrupted backup exited 0\n%s", stdout.String())
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("backup did not exit after SIGTERM")
	}
	ttl := sqlOut(t, bin, addr, true, `SELECT raw_config_sql FROM crdb_internal.zones WHERE database_name = 'sig' AND table_name IS NULL;`)
	if !strings.Contains(ttl, "gc.ttlseconds = 30") {
		t.Fatalf("ttl left raised after signal:\n%s\nstderr:\n%s", ttl, stderr.String())
	}
	if strings.Contains(ttl, "3600") {
		t.Fatalf("raised ttl is still in place:\n%s", ttl)
	}
	latest := filepath.Join(dest, "sig", "latest.json")
	if _, err := os.Stat(latest); !os.IsNotExist(err) {
		body, _ := os.ReadFile(latest)
		t.Fatalf("killed backup published latest.json: %s", body)
	}
}
