// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseLibIsOnlyTheExtractedDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "cockroach")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "lib")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, geosLibrary), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LD_LIBRARY_PATH", "/tmp/evil-lib")
	env, err := ProcessEnv(bin)
	if err != nil {
		t.Fatal(err)
	}
	got := envValue(env, ldLibraryPath)
	if got != lib {
		t.Fatalf("LD_LIBRARY_PATH %q, want %s", got, lib)
	}
	if strings.Contains(got, "evil-lib") {
		t.Fatalf("inherited library path leaked: %s", got)
	}
}

func TestReleaseLibRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "cockroach")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, geosLibrary), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "lib")); err != nil {
		t.Fatal(err)
	}
	if _, err := ProcessEnv(bin); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink lib: %v", err)
	}
}

func TestShellAllowlistMatchesGo(t *testing.T) {
	names := mustAllowlist(t)
	setAllowlistEnv(t, names)
	setSecretEnv(t)
	got := shellPrintEnv(t, repoRoot(t))
	assertShellKeepsAllowlist(t, names, got)
	assertShellDropsSecrets(t, got)
	assertGoMatchesShell(t, got)
}

func mustAllowlist(t *testing.T) []string {
	t.Helper()
	names, err := parseAllowlist(allowlistText)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func setAllowlistEnv(t *testing.T, names []string) {
	t.Helper()
	for _, name := range names {
		if name == "PATH" {
			continue
		}
		t.Setenv(name, "kept-"+name)
	}
}

func setSecretEnv(t *testing.T) {
	t.Helper()
	t.Setenv("FGDB_DATABASE_URL", "postgres://user:secret@db.example/bank")
	t.Setenv("GOPROXY", "https://user:password@proxy.example")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GONOSUMDB", "example.com")
	t.Setenv("LD_LIBRARY_PATH", "/tmp/evil-lib")
}

func shellPrintEnv(t *testing.T, root string) map[string]string {
	t.Helper()
	cmd := exec.Command("bash", "-c", "set -euo pipefail; source fgdb/test/scripts/scrub-credentials.sh; fgdb_exec /usr/bin/printenv")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("printenv: %v\n%s", err, out)
	}
	return envMap(string(out))
}

func assertShellKeepsAllowlist(t *testing.T, names []string, got map[string]string) {
	t.Helper()
	allowed := map[string]struct{}{}
	for _, name := range names {
		allowed[name] = struct{}{}
		want := os.Getenv(name)
		if want == "" {
			continue
		}
		if got[name] != want {
			t.Fatalf("shell %s=%q, want %q", name, got[name], want)
		}
	}
	for key := range got {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("shell passed %s", key)
		}
	}
}

func assertShellDropsSecrets(t *testing.T, got map[string]string) {
	t.Helper()
	for _, banned := range []string{"FGDB_DATABASE_URL", "GOPROXY", "GOFLAGS", "GONOSUMDB", "LD_LIBRARY_PATH"} {
		if _, ok := got[banned]; ok {
			t.Fatalf("shell passed %s", banned)
		}
	}
}

func assertGoMatchesShell(t *testing.T, got map[string]string) {
	t.Helper()
	goEnv, err := CommandEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range goEnv {
		key, ok := envKey(kv)
		if !ok || key == "COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING" {
			continue
		}
		if _, allowed := got[key]; !allowed && os.Getenv(key) != "" {
			t.Fatalf("Go kept %s and the shell did not", key)
		}
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}

func envMap(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, val, ok := strings.Cut(line, "=")
		if ok {
			out[key] = val
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		script := filepath.Join(dir, "fgdb", "test", "scripts", "scrub-credentials.sh")
		if _, err := os.Stat(script); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
