// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package record is the history-recorder process.
// It is a separate OS process from the bank workload and the kv workload.
package record

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/invariants"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	insertAck = `INSERT INTO suite.acks (id) VALUES ($1)`
	upsertReg = `INSERT INTO suite.reg (k, v) VALUES (1, $1) ` +
		`ON CONFLICT (k) DO UPDATE SET v = excluded.v`
	readReg = `SELECT v FROM suite.reg WHERE k = 1`
	flagOut = "out"
)

// Config is the recorder command line.
type Config struct {
	Out      string
	URLs     []string
	Duration time.Duration
	Clients  int
}

// Run is the `record` subcommand. It exits 0 after a signal so the parent
// can read a flushed history file.
func Run(args []string) int {
	cfg, err := parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if err := serve(ctx, cfg); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String(flagOut, "", "history jsonl path")
	urls := fs.String("urls", "", "comma-separated postgres URLs")
	duration := fs.Duration("duration", 20*time.Minute, "how long to record")
	clients := fs.Int("clients", 3, "concurrent clients inside this process")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	cfg := Config{Out: *out, Duration: *duration, Clients: *clients, URLs: splitURLs(*urls)}
	if cfg.Out == "" || len(cfg.URLs) == 0 || cfg.Clients < 1 {
		return Config{}, fmt.Errorf("record needs --out, --urls, and --clients")
	}
	return cfg, nil
}

func splitURLs(text string) []string {
	var urls []string
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			urls = append(urls, part)
		}
	}
	return urls
}

func serve(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()
	pools, err := openPools(ctx, cfg.URLs)
	if err != nil {
		return err
	}
	defer closePools(pools)
	w, err := newWriter(cfg.Out)
	if err != nil {
		return err
	}
	defer w.close()
	var group sync.WaitGroup
	for i := 0; i < cfg.Clients; i++ {
		group.Add(1)
		go func(id int) {
			defer group.Done()
			oneClient(ctx, id, pools[id%len(pools)], w)
		}(i)
	}
	group.Wait()
	return nil
}

func openPools(ctx context.Context, urls []string) ([]*pgxpool.Pool, error) {
	pools := make([]*pgxpool.Pool, 0, len(urls))
	for _, url := range urls {
		cfg, err := pgxpool.ParseConfig(url)
		if err != nil {
			closePools(pools)
			return nil, err
		}
		cfg.MaxConns = 2
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			closePools(pools)
			return nil, err
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

func closePools(pools []*pgxpool.Pool) {
	for _, pool := range pools {
		pool.Close()
	}
}

func oneClient(ctx context.Context, id int, pool *pgxpool.Pool, w *histWriter) {
	for ctx.Err() == nil {
		writeOnce(ctx, id, pool, w)
		readOnce(ctx, id, pool, w)
	}
}

func writeOnce(ctx context.Context, id int, pool *pgxpool.Pool, w *histWriter) {
	value := int(atomic.AddInt64(&seq, 1))
	call := time.Now().UnixNano()
	err := writeTxn(ctx, pool, value)
	if ctx.Err() != nil && err != nil {
		return
	}
	_ = w.write(invariants.Op{
		Client: id, Kind: labels.KindWrite, Value: value,
		CallNS: call, ReturnNS: time.Now().UnixNano(), Result: classify(err),
	})
}

func readOnce(ctx context.Context, id int, pool *pgxpool.Pool, w *histWriter) {
	call := time.Now().UnixNano()
	value, err := readValue(ctx, pool)
	if ctx.Err() != nil && err != nil {
		return
	}
	_ = w.write(invariants.Op{
		Client: id, Kind: labels.KindRead, Value: value,
		CallNS: call, ReturnNS: time.Now().UnixNano(), Result: classify(err),
	})
}

var seq int64

func writeTxn(ctx context.Context, pool *pgxpool.Pool, value int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, insertAck, value); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, upsertReg, value); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func readValue(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var value int
	err := pool.QueryRow(ctx, readReg).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return value, err
}

func classify(err error) string {
	if err == nil {
		return labels.ResultOK
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "40001" {
		return labels.ResultFail
	}
	if errors.As(err, &pgErr) {
		return labels.ResultFail
	}
	if ambiguous(err.Error()) {
		return labels.ResultUnknown
	}
	return labels.ResultFail
}

func ambiguous(text string) bool {
	low := strings.ToLower(text)
	for _, needle := range []string{"timeout", "eof", "connection reset", "broken pipe", "deadline"} {
		if strings.Contains(low, needle) {
			return true
		}
	}
	return false
}

type histWriter struct {
	mu sync.Mutex
	f  *os.File
}

func newWriter(path string) (*histWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &histWriter{f: f}, nil
}

func (w *histWriter) write(op invariants.Op) error {
	body, err := json.Marshal(op)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.f.Write(body); err != nil {
		return err
	}
	return w.f.Sync()
}

func (w *histWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.f.Close()
}
