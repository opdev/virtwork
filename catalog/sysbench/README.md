# sysbench — CPU Benchmarking Workload

Single-VM continuous CPU stress test using [sysbench](https://github.com/akopytov/sysbench).
Runs repeated benchmark iterations with configurable threads, CPU methods, and timing to
generate sustained CPU load and throughput metrics.

## Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `threads` | int | `4` | Number of worker threads (`--threads`) |
| `cpu-method` | string | `sum` | CPU stressor method: `sum`, `sqrt`, or `pi` |
| `test-duration` | int | `60` | Seconds per test iteration (`--time`) |
| `loop-delay` | int | `10` | Seconds to pause between iterations |

### Parameter details

**threads** — Each thread performs independent CPU operations. Set this relative to VM
CPU cores; values higher than the core count are valid and test contention behavior.

**cpu-method** — Controls which mathematical operation each thread performs:

- `sum` — integer addition (fastest, lowest per-op latency)
- `sqrt` — square root calculations (moderate load)
- `pi` — pi calculation via series (highest complexity per operation)

**test-duration** — Duration of each benchmark pass. Use `0` for infinite (runs until
stopped). Longer durations produce more stable throughput numbers.

**loop-delay** — Pause between iterations. Allows metrics scraping systems to observe
distinct test windows. Set to `0` for back-to-back runs.

## Usage

```bash
# Default: 4 threads, sum method, 60s tests, 10s delay
virtwork run --from-catalog sysbench --catalog-dir ./catalog --dry-run

# High-intensity: 8 threads, pi method, 5-minute tests
virtwork run --from-catalog sysbench --catalog-dir ./catalog \
  --params sysbench.threads=8,sysbench.cpu-method=pi,sysbench.test-duration=300

# Rapid iteration: 2 threads, 10s tests, 5s delay
virtwork run --from-catalog sysbench --catalog-dir ./catalog \
  --params sysbench.threads=2,sysbench.test-duration=10,sysbench.loop-delay=5

# Cluster-wide stress: 4 VMs
virtwork run --from-catalog sysbench --catalog-dir ./catalog \
  --vm-count 4 --params sysbench.threads=4
```

## What it measures

- **Throughput** (events/sec) — CPU operations completed per second
- **Latency** (avg/p95/p99 in ms) — per-event timing distribution
- **CPU utilization** — should approach 100% per thread under sustained load

## Monitoring

SSH into the VM and inspect the service:

```bash
systemctl status workload.service
journalctl -u workload.service -f
```

sysbench prints a summary after each iteration with events/sec, latency
percentiles, and total events processed.

## Troubleshooting

**Service fails immediately** — sysbench package not installed. Check
`/var/log/cloud-init-output.log` for dnf errors.

**Lower-than-expected throughput** — VM may be oversubscribed. Reduce `threads`
or deploy on dedicated nodes.

**CPU% below expected** — `test-duration` too short relative to `loop-delay`.
Increase duration or decrease delay.
