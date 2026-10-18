// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt

package main

import (
	"context"
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

type appBackup struct {
	bin, addr, dbURL, src string
}

type backupStep struct {
	name string
	run  func(*testing.T, appBackup)
}

// TestInPlaceImportListenerRefusalKeepsRows drives cmdRestore, the real
// restore path, against a disposable single-node cluster. A refused import
// listener must not commit the in-place drop.
func TestInPlaceImportListenerRefusalKeepsRows(t *testing.T) {
	fx := startAppBackupAt(t, "127.0.0.1:26331", "127.0.0.1:18131")
	for _, step := range appBackupSteps() {
		if t.Failed() {
			return
		}
		t.Run(step.name, func(t *testing.T) {
			step.run(t, fx)
		})
	}
}

func appBackupSteps() []backupStep {
	return []backupStep{
		{name: "wildcard_bind", run: refuseWildcardKeepsRows},
		{name: "resolved_wildcard_close_failure", run: refuseResolvedClose},
		{name: "occupied_port", run: occupiedPortKeepsRows},
		{name: "bad_tls", run: badTLSKeepsRows},
		{name: "plan_wildcard", run: planWildcardKeepsRows},
		{name: "plan_releases_listener", run: planReleasesListener},
		{name: "copy_ignores_wildcard", run: copyIgnoresWildcard},
		{name: "in_place_import", run: inPlaceImportSucceeds},
		{name: "swap", run: swapKeepsPreviousRows},
	}
}

func startAppBackupAt(t *testing.T, addr, httpAddr string) appBackup {
	t.Helper()
	bin := cockroachBin(t)
	base := t.TempDir()
	startNode(t, bin, filepath.Join(base, "node"), addr, httpAddr, true)
	fx := appBackup{
		bin:   bin,
		addr:  addr,
		dbURL: "postgresql://root@" + addr + "/defaultdb?sslmode=disable",
	}
	sql(t, bin, addr, true, `
		CREATE DATABASE app;
		CREATE TABLE app.public.item (id INT PRIMARY KEY, note STRING NOT NULL);
		INSERT INTO app.public.item VALUES (1, 'sentinel');
	`)
	fx.src = backupApp(t, fx, base)
	markLive(t, bin, addr)
	return fx
}

func backupApp(t *testing.T, fx appBackup, base string) string {
	t.Helper()
	dest := filepath.Join(base, "backups")
	code, text := runQuiet(t, func() int {
		return cmdBackup([]string{"--url", fx.dbURL, "--dest", dest, "--database", "app", "--name", "app"})
	})
	if code != 0 {
		t.Fatalf("backup exit %d\n%s", code, text)
	}
	return filepath.Join(dest, "app", "latest")
}

func refuseWildcardKeepsRows(t *testing.T, fx appBackup) {
	t.Helper()
	code, text := runQuiet(t, func() int {
		return cmdRestore(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", "0.0.0.0:0"))
	})
	if wantRefusal(code, text, "--serve-advertise") {
		t.Fatalf("exit %d, want refusal\n%s", code, text)
	}
	assertLiveTarget(t, fx)
}

func refuseResolvedClose(t *testing.T, fx appBackup) {
	t.Helper()
	stubResolvedListen(t, errors.New("close broke"))
	code, text := runQuiet(t, func() int {
		return cmdRestore(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", "wild4.example:8080"))
	})
	if wantRefusal(code, text, "--serve-advertise", "close broke") {
		t.Fatalf("exit %d, want close failure with the wildcard refusal\n%s", code, text)
	}
	assertLiveTarget(t, fx)
}

func occupiedPortKeepsRows(t *testing.T, fx appBackup) {
	t.Helper()
	ln := mustListen(t, "127.0.0.1:0")
	defer ln.Close()
	code, text := runQuiet(t, func() int {
		return cmdRestore(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", ln.Addr().String()))
	})
	if wantRefusal(code, text, "listen for IMPORT") {
		t.Fatalf("exit %d, want bind failure\n%s", code, text)
	}
	assertLiveTarget(t, fx)
}

func badTLSKeepsRows(t *testing.T, fx appBackup) {
	t.Helper()
	cert, key := writeBadTLS(t)
	code, text := runQuiet(t, func() int {
		return cmdRestore(inPlaceImportArgs(fx.dbURL, fx.src,
			"--serve-addr", "127.0.0.1:0",
			"--serve-tls-cert", cert,
			"--serve-tls-key", key,
		))
	})
	if wantRefusal(code, text, "TLS") {
		t.Fatalf("exit %d, want TLS failure\n%s", code, text)
	}
	assertLiveTarget(t, fx)
}

func planWildcardKeepsRows(t *testing.T, fx appBackup) {
	t.Helper()
	code, stdout, stderr := capturePlan(t, append(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", "0.0.0.0:0"), "--plan"))
	if code != 4 || missingAny(stdout, "REFUSE", "--serve-advertise", "nothing changed") {
		t.Fatalf("plan exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertLiveTarget(t, fx)
}

func planReleasesListener(t *testing.T, fx appBackup) {
	t.Helper()
	bind := reserveLoopback(t)
	code, stdout, _ := capturePlan(t, append(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", bind), "--plan"))
	if code != 0 || missingAny(stdout, "import listener "+bind, "nothing changed") {
		t.Fatalf("plan exit %d\n%s", code, stdout)
	}
	if err := rebind(bind); err != nil {
		t.Fatalf("plan left %s bound: %v", bind, err)
	}
	assertLiveTarget(t, fx)
}

func copyIgnoresWildcard(t *testing.T, fx appBackup) {
	t.Helper()
	// COPY does not serve files, so a wildcard bind is not a restore error.
	// This is a restore into an empty target. The database has to be dropped
	// first because CREATE DATABASE of a name that already exists aborts the
	// transaction that in-place COPY uses.
	sql(t, fx.bin, fx.addr, true, `DROP DATABASE app CASCADE`)
	code, text := runQuiet(t, func() int {
		return cmdRestore([]string{
			"--url", fx.dbURL, "--src", fx.src, "--load=copy", "--serve-addr", "0.0.0.0:1",
		})
	})
	if code != 0 {
		t.Fatalf("copy exit %d\n%s", code, text)
	}
	assertSentinel(t, fx)
}

func inPlaceImportSucceeds(t *testing.T, fx appBackup) {
	t.Helper()
	markLive(t, fx.bin, fx.addr)
	code, text := runQuiet(t, func() int {
		return cmdRestore(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", "0.0.0.0:0", "--serve-advertise", "127.0.0.1"))
	})
	if code != 0 {
		t.Fatalf("in-place import exit %d\n%s", code, text)
	}
	assertSentinel(t, fx)
}

func swapKeepsPreviousRows(t *testing.T, fx appBackup) {
	t.Helper()
	markLive(t, fx.bin, fx.addr)
	code, text := runQuiet(t, func() int {
		return cmdRestore([]string{"--url", fx.dbURL, "--src", fx.src, "--force=swap", "--load=import"})
	})
	if code != 0 {
		t.Fatalf("swap exit %d\n%s", code, text)
	}
	assertSentinel(t, fx)
	assertOldCopyKept(t, fx)
}

func TestPlanCloseErrorLeavesRows(t *testing.T) {
	fx := startAppBackupAt(t, "127.0.0.1:26333", "127.0.0.1:18133")
	stubFixedListener(t, net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	code, stdout, stderr := capturePlan(t, append(inPlaceImportArgs(fx.dbURL, fx.src, "--serve-addr", "192.0.2.10:9"), "--plan"))
	if code == 4 {
		t.Fatalf("close failure was reported as a plan refusal\n%s\n%s", stdout, stderr)
	}
	if code == 0 || !strings.Contains(stderr, "close broke") {
		t.Fatalf("plan close exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertLiveTarget(t, fx)
}

func TestHeldListenerCloseErrorSkipsDrop(t *testing.T) {
	fx := startAppBackupAt(t, "127.0.0.1:26335", "127.0.0.1:18135")
	ctx := context.Background()
	db := mustConnect(t, ctx, fx.dbURL)
	defer db.Close(ctx)
	stubFixedListener(t, net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	_, err := restoreKeepingListener(ctx, db, appBundle(t), map[string]bool{"app": true}, importOpt("192.0.2.10:9"), RestoreResult{})
	if err == nil || !strings.Contains(err.Error(), "close broke") || !strings.Contains(err.Error(), "refusing to restore") {
		t.Fatalf("held close error = %v", err)
	}
	assertLiveTarget(t, fx)
}

func TestTestingProbeFailureClosesListener(t *testing.T) {
	fx := startAppBackupAt(t, "127.0.0.1:26337", "127.0.0.1:18137")
	ctx := context.Background()
	db := mustConnect(t, ctx, fx.dbURL)
	defer db.Close(ctx)
	abortTransaction(t, ctx, db)
	ln := fixedListener(net.ParseIP("192.0.2.10"), 9, errors.New("close broke"))
	_, err := loadImport(ctx, importRequest{
		db:       db,
		store:    &localStore{root: t.TempDir()},
		opt:      RestoreOptions{TestingMode: true, Load: "import"},
		listener: ln,
	})
	if err == nil || !strings.Contains(err.Error(), "close broke") {
		t.Fatalf("probe close error = %v", err)
	}
	if !ln.closed {
		t.Fatal("listener left open after the testing probe failed")
	}
}

func inPlaceImportArgs(dbURL, src string, extra ...string) []string {
	args := []string{"--url", dbURL, "--src", src, "--force=in-place", "--load=import"}
	return append(args, extra...)
}

func importOpt(addr string) RestoreOptions {
	return RestoreOptions{Load: "import", ServeAddr: addr, ImportListen: "127.0.0.1:0"}
}

func appBundle(t *testing.T) restoreBundle {
	t.Helper()
	return restoreBundle{
		store: &localStore{root: t.TempDir()},
		objects: ObjectsFile{Databases: []NamedSQL{{
			Name: "app",
			SQL:  "CREATE DATABASE app;",
		}}},
	}
}

func mustConnect(t *testing.T, ctx context.Context, dbURL string) *database {
	t.Helper()
	db, err := connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func abortTransaction(t *testing.T, ctx context.Context, db *database) {
	t.Helper()
	if _, err := db.exec(ctx, "BEGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.exec(ctx, "SELECT 1/0"); err == nil {
		t.Fatal("expected the transaction to abort")
	}
}

func markLive(t *testing.T, bin, addr string) {
	t.Helper()
	sql(t, bin, addr, true, `
		ALTER TABLE app.public.item ADD COLUMN marker STRING;
		UPDATE app.public.item SET note = 'live-row', marker = 'kept' WHERE id = 1;
	`)
}

func assertLiveTarget(t *testing.T, fx appBackup) {
	t.Helper()
	rows := sqlOut(t, fx.bin, fx.addr, true, `SELECT note, marker FROM app.public.item`)
	if !strings.Contains(rows, "live-row") || !strings.Contains(rows, "kept") {
		t.Fatalf("live rows changed:\n%s", rows)
	}
	schema := sqlDBOut(t, fx.bin, fx.addr, "app", `SELECT create_statement FROM [SHOW CREATE TABLE item]`)
	if !strings.Contains(schema, "marker") {
		t.Fatalf("schema changed:\n%s", schema)
	}
}

func assertSentinel(t *testing.T, fx appBackup) {
	t.Helper()
	rows := sqlOut(t, fx.bin, fx.addr, true, `SELECT note FROM app.public.item`)
	if !strings.Contains(rows, "sentinel") {
		t.Fatalf("restored rows:\n%s", rows)
	}
	schema := sqlDBOut(t, fx.bin, fx.addr, "app", `SELECT create_statement FROM [SHOW CREATE TABLE item]`)
	if strings.Contains(schema, "marker") {
		t.Fatalf("restored schema still has the live column:\n%s", schema)
	}
}

func assertOldCopyKept(t *testing.T, fx appBackup) {
	t.Helper()
	old := oldCopyName(t, fx.bin, fx.addr)
	note := sqlOut(t, fx.bin, fx.addr, true, `SELECT note, marker FROM `+quoteIdent(old)+`.public.item`)
	if !strings.Contains(note, "live-row") || !strings.Contains(note, "kept") {
		t.Fatalf("swap dropped the previous rows:\n%s", note)
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

func wantRefusal(code int, text string, parts ...string) bool {
	return code == 0 || missingAny(text, parts...)
}

func missingAny(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return true
		}
	}
	return false
}

func capturePlan(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var code int
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = cmdRestore(args)
		})
	})
	return code, stdout, stderr
}

func mustListen(t *testing.T, addr string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func reserveLoopback(t *testing.T) string {
	t.Helper()
	ln := mustListen(t, "127.0.0.1:0")
	bind := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return bind
}

func rebind(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ln.Close()
}

func writeBadTLS(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cert := filepath.Join(dir, "bad.crt")
	key := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(cert, []byte("not a cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func stubResolvedListen(t *testing.T, closeErr error) {
	t.Helper()
	prev := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return listenResolvedOrReal(network, address, closeErr)
	}
	t.Cleanup(func() { listenTCP = prev })
}

func stubFixedListener(t *testing.T, ip net.IP, port int, closeErr error) {
	t.Helper()
	prev := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return fixedListener(ip, port, closeErr), nil
	}
	t.Cleanup(func() { listenTCP = prev })
}

func fixedListener(ip net.IP, port int, closeErr error) *closeErrListener {
	return &closeErrListener{addr: &net.TCPAddr{IP: ip, Port: port}, err: closeErr}
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
	return fixedListener(net.IPv4zero, n, closeErr), nil
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
