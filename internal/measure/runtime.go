package measure

import (
	"math"
	"runtime"
	"runtime/metrics"
	"syscall"
	"time"
)

// Runtime metric names read by the harness.
const (
	MetricMutexWait   = "/sync/mutex/wait/total:seconds"
	MetricSchedLat    = "/sched/latencies:seconds"
	MetricGCCycles    = "/gc/cycles/total:gc-cycles"
	MetricGOMAXPROCS  = "/sched/gomaxprocs:threads"
	metricSampleCount = 4
)

var epoch = time.Now()

// Now returns monotonic nanoseconds since process start. It is the only clock
// the harness uses for latency samples.
func Now() int64 { return int64(time.Since(epoch)) }

// RuntimeSnapshot is one reading of the runtime metrics the harness uses.
type RuntimeSnapshot struct {
	MutexWaitS float64
	GCCycles   uint64
	GOMAXPROCS uint64
	SchedLat   *metrics.Float64Histogram
	Err        string
}

// ReadRuntime reads the runtime metrics. A missing or wrongly typed metric
// is reported in Err, never by panicking.
func ReadRuntime() RuntimeSnapshot {
	s := []metrics.Sample{
		{Name: MetricMutexWait},
		{Name: MetricSchedLat},
		{Name: MetricGCCycles},
		{Name: MetricGOMAXPROCS},
	}
	metrics.Read(s)
	var out RuntimeSnapshot
	bad := func(name string) {
		if out.Err != "" {
			out.Err += "; "
		}
		out.Err += "metric unavailable: " + name
	}
	if s[0].Value.Kind() == metrics.KindFloat64 {
		out.MutexWaitS = s[0].Value.Float64()
	} else {
		bad(MetricMutexWait)
	}
	if s[1].Value.Kind() == metrics.KindFloat64Histogram {
		h := s[1].Value.Float64Histogram()
		cp := &metrics.Float64Histogram{
			Counts:  append([]uint64(nil), h.Counts...),
			Buckets: append([]float64(nil), h.Buckets...),
		}
		out.SchedLat = cp
	} else {
		bad(MetricSchedLat)
	}
	if s[2].Value.Kind() == metrics.KindUint64 {
		out.GCCycles = s[2].Value.Uint64()
	} else {
		bad(MetricGCCycles)
	}
	if s[3].Value.Kind() == metrics.KindUint64 {
		out.GOMAXPROCS = s[3].Value.Uint64()
	} else {
		bad(MetricGOMAXPROCS)
	}
	return out
}

// SchedLatencyDelta returns the p50 and p99 of the bucket difference between
// two cumulative /sched/latencies readings, in seconds (bucket upper bound;
// the lower bound when the upper one is +Inf). ok is false when there are no
// samples in the interval or the readings are incompatible.
func SchedLatencyDelta(a, b *metrics.Float64Histogram) (p50, p99 float64, samples uint64, ok bool) {
	if a == nil || b == nil || len(a.Counts) != len(b.Counts) {
		return 0, 0, 0, false
	}
	diff := make([]uint64, len(b.Counts))
	for i := range b.Counts {
		if b.Counts[i] >= a.Counts[i] {
			diff[i] = b.Counts[i] - a.Counts[i]
		}
		samples += diff[i]
	}
	if samples == 0 {
		return 0, 0, 0, false
	}
	q := func(p float64) float64 {
		rank := uint64(math.Ceil(p * float64(samples)))
		if rank < 1 {
			rank = 1
		}
		var cum uint64
		for i, c := range diff {
			cum += c
			if cum >= rank {
				up := b.Buckets[i+1]
				if math.IsInf(up, 1) {
					return b.Buckets[i]
				}
				return up
			}
		}
		return b.Buckets[len(b.Buckets)-1]
	}
	return q(0.50), q(0.99), samples, true
}

// CPUTimes returns user and system CPU seconds of the whole process.
func CPUTimes() (user, sys float64, err error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, 0, err
	}
	tv := func(t syscall.Timeval) float64 {
		return float64(t.Sec) + float64(t.Usec)/1e6
	}
	return tv(ru.Utime), tv(ru.Stime), nil
}

// ClockInfo characterises the monotonic clock used by Now.
type ClockInfo struct {
	ResolutionNs int64   `json:"clock_resolution_ns"`
	ReadNs       float64 `json:"clock_read_ns"`
}

// MeasureClock estimates the clock resolution as the smallest positive step
// between two consecutive distinct readings, and the read cost as the mean
// cost of a call to Now.
func MeasureClock() ClockInfo {
	const reads = 1_000_000
	var sink int64
	t0 := Now()
	for i := 0; i < reads; i++ {
		sink += Now()
	}
	readNs := float64(Now()-t0) / reads
	_ = sink

	res := int64(math.MaxInt64)
	for i := 0; i < 20_000; i++ {
		a := Now()
		b := Now()
		for b == a {
			b = Now()
		}
		if d := b - a; d > 0 && d < res {
			res = d
		}
	}
	runtime.Gosched()
	return ClockInfo{ResolutionNs: res, ReadNs: readNs}
}
