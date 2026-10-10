// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestUsageMentionsPlanExit(t *testing.T) {
	text := captureStderr(t, usage)
	for _, want := range []string{"0", "1", "2", "4", "--plan", "command line"} {
		if !strings.Contains(text, want) {
			t.Fatalf("usage missing %q:\n%s", want, text)
		}
	}
}

func TestFailIsRuntimeExit(t *testing.T) {
	var code int
	text := captureStderr(t, func() {
		code = fail(false, errors.New("config broke"))
	})
	if code != 1 {
		t.Fatalf("fail exit = %d", code)
	}
	if !strings.Contains(text, "config broke") {
		t.Fatalf("stderr = %s", text)
	}
}

func TestRestoreStatusCodes(t *testing.T) {
	if restoreStatus(&planRefusalError{message: "no"}) != 4 {
		t.Fatal("plan refusal should exit 4")
	}
	if restoreStatus(errors.New("restore preflight refused")) != 1 {
		t.Fatal("plain refusal should exit 1")
	}
}

func TestBadCommandLineExits2(t *testing.T) {
	if code := cmdBackup(nil); code != 2 {
		t.Fatalf("backup with no flags exit = %d", code)
	}
	if code := cmdPrune([]string{"--url", "postgres://root@127.0.0.1:1/db"}); code != 2 {
		t.Fatalf("prune missing --name exit = %d", code)
	}
	text := captureStderr(t, func() {
		if code := cmdRestore([]string{"-h"}); code != 2 {
			t.Fatalf("restore -h exit = %d", code)
		}
	})
	if !strings.Contains(text, helpTestingMode) {
		t.Fatalf("testing-mode help missing:\n%s", text)
	}
	if strings.Contains(strings.ToLower(helpTestingMode), "s3") {
		t.Fatalf("testing-mode help mentions S3: %s", helpTestingMode)
	}
}

func TestUnlockMissingDestOrName(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--dest", t.TempDir()},
		{"--name", "daily"},
	} {
		var code int
		text := captureStderr(t, func() {
			code = cmdUnlock(args)
		})
		if code != 2 {
			t.Fatalf("unlock %q exit = %d", args, code)
		}
		if !strings.Contains(text, "unlock requires --dest and --name") {
			t.Fatalf("unlock %q stderr = %s", args, text)
		}
	}
	var code int
	text := captureStderr(t, func() {
		code = cmdUnlock([]string{"--dest", t.TempDir(), "--name", "daily"})
	})
	if code != 1 || !strings.Contains(text, "--yes") {
		t.Fatalf("unlock without --yes exit = %d stderr = %s", code, text)
	}
}

func TestRuntimeFlagErrorsExit1(t *testing.T) {
	var code int
	text := captureStderr(t, func() {
		code = cmdBackup([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "--url", "postgres://x", "--dest", t.TempDir()})
	})
	if code != 1 {
		t.Fatalf("missing config exit = %d\n%s", code, text)
	}
	text = captureStderr(t, func() {
		code = cmdBackup([]string{"--url", "postgres://x", "--dest", "s3://bucket/p", "--s3-import-auth", "nope"})
	})
	if code != 1 {
		t.Fatalf("bad auth exit = %d\n%s", code, text)
	}
	for _, word := range []string{"auto", "implicit", "specified", "served"} {
		if !strings.Contains(text, word) {
			t.Fatalf("auth error missing %q: %s", word, text)
		}
	}
	text = captureStderr(t, func() {
		code = cmdRestore([]string{"--url", "postgres://x", "--src", t.TempDir(), "--force", "in-place"})
	})
	if code != 2 {
		t.Fatalf("--force in-place with a space exit = %d\n%s", code, text)
	}
}

