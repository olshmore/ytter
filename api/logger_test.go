package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGrpcLogger(t *testing.T) {
	testCases := []struct {
		name      string
		handler   grpc.UnaryHandler
		expectErr bool
	}{
		{
			name: "SuccessfulRequest",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				return "success", nil
			},
			expectErr: false,
		},
		{
			name: "ErrorRequest",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				return nil, status.Errorf(codes.Internal, "internal error")
			},
			expectErr: true,
		},
		{
			name: "UnknownError",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				return nil, status.Errorf(codes.Unknown, "unknown error")
			},
			expectErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			info := &grpc.UnaryServerInfo{
				FullMethod: "/test.Method",
			}

			result, err := GrpcLogger(
				context.Background(),
				"test request",
				info,
				tc.handler,
			)

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

func TestHttpLogger(t *testing.T) {
	testCases := []struct {
		name           string
		handler        http.HandlerFunc
		expectedStatus int
	}{
		{
			name: "SuccessfulRequest",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "NotFoundRequest",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("not found"))
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "InternalServerError",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			loggedHandler := HttpLogger(tc.handler)
			loggedHandler.ServeHTTP(rec, req)

			require.Equal(t, tc.expectedStatus, rec.Code)
		})
	}
}

func TestResponseRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	responseRecorder := &ResponseRecorder{
		ResponseWriter: rec,
		StatusCode:     http.StatusOK,
	}

	// Test WriteHeader
	responseRecorder.WriteHeader(http.StatusCreated)
	require.Equal(t, http.StatusCreated, responseRecorder.StatusCode)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Test Write
	body := []byte("test body")
	n, err := responseRecorder.Write(body)
	require.NoError(t, err)
	require.Equal(t, len(body), n)
	require.Equal(t, body, responseRecorder.Body)
	require.Equal(t, body, rec.Body.Bytes())
}
