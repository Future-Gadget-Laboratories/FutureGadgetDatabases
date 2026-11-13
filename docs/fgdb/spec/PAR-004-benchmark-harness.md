> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-004 benchmark harness and baseline

**Status: Proposed, v1.2**

## Scope and non-goals

This specification defines the benchmark tooling, pinned run configuration, result format, and regression gates for PAR-004. It does not change database or core code. A later PR will add the harness under `fgdb/bench/`; another later PR will wire it into CI.

This version covers the required macro workloads M-KV0, M-KV95, M-YCSB-A..F, M-TPCC-D, and M-BANK, plus the microbenchmark suites U-STORAGE, U-OPT, and U-SQL. M-RECOVERY is deferred to a later version. It defines how to record the v23.2.15 baseline; it does not claim that baseline has already been captured.

## Systems

**B0** is stock v23.2.15 `cockroach-oss`, Go go1.21.12, commit `3497fb02dce0beb6fc2bbf76c1ed7ad69cc31344`, as pinned in [the roadmap](../roadmap.md) and [license notes](../license.md). **S0** is the FGDb build under test. Both are Apache-2.0 systems. The fixed baseline comparison is S0 against B0, on the same hardware and with the same workload settings. Other system comparisons are only referenced in the benchmark plan and are outside this harness specification.

License limits may affect whether some third-party benchmark results can be used or published; verify applicable terms before each campaign.

## Workloads

Each run records the workload ID, its versioned invocation/configuration, and any permitted adjustments in `config`.

| ID | Measures |
| --- | --- |
| M-KV0 | Uniform-key, 100% write throughput and the write path. |
| M-KV95 | Point-read throughput at 95% reads and the read path. |
| M-YCSB-A..F | Standard workload mixes: A update-heavy, B read-mostly, C read-only, D read-latest, E short scans, and F read-modify-write. Record the specific letter on each run. |
| M-TPCC-D | TPC-C-derived mixed OLTP and multi-statement transactions, with a fixed warehouse count per hardware class. Report derived new orders per minute; do not call the result tpmC. |
| M-BANK | Account transfers under load, including transactional correctness and contention behavior. |
| U-STORAGE | MVCC storage reads, writes, scans, garbage collection, export, and intent operations. |
| U-OPT | SQL optimizer phase, end-to-end, and foreign-key insert benchmark performance. |
| U-SQL | In-process SQL query and planning performance. |

## Pinned configuration contract

Every accepted run must record the following before results can be compared:

- **Binaries:** system name, binary version, source commit SHA, build toolchain version, and build flags. Pin the B0 tag and commit; identify the S0 commit under test.
- **Workload:** workload ID and variant, tool name/version, exact arguments, data size and loading method, relevant settings, and a configuration hash where available.
- **Cluster:** node count, replication factor, isolation level, and all non-default database settings.
- **Hardware class:** a stable class ID and descriptive fields for CPU model/count, memory, storage type, network, operating system/kernel, filesystem, and client placement. Record material frequency or performance settings. No machine or lab names are part of the public class description.
- **Run protocol:** warm-up duration, measured duration, number of repetitions, and random seed. Macro runs use at least five repetitions, at least five minutes of discarded warm-up, and at least ten minutes measured, except workloads that run to completion. Micro runs use ten repetitions. Record exact values when they differ.
- **Environment:** benchmark tool versions and any configuration needed to reproduce the run. An unrecorded change invalidates the comparison.

## Result schema v1

Each run is one JSON object. `schema_version` is the string `"1"`. Timestamps use RFC 3339 UTC. Durations use seconds; throughput units are `ops_s` or, for M-TPCC-D, `new_orders_per_min`; latency values use milliseconds; `cv` is a unitless fraction (for example, `0.02` means 2%). The micro `time_ns_op` measurement is in nanoseconds per operation, the one exception to the seconds convention. Macro top-level throughput and latency metrics are medians across repetitions. Each micro benchmark stores per-repetition samples and the medians of those samples in its top-level `time_ns_op`, `bytes_op`, and `allocs_op` fields. The latency fields summarize the measured-window client latency distribution. Per-repetition `errors` count failed operations during that window; top-level `errors_total` is their sum. Any `errors_total` greater than zero makes a run invalid for gating. For M-BANK, the sum of per-repetition `correctness_violations` must also be zero or the gate fails outright. The schema is a discriminated union: `kind="macro"` requires the macro fields below; `kind="micro"` requires only common fields plus `micro` and `artifact`, with macro-only fields absent.

