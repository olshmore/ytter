package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type FeatureID string

const (
	FeatureIntakeSummary  FeatureID = "intake_summary"
	FeatureGuestAssistant FeatureID = "guest_assistant"
	FeatureHostAssistant  FeatureID = "host_assistant"
)

type ResponseMode string

const (
	ResponseModeSuccess  ResponseMode = "success"
	ResponseModeFallback ResponseMode = "fallback"
	ResponseModeDisabled ResponseMode = "disabled"
)

type GenerateRequest struct {
	Feature      FeatureID
	Model        string
	SystemPrompt string
	UserPrompt   string
	Schema       *Schema
	Temperature  *float64
	MaxTokens    int
}

type GenerateResponse struct {
	Feature      FeatureID       `json:"feature"`
	Mode         ResponseMode    `json:"mode"`
	Model        string          `json:"model"`
	TraceID      string          `json:"trace_id"`
	JSON         json.RawMessage `json:"json"`
	Confidence   float64         `json:"confidence"`
	PromptTokens int             `json:"prompt_tokens"`
	OutputTokens int             `json:"output_tokens"`
	LatencyMS    int64           `json:"latency_ms"`
	Retries      int             `json:"retries"`
	Fallback     *FallbackInfo   `json:"fallback,omitempty"`
}

type FallbackInfo struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Gateway is the AI completion contract used by booking intelligence features.
type Gateway interface {
	// Generate returns schema-valid JSON or a fallback response (nil error).
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	Enabled() bool
}

// ErrInvalidRequest is caller misconfiguration, not a fallback scenario.
var ErrInvalidRequest = errors.New("ai: invalid request")

func (r GenerateRequest) validate() error {
	if r.Feature == "" {
		return errors.New("ai: feature is required")
	}
	if r.SystemPrompt == "" && r.UserPrompt == "" {
		return errors.New("ai: at least one of system or user prompt is required")
	}
	return nil
}

func elapsedMS(start time.Time) int64 {
	d := time.Since(start).Milliseconds()
	if d < 0 {
		return 0
	}
	return d
}
