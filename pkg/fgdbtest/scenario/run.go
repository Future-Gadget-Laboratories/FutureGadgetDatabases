// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/cluster"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/faults"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/workdir"
)

// Run starts a 3-node cluster and records one claim outcome per interim step.
// Partition and disk faults are recorded as not tested. They are not skipped quietly.
func Run(ctx context.Context, cfg Config) (rep *report.Report, err error) {
	cfg = ApplyDefaults(cfg)
	rep = report.New(cfg.Meta)
	r := &run{ctx: ctx, cfg: cfg, rep: rep, history: filepath.Join(cfg.WorkDir, "history.jsonl")}
	defer func() {
		err = joinCleanup(err, r.finish())
	}()
	if err = rejectOnlySteps(cfg); err != nil {
		r.noteSetup(err)
		return rep, err
	}
	if err = r.prepare(); err != nil {
		r.noteSetup(err)
		return rep, err
	}
	r.phases()
	r.failUnrecorded("step did not run")
	if r.failed {
		err = errors.New("interim cluster slice failed")
	}
	return rep, err
}

type run struct {
	ctx       context.Context
	cfg       Config
	rep       *report.Report
	cluster   *cluster.Cluster
	clients   *procSet
	history   string
	clientBin string
	failed    bool
	halt      func(context.Context) error
}

const cleanupStep = "cleanup"

// joinCleanup keeps a stop or reap error attached to the scenario result.
func joinCleanup(result, cleanup error) error {
	return errors.Join(result, cleanup)
}

func (r *run) finish() error {
	err := r.stopAll()
	if err != nil {
		r.failed = true
		r.rep.AddStep(report.StepResult{
			ID: cleanupStep, Outcome: labels.Fail, Detail: err.Error(), Required: true,
		})
	}
	r.rep.Set(ClaimPartition, labels.NotTested, faults.Partition().Detail)
	r.rep.Set(ClaimDisk, labels.NotTested, faults.Disk().Detail)
	if r.cfg.OutputDir != "" {
		err = errors.Join(err, r.rep.Write(r.cfg.OutputDir))
	}
	return err
}

func (r *run) stopAll() error {
	var err error
	if r.clients != nil {
		err = errors.Join(err, r.clients.stop(), r.clients.reap())
	}
	err = errors.Join(err, r.haltNodes(context.Background()))
	if r.cluster != nil && r.halt == nil {
		err = errors.Join(err, r.cluster.Reap())
	}
	return err
}

func (r *run) haltNodes(ctx context.Context) error {
	if r.halt != nil {
		return r.halt(ctx)
	}
	if r.cluster == nil {
		return nil
	}
	return r.cluster.Stop(ctx)
}

func (r *run) prepare() error {
	if r.cfg.Candidate == "" {
		return errors.New("candidate cockroach binary is empty")
	}
	if err := workdir.Require(r.cfg.WorkDir); err != nil {
		return err
	}
	useSiblingLib(r.cfg.Candidate)
	useSiblingLib(r.cfg.Previous)
	if err := cluster.Version(r.ctx, r.cfg.Candidate, r.cfg.ExpectGo); err != nil {
		return err
	}
	bin, err := r.startBinary()
	if err != nil {
		return err
	}
	r.clientBin = bin
	spec := cluster.Spec{
		Binary: bin,
		Dir:    filepath.Join(r.cfg.WorkDir, "cluster"),
	}
	started, err := cluster.Start(r.ctx, spec)
	if err != nil {
		return err
	}
	r.cluster = started
	if err := r.bootstrap(); err != nil {
		return err
	}
	return r.startClients()
}

func (r *run) startBinary() (string, error) {
	if r.cfg.Previous == "" {
		return r.cfg.Candidate, nil
	}
	same, err := sameBinary(r.cfg.Previous, r.cfg.Candidate)
	if err != nil {
		return "", err
	}
	if same {
		return r.cfg.Candidate, nil
	}
	if err := cluster.Version(r.ctx, r.cfg.Previous, r.cfg.ExpectGo); err != nil {
		return "", err
	}
	return r.cfg.Previous, nil
}