| Field | Applies to | Type | Meaning |
| --- | --- | --- | --- |
| `schema_version` | all | string | Result format version, `"1"`. |
| `run_id` | all | string | Unique identifier for this run. |
| `kind` | all | string | `macro` or `micro`; selects the required fields. |
| `system` | all | string | For macro runs, `B0` or `S0`; for micro runs, `main` or `candidate`. |
| `binary_version` | all | string | Human-readable binary version. |
| `commit` | all | string | Source commit SHA used to build the binary. |
| `workload_id` | all | string | Workload ID and, for M-YCSB, the selected letter. |
| `config` | all | object | Pinned workload, cluster, tool, and run-protocol settings, including the seed. |
| `hardware_class` | all | object | Hardware class ID and descriptive hardware/environment fields. |
| `started_at` | all | string | RFC 3339 UTC start time. |
| `duration_s` | all | number | Measured-window duration in seconds, excluding warm-up. |
| `repetitions` | macro | array of objects | Per-repetition `throughput` (`value`, `unit`), `latency_ms` (`p50`, `p95`, `p99`, `p99_9`, `max`), and `errors`; M-BANK also requires `correctness_violations`. |
| `throughput` | macro | object | Median `{value, unit}` throughput across repetitions; unit is `ops_s`, except M-TPCC-D uses `new_orders_per_min`. |
| `latency_ms` | macro | object | Median `p50`, `p95`, `p99`, `p99_9`, and `max` latency in milliseconds across repetitions. |
| `errors_total` | macro | integer | Sum of per-repetition failed operations during the measured windows; must be zero for gating. |
| `correctness_violations_total` | M-BANK macro | integer | Sum of per-repetition correctness violations; must be zero for gating. |
| `cv` | macro | object | Sample coefficient of variation (standard deviation with n-1 denominator divided by mean) across repetitions for `throughput` values and `p99`; each value is a unitless fraction rounded to four decimal places. |
| `micro` | micro | object | Per-benchmark micro measurements, as defined below. |
| `artifact` | micro | string | Path to raw `benchstat` text. |

`micro.benchmarks` is an array of objects with a benchmark `name` (string), three top-level median fields (`time_ns_op`, `bytes_op`, and `allocs_op`, each a number), and a required `samples` object. The `samples` object contains one number per repetition in each of these arrays: `time_ns_op` (nanoseconds per operation), `bytes_op` (bytes per operation), and `allocs_op` (allocations per operation, a count). All three arrays have the same length, equal to `config.repetitions` and at least 10. Each top-level metric is the median of its matching `samples` array.

**Abridged illustrative example; required fields from Pinned configuration are omitted.**

```json
{
  "schema_version": "1",
  "run_id": "example-2026-01",
  "kind": "macro",
  "system": "B0",
  "binary_version": "v23.2.15",
  "commit": "0123456789abcdef",
  "workload_id": "M-KV95",
  "config": {
    "nodes": 3,
    "warmup_s": 300,
    "duration_s": 600,
    "repetitions": 5,
    "seed": 42
  },
  "hardware_class": {
    "id": "general-purpose-3node-v1",
    "cpu_model": "recorded CPU model",
    "cpu_count": 8,
    "memory_gib": 32,
    "storage_type": "recorded storage type",
    "network": "recorded network class"
  },
  "started_at": "2026-01-01T00:00:00Z",
  "duration_s": 600,
  "repetitions": [
    {"throughput": {"value": 1000, "unit": "ops_s"}, "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.3, "p99_9": 12.0, "max": 22.0}, "errors": 0},
    {"throughput": {"value": 1010, "unit": "ops_s"}, "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.4, "p99_9": 12.1, "max": 22.0}, "errors": 0},
    {"throughput": {"value": 995, "unit": "ops_s"}, "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.2, "p99_9": 11.9, "max": 22.0}, "errors": 0},
    {"throughput": {"value": 1005, "unit": "ops_s"}, "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.3, "p99_9": 12.0, "max": 22.0}, "errors": 0},
    {"throughput": {"value": 990, "unit": "ops_s"}, "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.2, "p99_9": 11.8, "max": 22.0}, "errors": 0}
  ],
  "throughput": {"value": 1000, "unit": "ops_s"},
  "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.3, "p99_9": 12.0, "max": 22.0},
  "errors_total": 0,
  "cv": {"throughput": 0.0079, "p99": 0.0101}
}
```

