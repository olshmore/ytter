package token

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNewPayload(t *testing.T) {
	username := "testuser"
	duration := time.Hour

	payload, err := NewPayload(username, duration)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, username, payload.Username)
	require.NotEqual(t, uuid.Nil, payload.ID)
	require.WithinDuration(t, time.Now(), payload.IssuedAt, time.Second)
	require.WithinDuration(t, time.Now().Add(duration), payload.ExpiredAt, time.Second)
}

func TestPayload_Valid(t *testing.T) {
	testCases := []struct {
		name      string
		payload   *Payload
		expectErr bool
	}{
		{
			name: "ValidPayload",
			payload: &Payload{
				ID:        uuid.New(),
				Username:  "testuser",
				IssuedAt:  time.Now(),
				ExpiredAt: time.Now().Add(time.Hour),
			},
			expectErr: false,
		},
		{
			name: "ExpiredPayload",
			payload: &Payload{
				ID:        uuid.New(),
				Username:  "testuser",
				IssuedAt:  time.Now().Add(-2 * time.Hour),
				ExpiredAt: time.Now().Add(-1 * time.Hour),
			},
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := tc.payload.Valid()
			if tc.expectErr {
				require.Error(t, err)
				require.Equal(t, ErrExpiredToken, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
