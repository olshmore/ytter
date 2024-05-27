package api

import (
	"context"

	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (server *Server) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	users, err := server.store.ListUsers(ctx, db.ListUsersParams{
		Limit:  req.GetLimit(),
		Offset: req.GetOffset(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %s", err)
	}

	res := &pb.ListUsersResponse{
		Users: ConvertUsers(users),
	}

	return res, nil
}
