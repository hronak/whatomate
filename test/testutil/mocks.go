package testutil

import (
	"context"
	"sync"

	"github.com/shridarpatil/whatomate/internal/queue"
)

// MockQueue is a mock implementation of queue.Queue.
type MockQueue struct {
	mu   sync.Mutex
	Jobs []*queue.RecipientJob

	// Configurable behavior
	EnqueueFunc  func(ctx context.Context, job *queue.RecipientJob) error
	EnqueuesFunc func(ctx context.Context, jobs []*queue.RecipientJob) error

	// Error to return
	Error error
}

// NewMockQueue creates a new mock queue.
func NewMockQueue() *MockQueue {
	return &MockQueue{
		Jobs: make([]*queue.RecipientJob, 0),
	}
}

// EnqueueRecipient mocks enqueueing a single job.
func (m *MockQueue) EnqueueRecipient(ctx context.Context, job *queue.RecipientJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Error != nil {
		return m.Error
	}

	m.Jobs = append(m.Jobs, job)

	if m.EnqueueFunc != nil {
		return m.EnqueueFunc(ctx, job)
	}
	return nil
}

// EnqueueRecipients mocks enqueueing multiple jobs.
func (m *MockQueue) EnqueueRecipients(ctx context.Context, jobs []*queue.RecipientJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Error != nil {
		return m.Error
	}

	m.Jobs = append(m.Jobs, jobs...)

	if m.EnqueuesFunc != nil {
		return m.EnqueuesFunc(ctx, jobs)
	}
	return nil
}

// Close is a no-op for the mock.
func (m *MockQueue) Close() error {
	return nil
}

// JobCount returns the number of jobs in the queue.
func (m *MockQueue) JobCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Jobs)
}

// GetJobs returns a copy of all jobs in the queue.
func (m *MockQueue) GetJobs() []*queue.RecipientJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	jobs := make([]*queue.RecipientJob, len(m.Jobs))
	copy(jobs, m.Jobs)
	return jobs
}

// Reset clears all jobs from the queue.
func (m *MockQueue) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Jobs = m.Jobs[:0]
	m.Error = nil
}
