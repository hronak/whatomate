package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

const (
	// StreamName is the Redis stream for campaign jobs
	StreamName = "whatomate:campaigns"

	// ConsumerGroup is the consumer group name for workers
	ConsumerGroup = "campaign-workers"

	// BlockTimeout is how long to block waiting for new messages
	BlockTimeout = 5 * time.Second

	// ClaimMinIdleTime is the minimum idle time before claiming a pending message
	ClaimMinIdleTime = 5 * time.Minute

	// ClaimInterval is how often stale pending messages are reclaimed. This
	// used to run only at startup, so a message orphaned by a crashed worker
	// waited for the next process restart.
	ClaimInterval = time.Minute

	// MaxDeliveries is how many times a job may be delivered before it is
	// treated as poison and moved to DeadLetterStream. Without this a job that
	// fails deterministically is redelivered forever.
	MaxDeliveries = 5

	// DeadLetterStream holds jobs that exhausted MaxDeliveries, so they can be
	// inspected instead of being silently dropped or retried indefinitely.
	DeadLetterStream = "whatomate:campaigns:dead"

	// errorBackoff is the pause after a stream read error.
	errorBackoff = time.Second

	// jobTimeout bounds one job's execution. The handler's context is detached
	// from shutdown cancellation so an in-flight send completes, and this keeps
	// that from becoming an unbounded wait.
	jobTimeout = 2 * time.Minute

	// ackTimeout bounds the bookkeeping calls (ACK, dead-letter) that must
	// still succeed while the consumer's own context is being cancelled.
	ackTimeout = 10 * time.Second
)

// RedisQueue implements the Queue interface using Redis Streams
type RedisQueue struct {
	client *redis.Client
	log    logf.Logger
}

// NewRedisQueue creates a new Redis queue
func NewRedisQueue(client *redis.Client, log logf.Logger) *RedisQueue {
	return &RedisQueue{
		client: client,
		log:    log,
	}
}

// EnqueueRecipient adds a single recipient job to the queue
func (q *RedisQueue) EnqueueRecipient(ctx context.Context, job *RecipientJob) error {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now()
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal recipient job: %w", err)
	}

	_, err = q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamName,
		Values: map[string]any{
			"type":    string(JobTypeRecipient),
			"payload": string(payload),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to enqueue recipient job: %w", err)
	}

	return nil
}

// EnqueueRecipients adds multiple recipient jobs to the queue using pipeline
func (q *RedisQueue) EnqueueRecipients(ctx context.Context, jobs []*RecipientJob) error {
	if len(jobs) == 0 {
		return nil
	}

	pipe := q.client.Pipeline()
	now := time.Now()

	for _, job := range jobs {
		if job.EnqueuedAt.IsZero() {
			job.EnqueuedAt = now
		}

		payload, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("failed to marshal recipient job: %w", err)
		}

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: StreamName,
			Values: map[string]any{
				"type":    string(JobTypeRecipient),
				"payload": string(payload),
			},
		})
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to enqueue recipient jobs: %w", err)
	}

	q.log.Info("Recipient jobs enqueued", "count", len(jobs), "campaign_id", jobs[0].CampaignID)
	return nil
}

// Close closes the queue connection
func (q *RedisQueue) Close() error {
	return nil // Redis client is managed externally
}

// RedisConsumer implements the Consumer interface using Redis Streams
type RedisConsumer struct {
	client     *redis.Client
	log        logf.Logger
	consumerID string
}

// NewRedisConsumer creates a new Redis consumer
func NewRedisConsumer(client *redis.Client, log logf.Logger) (*RedisConsumer, error) {
	// Generate unique consumer ID
	hostname, _ := os.Hostname()
	consumerID := fmt.Sprintf("worker-%s-%d", hostname, os.Getpid())

	consumer := &RedisConsumer{
		client:     client,
		log:        log,
		consumerID: consumerID,
	}

	// Create consumer group if it doesn't exist
	ctx := context.Background()
	err := client.XGroupCreateMkStream(ctx, StreamName, ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	log.Info("Redis consumer initialized", "consumer_id", consumerID)
	return consumer, nil
}

// Consume starts consuming jobs from the queue. It returns when ctx is
// cancelled, after the job currently being handled has finished.
func (c *RedisConsumer) Consume(ctx context.Context, handler JobHandler) error {
	c.log.Info("Starting to consume jobs", "consumer_id", c.consumerID)

	// Reclaim stale pending messages from crashed workers, then keep doing so
	// on a ticker rather than only at startup.
	if err := c.claimPendingMessages(ctx, handler); err != nil {
		c.log.Warn("Failed to claim pending messages", "error", err)
	}
	claimTicker := time.NewTicker(ClaimInterval)
	defer claimTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Info("Consumer shutting down")
			return ctx.Err()
		case <-claimTicker.C:
			if err := c.claimPendingMessages(ctx, handler); err != nil {
				c.log.Warn("Failed to claim pending messages", "error", err)
			}
			continue
		default:
		}

		// Read new messages from the stream
		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: c.consumerID,
			Streams:  []string{StreamName, ">"},
			Count:    1,
			Block:    BlockTimeout,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) {
				// No messages available, continue waiting
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Error("Failed to read from stream", "error", err)
			// Context-aware backoff: a bare Sleep here delayed shutdown.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(errorBackoff):
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.handleDelivery(ctx, msg, handler, 1)
			}
		}
	}
}

