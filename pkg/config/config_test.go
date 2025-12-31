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
				tmpDir := t.TempDir()
				return tmpDir, func() {}
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
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, config.Environment)
				require.NotEmpty(t, config.DBSource)
			}
		})
	}
}
