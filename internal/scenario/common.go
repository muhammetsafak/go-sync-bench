// Package scenario implements the three workloads of the experiment
// (S1 shared counter, S2 shared map, S3 producer-consumer queue), every
// implementation of each, the closed-loop driver and the G1 correctness check.
package scenario

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
)

// SampleEvery is the latency sampling period: every 16th op is timed.
const SampleEvery = 16

// DefaultDrainTimeout bounds everything between the stop flag and the exit of
// the last goroutine.
const DefaultDrainTimeout = 10 * time.Second

// ThresholdsNs are the exact counters kept for sampled acquire_wait values:
// 1, 2, 5, 10, 20 µs (park-threshold calibration) and 1 ms (starvation proxy).
var ThresholdsNs = [...]int64{1_000, 2_000, 5_000, 10_000, 20_000, 1_000_000}

const (
	idx10us = 3
	idx1ms  = 5
)

// Scenario and implementation names.
const (
	S1 = "s1"
	S2 = "s2"
	S3 = "s3"
)

// Impls lists the implementations of each scenario.
var Impls = map[string][]string{
	S1: {"atomic", "mutex", "rwmutex", "chan_reqresp", "chan_oneway"},
	S2: {"mutex", "rwmutex", "chan_owner"},
	S3: {"chan0", "chan1", "chan64", "chan1024", "cond64", "cond1024"},
}

// Spec identifies the workload of a cell.
type Spec struct {
	Scenario string
	Impl     string
	G        int
	WIters   int // busy-loop iterations for W, already calibrated
	ReadPct  int // S2 only: 50, 90 or 99
	Seed     uint64
}

// Validate checks the spec against the matrix rules.
func (s Spec) Validate() error {
	impls, ok := Impls[s.Scenario]
	if !ok {
		return fmt.Errorf("unknown scenario %q", s.Scenario)
	}
	if !slices.Contains(impls, s.Impl) {
		return fmt.Errorf("unknown impl %q for %s", s.Impl, s.Scenario)
	}
	if s.G < 1 || s.G > 1<<24-2 {
		return fmt.Errorf("G = %d out of range", s.G)
	}
	if s.WIters < 0 {
		return fmt.Errorf("negative W iterations")
	}
	switch s.Scenario {
	case S1:
		if s.Impl == "atomic" && s.WIters != 0 {
			return fmt.Errorf("atomic runs only with W = 0")
		}
	case S2:
		if s.ReadPct < 0 || s.ReadPct > 100 {
			return fmt.Errorf("read ratio %d out of range", s.ReadPct)
		}
	case S3:
		if s.G < 4 || s.G%2 != 0 {
			return fmt.Errorf("S3 needs an even G >= 4, got %d", s.G)
		}
	}
	return nil
}

// UsesMutex reports whether the cell's shared state is a sync.Mutex, i.e.
// whether the starving-bit sampler applies (S1/S2 mutex only).
func (s Spec) UsesMutex() bool {
	return (s.Scenario == S1 || s.Scenario == S2) && s.Impl == "mutex"
}

// Options configure one phase (warm-up, throughput or latency window).
type Options struct {
	Window          time.Duration
	Latency         bool // time every SampleEvery-th op
	StarvingSampler bool // run the 1 kHz state sampler during the window
	DrainTimeout    time.Duration
	// OnWindowStart and OnWindowEnd run on the controller right after the
	// start barrier opens and right after the stop flag is raised.
	OnWindowStart func()
	OnWindowEnd   func()
}

// Result is the outcome of one phase.
type Result struct {
	WindowNs  int64 // start barrier open -> stop flag raised
	DrainNs   int64 // stop flag raised -> last goroutine exited
	BarrierOK bool  // G3: every worker was parked on the barrier
	Ready     int

	Ops      uint64   // ops started inside the window (the throughput count)
	PerG     []uint64 // per-goroutine ops (clients; consumers in S3)
	PerGProd []uint64 // S3: per-producer items put
	DrainOps uint64   // S3: items taken after the stop flag
	Applied  uint64   // owner models: increments/accesses applied by the owner
	Sends    uint64   // S1 one-way: completed sends

	OpLat, Acquire, LockWait, PutWait measure.Histogram
	Samples, PostWindowSamples        uint64
	AcquireOver                       [len(ThresholdsNs)]uint64
	LockWaitSamples, LockWaitOver1ms  uint64

	StarvingSamples, StarvingSet uint64

	Correct   bool
	Detail    string
	StuckStep string
}

// worker is the per-goroutine state. Every worker is allocated on its own and
// padded to a multiple of 128 bytes, so no two workers share a cache line.
type worker struct {
	workerCore
	_ [128 - unsafe.Sizeof(workerCore{})%128]byte
}

type workerCore struct {
	id       int
	ops      uint64
	drainOps uint64
	writes   uint64
	k        uint32
	rng      rand.PCG
	sink     uint64
	sum, xor uint64

	samples, post   uint64
	over            [len(ThresholdsNs)]uint64
	lockSamples     uint64
	lockOver1ms     uint64
	opLat, acq      *measure.Histogram
	lockWait, putWt *measure.Histogram
}

func newWorkers(n int, seed uint64, latency bool) []*worker {
	ws := make([]*worker, n)
	for i := range ws {
		w := new(worker)
		w.id = i
		w.rng = *rand.NewPCG(seed, uint64(i)+1)
		w.k = uint32(w.rng.Uint64() & (SampleEvery - 1))
		if latency {
			w.opLat = new(measure.Histogram)
			w.acq = new(measure.Histogram)
		}
		ws[i] = w
	}
	return ws
}

