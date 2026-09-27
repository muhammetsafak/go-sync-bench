package scenario

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
	"github.com/muhammetsafak/go-sync-bench/internal/work"
)

// paddedMutex keeps a sync.Mutex alone on its cache line.
type paddedMutex struct {
	_  [128]byte
	mu sync.Mutex
	_  [120]byte
}

type paddedRWMutex struct {
	_  [128]byte
	mu sync.RWMutex
	_  [104]byte
}

// paddedCounter keeps the S1 counter on its own cache line.
type paddedCounter struct {
	_ [128]byte
	n uint64
	a atomic.Uint64
	_ [112]byte
}

// counterReq is one request/response message: the client's reply channel.
type counterReq struct{ reply chan uint64 }

// runCounter is S1: G clients increment one shared counter.
func runCounter(spec Spec, opt Options) *Result {
	res := &Result{}
	ws := newWorkers(spec.G, spec.Seed, opt.Latency)
	c := new(paddedCounter)
	var (
		client     func(w *worker, p *phase)
		owner      func()
		closeOwner func()
		onStart    func()
		onEnd      func()
		applied    uint64 // owner models, written only by the owner
		appliedWin uint64
	)
	wIters := spec.WIters
	ph := newPhase(opt)

	switch spec.Impl {
	case "atomic":
		client = func(w *worker, p *phase) {
			lat := p.latency
			for !p.stop.Load() {
				if lat && w.tick() {
					t0 := measure.Now()
					c.a.Add(1)
					t1 := measure.Now()
					w.record(t0, t1, t1, &p.stop)
				} else {
					c.a.Add(1)
				}
				w.ops++
			}
		}
	case "mutex":
		pm := new(paddedMutex)
		client = func(w *worker, p *phase) {
			lat := p.latency
			for !p.stop.Load() {
				if lat && w.tick() {
					t0 := measure.Now()
					pm.mu.Lock()
					t1 := measure.Now()
					work.Spin(wIters)
					c.n++
					pm.mu.Unlock()
					t2 := measure.Now()
					w.record(t0, t1, t2, &p.stop)
				} else {
					pm.mu.Lock()
					work.Spin(wIters)
					c.n++
					pm.mu.Unlock()
				}
				w.ops++
			}
		}
		if opt.StarvingSampler {
			var s *measure.StarvingSampler
			onStart = func() { s = measure.StartStarvingSampler(&pm.mu) }
			onEnd = func() { res.StarvingSamples, res.StarvingSet = s.Stop() }
		}
	case "rwmutex":
		pm := new(paddedRWMutex)
		client = func(w *worker, p *phase) {
			lat := p.latency
			for !p.stop.Load() {
				if lat && w.tick() {
					t0 := measure.Now()
					pm.mu.Lock()
					t1 := measure.Now()
					work.Spin(wIters)
					c.n++
					pm.mu.Unlock()
					t2 := measure.Now()
					w.record(t0, t1, t2, &p.stop)
				} else {
					pm.mu.Lock()
					work.Spin(wIters)
					c.n++
					pm.mu.Unlock()
				}
				w.ops++
			}
		}
	case "chan_reqresp":
		req := make(chan counterReq) // unbuffered, shared by every client
		owner = func() {
			for r := range req {
				work.Spin(wIters)
				c.n++
				applied++
				r.reply <- c.n
			}
		}
		closeOwner = func() { close(req) }
		client = func(w *worker, p *phase) {
			r := counterReq{reply: make(chan uint64, 1)} // reused for every op
			lat := p.latency
			for !p.stop.Load() {
				if lat && w.tick() {
					t0 := measure.Now()
					req <- r
					t1 := measure.Now()
					w.sink += <-r.reply
					t2 := measure.Now()
					w.record(t0, t1, t2, &p.stop)
				} else {
					req <- r
					w.sink += <-r.reply
				}
				w.ops++
			}
		}
	case "chan_oneway":
		req := make(chan struct{})
		stop := &ph.stop
		owner = func() {
			for range req {
				work.Spin(wIters)
				c.n++
				applied++
				if !stop.Load() {
					appliedWin++
				}
			}
		}
		closeOwner = func() { close(req) }
		client = func(w *worker, p *phase) {
			lat := p.latency
			for !p.stop.Load() {
				if lat && w.tick() {
					t0 := measure.Now()
					req <- struct{}{}
					t1 := measure.Now()
					w.record(t0, t1, t1, &p.stop)
				} else {
					req <- struct{}{}
				}
				w.ops++
			}
		}
	}

	clientServer(ph, opt, res, ws, client, owner, closeOwner, onStart, onEnd)
	// A stuck drain may leave goroutines still writing the counter and the
	// owner's tallies (applied, appliedWin): neither is read nor written
	// unless the drain completed.
	if res.StuckStep != "" {
		return finishCounter(spec, res, ws, 0, 0, 0)
	}
	total := c.n
	if spec.Impl == "atomic" {
		total = c.a.Load()
	}
	return finishCounter(spec, res, ws, total, applied, appliedWin)
}

func finishCounter(spec Spec, res *Result, ws []*worker, total, applied, appliedWin uint64) *Result {
	collect(ws, res, &res.PerG)
	if res.StuckStep != "" {
		res.Detail = "drain stuck at " + res.StuckStep
		return res
	}
	var sum uint64
	for _, w := range ws {
		sum += w.ops
	}
	switch spec.Impl {
	case "chan_oneway":
		res.Sends = sum
		res.Applied = applied
		res.Ops = appliedWin
		res.Correct = total == sum && applied == sum
		res.Detail = fmt.Sprintf("counter=%d sends=%d applied=%d applied_in_window=%d", total, sum, applied, appliedWin)
	case "chan_reqresp":
		res.Applied = applied
		res.Correct = total == sum && applied == sum
		res.Detail = fmt.Sprintf("counter=%d client_ops=%d applied=%d", total, sum, applied)
	default:
		res.Correct = total == sum
		res.Detail = fmt.Sprintf("counter=%d client_ops=%d", total, sum)
	}
	return res
}
