package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
	"github.com/muhammetsafak/go-sync-bench/internal/scenario"
	"github.com/muhammetsafak/go-sync-bench/internal/work"
)

// perGoroutineLimit is the largest G whose per-goroutine op counts are
// written in full; above it only the summary is kept.
const perGoroutineLimit = 1024

// exitG2 is the exit code of a repetition that failed the cpuset gate: the
// configuration is wrong and the round must stop.
const exitG2 = 3

// prewarm is the busy time spent before the warm-up calibration.
const prewarm = 250 * time.Millisecond

// Record is one repetition: the raw JSON of the design's schema.
type Record struct {
	CellID     string  `json:"cell_id"`
	Scenario   string  `json:"scenario"`
	Impl       string  `json:"impl"`
	G          int     `json:"G"`
	P          int     `json:"P"`
	WNominalNs int64   `json:"W_nominal_ns"`
	WRealized  float64 `json:"W_realized_ns"`
	WIters     int     `json:"W_iters"`
	ReadRatio  *int    `json:"read_ratio,omitempty"`
	Round      int     `json:"round"`
	RepSeed    uint64  `json:"rep_seed"`
	OrderSeed  uint64  `json:"order_seed"`
	RerunOf    string  `json:"rerun_of,omitempty"`
	Set        string  `json:"set,omitempty"`

	Env         Env         `json:"env"`
	Calib       Calib       `json:"calib"`
	Windows     Windows     `json:"windows"`
	Throughput  Throughput  `json:"throughput"`
	Latency     Latency     `json:"latency"`
	Fairness    Fairness    `json:"fairness"`
	Correctness Correctness `json:"correctness"`
	Gates       HardGates   `json:"gates"`
	WallS       float64     `json:"wall_s"`
}

type Env struct {
	GoVersion      string  `json:"go_version"`
	GOMAXPROCS     int     `json:"gomaxprocs"`
	RtGOMAXPROCS   uint64  `json:"rt_gomaxprocs"`
	NumCPU         int     `json:"num_cpu"`
	Cpuset         string  `json:"cpuset"`
	ClockResNs     int64   `json:"clock_resolution_ns"`
	ClockReadNs    float64 `json:"clock_read_ns"`
	SelftestResult string  `json:"selftest_result"`
	RuntimeErr     string  `json:"runtime_metrics_error,omitempty"`
}

type Calib struct {
	// WarmupNsPerIter sizes the busy work of the warm-up only; the measured
	// windows use StartNsPerIter, taken after the warm-up.
	WarmupNsPerIter float64 `json:"warmup_ns_per_iter"`
	StartNsPerIter  float64 `json:"start_ns_per_iter"`
	EndNsPerIter    float64 `json:"end_ns_per_iter"`
	Drift           float64 `json:"drift"`
}

type Windows struct {
	WarmupS     float64 `json:"warmup_s"`
	ThroughputS float64 `json:"throughput_s"`
	LatencyS    float64 `json:"latency_s"`
}

type Throughput struct {
	WindowS         float64             `json:"window_s"`
	Ops             uint64              `json:"ops"`
	OpsPerSec       float64             `json:"ops_per_sec"`
	NsPerOp         *float64            `json:"ns_per_op"`
	PerGoroutineOps []uint64            `json:"per_goroutine_ops,omitempty"`
	Summary         measure.OpsSummary  `json:"summary"`
	Producers       *measure.OpsSummary `json:"producers,omitempty"`
	ProducerOps     []uint64            `json:"producer_ops,omitempty"`
	FairnessJain    *float64            `json:"fairness_jain"`
	FairnessMinMax  *float64            `json:"fairness_min_max"`
	ProducerJain    *float64            `json:"producer_jain,omitempty"`
	ZeroOpG         int                 `json:"zero_op_goroutines"`
	Sends           *uint64             `json:"sends,omitempty"`
	Applied         *uint64             `json:"applied,omitempty"`
	DrainOps        uint64              `json:"drain_ops"`
	CPUUserS        *float64            `json:"cpu_user_s"`
	CPUSysS         *float64            `json:"cpu_sys_s"`
	CPUPerOpNs      *float64            `json:"cpu_per_op_ns"`
	RtMutexWaitS    *float64            `json:"rt_mutex_wait_s"`
	RtSchedLat      *[2]float64         `json:"rt_sched_latency_p50_p99"`
	RtSchedSamples  uint64              `json:"rt_sched_latency_samples"`
	GCCycles        *uint64             `json:"gc_cycles"`
	WindowTailFrac  *float64            `json:"window_tail_frac"`
	DrainFrac       float64             `json:"drain_frac"`
	DrainS          float64             `json:"drain_s"`
}

