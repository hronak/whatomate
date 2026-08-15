package calling

import (
	"sync"
	"testing"
	"time"
)

// AudioPlayer's stop signal and RTP counters were read and reassigned from
// different goroutines: the playback loop advanced the counters while the
// transfer path read Sequence(), and ResetAfterInterrupt swapped the stop
// channel while PlayFile was selecting on it.

func TestAudioPlayer_StopIsIdempotent(t *testing.T) {
	p := NewAudioPlayer(nil)
	p.Stop()
	p.Stop()

	if !p.IsStopped() {
		t.Fatal("player should report stopped")
	}
}

func TestAudioPlayer_ResetAfterInterruptClearsStop(t *testing.T) {
	p := NewAudioPlayer(nil)
	p.Stop()
	if !p.IsStopped() {
		t.Fatal("player should be stopped")
	}

	p.ResetAfterInterrupt()
	if p.IsStopped() {
		t.Fatal("player should be reusable after ResetAfterInterrupt")
	}
}

func TestAudioPlayer_SequenceAccessorsAreRaceFree(t *testing.T) {
	p := NewAudioPlayer(nil)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer: the playback loop reserving frames.
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			p.nextRTP()
		}
	})

	// Readers: the transfer path reading the high-water mark, and the stop
	// signal being swapped underneath.
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				p.Sequence()
				p.IsStopped()
				p.SetSequence(10, 20)
			}
		})
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestAudioPlayer_NextRTPAdvancesByOneFrame(t *testing.T) {
	p := NewAudioPlayer(nil)
	p.SetSequence(100, 1000)

	// SetSequence moves one frame past the given mark so the receiver does not
	// discard the next packet as old.
	seq1, ts1 := p.nextRTP()
	if seq1 != 101 || ts1 != 1000+samplesPerFrame {
		t.Fatalf("first frame = (%d,%d), want (101,%d)", seq1, ts1, 1000+samplesPerFrame)
	}

	seq2, ts2 := p.nextRTP()
	if seq2 != seq1+1 || ts2 != ts1+samplesPerFrame {
		t.Fatalf("second frame = (%d,%d), want (%d,%d)", seq2, ts2, seq1+1, ts1+samplesPerFrame)
	}
}

func TestAudioPlayer_WaitReturnsWhenPlaybackExits(t *testing.T) {
	// Wait replaces a 25ms sleep that merely hoped the playback goroutine had
	// noticed Stop. A caller about to hand this player's track to a bridge
	// needs to know it has actually stopped writing.
	p := NewAudioPlayer(nil)

	running := make(chan struct{})
	p.Go(func() {
		close(running)
		<-p.stopSignal().Done()
	})
	<-running

	p.Stop()

	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after the playback goroutine exited")
	}
}

func TestAudioPlayer_WaitReturnsImmediatelyWhenNeverStarted(t *testing.T) {
	// Players that are constructed but never Go'd (the IVR path plays
	// synchronously) must not deadlock a caller that waits on them.
	p := NewAudioPlayer(nil)

	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked on a player that was never started")
	}
}
