// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestStartField(t *testing.T) {
	stat := "1 (bash foo) S " + strings.Repeat("0 ", startIndex-1) + "99"
	got, err := startField(stat)
	if err != nil || got != "99" {
		t.Fatalf("start time %q, err %v", got, err)
	}
	body, err := os.ReadFile(fmt.Sprintf(procStatFmt, os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	ticks, err := startField(string(body))
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	id, err := Capture(os.Getpid(), exe)
	if err != nil {
		t.Fatal(err)
	}
	if ticks != id.Start {
		t.Fatalf("stat start %s, captured %s", ticks, id.Start)
	}
}

func TestSignalRejectsMismatchedStartTime(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	id, err := Capture(cmd.Process.Pid, cmd.Path)
	if err != nil {
		t.Fatal(err)
	}
	bad := id
	bad.Start = "1"
	if err := Signal(bad, unix.SIGTERM); !errors.Is(err, ErrMismatch) {
		t.Fatalf("mismatched start time: %v", err)
	}
	if err := Match(id); err != nil {
		t.Fatal("a mismatched identity was signaled")
	}
	bad.Start = id.Start
	bad.Exe = "/bin/false"
	if err := Signal(bad, unix.SIGTERM); !errors.Is(err, ErrMismatch) {
		t.Fatalf("mismatched exe: %v", err)
	}
	if err := Match(id); err != nil {
		t.Fatal("a mismatched exe was signaled")
	}
	if err := Signal(id, unix.SIGTERM); err != nil {
		t.Fatal(err)
	}
}

func TestFaultMismatchDoesNotSignal(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	id, err := Capture(cmd.Process.Pid, cmd.Path)
	if err != nil {
		t.Fatal(err)
	}
	bad := id
	bad.Start = "1"
	c := &Cluster{nodes: []Node{{Index: 0, ID: bad, PidFile: filepathUnused(t)}}}
	if err := c.KillNode(0); !errors.Is(err, ErrMismatch) {
		t.Fatalf("kill mismatch: %v", err)
	}
	if err := c.PauseNode(0); !errors.Is(err, ErrMismatch) {
		t.Fatalf("pause mismatch: %v", err)
	}
	err = c.StopNode(context.Background(), 0)
	if err == nil || !strings.Contains(err.Error(), notFoundText) {
		t.Fatalf("cleanup hid the mismatch: %v", err)
	}
	if err := Match(id); err != nil {
		t.Fatal("a mismatched pid was signaled")
	}
}

func TestCaptureFailureIsStillReaped(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	live, err := Capture(cmd.Process.Pid, cmd.Path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pidPath := dir + "/node.pid"
	body := []byte(fmt.Sprintf("%d\n", live.PID))
	if err := os.WriteFile(pidPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Cluster{nodes: []Node{{
		Index: 0, Binary: "/bin/false", PidFile: pidPath, StoreDir: dir,
	}}}
	err = c.noteStarted(0, c.nodes[0])
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%d", live.PID)) {
		t.Fatalf("capture error did not name the pid: %v", err)
	}
	if !c.nodes[0].tracked {
		t.Fatal("pidfd was not recorded before capture failed")
	}
	if err := Match(live); err != nil {
		t.Fatal("capture failure killed the process")
	}
	if err := c.Reap(); err != nil {
		t.Fatal(err)
	}
	if err := Match(live); !errors.Is(err, ErrGone) {
		t.Fatalf("reap left the process: %v", err)
	}
}

func filepathUnused(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/missing.pid"
}

func TestCommandEnvDropsCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "secret-actions")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret-aws")
	t.Setenv("MY_PASSWORD", "secret-pass")
	setSecretEnv(t)
	t.Setenv("FGDB_COCKROACH", "/tmp/cockroach")
	t.Setenv("PATH", "/usr/bin")
	env, err := CommandEnv()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(env, "\n")
	for _, leaked := range []string{
		"secret-token", "secret-actions", "secret-aws", "secret-pass",
		"GITHUB_TOKEN", "ACTIONS_", "FGDB_DATABASE_URL", "postgres://user:secret",
		"GOPROXY=", "password@proxy", "GOFLAGS=", "GONOSUMDB=", "LD_LIBRARY_PATH=", "/tmp/evil-lib",
	} {
		if strings.Contains(text, leaked) {
			t.Fatalf("environment kept %s\n%s", leaked, text)
		}
	}
	if !strings.Contains(text, "FGDB_COCKROACH=/tmp/cockroach") {
		t.Fatalf("missing allowlisted variable\n%s", text)
	}
}

func TestFaultGoneIsAnError(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := Capture(cmd.Process.Pid, cmd.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if _, err := cmd.Process.Wait(); err != nil {
		t.Fatal(err)
	}
	c := &Cluster{nodes: []Node{{Index: 0, ID: id, PidFile: filepathUnused(t)}}}
	if err := c.KillNode(0); !errors.Is(err, ErrGone) {
		t.Fatalf("kill of a gone process: %v", err)
	}
	if err := c.PauseNode(0); !errors.Is(err, ErrGone) {
		t.Fatalf("pause of a gone process: %v", err)
	}
	if err := c.StopNode(context.Background(), 0); err != nil {
		t.Fatalf("cleanup rejected a gone process: %v", err)
	}
}

func TestAssignPortsAreDistinct(t *testing.T) {
	sql, http, err := assignPorts(3)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, port := range append(append([]int{}, sql...), http...) {
		if port <= 0 || seen[port] {
			t.Fatalf("ports sql %v http %v", sql, http)
		}
		seen[port] = true
	}
}