// HistOut is a histogram with its quantiles; values in ns.
type HistOut struct {
	N       uint64     `json:"n"`
	P50     *int64     `json:"p50"`
	P99     *int64     `json:"p99"`
	P999    *int64     `json:"p999"`
	Max     *int64     `json:"max"`
	Buckets [][2]int64 `json:"buckets"`
}

type Latency struct {
	WindowS            float64            `json:"window_s"`
	Ops                uint64             `json:"ops"`
	SampleRate         string             `json:"sample_rate"`
	SampleCount        uint64             `json:"sample_count"`
	PostWindowSamples  uint64             `json:"post_window_samples"`
	OpLatency          HistOut            `json:"op_latency_hist"`
	AcquireWait        HistOut            `json:"acquire_wait_hist"`
	PutWait            *HistOut           `json:"put_wait_hist,omitempty"`
	LockWait           *HistOut           `json:"lock_wait_hist,omitempty"`
	LockWaitsOver1ms   *uint64            `json:"lock_waits_over_1ms,omitempty"`
	WaitsOver1msSample uint64             `json:"waits_over_1ms_sampled"`
	WaitsOver1msEst    uint64             `json:"waits_over_1ms_est"`
	SlowShareByUs      map[string]float64 `json:"slow_share_by_threshold_us"`
	StarvingFraction   *float64           `json:"starving_fraction"`
	StarvingSamples    *uint64            `json:"starving_samples"`
	StarvingNullReason string             `json:"starving_null_reason,omitempty"`
	OpsRatio           *float64           `json:"ops_ratio"`
}

type Fairness struct {
	QuantumBoundRaw  *float64 `json:"quantum_bound_raw"`
	QuantumBound     *float64 `json:"quantum_bound"`
	ExpectedJain     *float64 `json:"expected_quantum_jain"`
	SlowAcquireShare *float64 `json:"slow_acquire_share"`
	Source           string   `json:"fairness_source"`
}

type Correctness struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// HardGates records G1–G3; a repetition with any of them false is invalid.
type HardGates struct {
	G1Correct bool   `json:"g1_correct"`
	G2Cpuset  bool   `json:"g2_cpuset"`
	G3Barrier bool   `json:"g3_barrier"`
	Valid     bool   `json:"valid"`
	StuckStep string `json:"stuck_step,omitempty"`
	Error     string `json:"error,omitempty"`
}

func f64(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}
func u64(v uint64) *uint64 { return &v }
func i64(v int64) *int64   { return &v }

func histOut(h *measure.Histogram) HistOut {
	o := HistOut{N: h.N, Buckets: h.Sparse()}
	if h.N == 0 {
		return o
	}
	o.P50 = i64(h.Quantile(0.50))
	o.P99 = i64(h.Quantile(0.99))
	if h.N >= 10_000 { // G7
		o.P999 = i64(h.Quantile(0.999))
	}
	o.Max = i64(h.Max)
	return o
}

type runFlags struct {
	scenario, impl, cellID, cpuset, out, rerunOf, selftest, set string
	g, p, read, round                                           int
	w                                                           int64
	repSeed, orderSeed                                          uint64
	warmup, window, latWindow                                   time.Duration
	starving                                                    bool
}

func cmdRun(args []string) int {
	var f runFlags
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.StringVar(&f.scenario, "scenario", "", "s1, s2 or s3")
	fs.StringVar(&f.impl, "impl", "", "implementation")
	fs.IntVar(&f.g, "G", 1, "worker goroutines")
	fs.IntVar(&f.p, "P", 0, "expected GOMAXPROCS and NumCPU (G2); 0 skips the check")
	fs.Int64Var(&f.w, "W", 0, "nominal fake work in ns")
	fs.IntVar(&f.read, "read", 0, "S2 read percentage")
	fs.IntVar(&f.round, "round", 0, "round number")
	fs.Uint64Var(&f.repSeed, "rep-seed", 1, "PCG seed of the repetition")
	fs.Uint64Var(&f.orderSeed, "order-seed", 0, "seed of the round order (recorded only)")
	fs.StringVar(&f.cellID, "cell-id", "", "cell id (default: derived)")
	fs.StringVar(&f.cpuset, "cpuset", "", "cpuset string (recorded only)")
	fs.StringVar(&f.out, "out", "", "output file (default stdout)")
	fs.StringVar(&f.rerunOf, "rerun-of", "", "id of the repetition this one replaces")
	fs.StringVar(&f.selftest, "selftest-result", "", "result of the container's starving-bit self-test")
	fs.StringVar(&f.set, "set", "", "matrix set name (recorded only)")
	fs.DurationVar(&f.warmup, "warmup", 500*time.Millisecond, "warm-up window")
	fs.DurationVar(&f.window, "window", 1500*time.Millisecond, "throughput window")
	fs.DurationVar(&f.latWindow, "latency-window", 2*time.Second, "latency window")
	fs.BoolVar(&f.starving, "starving", true, "run the starving-bit sampler in mutex cells")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rec, code := runRep(f)
	if err := writeJSON(f.out, rec); err != nil {
		fmt.Fprintln(os.Stderr, "syncbench run:", err)
		return 1
	}
	return code
}

