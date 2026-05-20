package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/olshmore/ytter/pkg/config"
)

// newTestGateway builds a gateway pointed at the given test server URL with
// safe defaults for unit tests.
func newTestGateway(t *testing.T, baseURL string) *openAIGateway {
	t.Helper()
	cfg := config.Config{
		AIProvider:           "openai",
		OpenAIAPIKey:         "test-key",
		OpenAIBaseURL:        baseURL,
		OpenAIModelAssistant: "gpt-4.1",
		AITimeoutMS:          2000,
		AIMaxTokens:          128,
		AITemperature:        0.0,
		AIMaxRetries:         1,
		AIEnableLogging:      false,
	}
	gw, ok := NewOpenAIGateway(cfg).(*openAIGateway)
	require.True(t, ok)
	return gw
}

func writeChatCompletion(t *testing.T, w http.ResponseWriter, content string, usage chatResponseUsage) {
	t.Helper()
	resp := chatResponseBody{
		Choices: []chatResponseChoice{{
			Message: struct {
				Content string `json:"content"`
			}{Content: content},
			FinishReason: "stop",
		}},
		Usage: usage,
	}
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(resp))
}

var testJSONSchema = &Schema{
	Name:     "test_response",
	Type:     "object",
	Required: []string{"status", "value"},
	Properties: map[string]*Schema{
		"status": {Type: "string", Enum: []string{"ok"}},
		"value":  {Type: "integer"},
	},
}

func testGenerateRequest() GenerateRequest {
	return GenerateRequest{
		Feature:      FeatureGuestAssistant,
		SystemPrompt: "Return JSON only.",
		UserPrompt:   "ping",
		Schema:       testJSONSchema,
		MaxTokens:    128,
	}
}

func TestOpenAIGateway_GenerateJSONSchemaSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var body chatRequestBody
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "gpt-4.1", body.Model)
		require.NotNil(t, body.ResponseFormat)
		require.Equal(t, "json_schema", body.ResponseFormat["type"])

		writeChatCompletion(t, w,
			`{"status":"ok","message":"connectivity confirmed","value":42}`,
			chatResponseUsage{PromptTokens: 7, CompletionTokens: 11},
		)
	}))
	defer srv.Close()

	gw := newTestGateway(t, srv.URL)
	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ResponseModeSuccess, resp.Mode)
	require.Equal(t, FeatureGuestAssistant, resp.Feature)
	require.Equal(t, "gpt-4.1", resp.Model)
	require.NotEmpty(t, resp.TraceID)
	require.Equal(t, 7, resp.PromptTokens)
	require.Equal(t, 11, resp.OutputTokens)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.JSON, &payload))
	require.Equal(t, "ok", payload["status"])
	require.EqualValues(t, 42, payload["value"])
}

func TestOpenAIGateway_SchemaInvalidRetriesOnceThenFallback(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// Always return schema-invalid content so we can verify retry-then-fallback.
		writeChatCompletion(t, w,
			`{"status":"nope"}`,
			chatResponseUsage{PromptTokens: 3, CompletionTokens: 3},
		)
	}))
	defer srv.Close()

	gw := newTestGateway(t, srv.URL)
	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ResponseModeFallback, resp.Mode)
	require.NotNil(t, resp.Fallback)
	require.Equal(t, fallbackReasonSchemaInvalid, resp.Fallback.Reason)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls), "should retry once then fallback")
}

func TestOpenAIGateway_SchemaRetrySucceedsOnSecondAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			writeChatCompletion(t, w,
				`{"status":"nope"}`,
				chatResponseUsage{PromptTokens: 3, CompletionTokens: 3},
			)
			return
		}

		var body chatRequestBody
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		// The second attempt must include the stricter retry instruction.
		var systemContent string
		for _, m := range body.Messages {
			if m.Role == "system" {
				systemContent = m.Content
			}
		}
		require.True(t, strings.Contains(systemContent, "failed schema validation"),
			"retry should append stricter system instruction")

		writeChatCompletion(t, w,
			`{"status":"ok","message":"recovered","value":1}`,
			chatResponseUsage{PromptTokens: 5, CompletionTokens: 5},
		)
	}))
	defer srv.Close()

	gw := newTestGateway(t, srv.URL)
	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.Equal(t, ResponseModeSuccess, resp.Mode)
	require.Equal(t, 1, resp.Retries)
}

func TestOpenAIGateway_ProviderErrorFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom","type":"server_error"}}`))
	}))
	defer srv.Close()

	gw := newTestGateway(t, srv.URL)
	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.Equal(t, ResponseModeFallback, resp.Mode)
	require.NotNil(t, resp.Fallback)
	require.Equal(t, fallbackReasonProviderError, resp.Fallback.Reason)
}

func TestOpenAIGateway_TimeoutFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold the request open longer than the gateway timeout.
		select {
		case <-time.After(2 * time.Second):
			writeChatCompletion(t, w, `{"status":"ok","message":"late","value":1}`, chatResponseUsage{})
		case <-r.Context().Done():
			return
		}
	}))
	defer srv.Close()

	gw := newTestGateway(t, srv.URL)
	gw.timeout = 100 * time.Millisecond
	gw.httpClient.Timeout = 100 * time.Millisecond

	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.Equal(t, ResponseModeFallback, resp.Mode)
	require.NotNil(t, resp.Fallback)
	require.Equal(t, fallbackReasonTimeout, resp.Fallback.Reason)
}

func TestOpenAIGateway_StripsMarkdownFences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeChatCompletion(t, w,
			"```json\n{\"status\":\"ok\",\"message\":\"fenced\",\"value\":7}\n```",
			chatResponseUsage{},
		)
	}))
	defer srv.Close()

	gw := newTestGateway(t, srv.URL)
	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.Equal(t, ResponseModeSuccess, resp.Mode)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.JSON, &payload))
	require.EqualValues(t, 7, payload["value"])
}

func TestFallbackGateway_AlwaysDisabled(t *testing.T) {
	gw := NewFallbackGateway(false)
	require.False(t, gw.Enabled())

	resp, err := gw.Generate(context.Background(), testGenerateRequest())
	require.NoError(t, err)
	require.Equal(t, ResponseModeDisabled, resp.Mode)
	require.NotNil(t, resp.Fallback)
	require.Equal(t, fallbackReasonDisabled, resp.Fallback.Reason)
}

func TestNewGatewayFromConfig(t *testing.T) {
	t.Run("missing api key -> fallback", func(t *testing.T) {
		gw := NewGatewayFromConfig(config.Config{AIProvider: "openai"})
		require.False(t, gw.Enabled())
	})

	t.Run("unknown provider -> fallback", func(t *testing.T) {
		gw := NewGatewayFromConfig(config.Config{AIProvider: "anthropic", OpenAIAPIKey: "k"})
		require.False(t, gw.Enabled())
	})

	t.Run("openai with api key -> enabled", func(t *testing.T) {
		gw := NewGatewayFromConfig(config.Config{AIProvider: "openai", OpenAIAPIKey: "k"})
		require.True(t, gw.Enabled())
	})
}

func TestGenerateRequestValidate(t *testing.T) {
	require.Error(t, GenerateRequest{}.validate())
	require.Error(t, GenerateRequest{Feature: FeatureGuestAssistant}.validate())
	require.NoError(t, GenerateRequest{Feature: FeatureGuestAssistant, UserPrompt: "hi"}.validate())
}
