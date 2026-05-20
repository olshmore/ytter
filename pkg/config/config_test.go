package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	testCases := []struct {
		name      string
		setup     func() (string, func())
		expectErr bool
	}{
		{
			name: "OK",
			setup: func() (string, func()) {
				// Create a temporary directory
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "app.env")

				// Write test config
				content := `ENVIRONMENT=test
										DB_URL=postgres://user:pass@localhost/db
										GRPC_SERVER_ADDRESS=0.0.0.0:9090
										HTTP_SERVER_ADDRESS=0.0.0.0:8080
										TOKEN_SYMMETRIC_KEY=12345678901234567890123456789012
										ACCESS_TOKEN_DURATION=15m
										REFRESH_TOKEN_DURATION=24h
										`
				err := os.WriteFile(configFile, []byte(content), 0644)
				require.NoError(t, err)

				return tmpDir, func() {}
			},
			expectErr: false,
		},
		{
			name: "MissingConfigFile",
			setup: func() (string, func()) {
				return t.TempDir(), func() {}
			},
			expectErr: true,
		},
		{
			name: "MinimumSecretsUsesDefaults",
			setup: func() (string, func()) {
				envKeys := []string{
					"ENVIRONMENT",
					"MIGRATION_URL",
					"GRPC_SERVER_ADDRESS",
					"HTTP_SERVER_ADDRESS",
					"REDIS_ADDRESS",
					"EMAIL_SENDER_NAME",
				}
				saved := make(map[string]string, len(envKeys))
				for _, key := range envKeys {
					saved[key] = os.Getenv(key)
					os.Unsetenv(key)
				}

				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "app.env")
				content := `DB_URL=postgres://user:pass@localhost/db
TOKEN_SYMMETRIC_KEY=12345678901234567890123456789012
EMAIL_SENDER_PASSWORD=email-secret
GOOGLE_CLIENT_SECRET=google-secret
OPENAI_API_KEY=openai-secret
`
				err := os.WriteFile(configFile, []byte(content), 0644)
				require.NoError(t, err)
				return tmpDir, func() {
					for _, key := range envKeys {
						if v, ok := saved[key]; ok && v != "" {
							os.Setenv(key, v)
						} else {
							os.Unsetenv(key)
						}
					}
				}
			},
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			path, cleanup := tc.setup()
			defer cleanup()

			config, err := LoadConfig(path)

			if tc.expectErr {
				require.Error(t, err)
				if tc.name == "MissingConfigFile" {
					require.Contains(t, err.Error(), `Config File "app" Not Found`)
				}
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, config.Environment)
				require.NotEmpty(t, config.DBSource)
				if tc.name == "MinimumSecretsUsesDefaults" {
					require.Equal(t, "production", config.Environment)
					require.Equal(t, "file://db/migration", config.MigrationURL)
					require.Equal(t, "0.0.0.0:50051", config.GRPCServerAddress)
					require.Equal(t, "redis:6379", config.RedisAddress)
					require.Equal(t, "Ytter", config.EmailSenderName)
				}
			}
		})
	}
}
