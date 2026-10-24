// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package capabilities checks observable behavior of an isolated OSS binary.
package capabilities

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/cluster"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/jackc/pgx/v5"
)

const (
	ClaimVersion = "OSS-001"
	ClaimImport  = "OSS-002"
	ClaimBackup  = "OSS-003"
	stepSetup    = "OSS-SETUP"
	suiteTimeout = 10 * time.Minute
)

// Config supplies the binary and a parent for disposable stores.
// Meta records the suite source revision and supplied candidate checksum.
type Config struct {
	Binary   string
	WorkDir  string
	ExpectGo string
	Meta     report.Meta
}

// Run records version, IMPORT, and unsupported BACKUP outcomes separately.
// A missing binary or failed setup is an error; it never becomes a passing skip.
func Run(ctx context.Context, cfg Config) (*report.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, suiteTimeout)
	defer cancel()
	rep := newReport(cfg.Meta)
	if cfg.Binary == "" || cfg.WorkDir == "" {
		err := errors.New("OSS capabilities require a binary and work directory")
		note(rep, stepSetup, "", err)
		return rep, err
	}
	err := cluster.Version(ctx, cfg.Binary, cfg.ExpectGo)
	note(rep, ClaimVersion, ClaimVersion, err)
	if err != nil {
		return rep, err
	}
	return runCluster(ctx, cfg, rep)
}

func newReport(meta report.Meta) *report.Report {
	rep := report.New(meta)
	for _, id := range []string{ClaimVersion, ClaimImport, ClaimBackup} {
		rep.Set(id, labels.NotTested, "capability check did not run")
	}
	return rep
}

func runCluster(ctx context.Context, cfg Config, rep *report.Report) (_ *report.Report, result error) {
	if err := os.MkdirAll(cfg.WorkDir, 0o700); err != nil {
		note(rep, stepSetup, "", err)
		return rep, err
	}
	dir, err := os.MkdirTemp(cfg.WorkDir, "oss-capabilities-")
	if err != nil {
		note(rep, stepSetup, "", err)
		return rep, err
	}
	var c *cluster.Cluster
	defer func() { result = errors.Join(result, cleanup(c, dir, rep)) }()
	c, err = cluster.Start(ctx, cluster.Spec{Binary: cfg.Binary, Dir: dir, Nodes: 1})
	if err == nil {
		err = c.Init(ctx)
	}
	note(rep, stepSetup, "", err)
	if err != nil {
		return rep, err
	}
	conn, err := connect(ctx, c)
	if err != nil {
		note(rep, "OSS-CONNECT", "", err)
		return rep, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := conn.Close(closeCtx)
		note(rep, "OSS-CONNECTION-CLEANUP", "", err)
		result = errors.Join(result, err)
	}()
	importErr := importCSV(ctx, conn)
	note(rep, ClaimImport, ClaimImport, importErr)
	backupErr := rejectBackup(ctx, conn)
	note(rep, ClaimBackup, ClaimBackup, backupErr)
	return rep, errors.Join(importErr, backupErr)
}

func connect(ctx context.Context, c *cluster.Cluster) (*pgx.Conn, error) {
	url, err := c.SQLURL(0)
	if err != nil {
		return nil, err
	}
	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var last error
	for {
		conn, err := connectOnce(readyCtx, url)
		if err == nil {
			return conn, nil
		}
		last = err
		select {
		case <-readyCtx.Done():
			return nil, fmt.Errorf("OSS SQL readiness: %w", errors.Join(readyCtx.Err(), last))
		case <-tick.C:
		}
	}
}

func connectOnce(ctx context.Context, url string) (*pgx.Conn, error) {
	attempt, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return pgx.Connect(attempt, url)
}

func cleanup(c *cluster.Cluster, dir string, rep *report.Report) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	if c != nil {
		err = errors.Join(c.Stop(ctx), c.Reap())
	}
	if err == nil {
		err = os.RemoveAll(dir)
	}
	if err != nil {
		err = fmt.Errorf("OSS capabilities cleanup; work directory %s: %w", dir, err)
	}
	note(rep, "OSS-CLEANUP", "", err)
	return err
}

func note(rep *report.Report, step, claim string, err error) {
	outcome, detail := labels.Pass, ""
	if err != nil {
		outcome, detail = labels.Fail, err.Error()
	}
	if claim != "" {
		rep.Set(claim, outcome, detail)
	}
	rep.AddStep(report.StepResult{ID: step, Outcome: outcome, Detail: detail, Required: true})
}
