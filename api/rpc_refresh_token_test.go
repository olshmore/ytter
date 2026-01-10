package api

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/token"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRefreshTokenAPI(t *testing.T) {
	user, _ := randomUser(t)

	testCases := []struct {
		name          string
		setupToken    func(tokenMaker token.Maker) (string, *token.Payload)
		buildStubs    func(store *mockdb.MockStore, payload *token.Payload, refreshToken string)
		checkResponse func(t *testing.T, res *pb.RefreshTokenResponse, err error)
	}{
		{
			name: "OK",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				refreshToken, payload, _ := tokenMaker.CreateToken(user.Username, utils.Role(user.Role), time.Minute)
				return refreshToken, payload
			},
			buildStubs: func(store *mockdb.MockStore, payload *token.Payload, refreshToken string) {
				session := db.Session{
					ID:           payload.ID,
					Username:     payload.Username,
					RefreshToken: refreshToken,
					IsBlocked:    false,
					ExpiresAt:    time.Now().Add(time.Minute),
				}

				store.EXPECT().
					GetSession(gomock.Any(), gomock.Eq(payload.ID)).
					Times(1).
					Return(session, nil)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.NotEmpty(t, res.AccessToken)
				require.WithinDuration(t, time.Now().Add(time.Minute), res.AccessTokenExpiresAt.AsTime(), time.Second*5)
			},
		},
		{
			name: "InvalidToken",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				return "invalid-token", nil
			},
			buildStubs: func(store *mockdb.MockStore, _ *token.Payload, _ string) {
				store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Times(0)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.Error(t, err)
				st, _ := status.FromError(err)
				require.Equal(t, codes.Unauthenticated, st.Code())
			},
		},
		{
			name: "SessionNotFound",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				refreshToken, payload, _ := tokenMaker.CreateToken(user.Username, utils.Role(user.Role), time.Minute)
				return refreshToken, payload
			},
			buildStubs: func(store *mockdb.MockStore, payload *token.Payload, _ string) {
				store.EXPECT().
					GetSession(gomock.Any(), gomock.Eq(payload.ID)).
					Times(1).
					Return(db.Session{}, db.ErrRecordNotFound)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.Error(t, err)
				st, _ := status.FromError(err)
				require.Equal(t, codes.NotFound, st.Code())
			},
		},
		{
			name: "BlockedSession",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				refreshToken, payload, _ := tokenMaker.CreateToken(user.Username, utils.Role(user.Role), time.Minute)
				return refreshToken, payload
			},
			buildStubs: func(store *mockdb.MockStore, payload *token.Payload, refreshToken string) {
				session := db.Session{
					ID:           payload.ID,
					Username:     payload.Username,
					RefreshToken: refreshToken,
					IsBlocked:    true,
					ExpiresAt:    time.Now().Add(time.Minute),
				}
				store.EXPECT().
					GetSession(gomock.Any(), gomock.Eq(payload.ID)).
					Times(1).
					Return(session, nil)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.Error(t, err)
				st, _ := status.FromError(err)
				require.Equal(t, codes.Unauthenticated, st.Code())
			},
		},
		{
			name: "UsernameMismatch",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				refreshToken, payload, _ := tokenMaker.CreateToken(user.Username, utils.Role(user.Role), time.Minute)
				return refreshToken, payload
			},
			buildStubs: func(store *mockdb.MockStore, payload *token.Payload, refreshToken string) {
				session := db.Session{
					ID:           payload.ID,
					Username:     "someone_else",
					RefreshToken: refreshToken,
					IsBlocked:    false,
					ExpiresAt:    time.Now().Add(time.Minute),
				}
				store.EXPECT().
					GetSession(gomock.Any(), gomock.Eq(payload.ID)).
					Times(1).
					Return(session, nil)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.Error(t, err)
				st, _ := status.FromError(err)
				require.Equal(t, codes.Unauthenticated, st.Code())
			},
		},
		{
			name: "TokenMismatch",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				refreshToken, payload, _ := tokenMaker.CreateToken(user.Username, utils.Role(user.Role), time.Minute)
				return refreshToken, payload
			},
			buildStubs: func(store *mockdb.MockStore, payload *token.Payload, _ string) {
				session := db.Session{
					ID:           payload.ID,
					Username:     payload.Username,
					RefreshToken: "another-token",
					IsBlocked:    false,
					ExpiresAt:    time.Now().Add(time.Minute),
				}
				store.EXPECT().
					GetSession(gomock.Any(), gomock.Eq(payload.ID)).
					Times(1).
					Return(session, nil)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.Error(t, err)
				st, _ := status.FromError(err)
				require.Equal(t, codes.Unauthenticated, st.Code())
			},
		},
		{
			name: "SessionExpired",
			setupToken: func(tokenMaker token.Maker) (string, *token.Payload) {
				refreshToken, payload, _ := tokenMaker.CreateToken(user.Username, utils.Role(user.Role), time.Minute)
				return refreshToken, payload
			},
			buildStubs: func(store *mockdb.MockStore, payload *token.Payload, refreshToken string) {
				session := db.Session{
					ID:           payload.ID,
					Username:     payload.Username,
					RefreshToken: refreshToken,
					IsBlocked:    false,
					ExpiresAt:    time.Now().Add(-time.Minute),
				}
				store.EXPECT().
					GetSession(gomock.Any(), gomock.Eq(payload.ID)).
					Times(1).
					Return(session, nil)
			},
			checkResponse: func(t *testing.T, res *pb.RefreshTokenResponse, err error) {
				require.Error(t, err)
				st, _ := status.FromError(err)
				require.Equal(t, codes.Unauthenticated, st.Code())
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			storeCtrl := gomock.NewController(t)
			defer storeCtrl.Finish()
			store := mockdb.NewMockStore(storeCtrl)

			server := newTestServer(t, store, nil)

			refreshToken, payload := tc.setupToken(server.tokenMaker)
			tc.buildStubs(store, payload, refreshToken)

			req := &pb.RefreshTokenRequest{
				RefreshToken: refreshToken,
			}

			res, err := server.RefreshToken(context.Background(), req)
			tc.checkResponse(t, res, err)
		})
	}
}
