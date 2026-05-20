package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/olshmore/ytter/pkg/config"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"

	fallbackReasonProviderError = "provider_error"
	fallbackReasonTimeout       = "timeout"
	fallbackReasonSchemaInvalid = "schema_invalid"
	fallbackReasonDisabled      = "ai_disabled"

	fallbackMessageGeneric = "AI service is temporarily unavailable; using safe fallback."
)

type openAIGateway struct {
	httpClient   *http.Client
	apiKey       string
	baseURL      string
	defaultModel string
	maxTokens    int
	temperature  float64
	maxRetries   int
	logEnabled   bool
	timeout      time.Duration
}

func NewOpenAIGateway(cfg config.Config) Gateway {
	baseURL := cfg.OpenAIBaseURL
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := cfg.AITimeout()

	return &openAIGateway{
		httpClient:   &http.Client{Timeout: timeout},
		apiKey:       cfg.OpenAIAPIKey,
		baseURL:      baseURL,
		defaultModel: cfg.OpenAIModelAssistant,
		maxTokens:    cfg.AIMaxTokens,
		temperature:  cfg.AITemperature,
		maxRetries:   cfg.AIMaxRetries,
		logEnabled:   cfg.AIEnableLogging,
		timeout:      timeout,
	}
}

func (g *openAIGateway) Enabled() bool {
	return g != nil && g.apiKey != ""
}

func (g *openAIGateway) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	model := req.Model
	if model == "" {
		model = g.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = g.maxTokens
	}
	temperature := g.temperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	traceID := uuid.NewString()
	start := time.Now()

	resp := &GenerateResponse{
		Feature: req.Feature,
		Model:   model,
		TraceID: traceID,
	}

	if !g.Enabled() {
		resp.Mode = ResponseModeDisabled
		resp.Fallback = &FallbackInfo{
			Reason:  fallbackReasonDisabled,
			Message: fallbackMessageGeneric,
		}
		resp.LatencyMS = elapsedMS(start)
		logResponse(g.logEnabled, resp, nil)
		return resp, nil
	}

	attempt := 0
	maxAttempts := g.maxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	systemPrompt := req.SystemPrompt
	for attempt < maxAttempts {
		callCtx, cancel := context.WithTimeout(ctx, g.timeout)
		raw, usage, err := g.callChatCompletion(callCtx, model, systemPrompt, req.UserPrompt, req.Schema, temperature, maxTokens)
		cancel()

		if err != nil {
			if isTimeout(err, ctx) {
				resp.Mode = ResponseModeFallback
				resp.Fallback = &FallbackInfo{Reason: fallbackReasonTimeout, Message: fallbackMessageGeneric}
			} else {
				resp.Mode = ResponseModeFallback
				resp.Fallback = &FallbackInfo{Reason: fallbackReasonProviderError, Message: fallbackMessageGeneric}
			}
			resp.Retries = attempt
			resp.LatencyMS = elapsedMS(start)
			logResponse(g.logEnabled, resp, err)
			return resp, nil
		}

		resp.PromptTokens = usage.PromptTokens
		resp.OutputTokens = usage.CompletionTokens

		if schemaErr := req.Schema.Validate(raw); schemaErr != nil {
			attempt++
			if attempt >= maxAttempts {
				resp.Mode = ResponseModeFallback
				resp.Fallback = &FallbackInfo{Reason: fallbackReasonSchemaInvalid, Message: fallbackMessageGeneric}
				resp.Retries = attempt - 1
				resp.LatencyMS = elapsedMS(start)
				logResponse(g.logEnabled, resp, schemaErr)
				return resp, nil
			}
			systemPrompt = systemPrompt + "\nIMPORTANT: previous response failed schema validation. " +
				"Return ONLY a JSON object matching the schema exactly, with no extra keys or commentary."
			continue
		}

		resp.JSON = raw
		resp.Mode = ResponseModeSuccess
		resp.Retries = attempt
		resp.Confidence = 1.0
		resp.LatencyMS = elapsedMS(start)
		logResponse(g.logEnabled, resp, nil)
		return resp, nil
	}

	resp.Mode = ResponseModeFallback
	resp.Fallback = &FallbackInfo{Reason: fallbackReasonProviderError, Message: fallbackMessageGeneric}
	resp.LatencyMS = elapsedMS(start)
	logResponse(g.logEnabled, resp, errors.New("ai: exhausted retries"))
	return resp, nil
}

type chatRequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequestBody struct {
	Model          string               `json:"model"`
	Messages       []chatRequestMessage `json:"messages"`
	Temperature    float64              `json:"temperature"`
	MaxTokens      int                  `json:"max_tokens,omitempty"`
	ResponseFormat map[string]any       `json:"response_format,omitempty"`
}

type chatResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type chatResponseChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type chatResponseBody struct {
	Choices []chatResponseChoice `json:"choices"`
	Usage   chatResponseUsage    `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (g *openAIGateway) callChatCompletion(
	ctx context.Context,
	model string,
	systemPrompt string,
	userPrompt string,
	schema *Schema,
	temperature float64,
	maxTokens int,
) (json.RawMessage, chatResponseUsage, error) {
	messages := make([]chatRequestMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, chatRequestMessage{Role: "system", Content: systemPrompt})
	}
	if userPrompt != "" {
		messages = append(messages, chatRequestMessage{Role: "user", Content: userPrompt})
	}

	body := chatRequestBody{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	if schema != nil {
		body.ResponseFormat = map[string]any{
			"type":        "json_schema",
			"json_schema": schema.toOpenAIJSONSchema(),
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, chatResponseUsage{}, fmt.Errorf("marshal request: %w", err)
	}

	url := g.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, chatResponseUsage{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	httpResp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, chatResponseUsage{}, err
	}
	defer httpResp.Body.Close()

	rawBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, chatResponseUsage{}, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode >= 400 {
		return nil, chatResponseUsage{}, fmt.Errorf("openai http %d: %s", httpResp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var parsed chatResponseBody
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, chatResponseUsage{}, fmt.Errorf("unmarshal response: %w", err)
	}
	if parsed.Error != nil {
		return nil, chatResponseUsage{}, fmt.Errorf("openai error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, chatResponseUsage{}, errors.New("openai: empty choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return nil, parsed.Usage, errors.New("openai: empty completion content")
	}

	content = stripCodeFence(content)

	return json.RawMessage(content), parsed.Usage, nil
}

func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func isTimeout(err error, ctx context.Context) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}
