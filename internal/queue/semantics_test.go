package queue_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/shridarpatil/whatomate/internal/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The retry convention used to be inverted: a permanently-missing campaign
// returned a plain error and was redelivered every five minutes forever, while
// a transient send failure returned nil and was never retried at all. These
// tests pin the convention that replaced it.

func TestPermanent_WrapsAndIsDetectable(t *testing.T) {
	cause := errors.New("campaign 123 not found")
	err := queue.Permanent(cause)

	require.Error(t, err)
	assert.True(t, errors.Is(err, queue.ErrPermanent),
		"a permanent failure must be recognisable, or the job is redelivered forever")
	assert.True(t, errors.Is(err, cause), "the cause must stay in the chain")
	assert.Contains(t, err.Error(), "campaign 123 not found")
}

func TestPermanent_NilStaysNil(t *testing.T) {
	// Wrapping a nil error must not manufacture a failure.
	assert.NoError(t, queue.Permanent(nil))
}

func TestPermanent_PlainErrorIsNotPermanent(t *testing.T) {
	// The default must be "retry": a transient failure that is mistaken for
	// permanent is a silently dropped message.
	assert.False(t, errors.Is(errors.New("connection refused"), queue.ErrPermanent))
	assert.False(t, errors.Is(fmt.Errorf("wrapped: %w", errors.New("timeout")), queue.ErrPermanent))
}

func TestAttemptFromContext(t *testing.T) {
	// Handlers use the attempt number to decide when to stop retrying and
	// record a final failure, so an unset context must not read as attempt 0.
	assert.Equal(t, 1, queue.AttemptFromContext(context.Background()),
		"an unmarked context must read as the first attempt")

	ctx := queue.WithAttempt(context.Background(), 4)
	assert.Equal(t, 4, queue.AttemptFromContext(ctx))

	assert.Equal(t, 1, queue.AttemptFromContext(queue.WithAttempt(context.Background(), 0)),
		"a nonsensical attempt count must fall back to 1")
}

func TestMaxDeliveriesIsBounded(t *testing.T) {
	// Guard the poison-message backstop against being disabled: without a
	// ceiling a deterministically-failing job is redelivered indefinitely.
	assert.Greater(t, queue.MaxDeliveries, 1, "retrying must actually happen")
	assert.Less(t, queue.MaxDeliveries, 100, "there must be a real ceiling")
	assert.NotEmpty(t, queue.DeadLetterStream, "exhausted jobs need somewhere to go")
}
