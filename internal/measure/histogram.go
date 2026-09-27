// Package measure holds the measurement primitives of the harness: the
// log-bucket histogram, fairness indices, runtime/metrics and getrusage
// readings, clock characterisation and the sync.Mutex state sampler.
package measure

import (
	"math"
	"math/bits"
)

// Histogram layout: values below 8 ns get one bucket each; from 8 ns on,
// every octave [2^k, 2^(k+1)) is split into 8 equal sub-buckets, so a bucket
// is at most 12.5% wide. The last octave starts at 2^37 ns (~137 s); larger
// values are clamped into the last bucket, and Max keeps the exact value.
const (
	subBits     = 3
	subBuckets  = 1 << subBits // 8
	maxOctave   = 37
	NumBuckets  = subBuckets + (maxOctave-subBits+1)*subBuckets // 288
	lastBucket  = NumBuckets - 1
	maxBucketNs = int64(1)<<(maxOctave+1) - 1
)

// Histogram is a fixed-size, allocation-free latency histogram. It is not
// safe for concurrent use: every worker goroutine owns its own.
type Histogram struct {
	Counts [NumBuckets]uint32
	N      uint64
	Max    int64
}

// bucketOf maps a value in nanoseconds to its bucket index.
func bucketOf(v int64) int {
	if v < subBuckets {
		if v < 0 {
			return 0
		}
		return int(v)
	}
	if v > maxBucketNs {
		return lastBucket
	}
	k := bits.Len64(uint64(v)) - 1 // octave, >= subBits
	sub := int(uint64(v)>>(k-subBits)) & (subBuckets - 1)
	return subBuckets + (k-subBits)*subBuckets + sub
}

// BucketUpper returns the inclusive upper bound, in nanoseconds, of bucket i.
func BucketUpper(i int) int64 {
	if i < subBuckets {
		return int64(i)
	}
	k := (i-subBuckets)/subBuckets + subBits
	sub := int64((i - subBuckets) % subBuckets)
	return ((subBuckets+sub+1)<<(k-subBits) - 1)
}

// Record adds one value in nanoseconds.
func (h *Histogram) Record(v int64) {
	if v < 0 {
		v = 0
	}
	i := bucketOf(v)
	if h.Counts[i] != math.MaxUint32 {
		h.Counts[i]++
	}
	h.N++
	if v > h.Max {
		h.Max = v
	}
}

// Merge adds o into h.
func (h *Histogram) Merge(o *Histogram) {
	for i, c := range o.Counts {
		s := uint64(h.Counts[i]) + uint64(c)
		if s > math.MaxUint32 {
			s = math.MaxUint32
		}
		h.Counts[i] = uint32(s)
	}
	h.N += o.N
	if o.Max > h.Max {
		h.Max = o.Max
	}
}

// Quantile returns the upper bound of the bucket that holds quantile q
// (0 < q <= 1): a pessimistic rounding. It returns -1 for an empty histogram.
// The result never exceeds Max.
func (h *Histogram) Quantile(q float64) int64 {
	var total uint64
	for _, c := range h.Counts {
		total += uint64(c)
	}
	if total == 0 {
		return -1
	}
	rank := uint64(math.Ceil(q * float64(total)))
	if rank < 1 {
		rank = 1
	}
	var cum uint64
	for i, c := range h.Counts {
		cum += uint64(c)
		if cum >= rank {
			u := BucketUpper(i)
			if u > h.Max {
				u = h.Max
			}
			return u
		}
	}
	return h.Max
}

// Sparse returns the non-empty buckets as [upper_ns, count] pairs, for the raw
// JSON record.
func (h *Histogram) Sparse() [][2]int64 {
	out := make([][2]int64, 0, 32)
	for i, c := range h.Counts {
		if c != 0 {
			out = append(out, [2]int64{BucketUpper(i), int64(c)})
		}
	}
	return out
}
