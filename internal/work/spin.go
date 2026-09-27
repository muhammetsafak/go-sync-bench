// Package work provides the calibrated busy loop that stands in for the
// serialised work W of a cell.
//
// The loop is a dependent xorshift chain: every step needs the previous one,
// there is no memory access, no system call and no runtime call. The result
// feeds a branch the compiler cannot evaluate, so the loop is never dropped.
package work

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Sink counts Spin results that were zero. An xorshift chain started from a
// non-zero value never reaches zero, so Sink stays 0 and Spin never writes
// shared memory: goroutines spinning in parallel (under an RLock, or outside
// the critical section) do not bounce a cache line or race on it. Its only
// purpose is to keep the result observable to the compiler.
var Sink atomic.Uint64

// Spin runs n dependent xorshift steps.
//
//go:noinline
func Spin(n int) {
	if n <= 0 {
		return
	}
	if spin(uint64(n)|1, n) == 0 {
		Sink.Add(1)
	}
}

//go:noinline
func spin(x uint64, n int) uint64 {
	for i := 0; i < n; i++ {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
	}
	return x
}

// Calibration is the result of one calibration run.
type Calibration struct {
	NsPerIter float64   `json:"ns_per_iter"`
	Trials    []float64 `json:"-"`
}

// Calibration defaults from the design: 21 trials of 1e6 iterations, median.
const (
	CalibTrials = 21
	CalibIters  = 1_000_000
)

// Calibrate measures the median cost of one Spin iteration on the current
// thread. trials and iters fall back to the design defaults when <= 0.
func Calibrate(trials, iters int) Calibration {
	if trials <= 0 {
		trials = CalibTrials
	}
	if iters <= 0 {
		iters = CalibIters
	}
	Spin(iters) // warm the code path once
	res := make([]float64, trials)
	for i := range res {
		t0 := time.Now()
		Spin(iters)
		res[i] = float64(time.Since(t0).Nanoseconds()) / float64(iters)
	}
	sorted := slices.Clone(res)
	slices.Sort(sorted)
	return Calibration{NsPerIter: sorted[len(sorted)/2], Trials: res}
}

// ItersFor returns the iteration count that realises the nominal work
// nominalNs at nsPerIter. A nominal of 0 is always 0 iterations; any
// positive nominal gets at least one iteration.
func ItersFor(nominalNs int64, nsPerIter float64) int {
	if nominalNs <= 0 || nsPerIter <= 0 {
		return 0
	}
	n := int(float64(nominalNs)/nsPerIter + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// CalibrateParallel runs the calibration on workers goroutines at once and
// returns the median of every trial of every worker.
//
// On the Docker VM of an Apple-silicon host the cost of one iteration depends
// on how many cores of the cluster are busy (the cluster clock drops as more
// cores run), so a single-threaded calibration taken between two phases reads
// the clock of an idle container, not the one the measured windows run at.
// Keeping all P of the container busy while calibrating reproduces the clock
// of the windows. Each worker keeps its own chain, so no memory is shared.
func CalibrateParallel(workers, trials, iters int) Calibration {
	if workers <= 1 {
		return Calibrate(trials, iters)
	}
	if trials <= 0 {
		trials = CalibTrials
	}
	if iters <= 0 {
		iters = CalibIters
	}
	res := make([][]float64, workers)
	sinks := make([]uint64, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x := uint64(w)*0x9e3779b97f4a7c15 | 1
			<-start
			x = spin(x, iters) // warm the code path once
			r := make([]float64, trials)
			for i := range r {
				t0 := time.Now()
				x = spin(x, iters)
				r[i] = float64(time.Since(t0).Nanoseconds()) / float64(iters)
			}
			res[w], sinks[w] = r, x
		}()
	}
	close(start)
	wg.Wait()
	var all []float64
	for w := range workers {
		all = append(all, res[w]...)
		if sinks[w] == 0 {
			Sink.Add(1)
		}
	}
	sorted := slices.Clone(all)
	slices.Sort(sorted)
	return Calibration{NsPerIter: sorted[len(sorted)/2], Trials: all}
}

// Drift is the relative difference between two calibrations, |end-start|/start.
func Drift(start, end float64) float64 {
	if start <= 0 {
		return 0
	}
	d := (end - start) / start
	if d < 0 {
		d = -d
	}
	return d
}
