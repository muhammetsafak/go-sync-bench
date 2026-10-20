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
STAMP=<stamp> caffeinate -dimsu ./bench/run.sh --cells results/<stamp>/rerun-cells.txt
                                           # rerun the listed cells into results/<stamp>/rerun/
docker run --rm --network none --read-only --tmpfs /tmp --user 65534:65534 \
  --cap-drop ALL --security-opt no-new-privileges \
  -v "$PWD/bench:/bench:ro" -v "$PWD/results/<stamp>:/results" \
  -v "$PWD/results/<stamp>/raw:/results/raw:ro" \
  -v "$PWD/results/<stamp>/rerun:/results/rerun:ro" \
  node:24-trixie-slim node /bench/report.mjs /results
                                           # summary.json, summary.csv, rerun-cells.txt;
                                           # merged/ as well once rerun/ exists
                                           # report.mjs [--headline FILE] [--second-run PATH]
                                           # adds the confidence conditions (below)
docker run --rm --network none --read-only --user 65534:65534 \
  --cap-drop ALL --security-opt no-new-privileges \
  -v "$PWD/bench:/bench:ro" -v "$PWD/results:/results:ro" \
  node:24-trixie-slim node /bench/compare-runs.mjs \
  [--headline /results/<a>/headline.txt] /results/<a> /results/<b>
                                           # second run vs first, cell by cell, ±5 %
