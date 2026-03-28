package validator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateString(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		minLength int
		maxLength int
		expectErr bool
	}{
		{
			name:      "ValidString",
			value:     "hello",
			minLength: 3,
			maxLength: 10,
			expectErr: false,
		},
		{
			name:      "TooShort",
			value:     "hi",
			minLength: 3,
			maxLength: 10,
			expectErr: true,
		},
		{
			name:      "TooLong",
			value:     "this is too long",
			minLength: 3,
			maxLength: 10,
			expectErr: true,
		},
		{
			name:      "ExactMin",
			value:     "abc",
			minLength: 3,
			maxLength: 10,
			expectErr: false,
		},
		{
			name:      "ExactMax",
			value:     "1234567890",
			minLength: 3,
			maxLength: 10,
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateString(tc.value, tc.minLength, tc.maxLength)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	testCases := []struct {
		name      string
		username  string
		expectErr bool
	}{
		{
			name:      "ValidUsername",
			username:  "john_doe123",
			expectErr: false,
		},
		{
			name:      "TooShort",
			username:  "ab",
			expectErr: true,
		},
		{
			name:      "ContainsUppercase",
			username:  "JohnDoe",
			expectErr: true,
		},
		{
			name:      "ContainsSpecialChars",
			username:  "john-doe",
			expectErr: true,
		},
		{
			name:      "ContainsSpaces",
			username:  "john doe",
			expectErr: true,
		},
		{
			name:      "ValidWithUnderscore",
			username:  "john_doe",
			expectErr: false,
		},
		{
			name:      "ValidWithNumbers",
			username:  "john123",
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUsername(tc.username)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		expectErr bool
	}{
		{
			name:      "ValidName",
			value:     "John Doe",
			expectErr: false,
		},
		{
			name:      "ValidNameNoSpaces",
			value:     "John",
			expectErr: false,
		},
		{
			name:      "ValidSingleLetter",
			value:     "J",
			expectErr: false,
		},
		{
			name:      "EmptyString",
			value:     "",
			expectErr: true,
		},
		{
			name:      "ContainsNumbers",
			value:     "John123",
			expectErr: true,
		},
		{
			name:      "ContainsSpecialChars",
			value:     "John-Doe",
			expectErr: true,
		},
		{
			name:      "ValidWithSpaces",
			value:     "John Michael Doe",
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateName(tc.value)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	testCases := []struct {
		name      string
		password  string
		expectErr bool
	}{
		{
			name:      "ValidPassword",
			password:  "password123",
			expectErr: false,
		},
		{
			name:      "TooShort",
			password:  "pass",
			expectErr: true,
		},
		{
			name:      "ExactMinLength",
			password:  "passwo",
			expectErr: false,
		},
		{
			name:      "LongPassword",
			password:  "this is a very long password that exceeds normal limits",
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.password)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	testCases := []struct {
		name      string
		email     string
		expectErr bool
	}{
		{
			name:      "ValidEmail",
			email:     "john@example.com",
			expectErr: false,
		},
		{
			name:      "InvalidEmail",
			email:     "invalid-email",
			expectErr: true,
		},
		{
			name:      "TooShort",
			email:     "ab",
			expectErr: true,
		},
		{
			name:      "NoAtSymbol",
			email:     "johnexample.com",
			expectErr: true,
		},
		{
			name:      "ValidEmailWithSubdomain",
			email:     "john@mail.example.com",
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEmailVerificationToken(t *testing.T) {
	testCases := []struct {
		name      string
		token     string
		expectErr bool
	}{
		{
			name:      "ValidUUID",
			token:     "123e4567-e89b-12d3-a456-426614174000",
			expectErr: false,
		},
		{
			name:      "InvalidUUID",
			token:     "invalid-token",
			expectErr: true,
		},
		{
			name:      "EmptyToken",
			token:     "",
			expectErr: true,
		},
		{
			name:      "ValidUUIDUppercase",
			token:     "123E4567-E89B-12D3-A456-426614174000",
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmailVerificationToken(tc.token)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateInt32(t *testing.T) {
	testCases := []struct {
		name      string
		value     int32
		expectErr bool
	}{
		{
			name:      "ValidZero",
			value:     0,
			expectErr: false,
		},
		{
			name:      "ValidPositive",
			value:     100,
			expectErr: false,
		},
		{
			name:      "InvalidNegative",
			value:     -1,
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInt32(tc.value)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateInt64(t *testing.T) {
	testCases := []struct {
		name      string
		value     int64
		expectErr bool
	}{
		{
			name:      "ValidPositive",
			value:     100,
			expectErr: false,
		},
		{
			name:      "InvalidZero",
			value:     0,
			expectErr: true,
		},
		{
			name:      "InvalidNegative",
			value:     -1,
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInt64(tc.value)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateStringLength(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		expectErr bool
	}{
		{
			name:      "ValidLength",
			value:     "this is exactly 32 characters long!",
			expectErr: false,
		},
		{
			name:      "TooShort",
			value:     "short",
			expectErr: true,
		},
		{
			name:      "TooLong",
			value:     "this is a very long string that exceeds the maximum length of 128 characters and should fail validation because it's way too long and keeps going",
			expectErr: true,
		},
		{
			name:      "ExactMin",
			value:     "12345678901234567890123456789012",
			expectErr: false,
		},
		{
			name:      "ExactMax",
			value:     "12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678",
			expectErr: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStringLength(tc.value)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateRole(t *testing.T) {
	testCases := []struct {
		name      string
		role      string
		expectErr bool
	}{
		{
			name:      "ValidAdmin",
			role:      "admin",
			expectErr: false,
		},
		{
			name:      "ValidHost",
			role:      "host",
			expectErr: false,
		},
		{
			name:      "ValidClient",
			role:      "client",
			expectErr: false,
		},
		{
			name:      "InvalidRole",
			role:      "user",
			expectErr: true,
		},
		{
			name:      "EmptyRole",
			role:      "",
			expectErr: true,
		},
		{
			name:      "InvalidRoleModerator",
			role:      "moderator",
			expectErr: true,
		},
		{
			name:      "CaseSensitiveAdmin",
			role:      "Admin",
			expectErr: true,
		},
		{
			name:      "CaseSensitiveHost",
			role:      "Host",
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRole(tc.role)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
