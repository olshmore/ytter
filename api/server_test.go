package api

import (
	"testing"

	"github.com/olshmore/ytter/pkg/config"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	testCases := []struct {
		name      string
		config    config.Config
		expectErr bool
	}{
		{
			name: "OK",
			config: config.Config{
				TokenSymmetricKey:   utils.RandomString(32),
				AccessTokenDuration: 0,
			},
			expectErr: false,
		},
		{
			name: "InvalidTokenKey",
			config: config.Config{
				TokenSymmetricKey:   "short",
				AccessTokenDuration: 0,
			},
			expectErr: true,
		},
		{
			name: "EmptyTokenKey",
			config: config.Config{
				TokenSymmetricKey:   "",
				AccessTokenDuration: 0,
			},
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			server, err := NewServer(tc.config, nil, nil)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, server)
			} else {
				require.NoError(t, err)
				require.NotNil(t, server)
				require.NotNil(t, server.tokenMaker)
			}
		})
	}
}