func runRep(f runFlags) (*Record, int) {
	t0 := time.Now()
	cell := Cell{Scenario: f.scenario, Impl: f.impl, G: f.g, P: f.p, W: f.w, Read: f.read}
	rec := &Record{
		CellID: f.cellID, Scenario: f.scenario, Impl: f.impl, G: f.g, P: f.p,
		WNominalNs: f.w, Round: f.round, RepSeed: f.repSeed, OrderSeed: f.orderSeed,
		RerunOf: f.rerunOf, Set: f.set,
		Windows: Windows{f.warmup.Seconds(), f.window.Seconds(), f.latWindow.Seconds()},
	}
	if rec.CellID == "" {
		rec.CellID = cell.ID()
	}
	if f.scenario == scenario.S2 {
		rec.ReadRatio = &f.read
	}
	finish := func(code int) (*Record, int) {
		rec.WallS = time.Since(t0).Seconds()
		return rec, code
	}

	// G2: the container must expose exactly P CPUs and the runtime must run P Ps.
	rt0 := measure.ReadRuntime()
	rec.Env = Env{
		GoVersion: runtime.Version(), GOMAXPROCS: runtime.GOMAXPROCS(0),
		RtGOMAXPROCS: rt0.GOMAXPROCS, NumCPU: runtime.NumCPU(), Cpuset: f.cpuset,
		SelftestResult: f.selftest, RuntimeErr: rt0.Err,
	}
	rec.Gates.G2Cpuset = f.p == 0 ||
		(rec.Env.NumCPU == f.p && rec.Env.RtGOMAXPROCS == uint64(f.p) && rec.Env.GOMAXPROCS == f.p)
	if !rec.Gates.G2Cpuset {
		rec.Gates.Error = fmt.Sprintf("G2: NumCPU=%d gomaxprocs=%d rt_gomaxprocs=%d, want P=%d",
			rec.Env.NumCPU, rec.Env.GOMAXPROCS, rec.Env.RtGOMAXPROCS, f.p)
		return finish(exitG2)
	}

	clk := measure.MeasureClock()
	rec.Env.ClockResNs, rec.Env.ClockReadNs = clk.ResolutionNs, clk.ReadNs
	// Bring the core out of its idle frequency state; the warm-up window then
	// runs the real workload, and only after it is the start calibration taken.
	// Calibrating before the warm-up read ~7-11 % slow on the Docker VM and G5
	// flagged a drift that was only the ramp-up.
	for t := time.Now(); time.Since(t) < prewarm; {
		work.Spin(10_000)
	}
	cw := work.CalibrateParallel(runtime.GOMAXPROCS(0), 0, 0)
	rec.Calib.WarmupNsPerIter = cw.NsPerIter

	spec := scenario.Spec{Scenario: f.scenario, Impl: f.impl, G: f.g,
		WIters: work.ItersFor(f.w, cw.NsPerIter), ReadPct: f.read, Seed: f.repSeed}
	if err := spec.Validate(); err != nil {
		rec.Gates.Error = err.Error()
		return finish(2)
	}

	// Warm-up: same workload, nothing recorded.
	wu, _ := scenario.Run(spec, scenario.Options{Window: f.warmup})
	ok := checkPhase(rec, "warmup", wu)

	// Start calibration: the busy-work size of the measured windows.
	c0 := work.CalibrateParallel(runtime.GOMAXPROCS(0), 0, 0)
	rec.Calib.StartNsPerIter = c0.NsPerIter
	spec.WIters = work.ItersFor(f.w, c0.NsPerIter)
	rec.WIters = spec.WIters
	rec.WRealized = float64(spec.WIters) * c0.NsPerIter

	// Throughput window: no clock reads in the workers.
	var before, after measure.RuntimeSnapshot
	var u0, s0, u1, s1 float64
	var cpuErr error
	tp, _ := scenario.Run(spec, scenario.Options{
		Window: f.window,
		OnWindowStart: func() {
			before = measure.ReadRuntime()
			u0, s0, cpuErr = measure.CPUTimes()
		},
		OnWindowEnd: func() {
			var err error
			u1, s1, err = measure.CPUTimes()
			if cpuErr == nil {
				cpuErr = err
			}
			after = measure.ReadRuntime()
		},
	})
	ok = checkPhase(rec, "throughput", tp) && ok
	fillThroughput(rec, spec, tp, before, after, u1-u0, s1-s0, cpuErr)

	// Latency window: every 16th op timed; starving-bit sampler on mutex cells.
	sampler := spec.UsesMutex() && f.starving
	lat, _ := scenario.Run(spec, scenario.Options{Window: f.latWindow, Latency: true, StarvingSampler: sampler})
	ok = checkPhase(rec, "latency", lat) && ok
	fillLatency(rec, spec, tp, lat, sampler, f)

	c1 := work.CalibrateParallel(runtime.GOMAXPROCS(0), 0, 0)
	rec.Calib.EndNsPerIter = c1.NsPerIter
	rec.Calib.Drift = work.Drift(c0.NsPerIter, c1.NsPerIter)

	fillFairness(rec, spec, f)
	rec.Gates.Valid = ok && rec.Gates.G2Cpuset
	if rec.Gates.StuckStep != "" {
		// Goroutines are still blocked; the record is written and the process
		// exits, which is the only way to reclaim them.
		return finish(4)
	}
	return finish(0)
}

