package ai

import (
	"github.com/olshmore/ytter/pkg/config"
)

// NewGatewayFromConfig returns the production gateway when an API key is
// configured, otherwise a safe fallback gateway. This is the only place
// main.go and server bootstrap should call to wire AI features.
func NewGatewayFromConfig(cfg config.Config) Gateway {
	if cfg.AIProvider != "" && cfg.AIProvider != "openai" {
		return NewFallbackGateway(cfg.AIEnableLogging)
	}
	if cfg.OpenAIAPIKey == "" {
		return NewFallbackGateway(cfg.AIEnableLogging)
	}
	return NewOpenAIGateway(cfg)
}
