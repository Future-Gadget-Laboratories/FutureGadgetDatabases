// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/cluster"
	"golang.org/x/sys/unix"
)

const clientBackstop = 30 * time.Minute
const clientStopWait = 10 * time.Second

const (
	tolerateFlag = "--tolerate-errors"
	rowsFlagFmt  = "--rows=%d"
)

func (r *run) startClients() error {
	work, err := r.collectURLs((*cluster.Cluster).WorkloadURL)
	if err != nil {
		return err
	}
	if len(work) != 3 {
		return fmt.Errorf("need 3 node URLs, have %d", len(work))
	}
	admin, err := r.collectURLs((*cluster.Cluster).SQLURL)
	if err != nil {
		return err
	}
	set := &procSet{}
	if err := set.start(r.cfg.WorkDir, "bank", r.clientBin, bankArgs(r.cfg.BankRows, work)); err != nil {
		set.stop()
		return err
	}
	if err := set.start(r.cfg.WorkDir, "kv", r.clientBin, kvArgs(r.cfg, work)); err != nil {
		set.stop()
		return err
	}
	rec, err := r.recorderBin()
	if err != nil {
		set.stop()
		return err
	}
	if err := set.start(r.cfg.WorkDir, "recorder", rec, recorderArgs(r.history, admin)); err != nil {
		set.stop()
		return err
	}
	r.clients = set
	if !sleepCtx(r.ctx, r.cfg.Warmup) {
		return r.ctx.Err()
	}
	err = set.running()
	r.note(ClaimClients, err)
	return err
}

func (r *run) collectURLs(pick func(*cluster.Cluster, int) (string, error)) ([]string, error) {
	urls := make([]string, 0, len(r.cluster.Nodes()))
	for i := range r.cluster.Nodes() {
		url, err := pick(r.cluster, i)
		if err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	return urls, nil
}

func bankArgs(rows int, urls []string) []string {
	// The run must use the same account count as init. The workload default
	// is 1000, and a transfer that names a missing account changes only one balance.
	args := []string{
		"workload", "run", "bank",
		durationArg(),
		"--concurrency=2",
		fmt.Sprintf(rowsFlagFmt, rows),
		tolerateFlag,
	}
	return append(args, urls[:2]...)
}

func kvArgs(cfg Config, urls []string) []string {
	args := []string{
		"workload", "run", "kv",
		durationArg(),
		fmt.Sprintf("--concurrency=%d", cfg.KVConcurrency),
		"--read-percent=50",
		tolerateFlag,
	}
	return append(args, urls[2])
}

func durationArg() string {
	return "--duration=" + clientBackstop.String()
}

func recorderArgs(history string, urls []string) []string {
	return []string{
		"record",
		"--out=" + history,
		"--urls=" + strings.Join(urls, ","),
		"--duration=" + clientBackstop.String(),
		"--clients=3",
	}
}

func (r *run) recorderBin() (string, error) {
	if bin := os.Getenv("FGDB_RECORDER_BIN"); bin != "" {
		return bin, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if strings.Contains(filepath.Base(exe), "fgdb-test") {
		return exe, nil
	}
	out := filepath.Join(r.cfg.WorkDir, "fgdb-test")
	cmd := exec.Command("go", "build", "-o", out, "./pkg/cmd/fgdb-test")
	cmd.Dir = r.cfg.RepoRoot
	log, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build history recorder: %w\n%s", err, log)
	}
	return out, nil
}

type procHandle struct {
	name string
	cmd  *exec.Cmd
	id   cluster.Identity
}

type procSet struct {
	handles []procHandle
	stopped bool
	stopErr error
}

func (p *procSet) start(dir, name, bin string, args []string) error {
	logPath := dir + "/" + name + ".log"
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return startClientErr(name, err)
	}
	// The child keeps the inherited descriptor. The parent does not.
	_ = logFile.Close()
	id, err := cluster.Capture(cmd.Process.Pid, bin)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return startClientErr(name, err)
	}
	p.handles = append(p.handles, procHandle{name: name, cmd: cmd, id: id})
	return nil
}

func (p *procSet) running() error {
	if len(p.handles) != 3 {
		return fmt.Errorf("expected 3 client processes, have %d", len(p.handles))
	}
	for _, handle := range p.handles {
		if err := cluster.Match(handle.id); err != nil {
			return fmt.Errorf("a client process exited during warmup")
		}
	}
	return nil
}

func (p *procSet) stop() error {
	if p == nil {
		return nil
	}
	if p.stopped {
		return p.stopErr
	}
	p.stopped = true
	var err error
	for _, handle := range p.handles {
		err = errors.Join(err, signalClient(handle, unix.SIGTERM))
	}
	deadline := time.Now().Add(clientStopWait)
	for _, handle := range p.handles {
		waitProc(handle.cmd, deadline)
	}
	p.stopErr = err
	return err
}

func (p *procSet) reap() error {
	if p == nil {
		return nil
	}
	var err error
	for _, handle := range p.handles {
		if one := cluster.ReapProcess(handle.id); one != nil {
			err = errors.Join(err, fmt.Errorf("%s: %w", handle.name, one))
		}
	}
	return err
}

func startClientErr(name string, err error) error {
	return fmt.Errorf("start %s: %w", name, err)
}

func signalClient(handle procHandle, sig unix.Signal) error {
	err := cluster.Signal(handle.id, sig)
	if err == nil || errors.Is(err, cluster.ErrGone) {
		return nil
	}
	return fmt.Errorf("%s: %w", handle.name, err)
}

func waitProc(cmd *exec.Cmd, deadline time.Time) {
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