// checkPhase folds one phase into G1/G3. The throughput phase detail is kept.
func checkPhase(rec *Record, name string, r *scenario.Result) bool {
	if r == nil {
		rec.Gates.Error = name + ": no result"
		return false
	}
	if name == "warmup" {
		rec.Gates.G1Correct, rec.Gates.G3Barrier = true, true
	}
	if r.StuckStep != "" && rec.Gates.StuckStep == "" {
		rec.Gates.StuckStep = name + ":" + r.StuckStep
	}
	rec.Gates.G1Correct = rec.Gates.G1Correct && r.Correct
	rec.Gates.G3Barrier = rec.Gates.G3Barrier && r.BarrierOK
	d := name + ": " + r.Detail
	if !r.BarrierOK {
		d += fmt.Sprintf(" barrier_ready=%d", r.Ready)
	}
	if rec.Correctness.Detail != "" {
		rec.Correctness.Detail += " | "
	}
	rec.Correctness.Detail += d
	rec.Correctness.OK = rec.Gates.G1Correct
	return r.Correct && r.BarrierOK && r.StuckStep == ""
}

func fillThroughput(rec *Record, spec scenario.Spec, r *scenario.Result,
	before, after measure.RuntimeSnapshot, du, ds float64, cpuErr error) {
	t := &rec.Throughput
	t.WindowS = float64(r.WindowNs) / 1e9
	t.Ops = r.Ops
	if t.WindowS > 0 {
		t.OpsPerSec = float64(r.Ops) / t.WindowS
	}
	if r.Ops > 0 {
		t.NsPerOp = f64(float64(r.WindowNs) / float64(r.Ops))
	}
	t.Summary = measure.Summarize(r.PerG)
	if len(r.PerG) <= perGoroutineLimit {
		t.PerGoroutineOps = r.PerG
	}
	t.ZeroOpG = t.Summary.Zero
	if len(r.PerG) > 1 {
		t.FairnessJain = f64(t.Summary.Jain)
		t.FairnessMinMax = f64(t.Summary.MinMax)
	}
	if spec.Scenario == scenario.S3 {
		ps := measure.Summarize(r.PerGProd)
		t.Producers = &ps
		if len(r.PerGProd) <= perGoroutineLimit {
			t.ProducerOps = r.PerGProd
		}
		if len(r.PerGProd) > 1 {
			t.ProducerJain = f64(ps.Jain)
		}
	}
	if spec.Scenario == scenario.S1 && spec.Impl == "chan_oneway" {
		t.Sends, t.Applied = u64(r.Sends), u64(r.Applied)
	}
	if spec.Scenario != scenario.S3 && (spec.Impl == "chan_reqresp" || spec.Impl == "chan_owner") {
		t.Applied = u64(r.Applied)
	}
	t.DrainOps = r.DrainOps
	t.DrainS = float64(r.DrainNs) / 1e9
	if r.WindowNs > 0 {
		t.DrainFrac = float64(r.DrainNs) / float64(r.WindowNs)
	}
	if cpuErr == nil {
		t.CPUUserS, t.CPUSysS = f64(du), f64(ds)
		if r.Ops > 0 {
			t.CPUPerOpNs = f64((du + ds) * 1e9 / float64(r.Ops))
		}
	}
	if before.Err == "" && after.Err == "" {
		t.RtMutexWaitS = f64(after.MutexWaitS - before.MutexWaitS)
		t.GCCycles = u64(after.GCCycles - before.GCCycles)
	} else if rec.Env.RuntimeErr == "" {
		rec.Env.RuntimeErr = before.Err + after.Err
	}
	if p50, p99, n, ok := measure.SchedLatencyDelta(before.SchedLat, after.SchedLat); ok {
		t.RtSchedLat = &[2]float64{p50, p99}
		t.RtSchedSamples = n
	}
}

