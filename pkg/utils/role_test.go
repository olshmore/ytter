package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRole_String(t *testing.T) {
	testCases := []struct {
		name     string
		role     Role
		expected string
	}{
		{
			name:     "AdminRole",
			role:     RoleAdmin,
			expected: "admin",
		},
		{
			name:     "MemberRole",
			role:     RoleMember,
			expected: "member",
		},
		{
			name:     "EmptyRole",
			role:     Role(""),
			expected: "",
		},
		{
			name:     "CustomRole",
			role:     Role("custom"),
			expected: "custom",
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			result := tc.role.String()
			require.Equal(t, tc.expected, result)
		})
	}
}