## Regression gates

**PERF-001 micro gate.** For each covered microbenchmark name, compare `main` with `candidate` using `benchstat` on the unpaired per-repetition `samples.time_ns_op` and `samples.allocs_op` arrays from their micro run records. Treat the `main` and `candidate` samples as independent; do not pair observations by array position. The stored top-level `time_ns_op` and `allocs_op` values are median checks and do not replace the sample arrays as benchstat inputs; `time/op` corresponds to nanoseconds per operation and `allocs/op` to allocations per operation. The `artifact` field holds raw benchstat text as provenance only. Fail on a statistically significant regression when either gated metric is more than 3% worse. The significance test is benchstat's test.

For each macro metric, define `delta = (median(candidate repetitions) - median(B0 repetitions)) / median(B0 repetitions)`. Calculate the 95% confidence interval as follows:

1. Use a percentile bootstrap with 10,000 draws. For each draw, independently sample with replacement `n` observations from the B0 repetitions and `m` observations from the candidate repetitions, where `n` and `m` are the respective repetition counts.
2. For each draw, compute the same `delta` from the two resampled medians, using throughput `value` or the applicable `latency_ms.p99` or `latency_ms.p99_9` metric.
3. Sort the 10,000 delta values. The interval endpoints are their 2.5th and 97.5th percentiles, calculated with the Hyndman–Fan type 7 quantile: for sorted values `x[0]` through `x[B-1]`, use `h = (B-1) * p` and `Q(p) = x[floor(h)] + (h - floor(h)) * (x[ceil(h)] - x[floor(h)])` for `p = 0.025` and `p = 0.975`.
4. Use Go's `math/rand` bootstrap generator, initialized as `rand.New(rand.NewSource(seed))`, where `seed` is the B0 baseline run's recorded `config.seed`. Record `config.seed` in the B0 baseline JSON. For each draw, generate the B0 resample first, then the candidate resample, choosing each observation index with `Intn` on that side's repetition count.

The gated candidate value is the stored top-level median. Fail iff the 95% confidence interval lies entirely on the worse side of zero and the point delta exceeds the applicable practical threshold. Compare throughput in the same unit recorded for both systems (`ops_s`, or `new_orders_per_min` for M-TPCC-D).

| Suite | Failure threshold |
| --- | --- |
| Macro throughput, nightly | More than 3% worse than B0. |
| Macro throughput, release | More than 2% worse than B0. |
| Macro p99 or p99.9 latency | More than 10% worse than B0. |

The first PAR-004 macro baseline gate applies these thresholds to M-KV0, M-KV95, M-YCSB-A..F, M-TPCC-D, and M-BANK. Stability must be established before a hardware class gates changes: coefficient of variation below 2% for throughput and below 5% for p99 latency. If a run exceeds either limit, investigate the environment and do not treat it as a valid gating comparison.

## Publishing

Publish only reproducible runs on fully describable hardware, with the versions, settings, workload details, and hardware description needed to reproduce them. Other runs may be retained for regression tracking and are not published.

## Acceptance checklist for the harness PR

- [ ] Add runner scripts and pinned configuration files under `fgdb/bench/`.
- [ ] Implement and document result schema v1; validate emitted JSON.
- [ ] Capture the B0 baseline for each required workload on a stable, fully described hardware class.
- [ ] Implement PERF-001 with `benchstat` against `main` and the specified significance and 3% thresholds.
- [ ] Implement macro comparisons against B0 using the decision rule in Regression gates, with specified confidence intervals, thresholds, and stability checks.
- [ ] Preserve run metadata and make results reproducible from recorded configuration.
- [ ] Document how to run the harness locally and interpret a failed gate.

## Open questions

- Which stable hardware class will be selected for the initial B0 baseline?
- Which JSON validation and schema compatibility policy will the runner enforce?
- Which workload tools and versions will be pinned for each suite?
