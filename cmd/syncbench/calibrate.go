package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
	"github.com/muhammetsafak/go-sync-bench/internal/scenario"
)

// CalibDist is one distribution of the park-threshold calibration.
type CalibDist struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Samples     uint64             `json:"samples"`
	P50         *int64             `json:"p50_ns"`
	P90         *int64             `json:"p90_ns"`
	P99         *int64             `json:"p99_ns"`
	Max         *int64             `json:"max_ns"`
	ShareOverUs map[string]float64 `json:"share_over_us"`
	Correct     *bool              `json:"correct,omitempty"`
}

// CalibReport is the output of the calibrate subcommand.
type CalibReport struct {
	GoVersion  string            `json:"go_version"`
	GOMAXPROCS int               `json:"gomaxprocs"`
	NumCPU     int               `json:"num_cpu"`
	Clock      measure.ClockInfo `json:"clock"`
	WindowS    float64           `json:"window_s"`
	Dists      []CalibDist       `json:"distributions"`
}

func distFrom(name, desc string, h *measure.Histogram) CalibDist {
	d := CalibDist{Name: name, Description: desc, Samples: h.N, ShareOverUs: map[string]float64{}}
	if h.N == 0 {
		return d
	}
	d.P50, d.P90, d.P99, d.Max = i64(h.Quantile(0.5)), i64(h.Quantile(0.9)), i64(h.Quantile(0.99)), i64(h.Max)
	return d
}

// shareFromThresholds uses the exact per-threshold counters of a scenario run.
func shareFromThresholds(d *CalibDist, over []uint64, samples uint64) {
	if samples == 0 {
		return
	}
	for i, th := range scenario.ThresholdsNs {
		d.ShareOverUs[fmt.Sprint(th/1000)] = float64(over[i]) / float64(samples)
	}
}

// cmdCalibrate measures the distributions that decide whether 10 µs separates
// "the op parked" from "the op did not park": an uncontended mutex (never
// parks), a two-goroutine contended mutex, a client/owner request-response
// pair and a one-way channel hand-off to a parked receiver (always parks).
// It is run in the P = 1 and P = 2 containers.
func cmdCalibrate(args []string) int {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	window := fs.Duration("window", time.Second, "window of each distribution")
	out := fs.String("out", "", "output file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep := CalibReport{
		GoVersion: runtime.Version(), GOMAXPROCS: runtime.GOMAXPROCS(0), NumCPU: runtime.NumCPU(),
		Clock: measure.MeasureClock(), WindowS: window.Seconds(),
	}
	runs := []struct {
		name, desc string
		spec       scenario.Spec
	}{
		{"mutex_g1", "S1 mutex, G=1: uncontended Lock (never parks)",
			scenario.Spec{Scenario: scenario.S1, Impl: "mutex", G: 1, Seed: 1}},
		{"mutex_g2", "S1 mutex, G=2: contended Lock",
			scenario.Spec{Scenario: scenario.S1, Impl: "mutex", G: 2, Seed: 2}},
		{"reqresp_g1", "S1 chan_reqresp, G=1: request send to the owner (client/owner ping-pong)",
			scenario.Spec{Scenario: scenario.S1, Impl: "chan_reqresp", G: 1, Seed: 3}},
	}
	code := 0
	for _, r := range runs {
		res, err := scenario.Run(r.spec, scenario.Options{Window: *window, Latency: true})
		if err != nil {
			fmt.Fprintln(os.Stderr, "syncbench calibrate:", err)
			return 1
		}
		d := distFrom(r.name, r.desc, &res.Acquire)
		shareFromThresholds(&d, res.AcquireOver[:], res.Samples)
		ok := res.Correct && res.BarrierOK && res.StuckStep == ""
		d.Correct = &ok
		if !ok {
			code = 1
		}
		rep.Dists = append(rep.Dists, d)
	}
	rep.Dists = append(rep.Dists, handoff(*window))
	if err := writeJSON(*out, rep); err != nil {
		fmt.Fprintln(os.Stderr, "syncbench calibrate:", err)
		return 1
	}
	return code
}

// handoff measures the one-way latency of an unbuffered channel send to a
// receiver that is parked on the receive: the sender stamps the time, sends
// it, and the receiver subtracts it from its own clock read. The receiver
// then waits for an acknowledgement-free pause, so every hand-off finds it
// parked again.
func handoff(window time.Duration) CalibDist {
	ch := make(chan int64)
	ready := make(chan struct{})
	var h measure.Histogram
	var over [len(scenario.ThresholdsNs)]uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for t0 := range ch {
			d := measure.Now() - t0
			h.Record(d)
			for j, th := range scenario.ThresholdsNs {
				if d > th {
					over[j]++
				}
			}
			ready <- struct{}{}
		}
	}()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		// Give the receiver time to go back to its receive and park.
		spinUntil(measure.Now() + 20_000)
		ch <- measure.Now()
		<-ready
	}
	close(ch)
	<-done
	d := distFrom("handoff_parked", "unbuffered send to a parked receiver: send -> receive (always parks)", &h)
	shareFromThresholds(&d, over[:], h.N)
	return d
}

func spinUntil(t int64) {
	for measure.Now() < t {
	}
}
