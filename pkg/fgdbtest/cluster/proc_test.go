// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
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
