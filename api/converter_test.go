package api

import (
	"testing"
	"time"

	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestConvertUser(t *testing.T) {
	now := time.Now()
	user := db.User{
		ID:                1,
		Username:          "testuser",
		FirstName:         "John",
		LastName:          "Doe",
		Email:             "john@example.com",
		HashedPassword:    "hashedpassword",
		PasswordChangedAt: now,
		CreatedAt:         now,
		UpdatedAt:         now,
		DeletedAt:         time.Time{},
		IsEmailVerified:   true,
	}

	pbUser := ConvertUser(user)

	require.NotNil(t, pbUser)
	require.Equal(t, user.Username, pbUser.Username)
	require.Equal(t, user.FirstName, pbUser.FirstName)
	require.Equal(t, user.LastName, pbUser.LastName)
	require.Equal(t, user.Email, pbUser.Email)
	require.Equal(t, timestamppb.New(user.PasswordChangedAt).AsTime(), pbUser.PasswordChangedAt.AsTime())
	require.Equal(t, timestamppb.New(user.CreatedAt).AsTime(), pbUser.CreatedAt.AsTime())
	require.Equal(t, timestamppb.New(user.UpdatedAt).AsTime(), pbUser.UpdatedAt.AsTime())
}

func TestConvertUsers(t *testing.T) {
	now := time.Now()
	users := []db.User{
		{
			ID:                1,
			Username:          "user1",
			FirstName:         "John",
			LastName:          "Doe",
			Email:             "john@example.com",
			HashedPassword:    "hashedpassword1",
			PasswordChangedAt: now,
			CreatedAt:         now,
			UpdatedAt:         now,
			DeletedAt:         time.Time{},
			IsEmailVerified:   true,
		},
		{
			ID:                2,
			Username:          "user2",
			FirstName:         "Jane",
			LastName:          "Smith",
			Email:             "jane@example.com",
			HashedPassword:    "hashedpassword2",
			PasswordChangedAt: now,
			CreatedAt:         now,
			UpdatedAt:         now,
			DeletedAt:         time.Time{},
			IsEmailVerified:   false,
		},
	}

	pbUsers := ConvertUsers(users)

	require.Len(t, pbUsers, 2)
	require.Equal(t, users[0].Username, pbUsers[0].Username)
	require.Equal(t, users[1].Username, pbUsers[1].Username)
}

func TestConvertUsers_EmptySlice(t *testing.T) {
	users := []db.User{}
	pbUsers := ConvertUsers(users)
	require.NotNil(t, pbUsers)
	require.Len(t, pbUsers, 0)
}
