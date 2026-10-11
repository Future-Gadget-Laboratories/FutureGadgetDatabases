// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const jsonFlag = "--json"

func (r *run) backup() error {
	if !r.want(stepBackup) {
		return errNotRun
	}
	bin, err := r.backupBin()
	if err != nil {
		return err
	}
	before, err := r.snapBank(0)
	if err != nil {
		return err
	}
	dest := filepath.Join(r.cfg.WorkDir, "backup")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	srcURL, err := r.cluster.SQLURL(0)
	if err != nil {
		return err
	}
	dstURL, err := r.cluster.SQLURL(1)
	if err != nil {
		return err
	}
	latest := filepath.Join(dest, "bank", "latest")
	if err := runTool(r.ctx, bin, "backup", jsonFlag, "--url", srcURL, "--dest", dest,
		"--database", "bank", "--name", "bank"); err != nil {
		return err
	}
	if err := runTool(r.ctx, bin, "verify", jsonFlag, "--src", latest); err != nil {
		return err
	}
	if _, err := r.cluster.SQL(r.ctx, 0, `DROP DATABASE bank CASCADE`); err != nil {
		return err
	}
	if err := runTool(r.ctx, bin, "restore", jsonFlag, "--url", dstURL, "--src", latest, "--load", "copy"); err != nil {
		return err
	}
	after, err := r.snapBank(1)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("restored bank %+v, want %+v", after, before)
	}
	return nil
}

func (r *run) backupBin() (string, error) {
	if r.cfg.BackupBin != "" {
		return r.cfg.BackupBin, nil
	}
	out := filepath.Join(r.cfg.WorkDir, "fgdb-backup")
	cmd := exec.CommandContext(r.ctx, "go", "build", "-o", out, ".")
	if err := useCleanEnv(cmd); err != nil {
		return "", err
	}
	cmd.Dir = filepath.Join(r.cfg.RepoRoot, "pkg", "cmd", "fgdb-backup")
	log, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build fgdb-backup: %w\n%s", err, log)
	}
	return out, nil
}

func runTool(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	if err := useCleanEnv(cmd); err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fgdb-backup %s: %w\n%s", args[0], err, out)
	}
	return nil
}