// tick advances the sampling counter and reports whether this op is timed.
func (w *worker) tick() bool {
	w.k++
	return w.k&(SampleEvery-1) == 0
}

// record stores one timed op: acquire_wait = t1-t0, op_latency = t2-t0.
func (w *worker) record(t0, t1, t2 int64, stop *atomic.Bool) {
	acq := t1 - t0
	w.acq.Record(acq)
	w.opLat.Record(t2 - t0)
	w.samples++
	for i, th := range ThresholdsNs {
		if acq > th {
			w.over[i]++
		}
	}
	if stop.Load() {
		w.post++
	}
}

// recordLock stores one S3 Cond lock_wait sample.
func (w *worker) recordLock(d int64) {
	w.lockWait.Record(d)
	w.lockSamples++
	if d > ThresholdsNs[idx1ms] {
		w.lockOver1ms++
	}
}

// phase is the shared control block of a phase: start barrier and stop flag.
type phase struct {
	stop    atomic.Bool
	_       [120]byte
	ready   atomic.Int64
	_       [120]byte
	start   chan struct{}
	latency bool
}

// barrier marks the worker ready and blocks until the window opens.
func (p *phase) barrier() {
	p.ready.Add(1)
	<-p.start
}

// group is a set of goroutines the controller waits for during the drain.
type group struct {
	wg   sync.WaitGroup
	done chan struct{}
}

func newGroup() *group { return &group{done: make(chan struct{})} }

func (g *group) goFn(f func()) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		f()
	}()
}

func (g *group) seal() {
	go func() {
		g.wg.Wait()
		close(g.done)
	}()
}

// window runs the common timeline: wait for the barrier, open the window,
// sleep, raise the stop flag. It returns the measured window length.
func window(p *phase, workers int, opt Options, res *Result, onStart, onEnd func()) {
	deadline := time.Now().Add(DefaultDrainTimeout)
	for p.ready.Load() < int64(workers) {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Microsecond)
	}
	res.Ready = int(p.ready.Load())
	res.BarrierOK = res.Ready == workers
	t0 := time.Now()
	close(p.start)
	if opt.OnWindowStart != nil {
		opt.OnWindowStart()
	}
	if onStart != nil {
		onStart()
	}
	time.Sleep(opt.Window)
	p.stop.Store(true)
	res.WindowNs = time.Since(t0).Nanoseconds()
	if onEnd != nil {
		onEnd()
	}
	if opt.OnWindowEnd != nil {
		opt.OnWindowEnd()
	}
}

// waitDrain waits for g within the remaining drain budget.
func waitDrain(g *group, deadline time.Time) bool {
	select {
	case <-g.done:
		return true
	case <-time.After(time.Until(deadline)):
		return false
	}
}

// collect merges worker-local counters and histograms into res.
func collect(ws []*worker, res *Result, perG *[]uint64) {
	xs := make([]uint64, len(ws))
	for i, w := range ws {
		xs[i] = w.ops
		res.Ops += w.ops
		res.DrainOps += w.drainOps
		res.Samples += w.samples
		res.PostWindowSamples += w.post
		for j := range w.over {
			res.AcquireOver[j] += w.over[j]
		}
		res.LockWaitSamples += w.lockSamples
		res.LockWaitOver1ms += w.lockOver1ms
		if w.acq != nil {
			res.Acquire.Merge(w.acq)
			res.OpLat.Merge(w.opLat)
		}
		if w.lockWait != nil {
			res.LockWait.Merge(w.lockWait)
		}
		if w.putWt != nil {
			res.PutWait.Merge(w.putWt)
		}
	}
	*perG = xs
}

// Run executes one phase of spec with opt.
func Run(spec Spec, opt Options) (*Result, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if opt.DrainTimeout <= 0 {
		opt.DrainTimeout = DefaultDrainTimeout
	}
	switch spec.Scenario {
	case S1:
		return runCounter(spec, opt), nil
	case S2:
		return runKV(spec, opt), nil
	default:
		return runQueue(spec, opt), nil
	}
}

// clientServer runs the S1/S2 shape: G client goroutines, optionally one owner
// goroutine that exits when closeOwner is called.
func clientServer(p *phase, opt Options, res *Result, ws []*worker,
	client func(w *worker, p *phase), owner func(), closeOwner func(),
	onStart, onEnd func()) {
	var og *group
	if owner != nil {
		og = newGroup()
		og.goFn(owner)
		og.seal()
	}
	cg := newGroup()
	for _, w := range ws {
		cg.goFn(func() {
			p.barrier()
			client(w, p)
		})
	}
	cg.seal()
	window(p, len(ws), opt, res, onStart, onEnd)

	t := time.Now()
	deadline := t.Add(opt.DrainTimeout)
	if !waitDrain(cg, deadline) {
		res.StuckStep = "clients_exit"
		return
	}
	if og != nil {
		closeOwner()
		if !waitDrain(og, deadline) {
			res.StuckStep = "owner_exit"
			return
		}
	}
	res.DrainNs = time.Since(t).Nanoseconds()
}

func newPhase(opt Options) *phase {
	return &phase{start: make(chan struct{}), latency: opt.Latency}
}

func sizeofWorker() uintptr { return unsafe.Sizeof(worker{}) }
