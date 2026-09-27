package measure

import (
	"math"
	"slices"
	"time"
)

// Labelling thresholds of the fairness attribution rule. They are
// assumptions of the design and are checked by the smoke run.
const (
	BandLow            = 0.8
	BandHigh           = 1.2
	SlowShareThreshold = 0.5
	ParkThresholdNs    = 10_000 // 10 µs: acquire_wait above this counts as "slow"
	PreemptQuantum     = 10 * time.Millisecond
)

// Fairness sources.
const (
	SourceSync        = "sync"
	SourceScheduler   = "scheduler"
	SourceUnexplained = "unexplained"
)

// Jain returns Jain's fairness index (Σx)² / (n·Σx²). It returns NaN for
// fewer than two values and 0 when every value is zero.
func Jain(xs []uint64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	var sum, sq float64
	for _, x := range xs {
		f := float64(x)
		sum += f
		sq += f * f
	}
	if sq == 0 {
		return 0
	}
	return sum * sum / (float64(len(xs)) * sq)
}

// OpsSummary is the compact description of per-goroutine op counts.
type OpsSummary struct {
	N      int     `json:"n"`
	Min    uint64  `json:"min"`
	Max    uint64  `json:"max"`
	Median float64 `json:"median"`
	Zero   int     `json:"zero"`
	Jain   float64 `json:"jain"`
	MinMax float64 `json:"min_max"`
}

// Summarize computes min, max, median, zero count, Jain and min/max ratio.
func Summarize(xs []uint64) OpsSummary {
	s := OpsSummary{N: len(xs), Jain: Jain(xs)}
	if len(xs) == 0 {
		return s
	}
	sorted := slices.Clone(xs)
	slices.Sort(sorted)
	s.Min, s.Max = sorted[0], sorted[len(sorted)-1]
	if n := len(sorted); n%2 == 1 {
		s.Median = float64(sorted[n/2])
	} else {
		s.Median = (float64(sorted[n/2-1]) + float64(sorted[n/2])) / 2
	}
	for _, x := range xs {
		if x == 0 {
			s.Zero++
		}
	}
	if s.Max > 0 {
		s.MinMax = float64(s.Min) / float64(s.Max)
	}
	return s
}

// QuantumBoundRaw is P·⌊window/10ms⌋/G without the clamp to 1.
func QuantumBoundRaw(p, g int, window time.Duration) float64 {
	if g <= 0 {
		return math.NaN()
	}
	quanta := float64(window / PreemptQuantum)
	return float64(p) * quanta / float64(g)
}

// QuantumBound is min(1, QuantumBoundRaw).
func QuantumBound(p, g int, window time.Duration) float64 {
	return math.Min(1, QuantumBoundRaw(p, g, window))
}

// ExpectedQuantumJain is the Jain index of the best split of b' slices per
// goroutine in whole slices: n = ⌊b'⌋ slices for a fraction (1-f) of the
// goroutines and n+1 for the rest, f = b'-n. For b' < 1 it reduces to b'.
func ExpectedQuantumJain(bRaw float64) float64 {
	if bRaw <= 0 || math.IsNaN(bRaw) {
		return math.NaN()
	}
	n := math.Floor(bRaw)
	f := bRaw - n
	den := f*(n+1)*(n+1) + (1-f)*n*n
	if den == 0 {
		return math.NaN()
	}
	return bRaw * bRaw / den
}

// FairnessSource applies the attribution rule of the design. g and p are the
// cell's goroutine and processor counts, b the clamped quantum bound, s the
// slow_acquire_share and j the Jain index.
func FairnessSource(g, p int, b, s, j float64) string {
	if g <= p || b >= 1 {
		return SourceSync
	}
	if s >= SlowShareThreshold {
		return SourceSync
	}
	if j >= BandLow*b && j <= BandHigh*b {
		return SourceScheduler
	}
	return SourceUnexplained
}
