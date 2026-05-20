package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Environment          string        `mapstructure:"ENVIRONMENT"`
	AllowedOrigins       []string      `mapstructure:"ALLOWED_ORIGINS"`
	FrontendBaseURL      string        `mapstructure:"FRONTEND_BASE_URL"`
	DBSource             string        `mapstructure:"DB_URL"`
	MigrationURL         string        `mapstructure:"MIGRATION_URL"`
	GRPCServerAddress    string        `mapstructure:"GRPC_SERVER_ADDRESS"`
	HTTPServerAddress    string        `mapstructure:"HTTP_SERVER_ADDRESS"`
	RedisAddress         string        `mapstructure:"REDIS_ADDRESS"`
	TokenSymmetricKey    string        `mapstructure:"TOKEN_SYMMETRIC_KEY"`
	AccessTokenDuration  time.Duration `mapstructure:"ACCESS_TOKEN_DURATION"`
	RefreshTokenDuration time.Duration `mapstructure:"REFRESH_TOKEN_DURATION"`
	EmailSenderName      string        `mapstructure:"EMAIL_SENDER_NAME"`
	EmailSenderAddress   string        `mapstructure:"EMAIL_SENDER_ADDRESS"`
	EmailSenderPassword  string        `mapstructure:"EMAIL_SENDER_PASSWORD"`
	GoogleClientID       string        `mapstructure:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret   string        `mapstructure:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL    string        `mapstructure:"GOOGLE_REDIRECT_URL"`
	AIProvider           string        `mapstructure:"AI_PROVIDER"`
	OpenAIAPIKey         string        `mapstructure:"OPENAI_API_KEY"`
	OpenAIBaseURL        string        `mapstructure:"OPENAI_BASE_URL"`
	OpenAIModelSummary   string        `mapstructure:"OPENAI_MODEL_SUMMARY"`
	OpenAIModelAssistant string        `mapstructure:"OPENAI_MODEL_ASSISTANT"`
	OpenAIModelEmbedding string        `mapstructure:"OPENAI_MODEL_EMBEDDING"`
	AITimeoutMS          int           `mapstructure:"AI_TIMEOUT_MS"`
	AIMaxTokens          int           `mapstructure:"AI_MAX_TOKENS"`
	AITemperature        float64       `mapstructure:"AI_TEMPERATURE"`
	AIMaxRetries         int           `mapstructure:"AI_MAX_RETRIES"`
	AIEnableLogging      bool          `mapstructure:"AI_ENABLE_LOGGING"`
}

func (c Config) AITimeout() time.Duration {
	if c.AITimeoutMS <= 0 {
		return 8 * time.Second
	}
	return time.Duration(c.AITimeoutMS) * time.Millisecond
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)

	viper.SetConfigName("app")

	viper.SetConfigType("env")

	viper.AutomaticEnv()

	// AI defaults
	viper.SetDefault("AI_PROVIDER", "openai")
	viper.SetDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
	viper.SetDefault("OPENAI_MODEL_SUMMARY", "gpt-4.1")
	viper.SetDefault("OPENAI_MODEL_ASSISTANT", "gpt-4.1")
	viper.SetDefault("OPENAI_MODEL_EMBEDDING", "text-embedding-3-large")
	viper.SetDefault("AI_TIMEOUT_MS", 8000)
	viper.SetDefault("AI_MAX_TOKENS", 1024)
	viper.SetDefault("AI_TEMPERATURE", 0.2)
	viper.SetDefault("AI_MAX_RETRIES", 1)
	viper.SetDefault("AI_ENABLE_LOGGING", true)

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	return
}
