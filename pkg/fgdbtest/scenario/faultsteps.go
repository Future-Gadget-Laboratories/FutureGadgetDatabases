// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/invariants"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

const bankSQL = `SELECT count(*)::INT, COALESCE(sum(balance), 0)::INT FROM bank.bank`

func (r *run) killOne() error {
	if !r.want(stepKill) {
		return errNotRun
	}
	if err := r.cluster.KillNode(0); err != nil {
		return err
	}
	if !sleepCtx(r.ctx, r.cfg.Outage) {
		return r.ctx.Err()
	}
	// Restart even when the invariant fails, so later steps are not dial errors.
	bankErr := r.bankOn(1)
	if err := r.cluster.StartNode(r.ctx, 0); err != nil {
		return errors.Join(bankErr, err)
	}
	if err := r.waitHealthy(); err != nil {
		return errors.Join(bankErr, err)
	}
	return bankErr
}

func (r *run) pauseOne() error {
	if !r.want(stepPause) {
		return errNotRun
	}
	if err := r.cluster.PauseNode(1); err != nil {
		return err
	}
	if !sleepCtx(r.ctx, r.cfg.PauseFor) {
		return r.ctx.Err()
	}
	if err := r.cluster.ResumeNode(1); err != nil {
		return err
	}
	return r.bankOn(0)
}

func (r *run) killTwo() error {
	if !r.want(stepKillTwo) {
		return errNotRun
	}
	if err := r.cluster.KillNode(1); err != nil {
		return err
	}
	if err := r.cluster.KillNode(2); err != nil {
		return err
	}
	// Count only acknowledgements that return while both nodes are already down.
	start := time.Now().UnixNano()
	if !sleepCtx(r.ctx, r.cfg.Outage) {
		return r.ctx.Err()
	}
	end := time.Now().UnixNano()
	if err := r.noAcksDuring(start, end); err != nil {
		return err
	}
	if err := r.cluster.StartNode(r.ctx, 1); err != nil {
		return err
	}
	if err := r.cluster.StartNode(r.ctx, 2); err != nil {
		return err
	}
	if err := r.waitHealthy(); err != nil {
		return err
	}
	if !sleepCtx(r.ctx, r.cfg.RecoverWait) {
		return r.ctx.Err()
	}
	if err := r.recovered(end); err != nil {
		return err
	}
	if err := r.bankAll(); err != nil {
		return err
	}
	return r.acksPresent()
}

func (r *run) noAcksDuring(start, end int64) error {
	ops, err := invariants.ReadHistory(r.history)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return fmt.Errorf("history recorder produced no operations")
	}
	n := invariants.OKWritesInWindow(ops, start, end)
	if n > 0 {
		return fmt.Errorf("%d acknowledged writes while two nodes were down", n)
	}
	return nil
}

func (r *run) recovered(after int64) error {
	ops, err := invariants.ReadHistory(r.history)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if op.Kind == labels.KindWrite && op.Result == labels.ResultOK && op.CallNS > after {
			return nil
		}
	}
	return fmt.Errorf("no acknowledged write after the two nodes restarted")
}

func (r *run) acksPresent() error {
	ops, err := invariants.ReadHistory(r.history)
	if err != nil {
		return err
	}
	got, err := r.ackIDs()
	if err != nil {
		return err
	}
	var missing []int
	for _, value := range invariants.OKWriteValues(ops) {
		if _, ok := got[value]; !ok {
			missing = append(missing, value)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("acknowledged writes missing after recovery: %v", missing)
	}
	return nil
}

func (r *run) ackIDs() (map[int]struct{}, error) {
	out, err := r.cluster.SQL(r.ctx, 0, `SELECT id FROM suite.acks`)
	if err != nil {
		return nil, err
	}
	ids := map[int]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "id" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("ack id %q: %w", line, err)
		}
		ids[n] = struct{}{}
	}
	return ids, nil
}
