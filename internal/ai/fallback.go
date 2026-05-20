package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// fallbackGateway is used when no API key is configured
type fallbackGateway struct {
	logEnabled bool
}

func NewFallbackGateway(logEnabled bool) Gateway {
	return &fallbackGateway{logEnabled: logEnabled}
}

func (g *fallbackGateway) Enabled() bool { return false }

func (g *fallbackGateway) Generate(_ context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	start := time.Now()
	resp := &GenerateResponse{
		Feature: req.Feature,
		Mode:    ResponseModeDisabled,
		Model:   "disabled",
		TraceID: uuid.NewString(),
		Fallback: &FallbackInfo{
			Reason:  fallbackReasonDisabled,
			Message: "AI features are disabled in this environment.",
		},
		JSON:      json.RawMessage(`{}`),
		LatencyMS: elapsedMS(start),
	}
	logResponse(g.logEnabled, resp, nil)
	return resp, nil
}
