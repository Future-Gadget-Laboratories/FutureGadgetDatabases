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
)

const (
	checksumSQL = `SELECT count(*)::INT, COALESCE(sum(balance), 0)::INT, ` +
		`md5(COALESCE(string_agg(id::STRING || ':' || balance::STRING, ',' ORDER BY id), '')) ` +
		`FROM bank.bank`
	consistencySQL = `SELECT status FROM crdb_internal.check_consistency(true, ''::BYTES, ''::BYTES)`
	porcupineLimit = 60 * time.Second
)

func (r *run) bankAll() error {
	var first error
	for i := range r.cluster.Nodes() {
		if err := r.bankOn(i); err != nil && first == nil {
			first = err
		}
	}
	return first
}

const bankAttempts = 5

func (r *run) bankOn(index int) error {
	var last error
	for attempt := 0; attempt < bankAttempts; attempt++ {
		last = r.bankOnce(index)
		if last == nil || brokenBank(last) {
			return last
		}
		if !sleepCtx(r.ctx, time.Second) {
			return r.ctx.Err()
		}
	}
	return last
}

func (r *run) bankOnce(index int) error {
	out, err := r.cluster.SQL(r.ctx, index, bankSQL)
	if err != nil {
		return fmt.Errorf("bank check on node %d: %w", index+1, err)
	}
	count, sum, err := parsePair(out)
	if err != nil {
		return fmt.Errorf("bank check on node %d: %w", index+1, err)
	}
	if count != r.cfg.BankRows || sum != 0 {
		return bankBroken{node: index + 1, count: count, sum: sum, want: r.cfg.BankRows}
	}
	return nil
}

type bankBroken struct {
	node, count, sum, want int
}

func (e bankBroken) Error() string {
	return fmt.Sprintf("node %d bank is (%d, %d), want (%d, 0)", e.node, e.count, e.sum, e.want)
}

func brokenBank(err error) bool {
	var broken bankBroken
	return errors.As(err, &broken)
}

func parsePair(text string) (int, int, error) {
	var data []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "count") {
			continue
		}
		data = strings.Split(line, ",")
		break
	}
	if len(data) < 2 {
		return 0, 0, fmt.Errorf("unexpected bank result %q", text)
	}
	count, err := strconv.Atoi(strings.TrimSpace(data[0]))
	if err != nil {
		return 0, 0, err
	}
	sum, err := strconv.Atoi(strings.TrimSpace(data[1]))
	if err != nil {
		return 0, 0, err
	}
	return count, sum, nil
}

type bankSnap struct {
	count int
	sum   int
	hash  string
}

func (r *run) snapBank(index int) (bankSnap, error) {
	out, err := r.cluster.SQL(r.ctx, index, checksumSQL)
	if err != nil {
		return bankSnap{}, err
	}
	var snap bankSnap
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "count") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			return bankSnap{}, fmt.Errorf("unexpected checksum result %q", textOne(line))
		}
		snap.count, err = strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return bankSnap{}, err
		}
		snap.sum, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return bankSnap{}, err
		}
		snap.hash = strings.TrimSpace(parts[2])
		return snap, nil
	}
	return bankSnap{}, fmt.Errorf("checksum result was empty")
}

func textOne(text string) string {
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

func (r *run) porcupine() error {
	if !r.want(stepPorcupine) {
		return nil
	}
	ops, err := invariants.ReadHistory(r.history)
	if err != nil {
		return fmt.Errorf("porcupine check cannot run: %w", err)
	}
	if err := invariants.CheckHistory(ops, porcupineLimit); err != nil {
		return err
	}
	return nil
}

func (r *run) consistency() error {
	if !r.want(stepConsistency) {
		return nil
	}
	out, err := r.cluster.SQL(r.ctx, 0, consistencySQL)
	if err != nil {
		return fmt.Errorf("consistency check cannot run: %w", err)
	}
	return judgeConsistency(out)
}

func judgeConsistency(out string) error {
	rows := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "status" {
			continue
		}
		rows++
		if !consistentStatus(line) {
			return fmt.Errorf("consistency status %s", line)
		}
	}
	if rows == 0 {
		return fmt.Errorf("consistency check cannot run: no ranges were returned")
	}
	return nil
}

func consistentStatus(status string) bool {
	switch status {
	case "RANGE_CONSISTENT", "RANGE_CONSISTENT_STATS_ESTIMATED", "RANGE_INDETERMINATE":
		return true
	default:
		return false
	}
}
