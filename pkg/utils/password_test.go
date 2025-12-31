package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hashedPassword)
	require.NotEqual(t, password, hashedPassword)

	// Verify it's a valid bcrypt hash
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	require.NoError(t, err)
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword123"
	wrongPassword := "wrongpassword"

	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		password       string
		hashedPassword string
		expectError    bool
	}{
		{
			name:           "CorrectPassword",
			password:       password,
			hashedPassword: hashedPassword,
			expectError:    false,
		},
		{
			name:           "IncorrectPassword",
			password:       wrongPassword,
			hashedPassword: hashedPassword,
			expectError:    true,
		},
		{
			name:           "EmptyPassword",
			password:       "",
			hashedPassword: hashedPassword,
			expectError:    true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := CheckPassword(tc.password, tc.hashedPassword)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
