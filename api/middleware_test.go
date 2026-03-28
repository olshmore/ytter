package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func TestNewRoleConfig(t *testing.T) {
	config := NewRoleConfig()
	require.NotNil(t, config)
	require.Equal(t, 0, len(config))
}

func TestRoleConfig_RequireClientRole(t *testing.T) {
	config := NewRoleConfig()
	config = config.RequireClientRole("/test.Method1", "/test.Method2")

	require.Equal(t, []utils.Role{utils.RoleClient}, config["/test.Method1"])
	require.Equal(t, []utils.Role{utils.RoleClient}, config["/test.Method2"])
}

func TestRoleConfig_RequireAdminRole(t *testing.T) {
	config := NewRoleConfig()
	config = config.RequireAdminRole("/test.Method1", "/test.Method2")

	require.Equal(t, []utils.Role{utils.RoleAdmin}, config["/test.Method1"])
	require.Equal(t, []utils.Role{utils.RoleAdmin}, config["/test.Method2"])
}

func TestRoleConfig_RequireAuth(t *testing.T) {
	config := NewRoleConfig()
	roles := []utils.Role{utils.RoleAdmin, utils.RoleClient}
	config = config.RequireAuth(roles, "/test.Method1")

	require.Equal(t, roles, config["/test.Method1"])
}

func TestServer_RequireRoles(t *testing.T) {
	server := newTestServer(t, nil, nil)
	username := "testuser"
	duration := time.Minute

	testCases := []struct {
		name          string
		config        RoleConfig
		method        string
		setupCtx      func() context.Context
		expectErr     bool
		expectErrCode codes.Code
		expectPayload bool
	}{
		{
			name:   "MethodNotInConfig_NoAuthRequired",
			config: NewRoleConfig(),
			method: "/test.UnknownMethod",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectErr:     false,
			expectPayload: false,
		},
		{
			name:   "MethodRequiresAuth_ValidToken_AnyRole",
			config: NewRoleConfig().RequireAuth([]utils.Role{}, "/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			expectErr:     false,
			expectPayload: true,
		},
		{
			name:   "MethodRequiresClientRole_ValidToken_ClientRole",
			config: NewRoleConfig().RequireClientRole("/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			expectErr:     false,
			expectPayload: true,
		},
		{
			name:   "MethodRequiresClientRole_ValidToken_AdminRole",
			config: NewRoleConfig().RequireClientRole("/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleAdmin}, duration)
			},
			expectErr:     true,
			expectErrCode: codes.PermissionDenied,
			expectPayload: false,
		},
		{
			name:   "MethodRequiresAdminRole_ValidToken_AdminRole",
			config: NewRoleConfig().RequireAdminRole("/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleAdmin}, duration)
			},
			expectErr:     false,
			expectPayload: true,
		},
		{
			name:   "MethodRequiresAdminRole_ValidToken_MemberRole",
			config: NewRoleConfig().RequireAdminRole("/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			expectErr:     true,
			expectErrCode: codes.PermissionDenied,
			expectPayload: false,
		},
		{
			name:   "MethodRequiresAuth_NoToken",
			config: NewRoleConfig().RequireAuth([]utils.Role{}, "/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectErr:     true,
			expectErrCode: codes.Unauthenticated,
			expectPayload: false,
		},
		{
			name:   "MethodRequiresAuth_InvalidToken",
			config: NewRoleConfig().RequireAuth([]utils.Role{}, "/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer invalid-token"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr:     true,
			expectErrCode: codes.Unauthenticated,
			expectPayload: false,
		},
		{
			name:   "MethodRequiresAuth_MultipleRoles_ValidToken_MemberRole",
			config: NewRoleConfig().RequireAuth([]utils.Role{utils.RoleClient, utils.RoleAdmin}, "/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			expectErr:     false,
			expectPayload: true,
		},
		{
			name:   "MethodRequiresAuth_MultipleRoles_ValidToken_AdminRole",
			config: NewRoleConfig().RequireAuth([]utils.Role{utils.RoleClient, utils.RoleAdmin}, "/test.Method"),
			method: "/test.Method",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleAdmin}, duration)
			},
			expectErr:     false,
			expectPayload: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			interceptor := server.RequireRoles(tc.config)

			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				payload := GetAuthPayload(ctx)
				if tc.expectPayload {
					require.NotNil(t, payload)
					require.Equal(t, username, payload.Username)
				} else {
					require.Nil(t, payload)
				}
				return "success", nil
			}

			info := &grpc.UnaryServerInfo{
				FullMethod: tc.method,
			}

			ctx := tc.setupCtx()
			resp, err := interceptor(ctx, nil, info, handler)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, resp)
				if tc.expectErrCode != 0 {
					status, ok := err.(interface {
						GRPCStatus() interface{ Code() codes.Code }
					})
					if ok {
						require.Equal(t, tc.expectErrCode, status.GRPCStatus().Code())
					}
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, "success", resp)
			}
		})
	}
}

