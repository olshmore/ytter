package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/olshmore/ytter/token"
	"google.golang.org/grpc/metadata"
)

const (
	authorizationHeader = "authorization"
	authorizationBearer = "bearer"
)

func (server *Server) authoriseUser(ctx context.Context) (*token.Payload, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata")
	}

	headerValues := md.Get(authorizationHeader)
	if len(headerValues) == 0 {
		return nil, fmt.Errorf("no authorization header")
	}

	authHeader := headerValues[0]
	headerFields := strings.Fields(authHeader)
	if len(headerFields) < 2 {
		return nil, fmt.Errorf("invalid authorization header")
	}

	authType := strings.ToLower(headerFields[0])
	if authType != authorizationBearer {
		return nil, fmt.Errorf("unsupported authorization type: %s", authType)
	}

	accessToken := headerFields[1]
	payload, err := server.tokenMaker.VerifyToken(accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid access token: %s", err)
	}

	return payload, nil
}