func TestImportErrorNamesTheFlagThatWasSet(t *testing.T) {
	table := TableEntry{Database: "app", Schema: "public", Name: "t"}
	withServe := importReachError(table, RestoreOptions{ImportListen: "127.0.0.1:9", ServeAddr: "10.0.0.8:8080"}, errors.New("dial"))
	if !strings.Contains(withServe.Error(), "--serve-addr (10.0.0.8:8080)") || strings.Contains(withServe.Error(), "--import-listen") {
		t.Fatal(withServe)
	}
	withListen := importReachError(table, RestoreOptions{ImportListen: "127.0.0.1:9"}, errors.New("dial"))
	if !strings.Contains(withListen.Error(), "--import-listen (127.0.0.1:9)") || strings.Contains(withListen.Error(), "--serve-addr") {
		t.Fatal(withListen)
	}
}

func TestHalfTLSRefusedAtStartup(t *testing.T) {
	for _, args := range [][]string{
		{"--url", "postgres://root@127.0.0.1:1/db", "--src", t.TempDir(), "--serve-tls-cert", "only.crt"},
		{"--url", "postgres://root@127.0.0.1:1/db", "--src", t.TempDir(), "--serve-tls-key", "only.key"},
	} {
		var code int
		text := captureStderr(t, func() {
			code = cmdRestore(args)
		})
		if code != 1 {
			t.Fatalf("exit %d\n%s", code, text)
		}
		if !strings.Contains(text, "--serve-tls-cert") || !strings.Contains(text, "--serve-tls-key") {
			t.Fatalf("message: %s", text)
		}
	}
}

func TestBadTLSFilesAreReturned(t *testing.T) {
	dir := t.TempDir()
	_, closer, _, err := serveLocalBackup(importRequest{
		store: &localStore{root: dir},
		root:  Location{Kind: "file", Root: dir},
		opt: RestoreOptions{
			ImportListen: defaultImportListen,
			ServeTLSCert: filepath.Join(dir, "missing.crt"),
			ServeTLSKey:  filepath.Join(dir, "missing.key"),
		},
	})
	if closer != nil {
		closer()
	}
	if err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("tls start error = %v", err)
	}
}

func TestImportServerStartErrorIsReported(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	serveImportHTTP(&http.Server{Handler: http.NewServeMux()}, ln, errCh)
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("nil server error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTPS-style server start error was ignored")
	}
}

