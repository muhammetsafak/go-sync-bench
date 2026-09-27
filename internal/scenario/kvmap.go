package scenario

import (
	"fmt"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
	"github.com/muhammetsafak/go-sync-bench/internal/work"
)

// S2 map layout. A value encodes who wrote it: gid<<40 | seq<<10 | key.
// The initial value carries the reserved gid 0xFFFFFF.
const (
	KVKeys     = 1024
	kvKeyBits  = 10
	kvSeqBits  = 30
	kvGidShift = 40
	kvInitGid  = 0xFFFFFF
	kvKeyMask  = KVKeys - 1
	kvSeqMask  = 1<<kvSeqBits - 1
)

func kvInitial() map[uint64]uint64 {
	m := make(map[uint64]uint64, KVKeys)
	for k := uint64(0); k < KVKeys; k++ {
		m[k] = kvInitGid<<kvGidShift | k
	}
	return m
}

func kvValue(gid int, seq, key uint64) uint64 {
	return uint64(gid)<<kvGidShift | (seq&kvSeqMask)<<kvKeyBits | key
}

// kvReq is one access sent to the S2 owner goroutine.
type kvReq struct {
	key, val uint64
	write    bool
	reply    chan uint64
}

// runKV is S2: G goroutines read and write one shared map.
func runKV(spec Spec, opt Options) *Result {
	res := &Result{}
	ws := newWorkers(spec.G, spec.Seed, opt.Latency)
	for _, w := range ws {
		w.writes = 0
	}
	m := kvInitial()
	ph := newPhase(opt)
	wIters := spec.WIters
	readPct := uint64(spec.ReadPct)
	var (
		client     func(w *worker, p *phase)
		owner      func()
		closeOwner func()
		onStart    func()
		onEnd      func()
		totalW     uint64 // writes applied, counted under the write lock or by the owner
	)

	// next draws the next access of w: key and whether it is a read.
	next := func(w *worker) (uint64, bool) {
		r := w.rng.Uint64()
		return (r >> 8) & kvKeyMask, r%100 < readPct
	}

	switch spec.Impl {
	case "mutex":
		pm := new(paddedMutex)
		op := func(w *worker, key uint64, read bool) {
			if read {
				w.sink += m[key]
				work.Spin(wIters)
			} else {
				m[key] = kvValue(w.id, w.writes, key)
				w.writes++
				totalW++
				work.Spin(wIters)
			}
		}
		client = func(w *worker, p *phase) {
			lat := p.latency
			for !p.stop.Load() {
				key, read := next(w)
				if lat && w.tick() {
					t0 := measure.Now()
					pm.mu.Lock()
					t1 := measure.Now()
					op(w, key, read)
					pm.mu.Unlock()
					t2 := measure.Now()
					w.record(t0, t1, t2, &p.stop)
				} else {
					pm.mu.Lock()
					op(w, key, read)
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
		read := func(w *worker, key uint64) {
			pm.mu.RLock()
			w.sink += m[key]
			work.Spin(wIters)
			pm.mu.RUnlock()
		}
		write := func(w *worker, key uint64) {
			pm.mu.Lock()
			m[key] = kvValue(w.id, w.writes, key)
			w.writes++
			totalW++
			work.Spin(wIters)
			pm.mu.Unlock()
		}
		client = func(w *worker, p *phase) {
			lat := p.latency
			for !p.stop.Load() {
				key, isRead := next(w)
				if lat && w.tick() {
					t0 := measure.Now()
					var t1 int64
					if isRead {
						pm.mu.RLock()
						t1 = measure.Now()
						w.sink += m[key]
						work.Spin(wIters)
						pm.mu.RUnlock()
					} else {
						pm.mu.Lock()
						t1 = measure.Now()
						m[key] = kvValue(w.id, w.writes, key)
						w.writes++
						totalW++
						work.Spin(wIters)
						pm.mu.Unlock()
					}
					t2 := measure.Now()
					w.record(t0, t1, t2, &p.stop)
				} else if isRead {
					read(w, key)
				} else {
					write(w, key)
				}
				w.ops++
			}
		}
	case "chan_owner":
		req := make(chan *kvReq)
		owner = func() {
			for r := range req {
				var v uint64
				if r.write {
					m[r.key] = r.val
					totalW++
				} else {
					v = m[r.key]
				}
				work.Spin(wIters)
				r.reply <- v
			}
		}
		closeOwner = func() { close(req) }
		client = func(w *worker, p *phase) {
			r := &kvReq{reply: make(chan uint64, 1)} // reused for every op
			lat := p.latency
			for !p.stop.Load() {
				key, isRead := next(w)
				r.key, r.write = key, !isRead
				if !isRead {
					r.val = kvValue(w.id, w.writes, key)
					w.writes++
				}
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
	}

	clientServer(ph, opt, res, ws, client, owner, closeOwner, onStart, onEnd)
	collect(ws, res, &res.PerG)
	if res.StuckStep != "" {
		res.Detail = "drain stuck at " + res.StuckStep
		return res
	}
	res.Correct, res.Detail = checkKV(m, ws, totalW)
	return res
}

// checkKV is the S2 part of G1.
func checkKV(m map[uint64]uint64, ws []*worker, totalW uint64) (bool, string) {
	if len(m) != KVKeys {
		return false, fmt.Sprintf("map has %d keys, want %d", len(m), KVKeys)
	}
	unwritten := 0
	for k, v := range m {
		if v&kvKeyMask != k {
			return false, fmt.Sprintf("key %d holds value %#x with key bits %d", k, v, v&kvKeyMask)
		}
		gid := v >> kvGidShift
		if gid == kvInitGid {
			unwritten++
			continue
		}
		if gid >= uint64(len(ws)) {
			return false, fmt.Sprintf("key %d written by gid %d >= G=%d", k, gid, len(ws))
		}
		seq := (v >> kvKeyBits) & kvSeqMask
		if seq >= ws[gid].writes {
			return false, fmt.Sprintf("key %d: seq %d >= writes %d of gid %d", k, seq, ws[gid].writes, gid)
		}
	}
	var sum uint64
	for _, w := range ws {
		sum += w.writes
	}
	if sum != totalW {
		return false, fmt.Sprintf("goroutine writes %d != applied writes %d", sum, totalW)
	}
	return true, fmt.Sprintf("keys=%d unwritten_keys=%d writes=%d", len(m), unwritten, totalW)
}
