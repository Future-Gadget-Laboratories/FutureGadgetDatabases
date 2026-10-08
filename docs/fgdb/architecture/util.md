> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# pkg/util

`pkg/util` is a large toolbox. Most of it is data structures (`interval`, `hlc` aside, `encoding`, `cache`). Four packages show up in almost every stack trace that matters. The rest of this page is only those four, plus one line each for a few neighbors.

## hlc — hybrid logical clock

**File:** `pkg/util/hlc/hlc.go`, `timestamp.go`, `timestamp.proto`. The long comment is `pkg/util/hlc/doc.go`. Read that instead of a blog post. It is the contract.

A `Timestamp` is a wall time in nanoseconds plus a logical counter (`int32`). `Clock.Now()` returns one. If the physical clock did not move, the logical part increments, so two calls in the same nanosecond still order.

SQL transactions get a timestamp from this clock. MVCC uses it as the version suffix. Raft does not use it as the log index. The log index and the MVCC timestamp are different numbers.

`Clock` tracks the highest timestamp it has seen, including ones that arrived on RPC heartbeats, so a node does not hand out a timestamp behind a write it has already observed.

**Invariant:** timestamps from one clock are monotonic. Timestamps from two clocks are monotonic only if the wall clocks stay within `MaxOffset`. That is why [rpc.md](rpc.md) kills a node whose offset is too far, instead of serving reads that could miss a write.

**Gotcha:** `timestamp.proto` is the on-disk and on-the-wire form. Do not invent a third field and expect old stores to decode it. `Clock.Now` (`hlc.go`) increments the logical counter when the wall clock has not moved past the last timestamp, and resets it to 0 when the wall clock has. `Clock.SleepUntil` is the helper that waits until the HLC reaches a timestamp. It is not called from a normal `SELECT`.

## stop — Stopper

**File:** `pkg/util/stop/stopper.go`.

A `Stopper` is the process's shutdown handle. Background work (`Registry.Start`, gossip clients, the settings watcher) is started with `RunAsyncTask` so `Stop` can wait for it. Closers (engines, listeners) are registered on it too.

Quiesce is the first phase: stop accepting new work, cancel contexts. Stop is the second: wait, then close.

**Invariant:** a task that does not use the stopper can outlive the engine and write into a closed Pebble. New long-running goroutines in server code go through the stopper.

**Gotcha:** `Stop` from inside a task that the same stopper is waiting for deadlocks. Tests use `pkg/util/leaktest` to catch goroutines that escaped.

## log

**File:** `pkg/util/log/log.go`.

This is the logging package every other package imports. Severities are the usual ones. `V(level)` is verbose logging. `ExpensiveLogEnabled` (same file) is the check you use before formatting a large trace-level string, and it knows about tracing spans.

The package also owns the structured event log (`pkg/util/log/eventpb`) used for audit-ish SQL events. Role-based audit configuration is a no-op hook in `pkg/sql/auditlogging` (see [BOUNDARIES.md](BOUNDARIES.md)). The event log itself still records the events the OSS code emits.

**Gotcha:** `log.Ops.Fatalf` really exits. The clock-skew path uses it. A library that "logs fatal" on a bad user statement will take the node down. User errors return `error`. `Fatalf` is for "continuing would violate a correctness assumption."

## tracing

**File:** `pkg/util/tracing/tracer.go`.

A `Tracer` builds spans. SQL sessions, DistSender batches, and Raft all nest spans under the statement span when tracing is on. The tracer can run as a no-op (the default cost) or record spans. `maxRecordedSpansPerTrace` in `tracer.go` is 1000, so a trace of a giant plan is truncated.

`EXPLAIN ANALYZE` and `SHOW TRACE FOR SESSION` are the SQL-facing doors. They do not change the plan. They attach a recording span.

**Gotcha:** a span that is not finished is a leak, and it shows up in traces as a hang. `crdb_internal.node_inflight_trace_spans` (the virtual table, if you are looking) is fed from this registry. Recording every request in production is how you run a node out of memory. The 1000-span cap is per trace, not global.

## Neighbors, one line each

| Package | Why it shows up |
| --- | --- |
| `pkg/util/metric` | Counters and histograms. The package comment in `pkg/util/metric/doc.go` says they are what gets written to the timeseries DB the console graphs. |
| `pkg/util/retry` | Backoff helper (`Options`). DistSender and jobs use it. |
| `pkg/util/leaktest` | Test helper that fails if a test leaves goroutines behind. |
| `pkg/util/uuid` | Transaction ids, cluster id. |
| `pkg/util/encoding` | Key encoding used by `pkg/keys` and SQL row encoding. |
| `pkg/util/timeutil` | Clock source wrapper so tests can inject time. |

`pkg/util` does not depend on `pkg/sql` or `pkg/kv/kvserver`. Those depend on it. If you are about to import `pkg/sql` from a util package, that dependency is backwards.
