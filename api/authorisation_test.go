package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestAuthoriseUser(t *testing.T) {
	server := newTestServer(t, nil, nil)
	username := "testuser"
	duration := time.Minute

	testCases := []struct {
		name      string
		setupCtx  func() context.Context
		expectErr bool
	}{
		{
			name: "OK",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, duration)
			},
			expectErr: false,
		},
		{
			name: "NoMetadata",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectErr: true,
		},
		{
			name: "NoAuthorizationHeader",
			setupCtx: func() context.Context {
				md := metadata.MD{}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr: true,
		},
		{
			name: "InvalidHeaderFormat",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"invalid"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr: true,
		},
		{
			name: "UnsupportedAuthType",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"Basic token123"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr: true,
		},
		{
			name: "InvalidToken",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer invalid-token"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr: true,
		},
		{
			name: "ExpiredToken",
			setupCtx: func() context.Context {
				// Create an expired token
				accessToken, _, err := server.tokenMaker.CreateToken(username, -time.Hour)
				require.NoError(t, err)
				bearerToken := "bearer " + accessToken
				md := metadata.MD{
					authorizationHeader: []string{bearerToken},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.setupCtx()
			payload, err := server.authoriseUser(ctx)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, payload)
			} else {
				require.NoError(t, err)
				require.NotNil(t, payload)
				require.Equal(t, username, payload.Username)
			}
		})
	}
}
