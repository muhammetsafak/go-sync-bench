package scenario

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
	"github.com/muhammetsafak/go-sync-bench/internal/work"
)

// condQueue is the bounded ring buffer of the locked S3 model: one Mutex and
// two Conds. Waiters are woken with Signal; Broadcast is used only by close.
type condQueue struct {
	_        [128]byte
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	buf      []uint64
	head, n  int
	closed   bool
	_        [128]byte
}

func newCondQueue(capacity int) *condQueue {
	q := &condQueue{buf: make([]uint64, capacity)}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

// put blocks while the buffer is full. It returns the time Lock returned.
func (q *condQueue) put(v uint64) (locked int64) {
	q.mu.Lock()
	locked = measure.Now()
	for q.n == len(q.buf) {
		q.notFull.Wait()
	}
	q.buf[(q.head+q.n)%len(q.buf)] = v
	q.n++
	q.notEmpty.Signal()
	q.mu.Unlock()
	return locked
}

// get blocks while the buffer is empty and open. It returns the time Lock
// returned and the time the item was taken.
func (q *condQueue) get() (v uint64, ok bool, locked, taken int64) {
	q.mu.Lock()
	locked = measure.Now()
	for q.n == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if q.n == 0 {
		q.mu.Unlock()
		return 0, false, locked, 0
	}
	v = q.buf[q.head]
	q.head = (q.head + 1) % len(q.buf)
	q.n--
	taken = measure.Now()
	q.notFull.Signal()
	q.mu.Unlock()
	return v, true, locked, taken
}

func (q *condQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.notEmpty.Broadcast()
	q.mu.Unlock()
}

// queueCapacity parses "chan64" / "cond1024".
func queueCapacity(impl string) (kind string, capacity int) {
	for _, k := range []string{"chan", "cond"} {
		if rest, ok := strings.CutPrefix(impl, k); ok {
			n, _ := strconv.Atoi(rest)
			return k, n
		}
	}
	return "", -1
}

// runQueue is S3: G/2 producers and G/2 consumers share one bounded queue.
func runQueue(spec Spec, opt Options) *Result {
	res := &Result{}
	half := spec.G / 2
	prods := newWorkers(half, spec.Seed, opt.Latency)
	cons := newWorkers(half, spec.Seed^0x9e3779b97f4a7c15, opt.Latency)
	for i, w := range cons {
		w.id = half + i
	}
	lockTimed := false
	kind, capacity := queueCapacity(spec.Impl)
	if kind == "cond" && opt.Latency {
		lockTimed = true
	}
	for _, w := range prods {
		if opt.Latency {
			w.putWt = new(measure.Histogram)
		}
		if lockTimed {
			w.lockWait = new(measure.Histogram)
		}
	}
	if lockTimed {
		for _, w := range cons {
			w.lockWait = new(measure.Histogram)
		}
	}
	ph := newPhase(opt)
	wIters := spec.WIters

	var (
		producer, consumer func(w *worker, p *phase)
		closeQ             func()
	)
	switch kind {
	case "chan":
		ch := make(chan uint64, capacity)
		closeQ = func() { close(ch) }
		producer = func(w *worker, p *phase) {
			lat := p.latency
			id := uint64(w.id) << 32
			for !p.stop.Load() {
				v := id | (w.ops & 0xFFFFFFFF)
				if lat && w.tick() {
					t0 := measure.Now()
					ch <- v
					w.putWt.Record(measure.Now() - t0)
				} else {
					ch <- v
				}
				w.sum += v
				w.xor ^= v
				w.ops++
			}
		}
		consumer = func(w *worker, p *phase) {
			lat := p.latency
			for {
				inWindow := !p.stop.Load()
				var v uint64
				var ok bool
				if inWindow && lat && w.tick() {
					t0 := measure.Now()
					v, ok = <-ch
					t1 := measure.Now()
					if ok {
						w.record(t0, t1, t1, &p.stop)
					}
				} else {
					v, ok = <-ch
				}
				if !ok {
					return
				}
				w.sum += v
				w.xor ^= v
				if inWindow {
					w.ops++
				} else {
					w.drainOps++
				}
				work.Spin(wIters)
			}
		}
	case "cond":
		q := newCondQueue(capacity)
		closeQ = q.close
		producer = func(w *worker, p *phase) {
			lat := p.latency
			id := uint64(w.id) << 32
			for !p.stop.Load() {
				v := id | (w.ops & 0xFFFFFFFF)
				if lat && w.tick() {
					t0 := measure.Now()
					locked := q.put(v)
					w.putWt.Record(measure.Now() - t0)
					w.recordLock(locked - t0)
				} else {
					q.put(v)
				}
				w.sum += v
				w.xor ^= v
				w.ops++
			}
		}
		consumer = func(w *worker, p *phase) {
			lat := p.latency
			for {
				inWindow := !p.stop.Load()
				var v uint64
				var ok bool
				if inWindow && lat && w.tick() {
					t0 := measure.Now()
					var locked, taken int64
					v, ok, locked, taken = q.get()
					t2 := measure.Now()
					w.recordLock(locked - t0)
					if ok {
						w.record(t0, taken, t2, &p.stop)
					}
				} else {
					v, ok, _, _ = q.get()
				}
				if !ok {
					return
				}
				w.sum += v
				w.xor ^= v
				if inWindow {
					w.ops++
				} else {
					w.drainOps++
				}
				work.Spin(wIters)
			}
		}
	}

	pg, cg := newGroup(), newGroup()
	for _, w := range prods {
		pg.goFn(func() {
			ph.barrier()
			producer(w, ph)
		})
	}
	for _, w := range cons {
		cg.goFn(func() {
			ph.barrier()
			consumer(w, ph)
		})
	}
	pg.seal()
	cg.seal()
	window(ph, spec.G, opt, res, nil, nil)

	t := time.Now()
	deadline := t.Add(opt.DrainTimeout)
	switch {
	case !waitDrain(pg, deadline):
		res.StuckStep = "producers_exit"
	default:
		closeQ()
		if !waitDrain(cg, deadline) {
			res.StuckStep = "consumers_exit"
		}
	}
	if res.StuckStep != "" {
		res.Detail = "drain stuck at " + res.StuckStep
		return res
	}
	res.DrainNs = time.Since(t).Nanoseconds()

	collect(cons, res, &res.PerG)
	var put, psum, pxor, csum, cxor uint64
	res.PerGProd = make([]uint64, len(prods))
	for i, w := range prods {
		res.PerGProd[i] = w.ops
		put += w.ops
		psum += w.sum
		pxor ^= w.xor
		if w.putWt != nil {
			res.PutWait.Merge(w.putWt)
		}
		if w.lockWait != nil {
			res.LockWait.Merge(w.lockWait)
			res.LockWaitSamples += w.lockSamples
			res.LockWaitOver1ms += w.lockOver1ms
		}
	}
	for _, w := range cons {
		csum += w.sum
		cxor ^= w.xor
	}
	got := res.Ops + res.DrainOps
	res.Correct = put == got && psum == csum && pxor == cxor
	res.Detail = fmt.Sprintf("produced=%d consumed=%d (window=%d drain=%d) sum_ok=%t xor_ok=%t",
		put, got, res.Ops, res.DrainOps, psum == csum, pxor == cxor)
	return res
}
