package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
)

// JobType represents the type of job
type JobType string

const (
	// JobTypeRecipient is for processing a single recipient message
	JobTypeRecipient JobType = "recipient"
)

// RecipientJob represents a single recipient message job
type RecipientJob struct {
	CampaignID     uuid.UUID    `json:"campaign_id"`
	RecipientID    uuid.UUID    `json:"recipient_id"`
	OrganizationID uuid.UUID    `json:"organization_id"`
	PhoneNumber    string       `json:"phone_number"`
	RecipientName  string       `json:"recipient_name"`
	TemplateParams models.JSONB `json:"template_params"`
	// HeaderParams holds the TEXT-header variable's value (Meta restricts
	// these headers to one variable). Sent separately from TemplateParams to
	// avoid positional-key collisions between header and body.
	HeaderParams models.JSONB `json:"header_params"`
	EnqueuedAt   time.Time    `json:"enqueued_at"`
}

// Queue defines the interface for job queue operations
type Queue interface {
	// EnqueueRecipient adds a single recipient job to the queue
	EnqueueRecipient(ctx context.Context, job *RecipientJob) error

	// EnqueueRecipients adds multiple recipient jobs to the queue
	EnqueueRecipients(ctx context.Context, jobs []*RecipientJob) error

	// Close closes the queue connection
	Close() error
}

// ErrPermanent marks a failure that must not be retried.
//
// The retry convention was previously inverted: a permanently-missing campaign
// returned a plain error and was redelivered every 5 minutes forever, while a
// transient send failure returned nil and was never retried at all. The rules
// are now explicit — a handler returns:
//
//   - nil for a job it has fully dealt with (including one it deliberately
//     marked failed). The job is acknowledged.
//   - an error wrapping ErrPermanent when the job can never succeed. The job is
//     acknowledged so it stops coming back, and the reason is logged.
//   - any other error when the failure may be transient. The job is left
//     unacknowledged and redelivered, up to MaxDeliveries attempts, after which
//     it is moved to the dead-letter stream.
var ErrPermanent = errors.New("permanent job failure")

// Permanent wraps err to mark it as non-retryable. Returns nil for a nil err.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrPermanent, err)
}

// JobHandler handles different job types.
//
// See ErrPermanent for what a handler's return value means for redelivery.
// Handlers can read the current delivery attempt with AttemptFromContext.
type JobHandler interface {
	HandleRecipientJob(ctx context.Context, job *RecipientJob) error
}

// attemptKey types the context key holding the delivery attempt.
type attemptKey struct{}

// WithAttempt returns ctx carrying the delivery attempt number for a job.
func WithAttempt(ctx context.Context, attempt int) context.Context {
	return context.WithValue(ctx, attemptKey{}, attempt)
}

// AttemptFromContext returns the delivery attempt for the job being handled,
// starting at 1. It reports 1 when no attempt was recorded, so a handler can
// use it unconditionally.
func AttemptFromContext(ctx context.Context) int {
	if n, ok := ctx.Value(attemptKey{}).(int); ok && n > 0 {
		return n
	}
	return 1
}

// Consumer defines the interface for consuming jobs from the queue
type Consumer interface {
	// Consume starts consuming jobs from the queue
	// Returns when context is cancelled
	Consume(ctx context.Context, handler JobHandler) error

	// Close closes the consumer connection
	Close() error
}