func TestServeTLSStartsAndAdvertisesHTTPS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir)
	base, closer, _, err := serveLocalBackup(importRequest{
		store: &localStore{root: dir},
		root:  Location{Kind: "file", Root: dir},
		opt: RestoreOptions{
			ImportListen: defaultImportListen,
			ServeTLSCert: certFile,
			ServeTLSKey:  keyFile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closer()
	if !strings.HasPrefix(base, "https://127.0.0.1:") {
		t.Fatalf("base = %s", base)
	}
	conn, err := tls.Dial("tcp", hostportOf(t, base), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
}

func TestWildcardBindNeedsAdvertise(t *testing.T) {
	tests := []struct {
		addr      string
		advertise string
		wantErr   bool
	}{
		{addr: ":80", wantErr: true},
		{addr: ":0", wantErr: true},
		{addr: "0:80", wantErr: true},
		{addr: "0.0.0.0:80", wantErr: true},
		{addr: "0.0.0.0", wantErr: true},
		{addr: "[::]:80", wantErr: true},
		{addr: "[::]", wantErr: true},
		{addr: "[::0]:80", wantErr: true},
		{addr: "[::0]", wantErr: true},
		{addr: "[0::0]:80", wantErr: true},
		{addr: "[0:0:0:0:0:0:0:0]:80", wantErr: true},
		{addr: "[0000:0000:0000:0000:0000:0000:0000:0000]:80", wantErr: true},
		{addr: "[::0.0.0.0]:80", wantErr: true},
		{addr: "[::ffff:0.0.0.0]:80", wantErr: true},
		{addr: "[::ffff:0:0]:80", wantErr: true},
		{addr: "[::FFFF:0.0.0.0]:80", wantErr: true},
		{addr: "[::%lo]:80", wantErr: true},
		{addr: "[::ffff:0.0.0.0%lo]:80", wantErr: true},
		{addr: "[0:0:0:0:0:0:0:0%lo]:80", wantErr: true},
		{addr: "127.0.0.1:0"},
		{addr: "[::1]:80"},
		{addr: "localhost:80"},
		{addr: "192.0.2.10:80"},
		{addr: "[::1%lo]:80"},
		{addr: "0.0.0.0:80", advertise: "192.0.2.10"},
		{addr: ":8080", advertise: "192.0.2.10"},
		{addr: "0:80", advertise: "192.0.2.10"},
		{addr: "[::0]:80", advertise: "192.0.2.10"},
		{addr: "[::ffff:0.0.0.0%lo]:80", advertise: "192.0.2.10"},
	}
	for _, tt := range tests {
		err := refuseWildcardBind(tt.addr, tt.advertise)
		if tt.wantErr && err == nil {
			t.Errorf("%s was accepted", tt.addr)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("%s advertise %q: %v", tt.addr, tt.advertise, err)
		}
	}
	dir := t.TempDir()
	for _, addr := range []string{"0.0.0.0:0", ":0"} {
		_, closer, _, err := serveLocalBackup(importRequest{
			store: &localStore{root: dir},
			root:  Location{Kind: "file", Root: dir},
			opt:   RestoreOptions{ServeAddr: addr, ImportListen: defaultImportListen},
		})
		if closer != nil {
			closer()
		}
		if err == nil || !strings.Contains(err.Error(), "--serve-advertise") {
			t.Errorf("%s wildcard error = %v", addr, err)
		}
	}
}

func TestResolvedHostnameWildcardNeedsAdvertise(t *testing.T) {
	made := useResolvedListen(t)
	dir := t.TempDir()
	for _, tt := range []struct {
		name    string
		opt     RestoreOptions
		bind    string
		wantErr bool
	}{
		{name: "serve-addr ipv4", opt: RestoreOptions{ServeAddr: "wild4.example:8080", ImportListen: defaultImportListen}, bind: "wild4.example:8080", wantErr: true},
		{name: "import-listen ipv4", opt: RestoreOptions{ImportListen: "wild4.example:8080"}, bind: "wild4.example:8080", wantErr: true},
		{name: "serve-addr ipv6", opt: RestoreOptions{ServeAddr: "wild6.example:8080", ImportListen: defaultImportListen}, bind: "wild6.example:8080", wantErr: true},
		{name: "import-listen ipv6", opt: RestoreOptions{ImportListen: "wild6.example:8080"}, bind: "wild6.example:8080", wantErr: true},
		{name: "serve-addr advertised", opt: RestoreOptions{ServeAddr: "wild4.example:8080", ServeAdvertise: "192.0.2.10", ImportListen: defaultImportListen}, bind: "wild4.example:8080"},
		{name: "import-listen advertised", opt: RestoreOptions{ImportListen: "wild6.example:8080", ServeAdvertise: "192.0.2.10"}, bind: "wild6.example:8080"},
		{name: "serve-addr specific", opt: RestoreOptions{ServeAddr: "specific.example:8080", ImportListen: defaultImportListen}, bind: "specific.example:8080"},
		{name: "import-listen specific", opt: RestoreOptions{ImportListen: "specific.example:8080"}, bind: "specific.example:8080"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := len(*made)
			if tt.wantErr {
				assertResolvedWildcardRefused(t, dir, tt.opt, tt.bind, made, before)
				return
			}
			ln, err := listenImport(tt.opt)
			if err != nil {
				t.Fatal(err)
			}
			if len(*made) == before || (*made)[len(*made)-1].closed {
				t.Fatal("specific listener was closed or not opened")
			}
			ln.Close()
		})
	}
}

func assertResolvedWildcardRefused(t *testing.T, dir string, opt RestoreOptions, bind string, made *[]*addrListener, before int) {
	t.Helper()
	base, closer, _, err := serveLocalBackup(importRequest{
		store: &localStore{root: dir},
		root:  Location{Kind: "file", Root: dir},
		opt:   opt,
	})
	if closer != nil {
		closer()
	}
	if base != "" {
		t.Fatalf("url handed out: %s", base)
	}
	if err == nil || err.Error() != wildcardBindError(bind).Error() {
		t.Fatalf("error = %v", err)
	}
	if len(*made) == before || !(*made)[len(*made)-1].closed {
		t.Fatal("unspecified listener was left open")
	}
}

func useResolvedListen(t *testing.T) *[]*addrListener {
	t.Helper()
	prev := listenTCP
	var made []*addrListener
	listenTCP = func(network, address string) (net.Listener, error) {
		return resolvedListen(network, address, &made)
	}
	t.Cleanup(func() { listenTCP = prev })
	return &made
}

func resolvedListen(network, address string, made *[]*addrListener) (net.Listener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ip, ok := resolvedTestHost(host)
	if !ok {
		return net.Listen(network, address)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	ln := &addrListener{addr: &net.TCPAddr{IP: ip, Port: n}}
	*made = append(*made, ln)
	return ln, nil
}

func resolvedTestHost(host string) (net.IP, bool) {
	switch host {
	case "wild4.example":
		return net.IPv4zero, true
	case "wild6.example":
		return net.IPv6unspecified, true
	case "specific.example":
		return net.ParseIP("192.0.2.10"), true
	default:
		return nil, false
	}
}

type addrListener struct {
	addr   *net.TCPAddr
	closed bool
}

func (l *addrListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *addrListener) Close() error {
	l.closed = true
	return nil
}
func (l *addrListener) Addr() net.Addr { return l.addr }

func TestAdvertiseURLUsesBoundPort(t *testing.T) {
	dir := t.TempDir()
	base, closer, _, err := serveLocalBackup(importRequest{
		store: &localStore{root: dir},
		root:  Location{Kind: "file", Root: dir},
		opt: RestoreOptions{
			ServeAddr:      "0.0.0.0:0",
			ServeAdvertise: "192.0.2.10",
			ImportListen:   defaultImportListen,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closer()
	host, port := splitHost(t, base)
	if host != "192.0.2.10" || port == "" || port == "0" {
		t.Fatalf("base = %s", base)
	}
	specific, specificCloser, _, err := serveLocalBackup(importRequest{
		store: &localStore{root: dir},
		root:  Location{Kind: "file", Root: dir},
		opt:   RestoreOptions{ImportListen: defaultImportListen},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer specificCloser()
	host, port = splitHost(t, specific)
	if host != "127.0.0.1" || port == "" || port == "0" {
		t.Fatalf("specific base = %s", specific)
	}
}

func TestReadOnlyCommandsDoNotProbe(t *testing.T) {
	endpoint, puts := conditionalS3Server(t)
	loc, err := parseLocation("s3://lab/prefix", "us-east-1", endpoint, "", "", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openStore(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	if *puts != 0 {
		t.Fatalf("read-only open wrote %d probe objects", *puts)
	}
	loc.ProbeConditionalWrites = true
	if _, err := openStore(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	if *puts != 2 {
		t.Fatalf("write open puts = %d, want 2", *puts)
	}
	*puts = 0
	args := []string{"--src", "s3://lab/prefix/latest", "--s3-region", "us-east-1", "--s3-endpoint", endpoint}
	_ = captureStderr(t, func() { _ = cmdVerify(args) })
	_ = captureStderr(t, func() {
		_ = cmdList([]string{"--src", "s3://lab/prefix", "--s3-region", "us-east-1", "--s3-endpoint", endpoint})
	})
	_ = captureStderr(t, func() {
		_ = cmdRestore(append([]string{"--url", "postgres://root@127.0.0.1:1/db"}, args...))
	})
	if *puts != 0 {
		t.Fatalf("read-only commands wrote %d objects", *puts)
	}
	var code int
	text := captureStderr(t, func() {
		code = cmdUnlock([]string{"--dest", "s3://lab/prefix", "--name", "daily", "--yes", "--s3-region", "us-east-1", "--s3-endpoint", endpoint})
	})
	if *puts < 2 {
		t.Fatalf("unlock puts = %d, exit %d\n%s", *puts, code, text)
	}
}

func TestS3LockProbeErrorIsNotADestinationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>no</Message></Error>`)
	}))
	defer srv.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "testkey")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "testsecret")
	t.Setenv("AWS_REGION", "us-east-1")
	loc, err := parseLocation("s3://lab/fgdb", "us-east-1", srv.URL, "", "", "auto")
	if err != nil {
		t.Fatal(err)
	}
	loc.ProbeConditionalWrites = true
	_, err = acquireBackupLease(context.Background(), loc, "daily", "holder", 0, time.Minute)
	if err == nil {
		t.Fatal("expected probe failure")
	}
	if strings.Contains(err.Error(), lockNeedsObjectStore) {
		t.Fatalf("misleading lock error: %v", err)
	}
	if !strings.Contains(err.Error(), "conditional") {
		t.Fatalf("probe error not surfaced: %v", err)
	}
}

func TestPruneHelpOffersDryRun(t *testing.T) {
	text := captureStderr(t, func() {
		if code := cmdPrune([]string{"-h"}); code != 2 {
			t.Fatalf("prune -h exit = %d", code)
		}
	})
	if !strings.Contains(text, "dry-run") {
		t.Fatalf("prune help missing dry-run:\n%s", text)
	}
}

func TestPruneDryRunPlanAndBackupProbe(t *testing.T) {
	bin := cockroachBin(t)
	base := t.TempDir()
	addr := "127.0.0.1:26295"
	startNode(t, bin, filepath.Join(base, "node"), addr, "127.0.0.1:18095", true)
	dbURL := "postgresql://root@" + addr + "/defaultdb?sslmode=disable"
	sql(t, bin, addr, true, `
		CREATE DATABASE app;
		CREATE TABLE app.public.t (id INT PRIMARY KEY);
		INSERT INTO app.public.t VALUES (1);
	`)
	dest := filepath.Join(base, "backups")
	code, text := runQuiet(t, func() int {
		return cmdBackup([]string{"--url", dbURL, "--dest", dest, "--database", "app", "--name", "app"})
	})
	if code != 0 {
		t.Fatalf("backup exit %d\n%s", code, text)
	}
	src := filepath.Join(dest, "app", "latest")
	code, text = runQuiet(t, func() int {
		return cmdRestore([]string{"--url", dbURL, "--src", src, "--plan"})
	})
	if code != 4 {
		t.Fatalf("plan exit %d, want 4\n%s", code, text)
	}
	code, text = runQuiet(t, func() int {
		return cmdRestore([]string{"--url", dbURL, "--src", src})
	})
	if code != 1 {
		t.Fatalf("refused restore exit %d, want 1\n%s", code, text)
	}
	puts := backupThenRead(t, dbURL)
	if puts != 2 {
		t.Fatalf("backup probe puts = %d, want 2", puts)
	}
	sql(t, bin, addr, true, `DROP DATABASE app CASCADE`)
	code, text = runQuiet(t, func() int {
		return cmdRestore([]string{"--url", dbURL, "--src", src, "--testing-mode"})
	})
	if code != 0 {
		t.Fatalf("testing-mode restore exit %d\n%s", code, text)
	}
	left := sqlOut(t, bin, addr, true, `SHOW TABLES`)
	if strings.Contains(left, "fgdb_backup_probe") {
		t.Fatalf("probe table left behind:\n%s", left)
	}
	sql(t, bin, addr, true, `
		CREATE DATABASE "app__fgdb_old_20261015t120000000z-aaaa1111";
		CREATE DATABASE "app__fgdb_old_20261015t130000000z-bbbb2222";
	`)
	var dryCode int
	dryOut := captureStdout(t, func() {
		dryCode = cmdPrune([]string{"--url", dbURL, "--name", "app", "--keep", "0", "--dry-run"})
	})
	if dryCode != 0 || !strings.Contains(dryOut, "would drop") || !strings.Contains(dryOut, "dry-run: 2") {
		t.Fatalf("dry-run exit %d output:\n%s", dryCode, dryOut)
	}
	still := sqlOut(t, bin, addr, true, `SELECT count(*) FROM [SHOW DATABASES] WHERE database_name LIKE 'app!_!_fgdb!_old!_%' ESCAPE '!'`)
	if !strings.Contains(still, "2") {
		t.Fatalf("dry-run removed copies:\n%s", still)
	}
	code, text = runQuiet(t, func() int {
		return cmdPrune([]string{"--url", dbURL, "--name", "app", "--keep", "0"})
	})
	if code != 0 {
		t.Fatalf("prune exit %d\n%s", code, text)
	}
	gone := sqlOut(t, bin, addr, true, `SELECT count(*) FROM [SHOW DATABASES] WHERE database_name LIKE 'app!_!_fgdb!_old!_%' ESCAPE '!'`)
	if !strings.Contains(gone, "0") {
		t.Fatalf("copies remain:\n%s", gone)
	}
}

func backupThenRead(t *testing.T, dbURL string) int {
	t.Helper()
	backend := s3mem.New()
	if err := backend.CreateBucket("lab"); err != nil {
		t.Fatal(err)
	}
	var puts int
	inner := gofakes3.New(backend).Server()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, ".fgdb-if-none-match-") {
			puts++
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "testkey")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "testsecret")
	t.Setenv("AWS_REGION", "us-east-1")
	code, text := runQuiet(t, func() int {
		return cmdBackup([]string{
			"--url", dbURL, "--dest", "s3://lab/fgdb", "--database", "app", "--name", "s3app",
			"--s3-endpoint", srv.URL, "--s3-region", "us-east-1", "--allow-unsafe-overwrite",
		})
	})
	if code != 0 {
		t.Fatalf("s3 backup exit %d\n%s", code, text)
	}
	before := puts
	code, text = runQuiet(t, func() int {
		return cmdVerify([]string{"--src", "s3://lab/fgdb/s3app/latest", "--s3-endpoint", srv.URL, "--s3-region", "us-east-1"})
	})
	if code != 0 {
		t.Fatalf("verify exit %d\n%s", code, text)
	}
	code, text = runQuiet(t, func() int {
		return cmdList([]string{"--src", "s3://lab/fgdb/s3app", "--s3-endpoint", srv.URL, "--s3-region", "us-east-1"})
	})
	if code != 0 {
		t.Fatalf("list exit %d\n%s", code, text)
	}
	if puts != before {
		t.Fatalf("verify and list added %d probe writes", puts-before)
	}
	return puts
}

func conditionalS3Server(t *testing.T) (string, *int) {
	t.Helper()
	var puts int
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			puts++
			if present && r.Header.Get("If-None-Match") == "*" {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			present = true
			w.Header().Set("ETag", `"probe"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			present = false
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "testkey")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "testsecret")
	t.Setenv("AWS_REGION", "us-east-1")
	return srv.URL, &puts
}

func writeTestCert(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "import"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func hostportOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func splitHost(t *testing.T, raw string) (string, string) {
	t.Helper()
	host, port, err := net.SplitHostPort(hostportOf(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func runQuiet(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	var code int
	text := captureStderr(t, func() { code = fn() })
	return code, text
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	return captureFD(t, &os.Stderr, fn)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return captureFD(t, &os.Stdout, fn)
}

func captureFD(t *testing.T, target **os.File, fn func()) string {
	t.Helper()
	old := *target
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	*target = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	finished := false
	defer func() {
		if finished {
			return
		}
		_ = w.Close()
		*target = old
		<-done
	}()
	fn()
	_ = w.Close()
	*target = old
	<-done
	finished = true
	return buf.String()
}
