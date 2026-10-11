// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/report"
)

// Config is one interim cluster run.
type Config struct {
	Candidate     string
	Previous      string
	WorkDir       string
	OutputDir     string
	RepoRoot      string
	SQLPortBase   int
	HTTPPortBase  int
	BankRows      int
	KVConcurrency int
	Outage        time.Duration
	PauseFor      time.Duration
	RecoverWait   time.Duration
	ReplicateWait time.Duration
	Warmup        time.Duration
	ExpectGo      string
	BackupBin     string
	Meta          report.Meta
	OnlySteps     map[string]struct{}
}

// ApplyDefaults fills zero values used by a laptop-sized run.
func ApplyDefaults(cfg Config) Config {
	if cfg.BankRows == 0 {
		cfg.BankRows = 20
	}
	if cfg.KVConcurrency == 0 {
		cfg.KVConcurrency = 4
	}
	if cfg.Outage == 0 {
		cfg.Outage = 12 * time.Second
	}
	if cfg.PauseFor == 0 {
		cfg.PauseFor = 8 * time.Second
	}
	if cfg.RecoverWait == 0 {
		cfg.RecoverWait = 15 * time.Second
	}
	if cfg.ReplicateWait == 0 {
		cfg.ReplicateWait = 120 * time.Second
	}
	if cfg.Warmup == 0 {
		cfg.Warmup = 5 * time.Second
	}
	if cfg.SQLPortBase == 0 {
		cfg.SQLPortBase = envInt("FGDB_SQL_PORT_BASE", 27257)
	}
	if cfg.HTTPPortBase == 0 {
		cfg.HTTPPortBase = envInt("FGDB_HTTP_PORT_BASE", 28080)
	}
	return cfg
}

// ParseOnly splits a comma-separated step list. An empty list runs every step.
func ParseOnly(text string) map[string]struct{} {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = struct{}{}
		}
	}
	return out
}

func envInt(name string, fallback int) int {
	text := os.Getenv(name)
	if text == "" {
		return fallback
	}
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
