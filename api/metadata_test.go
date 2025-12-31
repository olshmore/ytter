package api

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func TestExtractMetadata(t *testing.T) {
	server := newTestServer(t, nil, nil)

	testCases := []struct {
		name     string
		setupCtx func() context.Context
		check    func(t *testing.T, mtdt *Metadata)
	}{
		{
			name: "GrpcGatewayUserAgent",
			setupCtx: func() context.Context {
				md := metadata.MD{
					grpcGatewayUserAgentHeader: []string{"Mozilla/5.0"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Equal(t, "Mozilla/5.0", mtdt.UserAgent)
			},
		},
		{
			name: "HttpUserAgent",
			setupCtx: func() context.Context {
				md := metadata.MD{
					userAgentHeader: []string{"Chrome/91.0"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Equal(t, "Chrome/91.0", mtdt.UserAgent)
			},
		},
		{
			name: "XForwardedForHeader",
			setupCtx: func() context.Context {
				md := metadata.MD{
					xForwardedForHeader: []string{"192.168.1.1"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Equal(t, "192.168.1.1", mtdt.ClientIP)
			},
		},
		{
			name: "PeerContext",
			setupCtx: func() context.Context {
				addr := &net.TCPAddr{
					IP:   net.IPv4(127, 0, 0, 1),
					Port: 8080,
				}
				p := &peer.Peer{
					Addr: addr,
				}
				return peer.NewContext(context.Background(), p)
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Equal(t, "127.0.0.1:8080", mtdt.ClientIP)
			},
		},
		{
			name: "NoMetadata",
			setupCtx: func() context.Context {
				return context.Background()
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Empty(t, mtdt.UserAgent)
				require.Empty(t, mtdt.ClientIP)
			},
		},
		{
			name: "GrpcGatewayUserAgentTakesPrecedence",
			setupCtx: func() context.Context {
				md := metadata.MD{
					grpcGatewayUserAgentHeader: []string{"Mozilla/5.0"},
					userAgentHeader:            []string{"Chrome/91.0"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Equal(t, "Mozilla/5.0", mtdt.UserAgent)
			},
		},
		{
			name: "XForwardedForTakesPrecedenceOverPeer",
			setupCtx: func() context.Context {
				addr := &net.TCPAddr{
					IP:   net.IPv4(127, 0, 0, 1),
					Port: 8080,
				}
				p := &peer.Peer{
					Addr: addr,
				}
				ctx := peer.NewContext(context.Background(), p)
				md := metadata.MD{
					xForwardedForHeader: []string{"192.168.1.1"},
				}
				return metadata.NewIncomingContext(ctx, md)
			},
			check: func(t *testing.T, mtdt *Metadata) {
				require.Equal(t, "192.168.1.1", mtdt.ClientIP)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.setupCtx()
			mtdt := server.extractMetadata(ctx)
			require.NotNil(t, mtdt)
			tc.check(t, mtdt)
		})
	}
}
