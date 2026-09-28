# go-sync-bench

Three ways to share state between goroutines, measured under the same load:
an atomic or a lock (`sync.Mutex`, `sync.RWMutex`), a channel, and — for
queues — a `sync.Cond`. The question is not only "which is faster" but
**what each one costs in throughput, tail latency and fairness as the number
of goroutines, cores and the work inside the critical section change.**

Go standard library only (`go.mod` has no `require`). Go 1.27.1.

| Scenario | Shared state | Implementations |
|---|---|---|
| `s1` counter | one `uint64` | `atomic`, `mutex`, `rwmutex` (write lock), `chan_reqresp` (owner goroutine, unbuffered request channel + reply), `chan_oneway` (owner goroutine, fire-and-forget increments) |
| `s2` map | `map[uint64]uint64`, read ratio 50/90/99 % | `mutex`, `rwmutex`, `chan_owner` |
| `s3` queue | bounded FIFO, G/2 producers and G/2 consumers | `chan0`, `chan1`, `chan64`, `chan1024` (buffered channel), `cond64`, `cond1024` (slice ring + `sync.Cond`, `Signal` per state change) |

Axes: G (goroutines) × P (`GOMAXPROCS`, also the cpuset size) × W (busy work
held inside the critical section, in nanoseconds). `bench/matrix.json` is the
single definition of the windows, the cpusets and both cell sets:

| Set | Cells | Rounds | Purpose |
|---|---|---|---|
| `main` | 845 | 5 | the published matrix |
| `smoke` | 24 | 3 | harness check, label cells, noise floor, G=1 |

## Commands

```sh
./bench/verify.sh                          # acceptance: vet, race tests, self-test x5, smoke run, report
./bench/run.sh --smoke                     # smoke set only
STAMP=$(date -u +%F) ./bench/run.sh --all  # main matrix; resumable with --resume
node bench/report.mjs results/<stamp>      # summary.json, summary.csv
```

Nothing is run on the host except Docker, `ps`, `pmset` and `jq`. Build,
vet, tests, self-test and every measurement run in containers with **no
network**, a read-only root filesystem, a private `/tmp`, no capabilities and
an unprivileged user; the only writable mount is `results/<stamp>/`. The image
build itself is offline (`docker build --network none`) once
`golang:1.27.1-trixie` and `debian:trixie-slim` are present.

## Protocol

**Budget.** The Docker VM has 12 vCPUs. vCPU 0-3 stay with the VM kernel and
dockerd; a P container is pinned to `P1=4`, `P2=4-5`, `P4=4-7`, `P8=4-11`
with `GOMAXPROCS=P`, 4 GiB of memory and no swap. Gate G2 checks inside the
container that the cpuset and `GOMAXPROCS` are what the run asked for; a
mismatch stops the run.

**Order.** A round runs every cell once; a cell's repetitions are always in
different rounds, never back to back. Each round is shuffled at two levels
from the run seed: the order of the P groups, then the order of the cells
inside each group. Seed and both orders are in `RUN.md`.

**Isolation.** One container per round × P. Inside it every repetition is
its own process, so rusage, `runtime/metrics` and the heap start clean. A
repetition has a 0.5 s warm-up, a 1.5 s throughput window and a 2.0 s
latency window (≈ 4.4 s wall in total, budget 5 s).

**Host gates.** E2 before every round: AC power and no recorded thermal or
CPU speed warning (`pmset`). E1 after every container: a fixed cool-down
(60 s, 5 s for smoke), then the summed CPU of host processes outside the
Docker VM must be below one core, waiting at most 10 minutes. The smoke run
records E1 but does not wait for it.

**Busy work W.** An xorshift loop in a `//go:noinline` function. Its speed
is calibrated inside the repetition, **with all P workers spinning in
parallel**: on Apple silicon the cluster clock depends on how many cores are
busy, so a single-threaded calibration measured a different CPU state than
the one the cells run in (≈ 10 % bimodal drift in early tests). The start
calibration runs after the warm-up; an end calibration after the latency
window gives the drift (gate G5).

**Starving bit.** `sync.Mutex` switches to starvation mode after a waiter
has waited 1 ms. The harness reads that bit from the mutex state word, which
is not a public API; a self-test at `GOMAXPROCS=2` in every container
confirms it can see the switch. If the self-test fails, `starving_fraction`
is `null` with a reason, and the rest of the record is kept.

### Gates

| Gate | Kind | Rule |
|---|---|---|
| G1 | hard | correctness check of the shared state after the window (counter total, map checksum, queue in = out) |
| G2 | hard | cpuset and `GOMAXPROCS` match; otherwise the run stops |
| G3 | hard | every goroutine passed the start barrier |
| G4 | soft | spread (max−min)/median ≤ 10 % for throughput, ≤ 25 % for p99 |
| G5 | soft | calibration drift ≤ 5 % |
| G6 | soft | latency window ops ≥ 0.9 × throughput window ops (observer effect) |
| G7 | soft | p99.9 only from ≥ 10 000 samples, otherwise `null` |
| G8 | soft | a percentile below 4 × clock resolution is printed as `< X ns` |
| G9 | soft | `window_tail_frac` < 1 % |

