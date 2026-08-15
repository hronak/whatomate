package calling

import (
	"testing"
	"time"
)

func TestAudioBridge_StopIsIdempotent(t *testing.T) {
	// Stop used to route through safeClose, which two goroutines could reach
	// at once.
	b := NewAudioBridge(nil, nil)
	b.Stop()
	b.Stop()

	select {
	case <-b.stop.Done():
	default:
		t.Fatal("the bridge stop signal should be fired")
	}
}

func TestAudioBridge_StartWithNoTracksReturnsAfterStop(t *testing.T) {
	// With no tracks, Start reserves the caller slot and blocks. Stop must
	// release it — otherwise the goroutine leaks for the life of the process.
	b := NewAudioBridge(nil, nil)

	returned := make(chan struct{})
	go func() {
		b.Start(nil, nil, nil, nil)
		close(returned)
	}()

	// Let Start reach its wait.
	time.Sleep(20 * time.Millisecond)
	b.Stop()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop; the reserved caller slot leaked")
	}
}

func TestAudioBridge_AttachCallerAfterStopIsNoop(t *testing.T) {
	// Documented contract: once stopped, AttachCaller must not block or panic,
	// because the slot goroutine has already exited.
	b := NewAudioBridge(nil, nil)
	b.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.AttachCaller(nil, nil)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AttachCaller blocked after Stop")
	}
}

func TestAudioBridge_SeedSequenceRecorded(t *testing.T) {
	b := NewAudioBridge(nil, nil)
	b.SeedSequence(5000, 90000)

	if b.seqOffset != 5000 || b.tsOffset != 90000 || !b.firstAgentSeq {
		t.Fatalf("SeedSequence did not record the high-water mark: %d/%d/%v",
			b.seqOffset, b.tsOffset, b.firstAgentSeq)
	}
}
