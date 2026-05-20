package ai

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// logResponse emits the per-request observability fields
func logResponse(enabled bool, resp *GenerateResponse, err error) {
	if !enabled || resp == nil {
		return
	}

	var event *zerolog.Event
	if err != nil {
		event = log.Error().Err(err)
	} else if resp.Mode == ResponseModeFallback {
		event = log.Warn()
	} else {
		event = log.Info()
	}

	event = event.
		Str("ai_feature", string(resp.Feature)).
		Str("ai_mode", string(resp.Mode)).
		Str("ai_model", resp.Model).
		Str("ai_trace_id", resp.TraceID).
		Int("ai_prompt_tokens", resp.PromptTokens).
		Int("ai_output_tokens", resp.OutputTokens).
		Int64("ai_latency_ms", resp.LatencyMS).
		Int("ai_retries", resp.Retries)

	if resp.Fallback != nil {
		event = event.Str("ai_fallback_reason", resp.Fallback.Reason)
	}

	event.Msg("ai request")
}
