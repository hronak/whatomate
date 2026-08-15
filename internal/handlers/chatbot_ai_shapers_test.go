package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingTransport intercepts an outgoing request without hitting the
// network, capturing the request body for assertions and returning a canned
// success response shaped like the provider it's pretending to be.
type capturingTransport struct {
	lastBody []byte
	response string
}

func (t *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		t.lastBody = body
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(t.response)),
		Header:     make(http.Header),
	}, nil
}

func TestGenerateOpenAIResponse_PayloadShaping(t *testing.T) {
	tests := []struct {
		name            string
		model           string
		temperature     float64
		wantTokenKey    string
		wantTemperature bool
	}{
		{
			name:            "reasoning model uses max_completion_tokens and drops temperature",
			model:           "gpt-5.6-terra",
			temperature:     0.7,
			wantTokenKey:    "max_completion_tokens",
			wantTemperature: false,
		},
		{
			name:            "legacy model keeps max_tokens and temperature",
			model:           "gpt-4o",
			temperature:     0.5,
			wantTokenKey:    "max_tokens",
			wantTemperature: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &capturingTransport{
				response: `{"choices":[{"message":{"content":"ok"}}]}`,
			}
			app := &App{
				Log:        testutil.NopLogger(),
				HTTPClient: &http.Client{Transport: transport},
			}

			settings := &models.ChatbotSettings{
				AI: models.AIConfig{
					Model:       tt.model,
					MaxTokens:   500,
					Temperature: tt.temperature,
				},
			}

			_, err := app.generateOpenAIResponse(t.Context(), settings, nil, "hello", "")
			require.NoError(t, err)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(transport.lastBody, &payload))

			_, hasWantKey := payload[tt.wantTokenKey]
			assert.True(t, hasWantKey, "expected payload to contain %q, got: %s", tt.wantTokenKey, transport.lastBody)

			otherKey := "max_tokens"
			if tt.wantTokenKey == "max_tokens" {
				otherKey = "max_completion_tokens"
			}
			_, hasOtherKey := payload[otherKey]
			assert.False(t, hasOtherKey, "expected payload NOT to contain %q, got: %s", otherKey, transport.lastBody)

			_, hasTemperature := payload["temperature"]
			assert.Equal(t, tt.wantTemperature, hasTemperature, "temperature presence mismatch, payload: %s", transport.lastBody)
		})
	}
}

func TestGenerateAnthropicResponse_PayloadShaping(t *testing.T) {
	tests := []struct {
		name            string
		model           string
		wantTemperature bool
	}{
		{
			name:            "claude-sonnet-5 rejects sampling params",
			model:           "claude-sonnet-5",
			wantTemperature: false,
		},
		{
			name:            "claude-opus-5 rejects sampling params",
			model:           "claude-opus-5",
			wantTemperature: false,
		},
		{
			name:            "claude-haiku-4-5 accepts temperature",
			model:           "claude-haiku-4-5",
			wantTemperature: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &capturingTransport{
				response: `{"content":[{"type":"text","text":"ok"}]}`,
			}
			app := &App{
				Log:        testutil.NopLogger(),
				HTTPClient: &http.Client{Transport: transport},
			}

			settings := &models.ChatbotSettings{
				AI: models.AIConfig{
					Model:       tt.model,
					MaxTokens:   500,
					Temperature: 0.5,
				},
			}

			_, err := app.generateAnthropicResponse(t.Context(), settings, nil, "hello", "")
			require.NoError(t, err)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(transport.lastBody, &payload))

			_, hasMaxTokens := payload["max_tokens"]
			assert.True(t, hasMaxTokens, "expected payload to contain max_tokens, got: %s", transport.lastBody)

			_, hasTemperature := payload["temperature"]
			assert.Equal(t, tt.wantTemperature, hasTemperature, "temperature presence mismatch, payload: %s", transport.lastBody)
		})
	}
}