```

Linux bare-metal profile (`linux-metal`, see Protocol), on the target host:

```sh
sudo BENCH_HOST_PREP_CONFIRM=1 ./bench/host-prep.sh   # once: GRUB isolation, CPUAffinity, tune unit; then reboot
sudo BENCH_HOST_PREP_CONFIRM=1 ./bench/host-prep.sh --runtime   # re-apply the runtime tunables only
sudo BENCH_HOST_PREP_CONFIRM=1 ./bench/host-prep.sh --revert    # undo everything prepare installed; exit 1 if a step failed
./bench/host-check.sh [--out FILE] [--phase NAME] [--skip-load] # gate E2 for linux-metal; exit 0 pass, 1 fail, 2 not Linux
BENCH_PROFILE=linux-metal ./bench/run.sh --smoke
STAMP=$(date -u +%F) BENCH_PROFILE=linux-metal ./bench/run.sh --all
```

`host-prep.sh` changes the host (boot parameters, systemd, services) and
refuses to run without `BENCH_HOST_PREP_CONFIRM=1`, root and Linux; it is
never run on a development machine. Before writing anything it checks the
topology (every profile CPU exists, the isolated and system sets each sit on
their own L3 domain, no SMT sibling inside them). If `update-grub` fails, or
`grub.cfg` lacks the new parameters, it removes its drop-in and regenerates
GRUB before it stops. It records the state of the sleep targets before
masking them, and `--revert` unmasks only what it masked; `--revert` runs
every step even when one fails and exits 1 if any did. On linux-metal the host needs only Docker
and `jq`; `host-check.sh` reads `/proc` and `/sys` and changes nothing.

`caffeinate -dimsu` (macOS) keeps the host awake. `run.sh` also refuses to
start unless `pmset` shows `sleep 0` (gate E2). The `:ro` mounts keep the raw data read-only while
the report writes its summaries. Omit the `rerun` mount before a rerun exists.

Nothing is run on the host except Docker, `ps`, `pmset` and `jq`. Build,
vet, tests, self-test and every measurement run in containers with **no
network**, a read-only root filesystem, a private `/tmp`, no capabilities and
an unprivileged user; the only writable mount is `results/<stamp>/`. The image
build itself is offline (`docker build --network none`) once
`golang:1.27.1-trixie` and `debian:trixie-slim` are present.

## Protocol

**Budget** (profile `macos`, the default). The Docker VM has 12 vCPUs. vCPU 0-3 stay with the VM kernel and
dockerd; a P container is pinned to `P1=4`, `P2=4-5`, `P4=4-7`, `P8=4-11`
with `GOMAXPROCS=P`, 4 GiB of memory and no swap. Gate G2 checks inside the
container that the cpuset and `GOMAXPROCS` are what the run asked for; a
mismatch stops the run.

**Budget, linux-metal.** Bare metal, Ryzen 9 5950X with SMT off (16
cores, two CCDs). CCD0 (CPU 0-7) keeps the kernel, IRQs, systemd and
dockerd (`irqaffinity=0-7`, systemd `CPUAffinity=0-7`); CCD1 (CPU 8-15) is
reserved for the containers with `isolcpus=managed_irq,8-15`,
`nohz_full=8-15` and `rcu_nocbs=8-15`. `managed_irq` keeps managed
interrupts off CCD1 without removing it from the scheduler domains, so
`/sys/devices/system/cpu/isolated` stays empty and the container's cpuset
is honoured. A P container is pinned to `P1=8`, `P2=8-9`, `P4=8-11`,
`P8=8-15` (`profiles["linux-metal"]` in `matrix.json`), so every cell runs
on one CCD and shares one L3. The CPU governor is `performance` (with
amd-pstate EPP also `performance`, read back after writing), boost is off,
and every movable IRQ, the default IRQ affinity and the unbound workqueues
stay on CPU 0-7; `host-prep.sh` sets them and `bench-host-tune.service`
re-applies them at every boot, before docker. Sleep targets are masked and
unattended-upgrades, snapd, apt-daily, fwupd, motd-news and irqbalance are
stopped. THP, NUMA balancing and cpuidle are recorded, not changed.

**Order.** A round runs every cell once; a cell's repetitions are always in
different rounds, never back to back. Each round is shuffled at two levels
from the run seed: the order of the P groups, then the order of the cells
inside each group. Seed and both orders are in `RUN.md`.

**Isolation.** One container per round × P. Inside it every repetition is
its own process, so rusage, `runtime/metrics` and the heap start clean. A
repetition has a 0.5 s warm-up, a 1.5 s throughput window and a 2.0 s
latency window (≈ 4.4 s wall in total, budget 5 s).

**Host gates** (`macos`). E2 before the run and every round: system sleep disabled
(`pmset -g` shows `sleep 0`; otherwise `run.sh` exits 6 and names the fix,
`sudo pmset -c sleep 0`), AC power and no recorded thermal or CPU speed
warning (`pmset`). A `caffeinate` assertion does not count: it ends with its
process. The smoke run records the sleep setting but does not enforce it. E1 after every container: a fixed cool-down
(60 s, 5 s for smoke), then the summed CPU of host processes outside the
Docker VM must be below one core, waiting at most 10 minutes. The smoke run
records E1 but does not wait for it.

**Host gates, linux-metal.** E2 is `host-check.sh`: the profile's cpusets
on one L3 domain, SMT off, `isolcpus`/`nohz_full`, PID 1 affined to CPU
0-7, governor/EPP, boost off, no movable IRQ on CCD1, not a virtual machine
(`systemd-detect-virt`, the `hypervisor` CPU flag), sleep targets masked,
background services inactive, Docker not the snap, and the 1-minute load
average. An IRQ counts as kept off CCD1 by `managed_irq` only when its
requested affinity, limited to online CPUs, lies on CCD1 and the kernel does
not report it as unmanaged (`/sys/kernel/debug/irq`, readable as root; without
root the record says `managed: null`, and the check notes it). Kernel, microcode, THP, cgroup version and k10temp temperatures
are recorded. It runs before the run
(with the load check), before every round and at the end (without it);
each record is kept under `results/<stamp>/host-check/`. The start record
becomes `environment.json`, the end result is merged into it as `.end`. A
failed check stops the run with exit 7 (`HOST_CHECK_ENFORCE=0` only logs
it, for a dry run; such a run cannot reach `high`). E1 reads the host load
from `/proc/stat` instead of `ps`, the same rule. `pmset` is not used.

**Confidence.** `report.mjs` suggests `low` when more than 15 % of the
cells are flagged, when any cell has fewer than 5 valid repetitions
(ADR-0050), or when a run directory has no environment record
(`environment.json` or `RUN.md`). It suggests `high` only when every condition holds,
otherwise `medium`, and lists what is missing:

- at least 5 repetitions per cell (median, IQR and min–max are reported);
- at most 5 % of the cells flagged, and no headline cell flagged;
- `environment.json` and every host-check record passed (start, rounds, end);
- noise floor: every headline pair (`--headline FILE`, one comparison per
  line: `<cellA> <cellB> <ops|p99>`) differs in that metric by at least 3 ×
  the spread ((max−min)/median) of the same metric in the noise-floor cell
  (s1 atomic, W=0, 2 ≤ G ≤ P) at the same P. A line without a metric, or a
  p99 below the clock resolution, does not count. The noise-floor cell
  needs 2 ≤ G ≤ P, so the matrix has one at P=2 (G=2), P=4 and P=8 but none
  at P=1: a headline with a P=1 cell cannot reach `high`;
- second run (`--second-run PATH`): every headline number (or every cell
  without a headline file) within ±5 % (`compare-runs.mjs`; a p99 below
  the clock resolution in one run only is incomparable and fails), on another
  boot (`boot_id`) and another day, and that run's environment passed too.

Open data (the raw records published with the result) is not something the
tool can check; it stays a publishing step.

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
re-run candidates. If more than 15 % of the cells fail a soft gate, the report
suggests low confidence.

**Rerun trigger** (`bench/gates.mjs`, `rerun_basis: trimmed_range_v2`). The
G4 flags above keep their thresholds and still describe the cell; which cells
run again is decided separately. The one repetition farthest from the median
is dropped (with three or more values), and a cell is a candidate when the
remaining range/median exceeds 10 % for throughput or 50 % for op_latency
p99. The p99 test applies only when the median p99 is at or above G8's floor
and the trimmed p99 range is wider than one clock tick. G5 is unchanged: two
or more drifted repetitions. `summary.json` states the rule under `method`,
keeps the design's original trigger for comparison under `gates.rerun_v1`,
and `rerun-cells.txt` lists the candidates.

**Cell rerun.** `run.sh --cells FILE` (with `STAMP` of the first pass) runs
every listed main-set cell again from scratch, all rounds, with the same
protocol and a seed of its own, into `results/<stamp>/rerun/`. The first pass
is only read. Every rerun record's attempt 1 carries `rerun_of` pointing at
the first-pass repetition it replaces (`../../raw/<cell>.r<round>`), and
`rerun/rerun_of.json` lists the link; `report.mjs results/<stamp>/rerun`
summarises the second pass on its own.

**Merged report.** Once `results/<stamp>/rerun/` exists, `report.mjs
results/<stamp>` reads the first pass merged with the rerun: every cell listed
in `rerun/cells.txt` is replaced entirely by its rerun (raw and timing), the
rest of the first pass is taken as is, and `summary.json`, `summary.csv` and
`rerun-cells.txt` are written to `results/<stamp>/merged/`; the first pass and
the rerun are only read. `summary.merged` records the file counts; a listed
cell with no rerun record keeps its first pass and is named in
`summary.merged.missing_rerun_cells` (and on stderr). A rerun cell that met
the G4 part of the trigger in the first pass and still meets it on its rerun
gets the `spread_persistent` flag (definition under
`method.spread_persistent`, list under `gates.spread_persistent`); the flag
thresholds do not change. `--no-merge` summarises the first pass alone.

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
  rerun-cells.txt        rerun candidates, the input of run.sh --cells
  rerun/                 a cell rerun: the same layout, plus cells.txt and rerun_of.json
  merged/                report of the first pass merged with rerun/ (summary.*, rerun-cells.txt)
  environment.json       linux-metal: the start host-check record, .end merged in at the end
  host-check/<time>-<phase>.json  linux-metal: every E2 record (start, round-<n>, end)
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

## Results — 2026-09-29

The published run is `results/2026-09-29/`. `RUN.md` there has the full
environment record, round by round.

- **Scope.** The `main` set: 845 cells x 5 rounds (S1/S2/S3 = 365/288/192
  cells), seed `932269622`, P groups 1/2/4/8. It ran on one host: Apple M4 Pro,
  12 CPUs, Docker 29.8.0, go1.27.1, on AC power, with no thermal warnings. The
  first pass ran 2026-09-29 and the rerun 2026-09-30 01:38-02:31Z.
- **Images.** The first pass used `sha256:640a046f219a…`. The rerun used
  `sha256:eabc1573265a…`. Only orchestration changed between them
  (`cmd/syncbench`, `bench/run.sh`: `--cells`, `rerun_of`, the sleep check).
  `internal/`, where the measurement runs, and `docker/` did not change.
- **Published data:** `results/2026-09-29/merged/summary.json` and
  `summary.csv`. They combine the first pass with a one-time rerun of 88
  cells. A rerun cell takes all 5 repetitions from `rerun/raw` and replaces
  the whole cell; each cell's `source` says which pass it comes from. The
  first pass alone is in `results/2026-09-29/summary.json` and `raw/`. Nothing
  is deleted.
- **Rerun rule (user decision 2026-09-29, option E).** The design's trigger,
  full range with the G4 thresholds, chose **294 cells (G4 290, G5 11)** on
  the first pass. That is more than the 84-cell rerun budget. The numbers are
  in the first-pass `summary.json` `gates.rerun_v1`. The rerun was triggered
  instead by the trimmed range (drop the highest and lowest repetition) over
  the G4 threshold, plus G5, with the budget raised to 88. The flag
  thresholds did not change: `spread_throughput` above 0.10,
  `spread_p99` above 0.25. `gates.rerun_v1` in the merged summary (273) is the
  same trigger recomputed on the merged data; it is not the original result.
  `method` in the summary gives the rules; this paragraph gives the reason.
  The spread floor of this host is wider than the design assumed, and the
  user chose this rule over publishing all 294 as flagged or repeating the
  full run.
- **Confidence.** 518 of the 845 cells carry at least one flag, so
  `confidence_suggestion` is `low`. A flagged cell is published with its
  flag, not dropped. Rank flagged cells lower.
- **`spread_persistent`.** 36 cells met the G4 part of the trigger on the
  first pass and still met it on the rerun (P1 11, P2 18, P4 1, P8 6). Cells
  are rerun once only. Do not build a claim on these cells alone.
- `gates.rerun_count` / `gates.rerun_cells` (36) count the cells still over
  the trigger after the rerun, not the cells that were rerun. The number
  rerun is `merged.replaced_cells` (88).
- **Host sleep.** The host slept once during the first pass (11:45-11:59Z).
  `RUN.md` names the repetition that spans the sleep
  (`s1-chan_oneway-g64-p1-w0` r3) and the repetitions around it.

## Results — 2026-10-18 and 2026-10-19 (x86_64, bare metal)

Two independent runs of the same `main` set on a second host. Run A and run B are each other's second run; neither is a second
run of the 2026-09-29 record, whose section above is unchanged. Run A is
`results/2026-10-18/`, run B is `results/2026-10-19/`.
Each has a `merged/` report (first pass plus the one-time rerun) and a
`RUN.md` with the environment record.

- **Host.** AMD Ryzen 9 5950X, SMT off, 16 CPUs, bare metal Ubuntu 26.04.1,
  kernel 7.0.0-38-generic, Docker 29.9.1, go1.27.1. Profile `linux-metal`:
  the measurement runs on cpuset 8-15 (isolated), the system on 0-7. Run B
  ran with the C2 idle state disabled (`environment.json`,
  `cpuidle-state2-after.txt`).
- **Scope.** 845 cells x 5 rounds each, P groups 1/2/4/8, cool-down 60 s
  after every container. The seed differs from the 2026-09-29 run (`4096595856`
  for run A), so the cell order differs too.
- **Confidence.** Both runs give `low`: 433 of 845 cells (run A) and 440 of
  845 cells (run B) carry at least one flag. `spread_persistent`: A 44 cells,
  B 39. Compared with 2026-09-29 (518 flagged), `calib_drift` does not occur
  here.
- **Second run.** Run A against run B: 220 of the 845 cells have at least one
  number outside +-5% (`bench/compare-runs.mjs`). 38 of them are on
  throughput: 31 in the Mutex/RWMutex/Cond family (`s1-mutex`, `s2-mutex`,
  `s3-cond`, `s1-rwmutex`, `s2-rwmutex`) and 7 S3 channel cells; no S1
  atomic, `chan_oneway` or `chan_reqresp` cell is outside on throughput.
  201 are outside on p99,
  182 of them on p99 only; 138 of those 201 are in the 5-13% band, one
  histogram bucket step.
- **Bimodal cells.** `s1-mutex-g64-p8-w0` and `s1-mutex-g10000-p8-w0` land on
  one of two levels from repetition to repetition on this host. The
  `s1-mutex-g64-p8-w0` medians are 16.27 M ops/s (A) and 26.25 M ops/s (B);
  a lower level of about 16 M ops/s appears in every run. The cause is not
  established: isolation, C2 and starvation were ruled out in the diagnostic
  runs (`results/diag-20261018T114457Z/`). Do not read the difference between
  the two runs of these cells as noise or as an architecture effect.
- **Goroutine fairness at G=4.** The `atomic` cells at G=4 are less even here
  than on Apple silicon: `min_max` is about 0.51 at P=4 and 0.59-0.66 at P=8
  (0.96 on the Mac).
- **Timeouts.** Every repetition that hit the 64 s limit in either run was at
  P=2 and G=10000 (`s1-atomic-g10000-p2-w0`, `s3-chan1024-g10000-p2-w1000`).
  The retry passed each time. The cause is not examined.
- **Platform comparison.** A difference between this host and the Apple
  silicon record mixes the architecture (aarch64, x86_64) with Docker
  Desktop's VM against bare metal, the kernel, core isolation and the core
  count. The 2026-09-29 record is a single run, so its own run-to-run spread
  is unknown.

## Limits

- Two machines, each with its own confounds. Apple silicon (Docker Desktop,
  aarch64): the vCPUs are not pinned to performance or efficiency cores; the
  host scheduler decides, and the cluster clock follows the number of busy
  cores. Ryzen (bare metal, x86_64): SMT off and the measurement on isolated
  cores 8-15, but two Mutex headline cells are bimodal (see above). A
  difference between the two hosts is not an architecture effect unless both
  Ryzen runs agree and the cell is not flagged.
- The Apple silicon record is one run, so its run-to-run spread is unknown;
  the Ryzen runs give one.
- Each host ran one Go release (go1.27.1) and one kernel or VM image.
- The starving bit is read from an unexported field; a Go release that
  changes the mutex layout breaks the self-test, not the measurement.
- Latency is sampled 1 in 16; a timestamp costs `env.clock_read_ns`. G6
  shows how much the sampling itself slowed the cell. The p99 and p99.9 come
  from a histogram whose bucket step is 6.7-12.5%, so a p99 difference of one
  step is the resolution of the instrument.

## Licence

Code and data: MIT (see `LICENSE`).