func TestServer_extractAuthPayload(t *testing.T) {
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
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
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
			name: "EmptyTokenAfterBearer",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer "},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			expectErr: true,
		},
		{
			name: "BearerOnly",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer"},
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
			payload, err := server.extractAuthPayload(ctx)

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

func TestGetAuthPayload(t *testing.T) {
	server := newTestServer(t, nil, nil)
	username := "testuser"
	duration := time.Minute

	testCases := []struct {
		name             string
		setupCtx         func() context.Context
		expectPayload    bool
		expectedUsername string
	}{
		{
			name: "PayloadInContext",
			setupCtx: func() context.Context {
				ctx := newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
				payload, err := server.extractAuthPayload(ctx)
				require.NoError(t, err)
				return context.WithValue(ctx, authPayloadKey, payload)
			},
			expectPayload:    true,
			expectedUsername: username,
		},
		{
			name: "NoPayloadInContext",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectPayload: false,
		},
		{
			name: "WrongTypeInContext",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), authPayloadKey, "not-a-payload")
			},
			expectPayload: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.setupCtx()
			payload := GetAuthPayload(ctx)

			if tc.expectPayload {
				require.NotNil(t, payload)
				require.Equal(t, tc.expectedUsername, payload.Username)
			} else {
				require.Nil(t, payload)
			}
		})
	}
}

func TestServer_MustGetAuthPayload(t *testing.T) {
	server := newTestServer(t, nil, nil)
	username := "testuser"
	duration := time.Minute

	testCases := []struct {
		name             string
		setupCtx         func() context.Context
		allowedRoles     []utils.Role
		expectErr        bool
		expectPayload    bool
		expectedUsername string
	}{
		{
			name: "PayloadInContext",
			setupCtx: func() context.Context {
				ctx := newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
				payload, err := server.extractAuthPayload(ctx)
				require.NoError(t, err)
				return context.WithValue(ctx, authPayloadKey, payload)
			},
			allowedRoles:     []utils.Role{utils.RoleClient},
			expectErr:        false,
			expectPayload:    true,
			expectedUsername: username,
		},
		{
			name: "NoPayloadInContext_FallbackToAuthorise",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			allowedRoles:     []utils.Role{utils.RoleClient},
			expectErr:        false,
			expectPayload:    true,
			expectedUsername: username,
		},
		{
			name: "NoPayloadInContext_NoToken",
			setupCtx: func() context.Context {
				return context.Background()
			},
			allowedRoles:  []utils.Role{utils.RoleClient},
			expectErr:     true,
			expectPayload: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.setupCtx()
			payload, err := server.MustGetAuthPayload(ctx, tc.allowedRoles)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, payload)
			} else {
				require.NoError(t, err)
				require.NotNil(t, payload)
				require.Equal(t, tc.expectedUsername, payload.Username)
			}
		})
	}
}

