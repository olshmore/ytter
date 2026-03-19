package api

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVerifyEmailAPI(t *testing.T) {
	user, _ := randomUser(t)
	verificationToken := uuid.New().String()

	testCases := []struct {
		name          string
		req           *pb.VerifyEmailRequest
		buildStubs    func(store *mockdb.MockStore)
		checkResponse func(t *testing.T, res *pb.VerifyEmailResponse, err error)
	}{
		{
			name: "OK",
			req: &pb.VerifyEmailRequest{
				VerificationToken: verificationToken,
			},
			buildStubs: func(store *mockdb.MockStore) {
				user.IsEmailVerified = true
				store.EXPECT().
					VerifyEmailTx(gomock.Any(), gomock.Eq(db.VerifyEmailTxParams{
						VerificationToken: verificationToken,
					})).
					Times(1).
					Return(db.VerifyEmailTxResult{
						User: user,
					}, nil)
			},
			checkResponse: func(t *testing.T, res *pb.VerifyEmailResponse, err error) {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.True(t, res.GetIsVerified())
			},
		},
		{
			name: "InvalidToken",
			req: &pb.VerifyEmailRequest{
				VerificationToken: "invalid-token",
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().
					VerifyEmailTx(gomock.Any(), gomock.Any()).
					Times(0)
			},
			checkResponse: func(t *testing.T, res *pb.VerifyEmailResponse, err error) {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
		{
			name: "EmptyToken",
			req: &pb.VerifyEmailRequest{
				VerificationToken: "",
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().
					VerifyEmailTx(gomock.Any(), gomock.Any()).
					Times(0)
			},
			checkResponse: func(t *testing.T, res *pb.VerifyEmailResponse, err error) {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
		{
			name: "InternalError",
			req: &pb.VerifyEmailRequest{
				VerificationToken: verificationToken,
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().
					VerifyEmailTx(gomock.Any(), gomock.Eq(db.VerifyEmailTxParams{
						VerificationToken: verificationToken,
					})).
					Times(1).
					Return(db.VerifyEmailTxResult{}, sql.ErrConnDone)
			},
			checkResponse: func(t *testing.T, res *pb.VerifyEmailResponse, err error) {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.Internal, st.Code())
			},
		},
		{
			name: "RecordNotFound",
			req: &pb.VerifyEmailRequest{
				VerificationToken: verificationToken,
			},
			buildStubs: func(store *mockdb.MockStore) {
				store.EXPECT().
					VerifyEmailTx(gomock.Any(), gomock.Eq(db.VerifyEmailTxParams{
						VerificationToken: verificationToken,
					})).
					Times(1).
					Return(db.VerifyEmailTxResult{}, db.ErrRecordNotFound)
			},
			checkResponse: func(t *testing.T, res *pb.VerifyEmailResponse, err error) {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
		{
			name: "EmailNotVerified",
			req: &pb.VerifyEmailRequest{
				VerificationToken: verificationToken,
			},
			buildStubs: func(store *mockdb.MockStore) {
				user.IsEmailVerified = false
				store.EXPECT().
					VerifyEmailTx(gomock.Any(), gomock.Eq(db.VerifyEmailTxParams{
						VerificationToken: verificationToken,
					})).
					Times(1).
					Return(db.VerifyEmailTxResult{
						User: user,
					}, nil)
			},
			checkResponse: func(t *testing.T, res *pb.VerifyEmailResponse, err error) {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.False(t, res.GetIsVerified())
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

			res, err := server.VerifyEmail(context.Background(), tc.req)
			tc.checkResponse(t, res, err)
		})
	}
}