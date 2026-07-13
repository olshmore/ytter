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
	EnableGRPCServer     bool          `mapstructure:"ENABLE_GRPC_SERVER"`
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
	StorageDriver        string        `mapstructure:"STORAGE_DRIVER"`
	StorageLocalDir      string        `mapstructure:"STORAGE_LOCAL_DIR"`
	StoragePublicBaseURL string        `mapstructure:"STORAGE_PUBLIC_BASE_URL"`
	S3Bucket             string        `mapstructure:"S3_BUCKET"`
	S3Region             string        `mapstructure:"S3_REGION"`
	S3PublicBaseURL      string        `mapstructure:"S3_PUBLIC_BASE_URL"`
}

func (c Config) AITimeout() time.Duration {
	if c.AITimeoutMS <= 0 {
		return 8 * time.Second
	}
	return time.Duration(c.AITimeoutMS) * time.Millisecond
}

func bindConfigEnvs(v *viper.Viper) {
	keys := []string{
		"ENVIRONMENT",
		"ALLOWED_ORIGINS",
		"FRONTEND_BASE_URL",
		"DB_URL",
		"MIGRATION_URL",
		"ENABLE_GRPC_SERVER",
		"GRPC_SERVER_ADDRESS",
		"HTTP_SERVER_ADDRESS",
		"REDIS_ADDRESS",
		"TOKEN_SYMMETRIC_KEY",
		"ACCESS_TOKEN_DURATION",
		"REFRESH_TOKEN_DURATION",
		"EMAIL_SENDER_NAME",
		"EMAIL_SENDER_ADDRESS",
		"EMAIL_SENDER_PASSWORD",
		"GOOGLE_CLIENT_ID",
		"GOOGLE_CLIENT_SECRET",
		"GOOGLE_REDIRECT_URL",
		"AI_PROVIDER",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"OPENAI_MODEL_SUMMARY",
		"OPENAI_MODEL_ASSISTANT",
		"OPENAI_MODEL_EMBEDDING",
		"AI_TIMEOUT_MS",
		"AI_MAX_TOKENS",
		"AI_TEMPERATURE",
		"AI_MAX_RETRIES",
		"AI_ENABLE_LOGGING",
		"STORAGE_DRIVER",
		"STORAGE_LOCAL_DIR",
		"STORAGE_PUBLIC_BASE_URL",
		"S3_BUCKET",
		"S3_REGION",
		"S3_PUBLIC_BASE_URL",
	}
	for _, key := range keys {
		_ = v.BindEnv(key)
	}
}

func applyDefaults(v *viper.Viper) {
	v.SetDefault("ENVIRONMENT", "production")
	v.SetDefault("MIGRATION_URL", "file://db/migration")
	v.SetDefault("ENABLE_GRPC_SERVER", true)
	v.SetDefault("GRPC_SERVER_ADDRESS", "0.0.0.0:50051")
	v.SetDefault("HTTP_SERVER_ADDRESS", "0.0.0.0:8080")
	v.SetDefault("REDIS_ADDRESS", "redis:6379")
	v.SetDefault("ACCESS_TOKEN_DURATION", "15m")
	v.SetDefault("REFRESH_TOKEN_DURATION", "24h")
	v.SetDefault("EMAIL_SENDER_NAME", "Ytter")

	// AI
	v.SetDefault("AI_PROVIDER", "openai")
	v.SetDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
	v.SetDefault("OPENAI_MODEL_SUMMARY", "gpt-4.1")
	v.SetDefault("OPENAI_MODEL_ASSISTANT", "gpt-4.1")
	v.SetDefault("OPENAI_MODEL_EMBEDDING", "text-embedding-3-large")
	v.SetDefault("AI_TIMEOUT_MS", 8000)
	v.SetDefault("AI_MAX_TOKENS", 1024)
	v.SetDefault("AI_TEMPERATURE", 0.2)
	v.SetDefault("AI_MAX_RETRIES", 1)
	v.SetDefault("AI_ENABLE_LOGGING", true)

	v.SetDefault("STORAGE_DRIVER", "local")
	v.SetDefault("STORAGE_LOCAL_DIR", "./data/branding")
	v.SetDefault("STORAGE_PUBLIC_BASE_URL", "http://localhost:8080/v1/public/media")
}

func LoadConfig(path string) (config Config, err error) {
	v := viper.New()

	v.AddConfigPath(path)

	v.SetConfigName("app")

	v.SetConfigType("env")
	
	v.AutomaticEnv()

	applyDefaults(v)
	
	bindConfigEnvs(v)

	err = v.ReadInConfig()
	if err != nil {
		return
	}

	err = v.Unmarshal(&config)
	return
}
