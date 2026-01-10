package api

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestListUsersAPI(t *testing.T) {

	testCases := []struct {
		name          string
		req           *pb.ListUsersRequest
		buildStubs    func(store *mockdb.MockStore)
		checkResponse func(t *testing.T, res *pb.ListUsersResponse, err error)
	}{
		{
			name: "OK",
			req: &pb.ListUsersRequest{
				Limit:  10,
				Offset: 0,
			},
			buildStubs: func(store *mockdb.MockStore) {
				arg := db.ListUsersParams{
					Limit:  10,
					Offset: 0,
				}
				store.EXPECT().
					ListUsers(gomock.Any(), gomock.Eq(arg)).
					Times(1).
					Return([]db.User{}, nil)
			},
			checkResponse: func(t *testing.T, res *pb.ListUsersResponse, err error) {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Len(t, res.Users, 0)
			},
		},
		{
			name: "Invalid Limit",
			req: &pb.ListUsersRequest{
				Limit:  -10,
				Offset: 0,
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().ListUsers(gomock.Any(), gomock.Any()).Times(0)
			},
			checkResponse: func(t *testing.T, res *pb.ListUsersResponse, err error) {
				require.Error(t, err)
				require.Equal(t, codes.InvalidArgument, status.Code(err))
			},
		},
		{
			name: "Invalid Offset",
			req: &pb.ListUsersRequest{
				Limit:  10,
				Offset: -1,
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().ListUsers(gomock.Any(), gomock.Any()).Times(0)
			},
			checkResponse: func(t *testing.T, res *pb.ListUsersResponse, err error) {
				require.Error(t, err)
				require.Equal(t, codes.InvalidArgument, status.Code(err))
			},
		},
		{
			name: "Invalid Limit and Offset",
			req: &pb.ListUsersRequest{
				Limit:  -10,
				Offset: -1,
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().ListUsers(gomock.Any(), gomock.Any()).Times(0)
			},
			checkResponse: func(t *testing.T, res *pb.ListUsersResponse, err error) {
				require.Error(t, err)
				require.Equal(t, codes.InvalidArgument, status.Code(err))
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			storeCtrl := gomock.NewController(t)
			defer storeCtrl.Finish()
			store := mockdb.NewMockStore(storeCtrl)

			tc.buildStubs(store)
			server := newTestServer(t, store, nil)

			res, err := server.ListUsers(context.Background(), tc.req)
			tc.checkResponse(t, res, err)
		})
	}
}
