// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package cluster

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SQL runs one statement through `cockroach sql` and returns stdout.
func (c *Cluster) SQL(ctx context.Context, index int, statement string) (string, error) {
	n, err := c.node(index)
	if err != nil {
		return "", err
	}
	return c.run(ctx, n.Binary, []string{
		"sql", insecureFlag, "--host=" + n.Listen, "--format=csv", "-e", statement,
	})
}

// Exec runs an arbitrary cockroach subcommand with the cluster environment.
func (c *Cluster) Exec(ctx context.Context, index int, args ...string) (string, error) {
	n, err := c.node(index)
	if err != nil {
		return "", err
	}
	return c.run(ctx, n.Binary, args)
}

func (c *Cluster) run(ctx context.Context, binary string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	if err := c.applyEnv(cmd); err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%w\n%s", err, trimOutput(stderr.String()))
	}
	return stdout.String(), nil
}

// runStart launches a --background node. Stdout is a file, not a pipe.
// A pipe stays open in the daemon, and cmd.Run would wait until the node exits.
func (c *Cluster) runStart(ctx context.Context, n Node, args []string) error {
	logFile, err := os.OpenFile(filepath.Join(n.StoreDir, "start.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.CommandContext(ctx, n.Binary, args...)
	if err := c.applyEnv(cmd); err != nil {
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		body, _ := os.ReadFile(logFile.Name())
		return fmt.Errorf("%w\n%s", err, trimOutput(string(body)))
	}
	return nil
}

func (c *Cluster) applyEnv(cmd *exec.Cmd) error {
	base := []string(nil)
	if c != nil {
		base = c.env
	}
	env, err := envFor(base, cmd.Path)
	if err != nil {
		return err
	}
	cmd.Env = env
	return nil
}

func envFor(base []string, binary string) ([]string, error) {
	if len(base) == 0 {
		var err error
		base, err = CommandEnv()
		if err != nil {
			return nil, err
		}
	}
	return withReleaseLib(base, binary)
}

func trimOutput(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 2000 {
		return text[len(text)-2000:]
	}
	return text
}

// Init initializes the cluster through the first node.
func (c *Cluster) Init(ctx context.Context) error {
	n, err := c.node(0)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, n.Binary, []string{"init", insecureFlag, "--host=" + n.Listen})
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return nil
}

// Version runs `version` on the cluster binary and checks Distribution: OSS.
func Version(ctx context.Context, binary, expectGo string) error {
	cmd := exec.CommandContext(ctx, binary, "version")
	env, err := ProcessEnv(binary)
	if err != nil {
		return err
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("version: %w\n%s", err, trimOutput(string(out)))
	}
	text := string(out)
	if !hasField(text, "Distribution:", "OSS") {
		return fmt.Errorf("version did not report Distribution: OSS\n%s", text)
	}
	if expectGo != "" && !strings.Contains(text, expectGo) {
		return fmt.Errorf("version did not report %s\n%s", expectGo, text)
	}
	return nil
}

func hasField(text, label, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), label) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return false
		}
		return fields[len(fields)-1] == want
	}
	return false
}

// RangeHealth sums unavailable and under-replicated range counts from node status.
func (c *Cluster) RangeHealth(ctx context.Context, index int) (unavailable, under int, err error) {
	n, err := c.node(index)
	if err != nil {
		return 0, 0, err
	}
	out, err := c.run(ctx, n.Binary, []string{
		"node", "status", "--ranges", insecureFlag, "--host=" + n.Listen, "--format=csv",
	})
	if err != nil {
		return 0, 0, err
	}
	return sumColumns(out, "ranges_unavailable", "ranges_underreplicated")
}

func sumColumns(text, left, right string) (int, int, error) {
	rows, err := csvRows(text)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) < 2 {
		return 0, 0, fmt.Errorf("node status returned no rows")
	}
	li, ri, err := headerIndexes(rows[0], left, right)
	if err != nil {
		return 0, 0, err
	}
	var a, b int
	for _, row := range rows[1:] {
		if len(row) <= li || len(row) <= ri {
			continue
		}
		av, aerr := atoi(row[li])
		bv, berr := atoi(row[ri])
		if aerr != nil || berr != nil {
			return 0, 0, fmt.Errorf("node status number: %v %v", aerr, berr)
		}
		a += av
		b += bv
	}
	return a, b, nil
}

func headerIndexes(header []string, left, right string) (int, int, error) {
	li, ri := -1, -1
	for i, name := range header {
		switch strings.TrimSpace(name) {
		case left:
			li = i
		case right:
			ri = i
		}
	}
	if li < 0 || ri < 0 {
		return 0, 0, fmt.Errorf("node status is missing %s or %s", left, right)
	}
	return li, ri, nil
}

func csvRows(text string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	return r.ReadAll()
}

func atoi(text string) (int, error) {
	n := 0
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("empty number")
	}
	neg := false
	if text[0] == '-' {
		neg = true
		text = text[1:]
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a number %q", text)
		}
		n = n*10 + int(ch-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

const (
	pgUser  = "postgresql://root@"
	pgQuery = "?sslmode=disable"
	pgDB    = "/defaultdb"
)

// SQLURL is a PostgreSQL URL for an insecure node, including defaultdb.
func (c *Cluster) SQLURL(index int) (string, error) {
	return c.pgURL(index, pgDB)
}

// WorkloadURL is a PostgreSQL URL with no database name.
// The workload command rejects a URL whose database is not its own.
func (c *Cluster) WorkloadURL(index int) (string, error) {
	return c.pgURL(index, "")
}

func (c *Cluster) pgURL(index int, database string) (string, error) {
	n, err := c.node(index)
	if err != nil {
		return "", err
	}
	return pgUser + n.Listen + database + pgQuery, nil
}

// Drain asks one node to shed leases before a planned stop.
func (c *Cluster) Drain(ctx context.Context, index int) error {
	n, err := c.node(index)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, n.Binary, []string{
		"node", "drain", "--self", insecureFlag, "--host=" + n.Listen,
	})
	if err != nil {
		return fmt.Errorf("drain node %d: %w", index+1, err)
	}
	return nil
}
