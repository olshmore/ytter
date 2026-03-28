package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/olshmore/ytter/pkg/token"
	"github.com/olshmore/ytter/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authorizationHeader = "authorization"
	authorizationBearer = "bearer"
)

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// authPayloadKey is the key used to store auth payload in context
	authPayloadKey contextKey = "auth_payload"
)

// RoleConfig maps gRPC method names to their required roles
type RoleConfig map[string][]utils.Role

// NewRoleConfig creates a new role configuration with common patterns
func NewRoleConfig() RoleConfig {
	return make(RoleConfig)
}

// RequireHostRole adds one or more methods to require host role
func (config RoleConfig) RequireHostRole(methods ...string) RoleConfig {
	return config.RequireAuth([]utils.Role{utils.RoleHost}, methods...)
}

// RequireClientRole adds one or more methods to require client role
func (config RoleConfig) RequireClientRole(methods ...string) RoleConfig {
	return config.RequireAuth([]utils.Role{utils.RoleClient}, methods...)
}

// RequireAdminRole adds one or more methods to require admin role
func (config RoleConfig) RequireAdminRole(methods ...string) RoleConfig {
	return config.RequireAuth([]utils.Role{utils.RoleAdmin}, methods...)
}

// RequireAuth adds one or more methods to require authentication with specific roles
func (config RoleConfig) RequireAuth(roles []utils.Role, methods ...string) RoleConfig {
	for _, method := range methods {
		config[method] = roles
	}
	return config
}

// RequireRoles creates a gRPC unary interceptor that checks if the user has the required role
func (server *Server) RequireRoles(config RoleConfig) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		allowedRoles, requiresAuth := config[info.FullMethod]
		if !requiresAuth {
			return handler(ctx, req)
		}

		payload, err := server.extractAuthPayload(ctx)
		if err != nil {
			return nil, unauthenticatedError(err)
		}

		if len(allowedRoles) > 0 && !hasPermission(payload.Roles, allowedRoles) {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}

		ctx = context.WithValue(ctx, authPayloadKey, payload)

		return handler(ctx, req)
	}
}

// extractAuthPayload extracts and verifies the authentication token from gRPC metadata
func (server *Server) extractAuthPayload(ctx context.Context) (*token.Payload, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no metadata")
	}

	headerValues := md.Get(authorizationHeader)
	if len(headerValues) == 0 {
		return nil, fmt.Errorf("no authorization header")
	}

	return server.verifyTokenFromHeader(headerValues[0])
}

// verifyTokenFromHeader parses and verifies a bearer token from an authorization header
func (server *Server) verifyTokenFromHeader(authHeader string) (*token.Payload, error) {
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

// GetAuthPayload extracts the auth payload from context.
func GetAuthPayload(ctx context.Context) *token.Payload {
	payload, ok := ctx.Value(authPayloadKey).(*token.Payload)
	if !ok {
		return nil
	}
	return payload
}

// authoriseUser extracts auth payload and checks if user has required roles
func (server *Server) authoriseUser(ctx context.Context, allowedRoles []utils.Role) (*token.Payload, error) {
	payload, err := server.extractAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	if !hasPermission(payload.Roles, allowedRoles) {
		return nil, fmt.Errorf("permission denied")
	}

	return payload, nil
}

// hasPermission checks if roles are in the allowed list
func hasPermission(userRoles []utils.Role, allowedRoles []utils.Role) bool {
	if len(userRoles) == 0 {
		return false
	}

	userRoleSet := make(map[utils.Role]struct{}, len(userRoles))
	for _, role := range userRoles {
		userRoleSet[role] = struct{}{}
	}

	for _, allowedRole := range allowedRoles {
		if _, ok := userRoleSet[allowedRole]; ok {
			return true
		}
	}
	return false
}

// MustGetAuthPayload extracts the auth payload from context.
func (server *Server) MustGetAuthPayload(ctx context.Context, allowedRoles []utils.Role) (*token.Payload, error) {
	payload := GetAuthPayload(ctx)
	if payload != nil {
		return payload, nil
	}

	return server.authoriseUser(ctx, allowedRoles)
}

// RequireRolesHTTP creates an HTTP middleware that checks if the user has the required role
func (server *Server) RequireRolesHTTP(config RoleConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := extractMethodFromGatewayContext(r.URL.Path)
			if method == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowedRoles, requiresAuth := config[method]
			if !requiresAuth {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httpError(w, codes.Unauthenticated, "no authorization header")
				return
			}

			payload, err := server.verifyTokenFromHeader(authHeader)
			if err != nil {
				httpError(w, codes.Unauthenticated, err.Error())
				return
			}

			if len(allowedRoles) > 0 && !hasPermission(payload.Roles, allowedRoles) {
				httpError(w, codes.PermissionDenied, "permission denied")
				return
			}

			ctx := context.WithValue(r.Context(), authPayloadKey, payload)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// httpError writes an HTTP error response as JSON
func httpError(w http.ResponseWriter, code codes.Code, message string) {
	w.Header().Set("Content-Type", "application/json")
	httpStatus := utils.GrpcCodeToHTTPStatus(code)
	w.WriteHeader(httpStatus)
	errorResponse := fmt.Sprintf(`{"error": "%s", "code": %d, "message": "%s"}`, code.String(), httpStatus, message)
	w.Write([]byte(errorResponse))
}

// extractMethodFromGatewayContext extracts the gRPC method name from the HTTP path
func extractMethodFromGatewayContext(path string) string {
	mapping, err := HTTPPathToGRPCMethodMap()
	if err != nil {
		return ""
	}
	return mapping[path]
}
