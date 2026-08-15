package whatsapp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClient builds a client pointed at srv with a fast retry schedule, so the
// retry tests do not spend real backoff time.
func testClient(t *testing.T, srv *httptest.Server, attempts int) *whatsapp.Client {
	t.Helper()
	return whatsapp.New(
		whatsapp.WithLogger(nopLogger()),
		whatsapp.WithBaseURL(srv.URL),
		whatsapp.WithHTTPClient(srv.Client()),
		whatsapp.WithRetry(attempts, time.Millisecond),
	)
}

func retryTestAccount(baseURL string) *whatsapp.Account {
	return &whatsapp.Account{
		PhoneID:     "phone-1",
		BusinessID:  "waba-1",
		AccessToken: "token",
		APIVersion:  "v21.0",
	}
}

func TestClient_RetriesServerErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.ok"}]}`))
	}))
	defer srv.Close()

	c := testClient(t, srv, 3)
	id, err := c.SendTextMessage(t.Context(), retryTestAccount(srv.URL),
		whatsapp.Recipient{Phone: "15551234567"}, "hi", "")

	require.NoError(t, err, "a 5xx should be retried until it succeeds")
	assert.Equal(t, "wamid.ok", id)
	assert.Equal(t, int32(3), calls.Load(), "should have taken three attempts")
}

func TestClient_DoesNotRetryClientRejections(t *testing.T) {
	// A rejection Meta has reasoned about — an invalid number — fails
	// identically on retry. Retrying only burns quota.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_ = writeMetaError(w, "Invalid phone number", 100)
	}))
	defer srv.Close()

	c := testClient(t, srv, 3)
	_, err := c.SendTextMessage(t.Context(), retryTestAccount(srv.URL),
		whatsapp.Recipient{Phone: "bad"}, "hi", "")

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "a considered rejection must not be retried")
}

func TestClient_RetriesRateLimits(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = writeMetaError(w, "rate limit hit", 130429)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.ok"}]}`))
	}))
	defer srv.Close()

	c := testClient(t, srv, 3)
	_, err := c.SendTextMessage(t.Context(), retryTestAccount(srv.URL),
		whatsapp.Recipient{Phone: "15551234567"}, "hi", "")

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load(), "throttling should be retried")
}

func TestClient_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := testClient(t, srv, 3)
	_, err := c.SendTextMessage(t.Context(), retryTestAccount(srv.URL),
		whatsapp.Recipient{Phone: "15551234567"}, "hi", "")

	require.Error(t, err)
	assert.Equal(t, int32(3), calls.Load(), "should stop at the configured attempt limit")
}

func TestClient_SentinelsMatchMetaCodes(t *testing.T) {
	// These are what callers branch on; a wrong code mapping silently changes
	// retry and re-engagement behaviour across the product.
	for _, tc := range []struct {
		name string
		code int
		want error
	}{
		{"rate limited", 130429, whatsapp.ErrRateLimited},
		{"application limit", 4, whatsapp.ErrRateLimited},
		{"invalid token", 190, whatsapp.ErrInvalidToken},
		{"re-engagement", 131047, whatsapp.ErrReengagementRequired},
		{"template missing", 132001, whatsapp.ErrTemplateNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = writeMetaError(w, tc.name, tc.code)
			}))
			defer srv.Close()

			c := testClient(t, srv, 1)
			_, err := c.SendTextMessage(t.Context(), retryTestAccount(srv.URL),
				whatsapp.Recipient{Phone: "15551234567"}, "hi", "")

			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.want),
				"code %d should map to %v, got %v", tc.code, tc.want, err)

			var apiErr *whatsapp.MetaAPIError
			require.True(t, errors.As(err, &apiErr), "the structured error must be recoverable")
			assert.Equal(t, tc.code, apiErr.Detail.Code)
		})
	}
}

func TestClient_RetryBodyIsResent(t *testing.T) {
	// A retry needs a fresh body reader. If the first attempt consumed it, the
	// retry would send an empty payload — the reason the multipart and form
	// callers used to bypass the retrying path entirely.
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		bodies = append(bodies, string(buf))
		if len(bodies) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.ok"}]}`))
	}))
	defer srv.Close()

	c := testClient(t, srv, 2)
	_, err := c.SendTextMessage(t.Context(), retryTestAccount(srv.URL),
		whatsapp.Recipient{Phone: "15551234567"}, "hello there", "")

	require.NoError(t, err)
	require.Len(t, bodies, 2)
	assert.Equal(t, bodies[0], bodies[1], "the retry must resend the same body, not an empty one")
	assert.Contains(t, bodies[1], "hello there")
}

// writeMetaError writes a Meta-shaped error body.
func writeMetaError(w http.ResponseWriter, message string, code int) error {
	return json.NewEncoder(w).Encode(whatsapp.MetaErrorResponse{
		Error: whatsapp.MetaErrorDetail{Message: message, Code: code},
	})
}
