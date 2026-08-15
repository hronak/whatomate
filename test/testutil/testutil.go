// Package testutil provides shared test utilities for the whatomate project.
package testutil

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

var (
	testRedis        *redis.Client
	testRedisOnce    sync.Once
	testRedisInitErr error
)

// TestContextWithTimeout returns a context with a custom timeout, cancelled
// when the test ends.
//
// Prefer t.Context() where no deadline is needed — it is the standard-library
// equivalent and this package's plain TestContext was deleted in its favour.
// This variant remains because t.Context() carries no timeout.
func TestContextWithTimeout(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// NewTestUUID generates a deterministic UUID for testing based on a seed string.
// This is useful for creating reproducible test data.
func NewTestUUID(seed string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))
}

// RandomUUID generates a new random UUID for testing.
func RandomUUID() uuid.UUID {
	return uuid.New()
}

// MustParseUUID parses a UUID string or fails the test.
func MustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err, "failed to parse UUID: %s", s)
	return id
}

// NopLogger returns a no-op logger for tests that don't need log output.
func NopLogger() logf.Logger {
	return logf.New(logf.Opts{
		Level:        logf.ErrorLevel, // Only log errors
		EnableCaller: false,
		EnableColor:  false,
	})
}

// TestLogger returns a logger suitable for test output.
func TestLogger() logf.Logger {
	return logf.New(logf.Opts{
		Level:        logf.DebugLevel,
		EnableCaller: true,
		EnableColor:  false,
	})
}

// StringPtr returns a pointer to the given string.
//
//go:fix inline
func StringPtr(s string) *string {
	return new(s)
}

// IntPtr returns a pointer to the given int.
//
//go:fix inline
func IntPtr(i int) *int {
	return new(i)
}

// TimePtr returns a pointer to the given time.
//
//go:fix inline
func TimePtr(t time.Time) *time.Time {
	return new(t)
}

// UUIDPtr returns a pointer to the given UUID.
//
//go:fix inline
func UUIDPtr(id uuid.UUID) *uuid.UUID {
	return new(id)
}

// BoolPtr returns a pointer to the given bool.
//
//go:fix inline
func BoolPtr(b bool) *bool {
	return new(b)
}

// AssertEventually retries an assertion function until it passes or times out.
// Useful for testing async operations.
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}

// RequireTestRedis returns a test Redis client, skipping the test when Redis
// is unavailable.
//
// Prefer this to SetupTestRedis. SetupTestRedis returns nil so a caller can
// treat Redis as optional, which means a test that genuinely needs it either
// panics on the nil or — worse — quietly asserts nothing.
func RequireTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := SetupTestRedis(t)
	if rdb == nil {
		t.Skip("TEST_REDIS_URL not set, skipping Redis test")
	}
	return rdb
}

// SetupTestRedis creates a connection to a test Redis instance.
// Requires TEST_REDIS_URL environment variable to be set.
// Returns nil when unavailable; use RequireTestRedis unless the test really
// can run without Redis.
func SetupTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		return nil
	}

	testRedisOnce.Do(func() {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			testRedisInitErr = err
			return
		}

		testRedis = redis.NewClient(opts)

		// Verify connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := testRedis.Ping(ctx).Err(); err != nil {
			testRedisInitErr = err
			return
		}
	})

	if testRedisInitErr != nil {
		t.Logf("Warning: failed to connect to test Redis: %v", testRedisInitErr)
		return nil
	}

	return testRedis
}
