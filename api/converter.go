package api

import (
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConvertUser(user db.User) *pb.User {
	return &pb.User{
		Id:                user.ID,
		Username:          user.Username,
		FirstName:         user.FirstName,
		LastName:          user.LastName,
		Email:             user.Email,
		PasswordChangedAt: timestamppb.New(user.PasswordChangedAt),
		CreatedAt:         timestamppb.New(user.CreatedAt),
		UpdatedAt:         timestamppb.New(user.UpdatedAt),
		DeletedAt:         timestamppb.New(user.DeletedAt),
		Role:              user.Role,
	}
}

func ConvertUsers(users []db.User) []*pb.User {
	pbUsers := make([]*pb.User, len(users))

	for i, user := range users {
		pbUsers[i] = ConvertUser(user)
	}

	return pbUsers
}
