// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	procExeFmt  = "/proc/%d/exe"
	procStatFmt = "/proc/%d/stat"
	exeDeleted  = " (deleted)"
	startIndex  = 19 // field 22 of /proc/pid/stat, after the comm field
)

// Identity is the process recorded at spawn.
// A later signal is sent only when the pid still has this exe and start time.
type Identity struct {
	PID   int
	Exe   string
	Start string
}

// ErrGone means the recorded process is no longer running.
var ErrGone = errors.New("process is gone")

// ErrMismatch means the pid now belongs to a different process.
var ErrMismatch = errors.New("process identity does not match")

// Capture records pid's exe and start time. The exe must be the binary we started.
func Capture(pid int, exe string) (Identity, error) {
	id, err := readIdentity(pid)
	if err != nil {
		return Identity{}, err
	}
	if !sameExe(id.Exe, exe) {
		return Identity{}, fmt.Errorf("pid %d exe is %s, want %s", pid, id.Exe, exe)
	}
	id.Exe = exe
	return id, nil
}

// Match reports whether pid is still the recorded process.
func Match(id Identity) error {
	return matchProc(id)
}

// Signal sends sig through a pidfd after Match succeeds.
// A reused pid is not signaled.
func Signal(id Identity, sig unix.Signal) error {
	if id.PID <= 0 {
		return ErrGone
	}
	fd, err := unix.PidfdOpen(id.PID, 0)
	if err != nil {
		return goneOr(err)
	}
	defer func() { _ = unix.Close(fd) }()
	if err := matchProc(id); err != nil {
		return err
	}
	err = unix.PidfdSendSignal(fd, sig, nil, 0)
	return goneOr(err)
}

func goneOr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ESRCH) || errors.Is(err, os.ErrNotExist) {
		return ErrGone
	}
	return err
}

func matchProc(id Identity) error {
	got, err := readIdentity(id.PID)
	if err != nil {
		return goneOr(err)
	}
	if got.Start != id.Start || !sameExe(got.Exe, id.Exe) {
		return ErrMismatch
	}
	return nil
}

func readIdentity(pid int) (Identity, error) {
	exe, err := os.Readlink(fmt.Sprintf(procExeFmt, pid))
	if err != nil {
		return Identity{}, err
	}
	body, err := os.ReadFile(fmt.Sprintf(procStatFmt, pid))
	if err != nil {
		return Identity{}, err
	}
	ticks, err := startField(string(body))
	if err != nil {
		return Identity{}, err
	}
	return Identity{PID: pid, Exe: exe, Start: ticks}, nil
}

func startField(stat string) (string, error) {
	end := strings.LastIndex(stat, ")")
	if end < 0 || end+1 >= len(stat) {
		return "", fmt.Errorf("stat has no command field")
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) <= startIndex {
		return "", fmt.Errorf("stat has no start time")
	}
	return fields[startIndex], nil
}

func sameExe(got, want string) bool {
	left := strings.TrimSuffix(got, exeDeleted)
	right := strings.TrimSuffix(want, exeDeleted)
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr != nil || rightErr != nil {
		return left == right
	}
	return os.SameFile(leftInfo, rightInfo)
}
