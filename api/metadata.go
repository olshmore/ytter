package api

import (
	"context"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

const (
	grpcGatewayUserAgentHeader = "grpcgateway-user-agent"
	userAgentHeader            = "user-agent"
	xForwardedForHeader        = "x-forwarded-for"
)

type Metadata struct {
	UserAgent string
	ClientIP  string
}

func (server *Server) extractMetadata(ctx context.Context) *Metadata {
	mtdt := &Metadata{}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userAgents := md.Get(grpcGatewayUserAgentHeader); len(userAgents) > 0 {
			// GRPC userAgent
			mtdt.UserAgent = userAgents[0]
		} else if userAgents := md.Get(userAgentHeader); len(userAgents) > 0 {
			// HTTP userAgent
			mtdt.UserAgent = userAgents[0]
		}

		if clientIPs := md.Get(xForwardedForHeader); len(clientIPs) > 0 {
			// GRPC ClientIP (x-forwarded-for takes precedence)
			mtdt.ClientIP = clientIPs[0]
		}
	}

	// Use peer context if x-forwarded-for is not set
	if mtdt.ClientIP == "" {
		if p, ok := peer.FromContext(ctx); ok {
			// HTTP ClientIP
			mtdt.ClientIP = p.Addr.String()
		}
	}

	return mtdt
}
