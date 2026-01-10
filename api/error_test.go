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
	testCases := []struct {
		name       string
		violations []*errdetails.BadRequest_FieldViolation
	}{
		{
			name: "MultipleViolations",
			violations: []*errdetails.BadRequest_FieldViolation{
				fieldViolation("username", errors.New("username is required")),
				fieldViolation("email", errors.New("email is invalid")),
			},
		},
		{
			name: "SingleViolation",
			violations: []*errdetails.BadRequest_FieldViolation{
				fieldViolation("username", errors.New("username is required")),
			},
		},
		{
			name:       "EmptyViolations",
			violations: []*errdetails.BadRequest_FieldViolation{},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := invalidArgumentError(tc.violations)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, codes.InvalidArgument, st.Code())

			if len(tc.violations) > 0 {
				details := st.Details()
				require.Len(t, details, 1)

				badRequest, ok := details[0].(*errdetails.BadRequest)
				require.True(t, ok)
				require.Len(t, badRequest.FieldViolations, len(tc.violations))
			}
		})
	}
}

func TestUnauthenticatedError(t *testing.T) {
	err := errors.New("token expired")
	authErr := unauthenticatedError(err)

	require.Error(t, authErr)
	st, ok := status.FromError(authErr)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
	require.Contains(t, st.Message(), "unauthenticated")
}
