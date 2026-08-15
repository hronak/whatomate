package calling

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newTestSession builds a CallSession with the signals a live session would
// have, without any WebRTC machinery.
func newTestSession() *CallSession {
	return &CallSession{
		ID:             "wacid.test",
		OrganizationID: uuid.New(),
		DTMFBuffer:     make(chan byte, 32),
		BridgeStarted:  newSignal(),
		done:           newSignal(),
	}
}

func TestSession_AccessorsAreRaceFree(t *testing.T) {
	// These fields used to be read directly by consumer goroutines while the
	// transfer paths reassigned them, which -race reports as a data race and
	// which produced send-on-closed panics in production. Every access now goes
	// through a mutex-guarded accessor; this hammers both sides at once so the
	// race detector would catch any that were missed.
	s := newTestSession()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Readers, standing in for the DTMF and RTP consumer goroutines.
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = s.dtmfChan()
				_ = s.bridgeStarted()
				_ = s.transferAccepted()
				_ = s.callID()
				_ = s.doneChan()
			}
		})
	}

	// Writers, standing in for the transfer rotation paths.
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				s.newTransferAccepted()
				s.mu.Lock()
				s.BridgeStarted = newSignal()
				s.mu.Unlock()
				s.fireBridgeStarted()
			}
		})
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestSession_DTMFChanNilAfterTeardown(t *testing.T) {
	// Teardown nils the buffer under the lock rather than closing it: dtmf.go
	// sends into this channel from the WebRTC read loop, and closing under it
	// panicked the process.
	s := newTestSession()
	if s.dtmfChan() == nil {
		t.Fatal("a live session should have a DTMF buffer")
	}

	s.mu.Lock()
	s.DTMFBuffer = nil
	s.mu.Unlock()

	if s.dtmfChan() != nil {
		t.Fatal("dtmfChan should report nil once the session is torn down")
	}
}

func TestSession_SendDTMFAfterTeardownIsDropped(t *testing.T) {
	// The send path must survive teardown. Previously this was a send on a
	// closed channel.
	s := newTestSession()
	s.mu.Lock()
	s.DTMFBuffer = nil
	s.mu.Unlock()

	// Must not panic, and must not block.
	done := make(chan struct{})
	go func() {
		defer close(done)
		sendDTMFDigit(s, '5', nopLogger())
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendDTMFDigit blocked after teardown")
	}
}

func TestSession_DoneChanReleasesConsumers(t *testing.T) {
	// Consumer goroutines learn about teardown from done. Before this existed
	// they learned it from DTMFBuffer being closed, which is what raced.
	s := newTestSession()

	released := make(chan struct{})
	go func() {
		<-s.doneChan()
		close(released)
	}()

	s.done.Fire()

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("consumers were not released by the done signal")
	}
}

func TestDurationSince(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-90 * time.Second)

	if got := durationSince(&earlier, now); got != 90 {
		t.Errorf("durationSince = %d, want 90", got)
	}
	if got := durationSince(nil, now); got != 0 {
		t.Errorf("durationSince(nil) = %d, want 0", got)
	}
}
