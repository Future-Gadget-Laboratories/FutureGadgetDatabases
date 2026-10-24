> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# PAR-004 benchmark harness and baseline

**Status: Proposed, v1**

## Scope and non-goals

This specification defines the benchmark tooling, pinned run configuration, result format, and regression gates for PAR-004. It does not change database or core code. A later PR will add the harness under `fgdb/bench/`; another later PR will wire it into CI.

This version covers the required macro workloads M-KV0, M-KV95, M-YCSB-A..F, M-TPCC-D, and M-BANK, plus the microbenchmark suites U-STORAGE, U-OPT, and U-SQL. It defines how to record the v23.2.15 baseline; it does not claim that baseline has already been captured.

## Systems

**B0** is stock v23.2.15 `cockroach-oss`, built from its pinned tag with the recorded toolchain. **S0** is the FGDb build under test. Both are Apache-2.0 systems. The fixed baseline comparison is S0 against B0, on the same hardware and with the same workload settings. Other system comparisons are only referenced in the benchmark plan and are outside this harness specification.

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

Each run is one JSON object. `schema_version` is the string `"1"`. Timestamps use RFC 3339 UTC. Durations use seconds; throughput uses operations per second; latency values use milliseconds; `cv` is a unitless fraction (for example, `0.02` means 2%). The latency fields summarize the measured-window client latency distribution. Errors count failed operations during that window.

| Field | Type | Meaning |
| --- | --- | --- |
| `schema_version` | string | Result format version, `"1"`. |
| `run_id` | string | Unique identifier for this run. |
| `system` | string | `B0` or `S0`. |
| `binary_version` | string | Human-readable binary version. |
| `commit` | string | Source commit SHA used to build the binary. |
| `workload_id` | string | Workload ID and, for M-YCSB, the selected letter. |
| `config` | object | Pinned workload, cluster, tool, and run-protocol settings, including the seed. |
| `hardware_class` | object | Hardware class ID and descriptive hardware/environment fields. |
| `started_at` | string | RFC 3339 UTC start time. |
| `duration_s` | number | Measured-window duration in seconds, excluding warm-up. |
| `repetitions` | array of numbers | Per-repetition throughput values in operations per second. |
| `throughput_ops_s` | number | Aggregate throughput in operations per second over the measured window. |
| `latency_ms` | object | `p50`, `p95`, `p99`, and `max` latency in milliseconds. |
| `errors` | integer | Failed operations during the measured window. |
| `cv` | object | Coefficient of variation across repetitions for `throughput` and `p99`; each value is a unitless fraction. |

Example:

```json
{
  "schema_version": "1",
  "run_id": "example-2026-01",
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
  "repetitions": [1000, 1010, 995, 1005, 990],
  "throughput_ops_s": 1000,
  "latency_ms": {"p50": 1.2, "p95": 4.1, "p99": 8.3, "max": 22.0},
  "errors": 0,
  "cv": {"throughput": 0.008, "p99": 0.03}
}
```

## Regression gates

**PERF-001 micro gate.** For each covered microbenchmark, compare the change against `main` using `benchstat`. Fail on a statistically significant regression when `time/op` or `allocs/op` is more than 3% worse. The significance test is benchstat's test.

Macro gates use the median result and a 95% bootstrap confidence interval. A regression is a failure only when statistically significant and beyond the applicable practical threshold:

| Suite | Failure threshold |
| --- | --- |
| Macro throughput, nightly | More than 3% worse than B0. |
| Macro throughput, release | More than 2% worse than B0. |
| Macro p99 or p99.9 latency | More than 10% worse than B0. |
| Recovery unavailability or time to normal | More than 10% worse than B0. |

The first PAR-004 macro baseline gate applies these thresholds to M-KV0, M-KV95, M-YCSB-A..F, M-TPCC-D, and M-BANK. Stability must be established before a hardware class gates changes: coefficient of variation below 2% for throughput and below 5% for p99 latency. If a run exceeds either limit, investigate the environment and do not treat it as a valid gating comparison.

## Publishing

Publish only reproducible runs on fully describable hardware, with the versions, settings, workload details, and hardware description needed to reproduce them. Other runs may be retained for regression tracking and are not published.

## Acceptance checklist for the harness PR

- [ ] Add runner scripts and pinned configuration files under `fgdb/bench/`.
- [ ] Implement and document result schema v1; validate emitted JSON.
- [ ] Capture the B0 baseline for each required workload on a stable, fully described hardware class.
- [ ] Implement PERF-001 with `benchstat` against `main` and the specified significance and 3% thresholds.
- [ ] Implement macro comparisons against B0 with the specified confidence intervals, thresholds, and stability checks.
- [ ] Preserve run metadata and make results reproducible from recorded configuration.
- [ ] Document how to run the harness locally and interpret a failed gate.

## Open questions

- Which stable hardware class will be selected for the initial B0 baseline?
- Which JSON validation and schema compatibility policy will the runner enforce?
- How will macro bootstrap confidence intervals and significance tests be implemented consistently across workloads?
- Which workload tools and versions will be pinned for each suite?
