package utils

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

func TestGrpcCodeToHTTPStatus(t *testing.T) {
	testCases := []struct {
		name           string
		grpcCode       codes.Code
		expectedStatus int
	}{
		{
			name:           "Unauthenticated",
			grpcCode:       codes.Unauthenticated,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "PermissionDenied",
			grpcCode:       codes.PermissionDenied,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "InvalidArgument",
			grpcCode:       codes.InvalidArgument,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "NotFound",
			grpcCode:       codes.NotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Internal",
			grpcCode:       codes.Internal,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "AlreadyExists",
			grpcCode:       codes.AlreadyExists,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "ResourceExhausted",
			grpcCode:       codes.ResourceExhausted,
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "FailedPrecondition",
			grpcCode:       codes.FailedPrecondition,
			expectedStatus: http.StatusPreconditionFailed,
		},
		{
			name:           "Aborted",
			grpcCode:       codes.Aborted,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "OutOfRange",
			grpcCode:       codes.OutOfRange,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unimplemented",
			grpcCode:       codes.Unimplemented,
			expectedStatus: http.StatusNotImplemented,
		},
		{
			name:           "Unavailable",
			grpcCode:       codes.Unavailable,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "DeadlineExceeded",
			grpcCode:       codes.DeadlineExceeded,
			expectedStatus: http.StatusGatewayTimeout,
		},
		{
			name:           "Canceled",
			grpcCode:       codes.Canceled,
			expectedStatus: http.StatusRequestTimeout,
		},
		{
			name:           "OK_DefaultCase",
			grpcCode:       codes.OK,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Unknown_DefaultCase",
			grpcCode:       codes.Unknown,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			result := GrpcCodeToHTTPStatus(tc.grpcCode)
			require.Equal(t, tc.expectedStatus, result)
		})
	}
}
