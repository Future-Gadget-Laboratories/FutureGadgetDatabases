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
	if err := deliver(n.ID, sig); err != nil {
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

func deliver(id Identity, sig unix.Signal) error {
	err := Signal(id, sig)
	if errors.Is(err, ErrGone) || errors.Is(err, ErrMismatch) {
		return nil
	}
	return err
}

func finishStop(ctx context.Context, index int, id Identity, waitErr error) error {
	if err := deliver(id, sigKill); err != nil {
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
	for _, n := range c.nodes {
		if one := reapOne(n.ID); one != nil {
			err = errors.Join(err, fmt.Errorf("node %d: %w", n.Index+1, one))
		}
	}
	return err
}

// ReapProcess is the final kill-and-reap for one recorded process.
func ReapProcess(id Identity) error {
	return reapOne(id)
}

func reapOne(id Identity) error {
	if id.PID <= 0 {
		return nil
	}
	if err := deliver(id, sigKill); err != nil {
		return err
	}
	if err := waitIdentity(context.Background(), id, stopTimeout); err != nil {
		return fmt.Errorf(stillRunning, id.PID)
	}
	return nil
}
