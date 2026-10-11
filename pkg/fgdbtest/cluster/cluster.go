// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package cluster starts a local cockroach-oss process group.
package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultNodes     = 3
	defaultCache     = "128MiB"
	defaultSQLMemory = "128MiB"
	startTimeout     = 3 * time.Minute
	stopTimeout      = 30 * time.Second
	diagnosticsEnv   = "COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=true"
	insecureFlag     = "--insecure"
)

// Spec describes one local cluster.
// SQL and HTTP ports are chosen when the cluster starts.
type Spec struct {
	Binary    string
	Dir       string
	Nodes     int
	Cache     string
	SQLMemory string
}

// Node is one cockroach-oss process.
type Node struct {
	Index    int
	Binary   string
	Listen   string
	HTTP     string
	StoreDir string
	PidFile  string
	Cache    string
	SQLMem   string
	ID       Identity
}

// Cluster is a group of nodes that share one join list.
type Cluster struct {
	spec  Spec
	nodes []Node
	env   []string
	join  string
}

// Start creates store directories and launches every node.
// A bind conflict picks a new set of ports and tries again.
func Start(ctx context.Context, spec Spec) (*Cluster, error) {
	spec = fillSpec(spec)
	if err := checkBinary(spec.Binary); err != nil {
		return nil, err
	}
	var last error
	for attempt := 0; attempt < portAttempts; attempt++ {
		c, err := launch(ctx, spec, attempt)
		if err == nil {
			return c, nil
		}
		last = err
		if !bindConflict(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("ports stayed busy: %w", last)
}

func checkBinary(path string) error {
	if path == "" {
		return fmt.Errorf("cockroach binary is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("cockroach binary: %w", err)
	}
	return nil
}

func launch(ctx context.Context, spec Spec, attempt int) (*Cluster, error) {
	spec.Dir = filepath.Join(spec.Dir, fmt.Sprintf("try-%d", attempt))
	c := &Cluster{spec: spec, env: append(os.Environ(), diagnosticsEnv)}
	if err := c.layout(); err != nil {
		return nil, err
	}
	for i := range c.nodes {
		if err := c.StartNode(ctx, i); err != nil {
			return nil, errors.Join(err, c.Stop(context.Background()))
		}
	}
	return c, nil
}

func fillSpec(spec Spec) Spec {
	if spec.Nodes == 0 {
		spec.Nodes = defaultNodes
	}
	if spec.Cache == "" {
		spec.Cache = defaultCache
	}
	if spec.SQLMemory == "" {
		spec.SQLMemory = defaultSQLMemory
	}
	return spec
}

func (c *Cluster) layout() error {
	if err := os.MkdirAll(c.spec.Dir, 0o755); err != nil {
		return err
	}
	sqlPorts, httpPorts, err := assignPorts(c.spec.Nodes)
	if err != nil {
		return err
	}
	c.nodes = make([]Node, c.spec.Nodes)
	addrs := make([]string, c.spec.Nodes)
	for i := 0; i < c.spec.Nodes; i++ {
		listen := fmt.Sprintf(listenFormat, sqlPorts[i])
		n := Node{
			Index:    i,
			Binary:   c.spec.Binary,
			Listen:   listen,
			HTTP:     fmt.Sprintf(listenFormat, httpPorts[i]),
			StoreDir: filepath.Join(c.spec.Dir, fmt.Sprintf("node%d", i+1)),
			PidFile:  filepath.Join(c.spec.Dir, fmt.Sprintf("node%d.pid", i+1)),
			Cache:    c.spec.Cache,
			SQLMem:   c.spec.SQLMemory,
		}
		if err := os.MkdirAll(n.StoreDir, 0o755); err != nil {
			return err
		}
		c.nodes[i] = n
		addrs[i] = listen
	}
	c.join = joinAddrs(addrs)
	return nil
}

func joinAddrs(addrs []string) string {
	out := ""
	for i, addr := range addrs {
		if i > 0 {
			out += ","
		}
		out += addr
	}
	return out
}

// Nodes returns the process records.
func (c *Cluster) Nodes() []Node {
	out := make([]Node, len(c.nodes))
	copy(out, c.nodes)
	return out
}

// SetBinary changes the binary used the next time that node starts.
func (c *Cluster) SetBinary(index int, binary string) error {
	if index < 0 || index >= len(c.nodes) {
		return fmt.Errorf("node index %d is out of range", index)
	}
	c.nodes[index].Binary = binary
	return nil
}

// StartNode launches one node. The binary daemonizes itself with --background.
func (c *Cluster) StartNode(ctx context.Context, index int) error {
	n, err := c.node(index)
	if err != nil {
		return err
	}
	if n.ID.PID > 0 {
		if err := Match(n.ID); err == nil {
			return fmt.Errorf("node %d is already running", index+1)
		}
	}
	_ = os.Remove(n.PidFile)
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	if err := c.runStart(startCtx, n, startArgs(n, c.join)); err != nil {
		return startNodeErr(index, err)
	}
	return c.noteStarted(index, n)
}

func startNodeErr(index int, err error) error {
	return fmt.Errorf("start node %d: %w", index+1, err)
}

func (c *Cluster) noteStarted(index int, n Node) error {
	pid, err := readPid(n.PidFile)
	if err != nil {
		return fmt.Errorf("start node %d: pid file: %w", index+1, err)
	}
	id, err := Capture(pid, n.Binary)
	if err != nil {
		return startNodeErr(index, err)
	}
	c.nodes[index].ID = id
	return nil
}

func startArgs(n Node, join string) []string {
	return []string{
		"start",
		insecureFlag,
		"--store=" + n.StoreDir,
		"--listen-addr=" + n.Listen,
		"--advertise-addr=" + n.Listen,
		"--http-addr=" + n.HTTP,
		"--join=" + join,
		"--pid-file=" + n.PidFile,
		"--cache=" + n.Cache,
		"--max-sql-memory=" + n.SQLMem,
		"--background",
	}
}

// Stop signals every node and waits for it to exit.
func (c *Cluster) Stop(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var err error
	for i := range c.nodes {
		err = errors.Join(err, c.StopNode(ctx, i))
	}
	return err
}

// StopNode sends SIGTERM, then SIGKILL if the process stays up.
func (c *Cluster) StopNode(ctx context.Context, index int) error {
	return c.signalNode(ctx, index, false)
}

// KillNode stops a node with SIGKILL. The store directory is left in place.
func (c *Cluster) KillNode(index int) error {
	return c.signalNode(context.Background(), index, true)
}

// PauseNode freezes a node with SIGSTOP.
func (c *Cluster) PauseNode(index int) error {
	return c.signalOwned(index, sigStop)
}

// ResumeNode continues a paused node with SIGCONT.
func (c *Cluster) ResumeNode(index int) error {
	return c.signalOwned(index, sigCont)
}

func (c *Cluster) node(index int) (Node, error) {
	if c == nil || index < 0 || index >= len(c.nodes) {
		return Node{}, fmt.Errorf("node index %d is out of range", index)
	}
	return c.nodes[index], nil
}