// handleDelivery runs one job and decides its fate: acknowledge on success or
// permanent failure, dead-letter once deliveries are exhausted, otherwise leave
// it pending for redelivery. See ErrPermanent for the handler contract.
func (c *RedisConsumer) handleDelivery(ctx context.Context, msg redis.XMessage, handler JobHandler, attempt int) {
	// The handler runs with a context detached from cancellation so an
	// in-flight job completes during shutdown instead of being abandoned
	// mid-send, but still bounded so it cannot hang shutdown forever.
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobTimeout)
	defer cancel()

	err := c.processMessage(WithAttempt(jobCtx, attempt), msg, handler)
	switch {
	case err == nil:
		c.ack(ctx, msg.ID)

	case errors.Is(err, ErrPermanent):
		c.log.Error("Permanent job failure, not retrying",
			"error", err, "message_id", msg.ID, "attempt", attempt)
		c.ack(ctx, msg.ID)

	case attempt >= MaxDeliveries:
		c.log.Error("Job exhausted retries, moving to dead-letter stream",
			"error", err, "message_id", msg.ID, "attempts", attempt)
		c.deadLetter(ctx, msg, err)
		c.ack(ctx, msg.ID)

	default:
		// Leave unacknowledged; claimPendingMessages redelivers it.
		c.log.Warn("Job failed, will retry",
			"error", err, "message_id", msg.ID, "attempt", attempt, "max", MaxDeliveries)
	}
}

// ack acknowledges a message, removing it from the pending entries list.
func (c *RedisConsumer) ack(ctx context.Context, msgID string) {
	// Use a detached context: during shutdown ctx is already cancelled, and
	// failing to ACK a completed job means doing its work twice.
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ackTimeout)
	defer cancel()
	if err := c.client.XAck(ackCtx, StreamName, ConsumerGroup, msgID).Err(); err != nil {
		c.log.Error("Failed to ACK message", "error", err, "message_id", msgID)
	}
}

// deadLetter copies a poison message to the dead-letter stream with the reason
// it failed, so it is inspectable rather than silently dropped.
func (c *RedisConsumer) deadLetter(ctx context.Context, msg redis.XMessage, cause error) {
	dlCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ackTimeout)
	defer cancel()

	values := make(map[string]any, len(msg.Values)+3)
	maps.Copy(values, msg.Values)
	values["original_id"] = msg.ID
	values["error"] = cause.Error()
	values["failed_at"] = time.Now().UTC().Format(time.RFC3339)

	if err := c.client.XAdd(dlCtx, &redis.XAddArgs{
		Stream: DeadLetterStream,
		Values: values,
	}).Err(); err != nil {
		c.log.Error("Failed to write dead-letter entry", "error", err, "message_id", msg.ID)
	}
}

// claimPendingMessages claims stale pending messages from crashed workers
func (c *RedisConsumer) claimPendingMessages(ctx context.Context, handler JobHandler) error {
	// Get pending messages that have been idle for too long
	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: StreamName,
		Group:  ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  100,
		Idle:   ClaimMinIdleTime,
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to get pending messages: %w", err)
	}

	if len(pending) == 0 {
		return nil
	}

	c.log.Info("Found stale pending messages to claim", "count", len(pending))

	// Claim and process each pending message
	for _, p := range pending {
		// Claim the message
		messages, err := c.client.XClaim(ctx, &redis.XClaimArgs{
			Stream:   StreamName,
			Group:    ConsumerGroup,
			Consumer: c.consumerID,
			MinIdle:  ClaimMinIdleTime,
			Messages: []string{p.ID},
		}).Result()

		if err != nil {
			c.log.Error("Failed to claim message", "error", err, "message_id", p.ID)
			continue
		}

		for _, msg := range messages {
			// RetryCount is Redis's own delivery counter for this entry, so a
			// poison message is recognised across worker restarts.
			c.handleDelivery(ctx, msg, handler, int(p.RetryCount))
		}
	}

	return nil
}

// processMessage processes a single message from the stream
func (c *RedisConsumer) processMessage(ctx context.Context, msg redis.XMessage, handler JobHandler) error {
	jobType, ok := msg.Values["type"].(string)
	if !ok {
		return fmt.Errorf("invalid message: missing type")
	}

	payload, ok := msg.Values["payload"].(string)
	if !ok {
		return fmt.Errorf("invalid message: missing payload")
	}

	switch JobType(jobType) {
	case JobTypeRecipient:
		var job RecipientJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			return fmt.Errorf("failed to unmarshal recipient job: %w", err)
		}
		c.log.Debug("Processing recipient job", "campaign_id", job.CampaignID, "recipient_id", job.RecipientID, "message_id", msg.ID)
		return handler.HandleRecipientJob(ctx, &job)

	default:
		return fmt.Errorf("unknown job type: %s", jobType)
	}
}

// Close closes the consumer connection
func (c *RedisConsumer) Close() error {
	return nil // Redis client is managed externally
}
