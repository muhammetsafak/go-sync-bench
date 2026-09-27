package scenario

import (
	"testing"
	"time"
)

// short runs one small phase of spec with latency sampling on.
func short(t *testing.T, spec Spec, latency bool) *Result {
	t.Helper()
	res, err := Run(spec, Options{Window: 30 * time.Millisecond, Latency: latency, StarvingSampler: latency && spec.UsesMutex()})
	if err != nil {
		t.Fatalf("%+v: %v", spec, err)
	}
	if res.StuckStep != "" {
		t.Fatalf("%+v: drain stuck at %s", spec, res.StuckStep)
	}
	if !res.Correct {
		t.Fatalf("%+v: G1 failed: %s", spec, res.Detail)
	}
	if !res.BarrierOK {
		t.Fatalf("%+v: barrier ready %d", spec, res.Ready)
	}
	if res.Ops == 0 {
		t.Fatalf("%+v: no ops in window", spec)
	}
	return res
}

func TestValidate(t *testing.T) {
	bad := []Spec{
		{Scenario: "s4", Impl: "mutex", G: 1},
		{Scenario: S1, Impl: "chan64", G: 1},
		{Scenario: S1, Impl: "atomic", G: 4, WIters: 3},
		{Scenario: S3, Impl: "chan0", G: 2},
		{Scenario: S3, Impl: "chan0", G: 5},
		{Scenario: S2, Impl: "mutex", G: 0},
	}
	for _, s := range bad {
		if s.Validate() == nil {
			t.Errorf("%+v: want error", s)
		}
	}
}

func TestCounter(t *testing.T) {
	for _, impl := range Impls[S1] {
		for _, g := range []int{1, 4, 64} {
			for _, w := range []int{0, 5} {
				if impl == "atomic" && w != 0 {
					continue
				}
				for _, lat := range []bool{false, true} {
					res := short(t, Spec{Scenario: S1, Impl: impl, G: g, WIters: w, Seed: 1}, lat)
					if len(res.PerG) != g {
						t.Fatalf("%s: per-goroutine len %d", impl, len(res.PerG))
					}
					if lat && res.Samples == 0 {
						t.Fatalf("%s G=%d: no latency samples", impl, g)
					}
					if !lat && res.Samples != 0 {
						t.Fatalf("%s: samples without latency mode", impl)
					}
				}
			}
		}
	}
}

func TestOneWayCountsOwner(t *testing.T) {
	res := short(t, Spec{Scenario: S1, Impl: "chan_oneway", G: 4, Seed: 2}, false)
	if res.Ops > res.Applied || res.Applied != res.Sends {
		t.Fatalf("ops=%d applied=%d sends=%d", res.Ops, res.Applied, res.Sends)
	}
}

func TestKV(t *testing.T) {
	for _, impl := range Impls[S2] {
		for _, pct := range []int{50, 90, 99} {
			for _, g := range []int{1, 4, 64} {
				short(t, Spec{Scenario: S2, Impl: impl, G: g, ReadPct: pct, WIters: 3, Seed: 3}, g == 4)
			}
		}
	}
}

func TestCheckKVDetectsCorruption(t *testing.T) {
	ws := newWorkers(2, 1, false)
	m := kvInitial()
	m[5] = kvValue(1, 0, 5)
	ws[1].writes = 1
	if ok, d := checkKV(m, ws, 1); !ok {
		t.Fatalf("valid map rejected: %s", d)
	}
	cases := map[string]func(map[uint64]uint64) (map[uint64]uint64, uint64){
		"key bits":    func(m map[uint64]uint64) (map[uint64]uint64, uint64) { m[7] = kvValue(0, 0, 8); return m, 1 },
		"gid":         func(m map[uint64]uint64) (map[uint64]uint64, uint64) { m[7] = kvValue(9, 0, 7); return m, 1 },
		"seq":         func(m map[uint64]uint64) (map[uint64]uint64, uint64) { m[7] = kvValue(1, 4, 7); return m, 1 },
		"total":       func(m map[uint64]uint64) (map[uint64]uint64, uint64) { return m, 2 },
		"missing key": func(m map[uint64]uint64) (map[uint64]uint64, uint64) { delete(m, 3); return m, 1 },
	}
	for name, mut := range cases {
		mm := kvInitial()
		mm[5] = kvValue(1, 0, 5)
		mm, total := mut(mm)
		if ok, _ := checkKV(mm, ws, total); ok {
			t.Errorf("%s: corruption not detected", name)
		}
	}
}

func TestQueue(t *testing.T) {
	for _, impl := range Impls[S3] {
		for _, g := range []int{4, 64} {
			for _, lat := range []bool{false, true} {
				res := short(t, Spec{Scenario: S3, Impl: impl, G: g, WIters: 3, Seed: 4}, lat)
				if len(res.PerG) != g/2 || len(res.PerGProd) != g/2 {
					t.Fatalf("%s: per-goroutine lengths %d/%d", impl, len(res.PerG), len(res.PerGProd))
				}
				if lat && impl[:4] == "cond" && res.LockWaitSamples == 0 {
					t.Fatalf("%s: no lock_wait samples", impl)
				}
				if lat && res.PutWait.N == 0 {
					t.Fatalf("%s: no put_wait samples", impl)
				}
			}
		}
	}
}

func TestCondQueueCloseWakesConsumers(t *testing.T) {
	q := newCondQueue(4)
	done := make(chan bool)
	for i := 0; i < 3; i++ {
		go func() {
			_, ok, _, _ := q.get()
			done <- ok
		}()
	}
	time.Sleep(10 * time.Millisecond)
	q.close()
	for i := 0; i < 3; i++ {
		select {
		case ok := <-done:
			if ok {
				t.Fatal("get on empty closed queue returned an item")
			}
		case <-time.After(time.Second):
			t.Fatal("consumer not woken by close")
		}
	}
}

func TestWorkerPadding(t *testing.T) {
	if s := sizeofWorker(); s%128 != 0 {
		t.Fatalf("worker size %d is not a multiple of 128", s)
	}
}
