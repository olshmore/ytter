package api

import (
	"testing"

	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestConfigureRoleBasedAccess(t *testing.T) {
	config := ConfigureRoleBasedAccess()

	require.NotNil(t, config)

	// Check that UpdateUser requires auth (any role)
	roles, exists := config[RouteUpdateUser]
	require.True(t, exists)
	require.Empty(t, roles) // Empty means any authenticated user

	// Check that ListUsers requires admin role
	roles, exists = config[RouteListUsers]
	require.True(t, exists)
	require.Len(t, roles, 1)
	require.Equal(t, utils.RoleAdmin, roles[0])

	// Check that public routes are not in config
	_, exists = config[RouteCreateUser]
	require.False(t, exists)

	_, exists = config[RouteLoginUser]
	require.False(t, exists)

	_, exists = config[RouteVerifyEmail]
	require.False(t, exists)

	_, exists = config[RouteRefreshToken]
	require.False(t, exists)
}

func TestHTTPPathToGRPCMethodMap(t *testing.T) {
	mapping, err := HTTPPathToGRPCMethodMap()

	require.NoError(t, err)
	require.NotNil(t, mapping)
	require.NotEmpty(t, mapping)

	// Verify that all expected routes are in the mapping
	expectedMethods := []string{
		RouteCreateUser,
		RouteLoginUser,
		RouteVerifyEmail,
		RouteRefreshToken,
		RouteListUsers,
		RouteUpdateUser,
	}

	for _, method := range expectedMethods {
		// Find the path that maps to this method
		found := false
		for _, mappedMethod := range mapping {
			if mappedMethod == method {
				found = true
				break
			}
		}
		require.True(t, found, "Method %s should be in mapping", method)
	}
}

func TestExtractMethodFromGatewayContext_ErrorCase(t *testing.T) {
	result := extractMethodFromGatewayContext("/nonexistent/path")
	require.Empty(t, result)
}
