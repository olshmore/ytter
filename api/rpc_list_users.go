package api

import (
	"context"

	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/validator"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (server *Server) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	violations := validateListUsersRequest(req)
	if violations != nil {
		return nil, invalidArgumentError(violations)
	}

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

func validateListUsersRequest(req *pb.ListUsersRequest) (violations []*errdetails.BadRequest_FieldViolation) {
	if err := validator.ValidateInt32(req.GetLimit()); err != nil {
		violations = append(violations, fieldViolation("limit", err))
	}

	if err := validator.ValidateInt32(req.GetOffset()); err != nil {
		violations = append(violations, fieldViolation("offset", err))
	}

	return violations
}
