// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"fmt"
	"strings"
)

const zoneReplicas = "num_replicas = 3"

var zoneStmts = []string{
	`ALTER RANGE meta CONFIGURE ZONE USING num_replicas = 3`,
	`ALTER RANGE liveness CONFIGURE ZONE USING num_replicas = 3`,
	`ALTER RANGE system CONFIGURE ZONE USING num_replicas = 3`,
	`ALTER DATABASE system CONFIGURE ZONE USING num_replicas = 3`,
}

var suiteStmts = []string{
	`CREATE DATABASE IF NOT EXISTS suite`,
	`CREATE TABLE IF NOT EXISTS suite.acks (id INT8 PRIMARY KEY)`,
	`CREATE TABLE IF NOT EXISTS suite.reg (k INT8 PRIMARY KEY, v INT8 NOT NULL)`,
}

func (r *run) bootstrap() error {
	if err := r.cluster.Init(r.ctx); err != nil {
		return err
	}
	for _, stmt := range zoneStmts {
		if _, err := r.cluster.SQL(r.ctx, 0, stmt); err != nil {
			return err
		}
	}
	out, err := r.cluster.SQL(r.ctx, 0, `SHOW ZONE CONFIGURATION FROM RANGE default`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, zoneReplicas) {
		return fmt.Errorf("default zone is not %s: %s", zoneReplicas, out)
	}
	if err := r.waitHealthy(); err != nil {
		r.note(ClaimRepl, err)
		return err
	}
	r.note(ClaimRepl, nil)
	for _, stmt := range suiteStmts {
		if _, err := r.cluster.SQL(r.ctx, 0, stmt); err != nil {
			return err
		}
	}
	return r.initWorkloads()
}

func (r *run) initWorkloads() error {
	url, err := r.cluster.WorkloadURL(0)
	if err != nil {
		return err
	}
	if _, err := r.cluster.Exec(r.ctx, 0, "workload", "init", "bank",
		fmt.Sprintf(rowsFlagFmt, r.cfg.BankRows), "--data-loader=insert", "--init-conns=2", url); err != nil {
		return fmt.Errorf("init bank: %w", err)
	}
	if _, err := r.cluster.Exec(r.ctx, 0, "workload", "init", "kv",
		"--data-loader=insert", "--init-conns=2", url); err != nil {
		return fmt.Errorf("init kv: %w", err)
	}
	return nil
}