A repetition that fails G1 or G3 is kept as `<cell>.r<round>.invalid.json`
and replaced once, with a new seed and `rerun_of` pointing at it. If the
replacement fails too, that is a harness error: `round` exits 9 and `run.sh`
stops, noting it in `RUN.md`. `--resume` keeps an invalid record it finds as
the next free `<cell>.r<round>.invalid[-N].json` and measures the repetition
again, so no evidence is overwritten. Soft gates only flag: `report.mjs` lists the cells and the
re-run candidates (G4, or G5 in two or more repetitions). If more than 15 %
of the cells fail a soft gate, the report suggests low confidence.

## Output

```
results/<stamp>/
  RUN.md                 seed, image, host, P and cell orders, exit codes, E1/E2
  run.log                container logs
  matrix.json            the matrix the run used
  raw/<cell>.r<n>.json   one repetition, verbatim
  selftest/r<n>-P<p>.json
  order/r<n>-P<p>.txt
  timing/r<n>-P<p>.jsonl wall time, exit code and gate status per repetition
  summary.json, summary.csv
```

### Raw record

| Field | Meaning |
|---|---|
| `cell_id`, `scenario`, `impl`, `G`, `P`, `W_nominal_ns`, `W_realized_ns`, `W_iters`, `read_ratio` | the cell |
| `round`, `rep_seed`, `order_seed`, `set`, `rerun_of` | where it ran |
| `env` | Go version, `GOMAXPROCS`, cpuset, clock resolution and read cost, self-test result |
| `calib` | ns per spin iteration (warm-up, start, end) and `drift` |
| `throughput.ops_per_sec`, `ns_per_op` | the throughput window, no timestamps taken per op |
| `throughput.fairness_jain`, `fairness_min_max`, `zero_op_goroutines`, `summary`, `per_goroutine_ops` (G ≤ 1024) | ops per goroutine; for `s3` the consumers, producers separately |
| `throughput.cpu_user_s`, `cpu_sys_s`, `cpu_per_op_ns` | rusage of the window |
| `throughput.rt_mutex_wait_s`, `rt_sched_latency_p50_p99`, `gc_cycles` | `runtime/metrics` deltas |
| `throughput.window_tail_frac` | operations that **finished** after the window closed, over all operations counted — work that started inside the window and was still holding the lock or waiting on a channel when the stop signal came |
| `throughput.drain_frac` | time from the stop signal until every goroutine returned, over the window length |
| `latency.op_latency_hist`, `acquire_wait_hist`, `put_wait_hist`, `lock_wait_hist` | 1-in-16 sampled histograms (`n`, `p50`, `p99`, `p999`, `max`, log buckets) |
| `latency.slow_share_by_threshold_us` | share of sampled acquisitions slower than 1, 2, 5, 10, 20, 1000 µs |
| `latency.starving_fraction` | share of samples with the mutex in starvation mode |
| `latency.ops_ratio` | latency-window ops over throughput-window ops (G6) |
| `fairness.quantum_bound`, `expected_quantum_jain`, `slow_acquire_share`, `fairness_source` | see below |
| `correctness`, `gates`, `wall_s` | G1-G3 and the wall time |

### Where unfairness comes from

With more runnable goroutines than P, the scheduler alone makes the ops per
goroutine uneven: in a window of length T with a 10 ms time slice, at most
`b = P·⌊T/10 ms⌋ / g` of the goroutines can run at all (`g` = G, or G/2
consumers in `s3`). `fairness_source` is:

- `sync` if g ≤ P or b ≥ 1 (the time slice does not bind), or if at least
  half of the acquisitions waited longer than the park threshold (10 µs);
- `unexplained` otherwise — flagged in the report, never silently assigned,
  and left out of fairness comparisons between models.

There is no `scheduler` source. The smoke run did not confirm the 10 ms
quantum model well enough to attribute a cell to the scheduler, so the
harness does not. Every cell carries `quantum_bound` (b, and the unclamped
`quantum_bound_raw`), `expected_quantum_jain` and the measured `jain` side
by side, in `summary.json` and `cells.csv`; reading them is left to the
reader.

## Limits

- One machine: Apple silicon, Docker Desktop, aarch64. The vCPUs are not
  pinned to performance or efficiency cores; the host scheduler decides, and
  the cluster clock follows the number of busy cores.
- The starving bit is read from an unexported field; a Go release that
  changes the mutex layout breaks the self-test, not the measurement.
- Latency is sampled 1 in 16; a timestamp costs `env.clock_read_ns`. G6
  shows how much the sampling itself slowed the cell.

## Licence

Code and data: MIT (see `LICENSE`).