func TestServer_RequireRolesHTTP(t *testing.T) {
	server := newTestServer(t, nil, nil)
	username := "testuser"
	duration := time.Minute

	mapping, err := HTTPPathToGRPCMethodMap()
	require.NoError(t, err)

	var updateUserPath string
	for path, method := range mapping {
		if method == RouteUpdateUser {
			updateUserPath = path
			break
		}
	}

	testCases := []struct {
		name          string
		config        RoleConfig
		path          string
		setupRequest  func() *http.Request
		expectStatus  int
		expectPayload bool
	}{
		{
			name:   "PathNotInMapping_AllowThrough",
			config: NewRoleConfig().RequireAuth([]utils.Role{}, "/test.Method"),
			path:   "/swagger/index.html",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/swagger/index.html", nil)
			},
			expectStatus:  http.StatusOK,
			expectPayload: false,
		},
		{
			name:   "MethodNotInConfig_NoAuthRequired",
			config: NewRoleConfig(),
			path:   "/unknown/path",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/unknown/path", nil)
			},
			expectStatus:  http.StatusOK,
			expectPayload: false,
		},
	}

	if updateUserPath != "" {
		updateUserTests := []struct {
			name          string
			config        RoleConfig
			setupRequest  func() *http.Request
			expectStatus  int
			expectPayload bool
		}{
			{
				name:   "MethodRequiresAuth_ValidToken_AnyRole",
				config: NewRoleConfig().RequireAuth([]utils.Role{}, RouteUpdateUser),
				setupRequest: func() *http.Request {
					req := httptest.NewRequest("PATCH", updateUserPath, nil)
					accessToken, _, err := server.tokenMaker.CreateToken(username, []utils.Role{utils.RoleClient}, duration)
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+accessToken)
					return req
				},
				expectStatus:  http.StatusOK,
				expectPayload: true,
			},
			{
				name:   "MethodRequiresAuth_NoToken",
				config: NewRoleConfig().RequireAuth([]utils.Role{}, RouteUpdateUser),
				setupRequest: func() *http.Request {
					return httptest.NewRequest("PATCH", updateUserPath, nil)
				},
				expectStatus:  http.StatusUnauthorized,
				expectPayload: false,
			},
			{
				name:   "MethodRequiresAuth_InvalidToken",
				config: NewRoleConfig().RequireAuth([]utils.Role{}, RouteUpdateUser),
				setupRequest: func() *http.Request {
					req := httptest.NewRequest("PATCH", updateUserPath, nil)
					req.Header.Set("Authorization", "Bearer invalid-token")
					return req
				},
				expectStatus:  http.StatusUnauthorized,
				expectPayload: false,
			},
			{
			name:   "MethodRequiresAdminRole_ValidToken_ClientRole",
				config: NewRoleConfig().RequireAdminRole(RouteUpdateUser),
				setupRequest: func() *http.Request {
					req := httptest.NewRequest("PATCH", updateUserPath, nil)
					accessToken, _, err := server.tokenMaker.CreateToken(username, []utils.Role{utils.RoleClient}, duration)
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+accessToken)
					return req
				},
				expectStatus:  http.StatusForbidden,
				expectPayload: false,
			},
			{
				name:   "MethodRequiresAdminRole_ValidToken_AdminRole",
				config: NewRoleConfig().RequireAdminRole(RouteUpdateUser),
				setupRequest: func() *http.Request {
					req := httptest.NewRequest("PATCH", updateUserPath, nil)
					accessToken, _, err := server.tokenMaker.CreateToken(username, []utils.Role{utils.RoleAdmin}, duration)
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+accessToken)
					return req
				},
				expectStatus:  http.StatusOK,
				expectPayload: true,
			},
			{
				name:   "MethodRequiresAuth_InvalidHeaderFormat",
				config: NewRoleConfig().RequireAuth([]utils.Role{}, RouteUpdateUser),
				setupRequest: func() *http.Request {
					req := httptest.NewRequest("PATCH", updateUserPath, nil)
					req.Header.Set("Authorization", "invalid")
					return req
				},
				expectStatus:  http.StatusUnauthorized,
				expectPayload: false,
			},
			{
				name:   "MethodRequiresAuth_UnsupportedAuthType",
				config: NewRoleConfig().RequireAuth([]utils.Role{}, RouteUpdateUser),
				setupRequest: func() *http.Request {
					req := httptest.NewRequest("PATCH", updateUserPath, nil)
					req.Header.Set("Authorization", "Basic token123")
					return req
				},
				expectStatus:  http.StatusUnauthorized,
				expectPayload: false,
			},
		}

		for _, ut := range updateUserTests {
			testCases = append(testCases, struct {
				name          string
				config        RoleConfig
				path          string
				setupRequest  func() *http.Request
				expectStatus  int
				expectPayload bool
			}{
				name:          ut.name,
				config:        ut.config,
				path:          updateUserPath,
				setupRequest:  ut.setupRequest,
				expectStatus:  ut.expectStatus,
				expectPayload: ut.expectPayload,
			})
		}
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			middleware := server.RequireRolesHTTP(tc.config)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload := GetAuthPayload(r.Context())
				if tc.expectPayload {
					require.NotNil(t, payload)
					require.Equal(t, username, payload.Username)
				} else {
					require.Nil(t, payload)
				}
				w.WriteHeader(http.StatusOK)
			})

			req := tc.setupRequest()
			rr := httptest.NewRecorder()

			middleware(handler).ServeHTTP(rr, req)

			require.Equal(t, tc.expectStatus, rr.Code)
		})
	}
}

