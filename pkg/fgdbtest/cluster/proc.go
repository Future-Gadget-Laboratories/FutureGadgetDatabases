// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	sigTerm      = unix.SIGTERM
	sigKill      = unix.SIGKILL
	sigStop      = unix.SIGSTOP
	sigCont      = unix.SIGCONT
	pollInterval = 200 * time.Millisecond
	survivorText = "still running"
	stillRunning = "pid %d " + survivorText
	notFoundText = "original process was not found"
)

func (c *Cluster) signalNode(ctx context.Context, index int, kill bool) error {
	n, err := c.node(index)
	if err != nil {
		return err
	}
	if n.ID.PID == 0 {
		return nil
	}
	sig := sigTerm
	if kill {
		sig = sigKill
	}
	if err := sendSignal(n.ID, sig, kill); err != nil {
		return signalNodeErr(index, err)
	}
	if err := waitIdentity(ctx, n.ID, stopTimeout); err != nil {
		if stopErr := finishStop(ctx, index, n.ID, err); stopErr != nil {
			return stopErr
		}
	}
	_ = os.Remove(n.PidFile)
	return nil
}

// sendSignal returns every Signal error from a fault, including ErrGone.
// Cleanup tolerates a process that has already exited.
func sendSignal(id Identity, sig unix.Signal, kill bool) error {
	err := Signal(id, sig)
	if kill {
		return err
	}
	return cleanupResult(err)
}

func cleanupResult(err error) error {
	if err == nil || errors.Is(err, ErrGone) {
		return nil
	}
	if errors.Is(err, ErrMismatch) {
		return fmt.Errorf("%s: %w", notFoundText, err)
	}
	return err
}

func finishStop(ctx context.Context, index int, id Identity, waitErr error) error {
	if err := cleanupResult(Signal(id, sigKill)); err != nil {
		return errors.Join(waitErr, signalNodeErr(index, err))
	}
	if err := waitIdentity(ctx, id, stopTimeout); err != nil {
		return fmt.Errorf("node %d did not exit: %w", index+1, err)
	}
	return nil
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

func waitIdentity(ctx context.Context, id Identity, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := matchProc(id)
		if errors.Is(err, ErrGone) || errors.Is(err, ErrMismatch) {
			return nil
		}
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(stillRunning, id.PID)
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func signalNodeErr(index int, err error) error {
	return fmt.Errorf("signal node %d: %w", index+1, err)
}

func (c *Cluster) signalOwned(index int, sig unix.Signal) error {
	n, err := c.node(index)
	if err != nil {
		return err
	}
	if n.ID.PID == 0 {
		return fmt.Errorf("node %d has no recorded process", index+1)
	}
	if err := Signal(n.ID, sig); err != nil {
		return signalNodeErr(index, err)
	}
	return nil
}

// RunningBinary reads the exe of a node whose pid is still the one we started.
func (c *Cluster) RunningBinary(index int) (string, error) {
	n, err := c.node(index)
	if err != nil {
		return "", err
	}
	return nodeExe(n)
}

func nodeExe(n Node) (string, error) {
	if err := Match(n.ID); err != nil {
		return "", binaryErr(n.Index, err)
	}
	target, err := os.Readlink(fmt.Sprintf(procExeFmt, n.ID.PID))
	if err != nil {
		return "", binaryErr(n.Index, err)
	}
	return target, nil
}

func binaryErr(index int, err error) error {
	return fmt.Errorf("node %d binary: %w", index+1, err)
}

// Reap sends SIGKILL to every recorded node and reports any that stay up.
func (c *Cluster) Reap() error {
	if c == nil {
		return nil
	}
	var err error
	for i := range c.nodes {
		if one := c.finishNode(i); one != nil {
			err = errors.Join(err, fmt.Errorf("node %d: %w", c.nodes[i].Index+1, one))
		}
	}
	return err
}

func (c *Cluster) finishNode(index int) error {
	if c.nodes[index].tracked {
		return c.killTracked(index)
	}
	if c.nodes[index].ID.Start == "" {
		return nil
	}
	return reapOne(c.nodes[index].ID)
}

// ReapProcess is the final kill-and-reap for one recorded process.
func ReapProcess(id Identity) error {
	return reapOne(id)
}

func reapOne(id Identity) error {
	if id.PID <= 0 {
		return nil
	}
	if id.Start == "" || id.Exe == "" {
		return manualCleanup(id.PID, nil)
	}
	if err := cleanupResult(Signal(id, sigKill)); err != nil {
		return err
	}
	if err := waitIdentity(context.Background(), id, stopTimeout); err != nil {
		return fmt.Errorf(stillRunning, id.PID)
	}
	return nil
}

func (c *Cluster) remember(index, pid int) {
	c.closeTracked(index)
	proc, err := os.FindProcess(pid)
	if err != nil {
		c.nodes[index].ID = Identity{PID: pid}
		return
	}
	fd, fdErr := unix.PidfdOpen(pid, 0)
	c.nodes[index].proc = proc
	c.nodes[index].ID = Identity{PID: pid}
	if fdErr != nil {
		return
	}
	c.nodes[index].pidfd = fd
	c.nodes[index].tracked = true
}

func (c *Cluster) closeTracked(index int) {
	n := &c.nodes[index]
	if !n.tracked {
		return
	}
	_ = unix.Close(n.pidfd)
	n.tracked = false
	n.pidfd = 0
}

func (c *Cluster) killTracked(index int) error {
	n := &c.nodes[index]
	pid := n.ID.PID
	if !n.tracked {
		if pid > 0 && n.ID.Start == "" {
			return manualCleanup(pid, nil)
		}
		return nil
	}
	fd := n.pidfd
	n.tracked = false
	n.pidfd = 0
	defer func() { _ = unix.Close(fd) }()
	err := unix.PidfdSendSignal(fd, sigKill, nil, 0)
	if err != nil && !errors.Is(err, unix.ESRCH) {
		return manualCleanup(pid, err)
	}
	if n.proc != nil {
		_, _ = n.proc.Wait()
	}
	return waitPidfd(context.Background(), fd, pid)
}

func waitPidfd(ctx context.Context, fd, pid int) error {
	deadline := time.Now().Add(stopTimeout)
	for {
		err := unix.PidfdSendSignal(fd, sigKill, nil, 0)
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(stillRunning, pid)
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
}

func manualCleanup(pid int, err error) error {
	if err == nil {
		return fmt.Errorf("manual cleanup required for pid %d", pid)
	}
	return fmt.Errorf("manual cleanup required for pid %d: %w", pid, err)
}
