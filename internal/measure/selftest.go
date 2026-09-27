package measure

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// Self-test limits from the design.
const (
	SelfTestValidTrials   = 5
	SelfTestMaxSetupRetry = 20
	selfTestHold          = 5 * time.Millisecond
	selfTestPollLimit     = time.Second
)

// SelfTestResult is the outcome of the starving-bit self-test.
type SelfTestResult struct {
	GoVersion       string         `json:"go_version"`
	GOMAXPROCS      int            `json:"gomaxprocs"`
	LayoutOK        bool           `json:"layout_ok"`
	LayoutDetail    string         `json:"layout_detail"`
	ValidTrials     int            `json:"valid_trials"`
	PassedTrials    int            `json:"passed_trials"`
	SetupRetries    int            `json:"setup_retries"`
	SetupInvalid    map[string]int `json:"setup_invalid"`
	SetupUnstable   bool           `json:"setup_unstable"`
	Failures        []string       `json:"failures"`
	SamplerSeenMs   []float64      `json:"sampler_seen_ms"`
	WokenClearUs    []float64      `json:"woken_clear_us"`
	DurationS       float64        `json:"duration_s"`
	Pass            bool           `json:"pass"`
	Result          string         `json:"result"` // pass | fail | setup_unstable
	TrialsAttempted int            `json:"trials_attempted"`
}

type trialOutcome int

const (
	trialPass trialOutcome = iota
	trialFail
	trialSetupTryLockFalse
	trialSetupWokenTimeout
	trialSetupWaiters
)

// SelfTest runs the starving-bit self-test of the design, steps (a)–(d).
// It must run with GOMAXPROCS = 2.
func SelfTest() SelfTestResult {
	t0 := time.Now()
	r := SelfTestResult{
		GoVersion:    runtime.Version(),
		GOMAXPROCS:   runtime.GOMAXPROCS(0),
		SetupInvalid: map[string]int{},
		Failures:     []string{},
	}
	r.LayoutOK, r.LayoutDetail = checkLayout()
	if !r.LayoutOK {
		r.Failures = append(r.Failures, "layout: "+r.LayoutDetail)
	}
	for r.LayoutOK && r.ValidTrials < SelfTestValidTrials {
		if r.SetupRetries > SelfTestMaxSetupRetry {
			r.SetupUnstable = true
			break
		}
		r.TrialsAttempted++
		out, detail, seenMs, clearUs := runTrial()
		switch out {
		case trialPass:
			r.ValidTrials++
			r.PassedTrials++
			r.SamplerSeenMs = append(r.SamplerSeenMs, seenMs)
			r.WokenClearUs = append(r.WokenClearUs, clearUs)
		case trialFail:
			r.ValidTrials++
			r.Failures = append(r.Failures, fmt.Sprintf("trial %d: %s", r.TrialsAttempted, detail))
		case trialSetupTryLockFalse:
			r.SetupRetries++
			r.SetupInvalid["trylock_false"]++
		case trialSetupWokenTimeout:
			r.SetupRetries++
			r.SetupInvalid["woken_timeout"]++
		case trialSetupWaiters:
			r.SetupRetries++
			r.SetupInvalid["waiters_timeout"]++
		}
	}
	r.DurationS = time.Since(t0).Seconds()
	switch {
	case !r.LayoutOK:
		r.Result = "fail"
	case r.SetupUnstable:
		r.Result = "setup_unstable"
	case r.PassedTrials == SelfTestValidTrials && r.ValidTrials == SelfTestValidTrials:
		r.Result = "pass"
		r.Pass = true
	default:
		r.Result = "fail"
	}
	return r
}

// checkLayout is step (a): size and the locked bit around Lock/Unlock.
func checkLayout() (bool, string) {
	var m sync.Mutex
	if sz := unsafe.Sizeof(m); sz != 8 {
		return false, fmt.Sprintf("unsafe.Sizeof(sync.Mutex{}) = %d, want 8", sz)
	}
	before := MutexState(&m)
	m.Lock()
	during := MutexState(&m)
	m.Unlock()
	after := MutexState(&m)
	if before&MutexLocked != 0 || during&MutexLocked == 0 || after&MutexLocked != 0 {
		return false, fmt.Sprintf("locked bit before/during/after = %d/%d/%d, want 0/1/0",
			before&MutexLocked, during&MutexLocked, after&MutexLocked)
	}
	return true, "sizeof=8, locked bit 0/1/0"
}

// pollUntil polls cond until it holds or limit elapses.
func pollUntil(limit time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(limit)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		runtime.Gosched()
	}
	return true
}

// runTrial is steps (b)–(d) of one trial. The calling goroutine is H.
func runTrial() (out trialOutcome, detail string, seenMs, clearUs float64) {
	var m sync.Mutex
	var wg sync.WaitGroup
	waiter := func() {
		defer wg.Done()
		m.Lock()
		m.Unlock()
	}
	waiters := func() int32 { return MutexState(&m) >> MutexWaiterShift }

	// finish lets every goroutine run to the end and checks step (d).
	finish := func() (bool, string) {
		wg.Wait()
		if st := MutexState(&m); st != 0 {
			return false, fmt.Sprintf("state after all goroutines = %#x, want 0", st)
		}
		return true, ""
	}

	// (b) H holds, W1 then W2 park on the mutex.
	m.Lock()
	wg.Add(1)
	go waiter()
	if !pollUntil(selfTestPollLimit, func() bool { return waiters() == 1 }) {
		m.Unlock()
		finish()
		return trialSetupWaiters, "W1 not parked within 1s", 0, 0
	}
	wg.Add(1)
	go waiter()
	if !pollUntil(selfTestPollLimit, func() bool { return waiters() == 2 }) {
		m.Unlock()
		finish()
		return trialSetupWaiters, "W2 not parked within 1s", 0, 0
	}

	// (c) Hold past the 1 ms starvation threshold, release and barge.
	time.Sleep(selfTestHold)
	m.Unlock()
	if !m.TryLock() {
		if ok, d := finish(); !ok {
			return trialFail, "after TryLock=false: " + d, 0, 0
		}
		return trialSetupTryLockFalse, "", 0, 0
	}
	tc := time.Now()
	if !pollUntil(selfTestPollLimit, func() bool { return MutexState(&m)&MutexWoken == 0 }) {
		m.Unlock()
		if ok, d := finish(); !ok {
			return trialFail, "after woken timeout: " + d, 0, 0
		}
		return trialSetupWokenTimeout, "", 0, 0
	}
	clearUs = float64(time.Since(tc).Nanoseconds()) / 1e3
	st := MutexState(&m)
	if st&MutexStarving == 0 {
		m.Unlock()
		finish()
		return trialFail, fmt.Sprintf("woken cleared but starving bit not set (state %#x)", st), 0, clearUs
	}

	// The production sampler must see the bit while H still holds the lock.
	s := StartStarvingSampler(&m)
	ts := time.Now()
	seen := pollUntil(selfTestPollLimit, func() bool {
		_, n := s.Counts()
		return n > 0
	})
	seenMs = float64(time.Since(ts).Nanoseconds()) / 1e6
	s.Stop()

	// (d) Negative control.
	m.Unlock()
	ok, d := finish()
	switch {
	case !seen:
		return trialFail, "1 kHz sampler did not see the starving bit within 1s", seenMs, clearUs
	case !ok:
		return trialFail, d, seenMs, clearUs
	}
	return trialPass, "", seenMs, clearUs
}