func TestHttpError(t *testing.T) {
	testCases := []struct {
		name         string
		code         codes.Code
		message      string
		expectStatus int
	}{
		{
			name:         "Unauthenticated",
			code:         codes.Unauthenticated,
			message:      "test error",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "PermissionDenied",
			code:         codes.PermissionDenied,
			message:      "test error",
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "InvalidArgument",
			code:         codes.InvalidArgument,
			message:      "test error",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "Internal",
			code:         codes.Internal,
			message:      "test error",
			expectStatus: http.StatusInternalServerError,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			httpError(rr, tc.code, tc.message)

			require.Equal(t, tc.expectStatus, rr.Code)
			require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			require.Contains(t, rr.Body.String(), tc.message)
			require.Contains(t, rr.Body.String(), tc.code.String())
		})
	}
}

func TestExtractMethodFromGatewayContext(t *testing.T) {
	testCases := []struct {
		name          string
		path          string
		shouldBeEmpty bool
	}{
		{
			name:          "SwaggerPath_ShouldBeEmpty",
			path:          "/swagger/index.html",
			shouldBeEmpty: true,
		},
		{
			name:          "UnknownPath_ShouldBeEmpty",
			path:          "/unknown/path",
			shouldBeEmpty: true,
		},
		{
			name:          "EmptyPath_ShouldBeEmpty",
			path:          "",
			shouldBeEmpty: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			result := extractMethodFromGatewayContext(tc.path)
			if tc.shouldBeEmpty {
				require.Empty(t, result)
			} else {
				require.NotEmpty(t, result)
			}
		})
	}

	mapping, err := HTTPPathToGRPCMethodMap()
	if err == nil && len(mapping) > 0 {
		var testPath string
		for path := range mapping {
			testPath = path
			break
		}

		t.Run("ValidPath_ShouldReturnMethod", func(t *testing.T) {
			result := extractMethodFromGatewayContext(testPath)
			require.NotEmpty(t, result)
			require.Equal(t, mapping[testPath], result)
		})
	}
}

func TestAuthoriseUser(t *testing.T) {
	server := newTestServer(t, nil, nil)
	username := "testuser"
	duration := time.Minute

	testCases := []struct {
		name         string
		setupCtx     func() context.Context
		allowedRoles []utils.Role
		expectErr    bool
	}{
		{
			name: "OK",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    false,
		},
		{
			name: "NoMetadata",
			setupCtx: func() context.Context {
				return context.Background()
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "NoAuthorizationHeader",
			setupCtx: func() context.Context {
				md := metadata.MD{}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "InvalidHeaderFormat",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"invalid"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "UnsupportedAuthType",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"Basic token123"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "InvalidToken",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer invalid-token"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "ExpiredToken",
			setupCtx: func() context.Context {
				accessToken, _, err := server.tokenMaker.CreateToken(username, []utils.Role{utils.RoleClient}, -time.Hour)
				require.NoError(t, err)
				bearerToken := "bearer " + accessToken
				md := metadata.MD{
					authorizationHeader: []string{bearerToken},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "EmptyTokenAfterBearer",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer "},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "BearerOnly",
			setupCtx: func() context.Context {
				md := metadata.MD{
					authorizationHeader: []string{"bearer"},
				}
				return metadata.NewIncomingContext(context.Background(), md)
			},
			allowedRoles: []utils.Role{utils.RoleClient, utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "PermissionDenied_MemberRoleNotAllowed",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleClient}, duration)
			},
			allowedRoles: []utils.Role{utils.RoleAdmin},
			expectErr:    true,
		},
		{
			name: "PermissionDenied_AdminRoleNotAllowed",
			setupCtx: func() context.Context {
				return newContextWithBearerToken(t, server.tokenMaker, username, []utils.Role{utils.RoleAdmin}, duration)
			},
			allowedRoles: []utils.Role{utils.RoleClient},
			expectErr:    true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.setupCtx()
			payload, err := server.authoriseUser(ctx, tc.allowedRoles)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, payload)
				if tc.name == "PermissionDenied_MemberRoleNotAllowed" || tc.name == "PermissionDenied_AdminRoleNotAllowed" {
					require.Contains(t, err.Error(), "permission denied")
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, payload)
				require.Equal(t, username, payload.Username)
			}
		})
	}
}
