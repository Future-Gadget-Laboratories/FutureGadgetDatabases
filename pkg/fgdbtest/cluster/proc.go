// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	sigTerm = syscall.SIGTERM
	sigKill = syscall.SIGKILL
	sigStop = syscall.SIGSTOP
	sigCont = syscall.SIGCONT
)

func (c *Cluster) signalNode(ctx context.Context, index int, kill bool) error {
	n, err := c.node(index)
	if err != nil {
		return err
	}
	pid, err := readPid(n.PidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !alive(pid) {
		_ = os.Remove(n.PidFile)
		return nil
	}
	sig := sigTerm
	if kill {
		sig = sigKill
	}
	if err := signal(pid, sig); err != nil {
		return fmt.Errorf("signal node %d: %w", index+1, err)
	}
	if err := waitGone(ctx, pid, stopTimeout); err != nil {
		_ = signal(pid, sigKill)
		if err := waitGone(ctx, pid, stopTimeout); err != nil {
			return fmt.Errorf("node %d did not exit: %w", index+1, err)
		}
	}
	_ = os.Remove(n.PidFile)
	return nil
}

func (c *Cluster) pid(index int) (int, error) {
	n, err := c.node(index)
	if err != nil {
		return 0, err
	}
	pid, err := readPid(n.PidFile)
	if err != nil {
		return 0, fmt.Errorf("node %d pid: %w", index+1, err)
	}
	if !alive(pid) {
		return 0, fmt.Errorf("node %d is not running", index+1)
	}
	return pid, nil
}

func readPid(path string) (int, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return 0, fmt.Errorf("pid file %s is empty", path)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("pid file %s has no pid", path)
	}
	return pid, nil
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func signal(pid int, sig syscall.Signal) error {
	err := syscall.Kill(pid, sig)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}

func waitGone(ctx context.Context, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for alive(pid) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pid %d still running", pid)
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// RunningBinary reads /proc/<pid>/exe for a live node.
func (c *Cluster) RunningBinary(index int) (string, error) {
	pid, err := c.pid(index)
	if err != nil {
		return "", err
	}
	link := fmt.Sprintf("/proc/%d/exe", pid)
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("node %d binary: %w", index+1, err)
	}
	return target, nil
}
