package measure

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Bit layout of the state word of sync.Mutex in go1.27.1
// (internal/sync/mutex.go). The self-test verifies it at run time.
const (
	MutexLocked      = 1 << 0
	MutexWoken       = 1 << 1
	MutexStarving    = 1 << 2
	MutexWaiterShift = 3
)

// MutexState atomically loads the state word of m. It relies on sync.Mutex
// being {state int32; sema uint32} with state at offset 0; SelfTest checks it.
func MutexState(m *sync.Mutex) int32 {
	return atomic.LoadInt32((*int32)(unsafe.Pointer(m)))
}

// StarvingSampler reads a mutex's state at 1 kHz and counts how often the
// starving bit is set.
type StarvingSampler struct {
	m        *sync.Mutex
	stop     chan struct{}
	done     chan struct{}
	samples  atomic.Uint64
	starving atomic.Uint64
}

// StartStarvingSampler starts the sampler goroutine on m.
func StartStarvingSampler(m *sync.Mutex) *StarvingSampler {
	s := &StarvingSampler{m: m, stop: make(chan struct{}), done: make(chan struct{})}
	go s.loop()
	return s
}

func (s *StarvingSampler) loop() {
	defer close(s.done)
	t := time.NewTicker(time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			st := MutexState(s.m)
			s.samples.Add(1)
			if st&MutexStarving != 0 {
				s.starving.Add(1)
			}
		}
	}
}

// Counts returns the samples taken so far and how many saw the starving bit.
func (s *StarvingSampler) Counts() (samples, starving uint64) {
	return s.samples.Load(), s.starving.Load()
}

// Stop stops the sampler and waits for it to exit.
func (s *StarvingSampler) Stop() (samples, starving uint64) {
	close(s.stop)
	<-s.done
	return s.Counts()
}
