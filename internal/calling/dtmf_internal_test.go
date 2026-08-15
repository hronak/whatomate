package calling

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestDecodeDTMFEvent_EmitsOnceOnEndBit(t *testing.T) {
	// RFC 4733 repeats a telephone-event packet many times and marks the last
	// with the end bit. Emitting on anything else would type each digit
	// several times.
	var lastEvent byte = 0xFF
	var lastEnd bool

	// Repeats without the end bit produce nothing.
	for range 5 {
		if _, ok := decodeDTMFEvent(5, false, &lastEvent, &lastEnd); ok {
			t.Fatal("a non-end packet must not emit a digit")
		}
	}

	digit, ok := decodeDTMFEvent(5, true, &lastEvent, &lastEnd)
	if !ok || digit != '5' {
		t.Fatalf("end packet = (%q,%v), want ('5',true)", digit, ok)
	}

	// Trailing end-bit repeats of the same event must be debounced.
	if _, ok := decodeDTMFEvent(5, true, &lastEvent, &lastEnd); ok {
		t.Fatal("a repeated end packet for the same event must not emit again")
	}
}

func TestDecodeDTMFEvent_DistinctDigitsInSequence(t *testing.T) {
	var lastEvent byte = 0xFF
	var lastEnd bool

	for _, tc := range []struct {
		event byte
		want  byte
	}{{1, '1'}, {2, '2'}, {10, '*'}, {11, '#'}, {0, '0'}} {
		digit, ok := decodeDTMFEvent(tc.event, true, &lastEvent, &lastEnd)
		if !ok || digit != tc.want {
			t.Fatalf("event %d = (%q,%v), want (%q,true)", tc.event, digit, ok, tc.want)
		}
	}
}

func TestDecodeDTMFEvent_UnknownEventIsIgnored(t *testing.T) {
	var lastEvent byte = 0xFF
	var lastEnd bool

	// Events 12-15 are A-D, which this product does not map.
	if _, ok := decodeDTMFEvent(99, true, &lastEvent, &lastEnd); ok {
		t.Fatal("an unmapped event must not emit a digit")
	}
}

// timeoutError is a net.Error reporting a timeout, like the one pion returns
// when a read deadline expires.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsTimeout(t *testing.T) {
	// The RTP consumers must treat a read-deadline expiry as "loop again" and
	// anything else as "the stream is finished". Getting this backwards either
	// spins forever or drops the call's audio.
	var netErr net.Error = timeoutError{}
	if !isTimeout(netErr) {
		t.Error("a net.Error with Timeout() true must be treated as a timeout")
	}
	if !isTimeout(errors.Join(errors.New("wrapped"), netErr)) {
		t.Error("a wrapped timeout must still be detected")
	}
	if isTimeout(errors.New("connection reset")) {
		t.Error("a plain error must not be treated as a timeout")
	}
	if isTimeout(nil) {
		t.Error("nil must not be treated as a timeout")
	}
}

func TestRTPReadDeadlineIsBounded(t *testing.T) {
	// A half-open connection never errors, so without a deadline the consumer
	// goroutines outlive the call. Guard the constant against being zeroed.
	if rtpReadDeadline <= 0 || rtpReadDeadline > time.Minute {
		t.Fatalf("rtpReadDeadline = %v; must be positive and short enough to notice a dead peer", rtpReadDeadline)
	}
}
