package measure

import (
	"math"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func TestBucketsContainTheirValues(t *testing.T) {
	prevUpper := int64(-1)
	for i := 0; i < NumBuckets; i++ {
		u := BucketUpper(i)
		if u <= prevUpper {
			t.Fatalf("bucket %d upper %d not increasing (prev %d)", i, u, prevUpper)
		}
		if got := bucketOf(u); got != i {
			t.Fatalf("bucketOf(upper(%d)=%d) = %d", i, u, got)
		}
		if got := bucketOf(prevUpper + 1); got != i {
			t.Fatalf("bucketOf(lower(%d)=%d) = %d", i, prevUpper+1, got)
		}
		lower := prevUpper + 1
		if lower >= 8 && float64(u-lower+1)/float64(lower) > 0.125+1e-9 {
			t.Fatalf("bucket %d [%d,%d] wider than 12.5%%", i, lower, u)
		}
		prevUpper = u
	}
	if bucketOf(-5) != 0 || bucketOf(math.MaxInt64) != NumBuckets-1 {
		t.Fatal("out-of-range values not clamped")
	}
}

func TestQuantile(t *testing.T) {
	var h Histogram
	if h.Quantile(0.5) != -1 {
		t.Fatal("empty histogram quantile")
	}
	for v := int64(1); v <= 1000; v++ {
		h.Record(v)
	}
	for _, c := range []struct {
		q    float64
		want int64
	}{{0.5, 500}, {0.99, 990}, {0.999, 999}, {1, 1000}} {
		got := h.Quantile(c.q)
		if got < c.want || float64(got) > float64(c.want)*1.125+1 {
			t.Errorf("q%.3f = %d, want in [%d, %d·1.125]", c.q, got, c.want, c.want)
		}
	}
	if h.Max != 1000 || h.N != 1000 {
		t.Fatalf("max %d n %d", h.Max, h.N)
	}
	var o Histogram
	o.Record(5_000_000)
	h.Merge(&o)
	if h.N != 1001 || h.Max != 5_000_000 || h.Quantile(1) != 5_000_000 {
		t.Fatalf("merge: n %d max %d q1 %d", h.N, h.Max, h.Quantile(1))
	}
	if len(o.Sparse()) != 1 {
		t.Fatal("sparse")
	}
}

func TestJain(t *testing.T) {
	if j := Jain([]uint64{5, 5, 5, 5}); math.Abs(j-1) > 1e-12 {
		t.Fatalf("equal shares: %v", j)
	}
	if j := Jain([]uint64{1, 0, 0, 0}); math.Abs(j-0.25) > 1e-12 {
		t.Fatalf("one of four: %v", j)
	}
	if !math.IsNaN(Jain([]uint64{3})) || Jain([]uint64{0, 0}) != 0 {
		t.Fatal("edge cases")
	}
	s := Summarize([]uint64{4, 0, 2, 10})
	if s.Min != 0 || s.Max != 10 || s.Median != 3 || s.Zero != 1 || s.MinMax != 0 {
		t.Fatalf("summary %+v", s)
	}
}

func TestQuantumBoundAndExpectedJain(t *testing.T) {
	w := 1500 * time.Millisecond
	if b := QuantumBoundRaw(2, 256, w); math.Abs(b-2*150.0/256) > 1e-12 {
		t.Fatalf("raw bound %v", b)
	}
	if QuantumBound(8, 4, w) != 1 {
		t.Fatal("clamp")
	}
	// b' = 1.17 (P=2, G=256): 17% of the goroutines get two slices.
	if j := ExpectedQuantumJain(1.171875); math.Abs(j-0.906) > 0.002 {
		t.Fatalf("J_exp(1.17) = %v, want ~0.906", j)
	}
	if j := ExpectedQuantumJain(0.4); math.Abs(j-0.4) > 1e-12 {
		t.Fatalf("J_exp(0.4) = %v", j)
	}
	if j := ExpectedQuantumJain(3); math.Abs(j-1) > 1e-12 {
		t.Fatalf("J_exp(3) = %v", j)
	}
}

func TestFairnessSource(t *testing.T) {
	cases := []struct {
		g, p int
		b, s float64
		want string
	}{
		{4, 8, 1, 0, SourceSync},          // g ≤ p
		{256, 2, 1, 0, SourceSync},        // b ≥ 1
		{10000, 8, 0.12, 0.9, SourceSync}, // most acquisitions park
		{10000, 8, 0.12, 0.5, SourceSync}, // threshold is inclusive
		{10000, 8, 0.12, 0.1, SourceUnexplained},
		{256, 1, 0.586, 0.0, SourceUnexplained}, // formerly "scheduler"
	}
	for _, c := range cases {
		if got := FairnessSource(c.g, c.p, c.b, c.s); got != c.want {
			t.Errorf("%+v: got %s", c, got)
		}
	}
}

func TestMutexLayout(t *testing.T) {
	var m sync.Mutex
	if unsafe.Sizeof(m) != 8 {
		t.Fatalf("sizeof %d", unsafe.Sizeof(m))
	}
	if ok, d := checkLayout(); !ok {
		t.Fatal(d)
	}
	m.Lock()
	done := make(chan struct{})
	go func() { m.Lock(); m.Unlock(); close(done) }()
	if !pollUntil(time.Second, func() bool { return MutexState(&m)>>MutexWaiterShift == 1 }) {
		t.Fatal("waiter count never reached 1")
	}
	m.Unlock()
	<-done
	if MutexState(&m) != 0 {
		t.Fatalf("state %#x after release", MutexState(&m))
	}
}

func TestSamplerSeesNothingOnIdleMutex(t *testing.T) {
	var m sync.Mutex
	s := StartStarvingSampler(&m)
	time.Sleep(20 * time.Millisecond)
	n, st := s.Stop()
	if n == 0 || st != 0 {
		t.Fatalf("samples %d starving %d", n, st)
	}
}

func TestRuntimeReadings(t *testing.T) {
	a := ReadRuntime()
	if a.Err != "" {
		t.Fatal(a.Err)
	}
	if a.GOMAXPROCS != uint64(runtime.GOMAXPROCS(0)) {
		t.Fatalf("gomaxprocs metric %d", a.GOMAXPROCS)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); runtime.Gosched() }()
	}
	wg.Wait()
	b := ReadRuntime()
	if _, _, n, ok := SchedLatencyDelta(a.SchedLat, b.SchedLat); !ok || n == 0 {
		t.Fatal("no scheduling latency samples")
	}
	if _, _, err := CPUTimes(); err != nil {
		t.Fatal(err)
	}
	c := MeasureClock()
	if c.ResolutionNs <= 0 || c.ReadNs <= 0 {
		t.Fatalf("clock %+v", c)
	}
}
