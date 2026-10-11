// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"fmt"
	"path/filepath"
)

func (r *run) upgrade() error {
	if !r.want(stepUpgrade) {
		return nil
	}
	if r.cfg.Previous == "" || r.cfg.Candidate == "" {
		return fmt.Errorf("rolling upgrade needs the previous release binary and the candidate binary")
	}
	same, err := sameBinary(r.cfg.Previous, r.cfg.Candidate)
	if err != nil {
		return err
	}
	if same {
		return fmt.Errorf("rolling upgrade needs two different binaries")
	}
	for i := range r.cluster.Nodes() {
		if err := r.rollOne(i); err != nil {
			return err
		}
	}
	return r.binsMatchCandidate()
}

func (r *run) rollOne(index int) error {
	if err := r.cluster.Drain(r.ctx, index); err != nil {
		return err
	}
	if err := r.cluster.StopNode(r.ctx, index); err != nil {
		return err
	}
	if err := r.cluster.SetBinary(index, r.cfg.Candidate); err != nil {
		return err
	}
	if err := r.cluster.StartNode(r.ctx, index); err != nil {
		return err
	}
	_, err := r.cluster.SQL(r.ctx, index, `SELECT 1`)
	if err != nil {
		return fmt.Errorf("node %d did not serve SQL after restart: %w", index+1, err)
	}
	return nil
}

func (r *run) binsMatchCandidate() error {
	want, err := filepath.EvalSymlinks(r.cfg.Candidate)
	if err != nil {
		return err
	}
	for i := range r.cluster.Nodes() {
		got, err := r.cluster.RunningBinary(i)
		if err != nil {
			return err
		}
		got, err = filepath.EvalSymlinks(got)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("node %d is running %s, want %s", i+1, got, want)
		}
	}
	return nil
}
