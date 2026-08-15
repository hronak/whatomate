package calling

import (
	"sync"
	"testing"
	"time"
)

// The signal type replaced a check-then-act safeClose that could be reached by
// two goroutines at once, each seeing an open channel and both closing it —
// a panic that took the whole process with it. These tests pin the properties
// that fix depends on.

func TestSignal_FireIsIdempotent(t *testing.T) {
	s := newSignal()
	s.Fire()
	s.Fire()
	s.Fire()

	select {
	case <-s.Done():
	default:
		t.Fatal("Done should be closed after Fire")
	}
}

func TestSignal_ConcurrentFireDoesNotPanic(t *testing.T) {
	// The original race, reproduced deliberately: many goroutines racing to
	// close the same signal. With safeClose this panicked with
	// "close of closed channel"; sync.Once makes it a no-op.
	s := newSignal()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 64 {
		wg.Go(func() {
			<-start
			s.Fire()
		})
	}
	close(start)
	wg.Wait()

	select {
	case <-s.Done():
	default:
		t.Fatal("Done should be closed")
	}
}

func TestSignal_NilIsSafe(t *testing.T) {
	// A nil signal must behave like the nil channel field it replaced: Fire is
	// a no-op and Done blocks forever, so a select arm on it simply never
	// fires rather than panicking.
	var s *signal
	s.Fire() // must not panic

	select {
	case <-s.Done():
		t.Fatal("a nil signal must never be ready")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestSignal_DoneUnblocksWaiters(t *testing.T) {
	s := newSignal()

	const waiters = 8
	released := make(chan struct{}, waiters)
	for range waiters {
		go func() {
			<-s.Done()
			released <- struct{}{}
		}()
	}

	s.Fire()

	for i := range waiters {
		select {
		case <-released:
		case <-time.After(2 * time.Second):
			t.Fatalf("waiter %d was not released by Fire", i)
		}
	}
}

func TestSignal_SupersededInstanceStillReleasesItsWaiters(t *testing.T) {
	// Transfer rotation installs a fresh signal per attempt. A goroutine parked
	// on the superseded instance must still be released by that instance's own
	// Fire — otherwise it leaks for the life of the call.
	old := newSignal()

	parked := make(chan struct{})
	go func() {
		<-old.Done()
		close(parked)
	}()

	_ = newSignal() // supersede
	old.Fire()

	select {
	case <-parked:
	case <-time.After(2 * time.Second):
		t.Fatal("a goroutine on the superseded signal was never released")
	}
}