func fillLatency(rec *Record, spec scenario.Spec, tp, r *scenario.Result, sampler bool, f runFlags) {
	l := &rec.Latency
	l.WindowS = float64(r.WindowNs) / 1e9
	l.Ops = r.Ops
	l.SampleRate = fmt.Sprintf("1/%d", scenario.SampleEvery)
	l.SampleCount = r.Samples
	l.PostWindowSamples = r.PostWindowSamples
	l.OpLatency = histOut(&r.OpLat)
	l.AcquireWait = histOut(&r.Acquire)
	if spec.Scenario == scenario.S3 {
		pw := histOut(&r.PutWait)
		l.PutWait = &pw
		if spec.Impl == "cond64" || spec.Impl == "cond1024" {
			lw := histOut(&r.LockWait)
			l.LockWait = &lw
			l.LockWaitsOver1ms = u64(r.LockWaitOver1ms)
		}
	}
	l.WaitsOver1msSample = r.AcquireOver[len(r.AcquireOver)-1]
	l.WaitsOver1msEst = l.WaitsOver1msSample * scenario.SampleEvery
	l.SlowShareByUs = map[string]float64{}
	if r.Samples > 0 {
		for i, th := range scenario.ThresholdsNs {
			l.SlowShareByUs[fmt.Sprint(th/1000)] = float64(r.AcquireOver[i]) / float64(r.Samples)
		}
		rec.Throughput.WindowTailFrac = f64(float64(r.PostWindowSamples) / float64(r.Samples))
	}
	switch {
	case !spec.UsesMutex():
	case !sampler:
		l.StarvingNullReason = "sampler disabled (self-test " + f.selftest + ")"
	case r.StarvingSamples == 0:
		l.StarvingNullReason = "no sampler ticks"
		l.StarvingSamples = u64(0)
	default:
		l.StarvingSamples = u64(r.StarvingSamples)
		l.StarvingFraction = f64(float64(r.StarvingSet) / float64(r.StarvingSamples))
	}
	if tp.Ops > 0 && tp.WindowNs > 0 && r.WindowNs > 0 {
		tpRate := float64(tp.Ops) / float64(tp.WindowNs)
		latRate := float64(r.Ops) / float64(r.WindowNs)
		l.OpsRatio = f64(latRate / tpRate)
	}
}

func fillFairness(rec *Record, spec scenario.Spec, f runFlags) {
	fr := &rec.Fairness
	p := f.p
	if p == 0 {
		p = runtime.GOMAXPROCS(0)
	}
	// In S3 the fairness index is over the consumers (G/2 of them).
	g := spec.G
	if spec.Scenario == scenario.S3 {
		g = spec.G / 2
	}
	raw := measure.QuantumBoundRaw(p, g, f.window)
	fr.QuantumBoundRaw = f64(raw)
	fr.QuantumBound = f64(math.Min(1, raw))
	fr.ExpectedJain = f64(math.Min(1, measure.ExpectedQuantumJain(raw)))
	if rec.Latency.SampleCount > 0 {
		fr.SlowAcquireShare = f64(rec.Latency.SlowShareByUs["10"])
	}
	s := 0.0
	if fr.SlowAcquireShare != nil {
		s = *fr.SlowAcquireShare
	}
	fr.Source = measure.FairnessSource(g, p, math.Min(1, raw), s)
}

// writeJSON writes v to path atomically (temp file + rename), or to stdout.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
