package api

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFieldViolation(t *testing.T) {
	field := "username"
	err := errors.New("username is required")

	violation := fieldViolation(field, err)

	require.NotNil(t, violation)
	require.Equal(t, field, violation.Field)
	require.Equal(t, err.Error(), violation.Description)
}

func TestInvalidArgumentError(t *testing.T) {
	violations := []*errdetails.BadRequest_FieldViolation{
		fieldViolation("username", errors.New("username is required")),
		fieldViolation("email", errors.New("email is invalid")),
	}

	err := invalidArgumentError(violations)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	details := st.Details()
	require.Len(t, details, 1)

	badRequest, ok := details[0].(*errdetails.BadRequest)
	require.True(t, ok)
	require.Len(t, badRequest.FieldViolations, 2)
}

func TestUnauthenticatedError(t *testing.T) {
	err := errors.New("token expired")
	authErr := unauthenticatedError(err)

	require.Error(t, authErr)
	st, ok := status.FromError(authErr)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
	require.Contains(t, st.Message(), "unauthorized")
}