func (r *run) phases() {
	r.note(ClaimUpgrade, r.upgrade())
	if err := r.bankAll(); err != nil {
		r.note(ClaimBank, err)
	} else {
		r.note(ClaimBank, nil)
	}
	r.note(ClaimKill, r.killOne())
	r.note(ClaimPause, r.pauseOne())
	r.note(ClaimKillTwo, r.killTwo())
	r.stopClients()
	r.note(ClaimBank, r.bankAll())
	r.note(ClaimPorcupine, r.porcupine())
	r.note(ClaimConsistency, r.consistency())
	r.note(ClaimBackup, r.backup())
}

func (r *run) want(step string) bool {
	if len(r.cfg.OnlySteps) == 0 {
		return true
	}
	_, ok := r.cfg.OnlySteps[step]
	return ok
}

const (
	tierInterim    = "interim"
	tierRelease    = "release"
	omittedDetail  = "omitted by FGDB_ONLY_STEPS"
	onlyStepsBlock = "FGDB_ONLY_STEPS is not allowed when the cluster is required or the tier is interim or release"
)

// rejectOnlySteps reports a filter that would skip required checks on a
// release run. A local tier may filter, and those runs are not a pass.
func rejectOnlySteps(cfg Config) error {
	if len(cfg.OnlySteps) == 0 {
		return nil
	}
	if cfg.RequireCluster || cfg.Meta.Tier == tierInterim || cfg.Meta.Tier == tierRelease {
		return errors.New(onlyStepsBlock)
	}
	return nil
}

// errNotRun means the step was filtered out. Its claim stays unrecorded
// until the omitted-check pass marks it not tested.
var errNotRun = errors.New("step was not selected")

func (r *run) note(id string, err error) {
	if errors.Is(err, errNotRun) {
		return
	}
	outcome := labels.Pass
	detail := ""
	if err != nil {
		outcome = labels.Fail
		detail = err.Error()
		r.failed = true
	}
	r.rep.Set(id, outcome, detail)
	r.rep.AddStep(report.StepResult{ID: id, Outcome: outcome, Detail: detail, Required: true})
}

func stepFor(id string) string {
	switch id {
	case ClaimUpgrade:
		return stepUpgrade
	case ClaimKill:
		return stepKill
	case ClaimPause:
		return stepPause
	case ClaimKillTwo:
		return stepKillTwo
	case ClaimPorcupine:
		return stepPorcupine
	case ClaimConsistency:
		return stepConsistency
	case ClaimBackup:
		return stepBackup
	default:
		return id
	}
}

func (r *run) noteSetup(err error) {
	if len(r.cfg.OnlySteps) == 0 {
		r.failUnrecorded(err.Error())
		return
	}
	r.rep.AddStep(report.StepResult{
		ID: "setup", Outcome: labels.Fail, Detail: err.Error(), Required: true,
	})
	r.failed = true
}

func (r *run) failUnrecorded(msg string) {
	outcome := labels.Fail
	detail := msg
	if len(r.cfg.OnlySteps) > 0 {
		outcome = labels.NotTested
		detail = omittedDetail
	}
	for _, id := range ClusterClaims() {
		if id == ClaimPartition || id == ClaimDisk {
			continue
		}
		r.missed(id, outcome, detail)
	}
}

func (r *run) missed(id, outcome, detail string) {
	if r.rep.Outcome(id) != "" {
		return
	}
	r.rep.Set(id, outcome, detail)
	r.rep.AddStep(report.StepResult{ID: id, Outcome: outcome, Detail: detail, Required: true})
	r.failed = true
}

func (r *run) stopClients() error {
	if r.clients == nil {
		return nil
	}
	return r.clients.stop()
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *run) waitHealthy() error {
	deadline := time.Now().Add(r.cfg.ReplicateWait)
	var last error
	for time.Now().Before(deadline) {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		unavail, under, err := r.health()
		if err == nil && unavail == 0 && under == 0 {
			return nil
		}
		last = fmt.Errorf("unavailable=%d under-replicated=%d err=%v", unavail, under, err)
		if !sleepCtx(r.ctx, 2*time.Second) {
			return r.ctx.Err()
		}
	}
	return fmt.Errorf("ranges did not recover: %v", last)
}

func (r *run) health() (int, int, error) {
	var last error
	for i := range r.cluster.Nodes() {
		unavail, under, err := r.cluster.RangeHealth(r.ctx, i)
		if err == nil {
			return unavail, under, nil
		}
		last = err
	}
	return 0, 0, last
}
