// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServedImportSkipsCopyAndImplicitS3(t *testing.T) {
	local := &localStore{}
	if servedImport(local, RestoreOptions{Load: "copy"}) {
		t.Fatal("COPY should not open an import listener")
	}
	if !servedImport(local, RestoreOptions{Load: "import"}) {
		t.Fatal("local IMPORT should open an import listener")
	}
	if servedImport(&s3Store{importAuth: "implicit"}, RestoreOptions{Load: "import"}) {
		t.Fatal("implicit S3 should not open an import listener")
	}
	if !servedImport(&s3Store{importAuth: "served"}, RestoreOptions{Load: "import"}) {
		t.Fatal("served S3 should open an import listener")
	}
}

// TestInPlaceImportListenerRefusalKeepsRows drives cmdRestore, the real
// restore path, against a disposable single-node cluster. A refused import
// listener must not commit the in-place drop.
func TestInPlaceImportListenerRefusalKeepsRows(t *testing.T) {
	bin := cockroachBin(t)
	base := t.TempDir()
	addr := "127.0.0.1:26331"
	startNode(t, bin, filepath.Join(base, "node"), addr, "127.0.0.1:18131", true)
	dbURL := "postgresql://root@" + addr + "/defaultdb?sslmode=disable"
	sql(t, bin, addr, true, `
		CREATE DATABASE app;
		CREATE TABLE app.public.item (id INT PRIMARY KEY, note STRING NOT NULL);
		INSERT INTO app.public.item VALUES (1, 'sentinel');
	`)
	dest := filepath.Join(base, "backups")
	code, text := runQuiet(t, func() int {
		return cmdBackup([]string{"--url", dbURL, "--dest", dest, "--database", "app", "--name", "app"})
	})
	if code != 0 {
		t.Fatalf("backup exit %d\n%s", code, text)
	}
	src := filepath.Join(dest, "app", "latest")
	markLive(t, bin, addr)

	t.Run("wildcard_bind", func(t *testing.T) {
		code, text := runQuiet(t, func() int {
			return cmdRestore(inPlaceImportArgs(dbURL, src, "--serve-addr", "0.0.0.0:0"))
		})
		if code == 0 || !strings.Contains(text, "--serve-advertise") {
			t.Fatalf("exit %d, want refusal\n%s", code, text)
		}
		assertLiveTarget(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	t.Run("resolved_wildcard_close_failure", func(t *testing.T) {
		prev := listenTCP
		listenTCP = func(network, address string) (net.Listener, error) {
			return listenResolvedOrReal(network, address, errors.New("close broke"))
		}
		defer func() { listenTCP = prev }()
		code, text := runQuiet(t, func() int {
			return cmdRestore(inPlaceImportArgs(dbURL, src, "--serve-addr", "wild4.example:8080"))
		})
		if code == 0 || !strings.Contains(text, "--serve-advertise") || !strings.Contains(text, "close broke") {
			t.Fatalf("exit %d, want close failure with the wildcard refusal\n%s", code, text)
		}
		assertLiveTarget(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	t.Run("occupied_port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		code, text := runQuiet(t, func() int {
			return cmdRestore(inPlaceImportArgs(dbURL, src, "--serve-addr", ln.Addr().String()))
		})
		if code == 0 || !strings.Contains(text, "listen for IMPORT") {
			t.Fatalf("exit %d, want bind failure\n%s", code, text)
		}
		assertLiveTarget(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	t.Run("bad_tls", func(t *testing.T) {
		dir := t.TempDir()
		cert := filepath.Join(dir, "bad.crt")
		key := filepath.Join(dir, "bad.key")
		if err := os.WriteFile(cert, []byte("not a cert"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(key, []byte("not a key"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, text := runQuiet(t, func() int {
			return cmdRestore(inPlaceImportArgs(dbURL, src,
				"--serve-addr", "127.0.0.1:0",
				"--serve-tls-cert", cert,
				"--serve-tls-key", key,
			))
		})
		if code == 0 || !strings.Contains(text, "TLS") {
			t.Fatalf("exit %d, want TLS failure\n%s", code, text)
		}
		assertLiveTarget(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	t.Run("plan_wildcard", func(t *testing.T) {
		var code int
		var stderr string
		stdout := captureStdout(t, func() {
			stderr = captureStderr(t, func() {
				code = cmdRestore(append(inPlaceImportArgs(dbURL, src, "--serve-addr", "0.0.0.0:0"), "--plan"))
			})
		})
		if code != 4 || !strings.Contains(stdout, "REFUSE") || !strings.Contains(stdout, "--serve-advertise") || !strings.Contains(stdout, "nothing changed") {
			t.Fatalf("plan exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		assertLiveTarget(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	t.Run("plan_releases_listener", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		bind := ln.Addr().String()
		if err := ln.Close(); err != nil {
			t.Fatal(err)
		}
		var code int
		stdout := captureStdout(t, func() {
			code = cmdRestore(append(inPlaceImportArgs(dbURL, src, "--serve-addr", bind), "--plan"))
		})
		if code != 0 || !strings.Contains(stdout, "import listener "+bind) || !strings.Contains(stdout, "nothing changed") {
			t.Fatalf("plan exit %d\n%s", code, stdout)
		}
		again, err := net.Listen("tcp", bind)
		if err != nil {
			t.Fatalf("plan left %s bound: %v", bind, err)
		}
		again.Close()
		assertLiveTarget(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	t.Run("copy_ignores_wildcard", func(t *testing.T) {
		// COPY does not serve files, so a wildcard bind is not a restore error.
		// This is a restore into an empty target. The database has to be
		// dropped first because CREATE DATABASE of a name that already exists
		// aborts the transaction that in-place COPY uses.
		sql(t, bin, addr, true, `DROP DATABASE app CASCADE`)
		code, text := runQuiet(t, func() int {
			return cmdRestore([]string{
				"--url", dbURL, "--src", src, "--load=copy", "--serve-addr", "0.0.0.0:1",
			})
		})
		if code != 0 {
			t.Fatalf("copy exit %d\n%s", code, text)
		}
		assertSentinel(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	markLive(t, bin, addr)
	t.Run("in_place_import", func(t *testing.T) {
		code, text := runQuiet(t, func() int {
			return cmdRestore(inPlaceImportArgs(dbURL, src, "--serve-addr", "0.0.0.0:0", "--serve-advertise", "127.0.0.1"))
		})
		if code != 0 {
			t.Fatalf("in-place import exit %d\n%s", code, text)
		}
		assertSentinel(t, bin, addr)
	})
	if t.Failed() {
		return
	}

	markLive(t, bin, addr)
	t.Run("swap", func(t *testing.T) {
		code, text := runQuiet(t, func() int {
			return cmdRestore([]string{"--url", dbURL, "--src", src, "--force=swap", "--load=import"})
		})
		if code != 0 {
			t.Fatalf("swap exit %d\n%s", code, text)
		}
		assertSentinel(t, bin, addr)
		old := oldCopyName(t, bin, addr)
		note := sqlOut(t, bin, addr, true, `SELECT note, marker FROM `+quoteIdent(old)+`.public.item`)
		if !strings.Contains(note, "live-row") || !strings.Contains(note, "kept") {
			t.Fatalf("swap dropped the previous rows:\n%s", note)
		}
	})
}

func inPlaceImportArgs(dbURL, src string, extra ...string) []string {
	args := []string{"--url", dbURL, "--src", src, "--force=in-place", "--load=import"}
	return append(args, extra...)
}

func markLive(t *testing.T, bin, addr string) {
	t.Helper()
	sql(t, bin, addr, true, `
		ALTER TABLE app.public.item ADD COLUMN marker STRING;
		UPDATE app.public.item SET note = 'live-row', marker = 'kept' WHERE id = 1;
	`)
}

func assertLiveTarget(t *testing.T, bin, addr string) {
	t.Helper()
	rows := sqlOut(t, bin, addr, true, `SELECT note, marker FROM app.public.item`)
	if !strings.Contains(rows, "live-row") || !strings.Contains(rows, "kept") {
		t.Fatalf("live rows changed:\n%s", rows)
	}
	schema := sqlDBOut(t, bin, addr, "app", `SELECT create_statement FROM [SHOW CREATE TABLE item]`)
	if !strings.Contains(schema, "marker") {
		t.Fatalf("schema changed:\n%s", schema)
	}
}

func assertSentinel(t *testing.T, bin, addr string) {
	t.Helper()
	rows := sqlOut(t, bin, addr, true, `SELECT note FROM app.public.item`)
	if !strings.Contains(rows, "sentinel") {
		t.Fatalf("restored rows:\n%s", rows)
	}
	schema := sqlDBOut(t, bin, addr, "app", `SELECT create_statement FROM [SHOW CREATE TABLE item]`)
	if strings.Contains(schema, "marker") {
		t.Fatalf("restored schema still has the live column:\n%s", schema)
	}
}

func oldCopyName(t *testing.T, bin, addr string) string {
	t.Helper()
	out := sqlOut(t, bin, addr, true, `SELECT database_name FROM [SHOW DATABASES] WHERE database_name LIKE 'app!_!_fgdb!_old!_%' ESCAPE '!'`)
	for _, field := range strings.Fields(out) {
		if strings.Contains(field, "__fgdb_old_") {
			return field
		}
	}
	t.Fatalf("old copy missing:\n%s", out)
	return ""
}

func listenResolvedOrReal(network, address string, closeErr error) (net.Listener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host != "wild4.example" {
		return net.Listen(network, address)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	return &closeErrListener{addr: &net.TCPAddr{IP: net.IPv4zero, Port: n}, err: closeErr}, nil
}

type closeErrListener struct {
	addr   *net.TCPAddr
	err    error
	closed bool
}

func (l *closeErrListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *closeErrListener) Close() error {
	l.closed = true
	return l.err
}
func (l *closeErrListener) Addr() net.Addr { return l.addr }
