package api

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/storage"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/config"
	"github.com/olshmore/ytter/pkg/token"
	"github.com/olshmore/ytter/pkg/utils"
)

func brandingTestServer(t *testing.T, store db.Store, objectStore storage.ObjectStore) *Server {
	t.Helper()
	server := &Server{
		config: config.Config{
			TokenSymmetricKey:    utils.RandomString(32),
			StorageDriver:        "local",
			StorageLocalDir:      t.TempDir(),
			StoragePublicBaseURL: "http://localhost:8080/v1/public/media",
		},
		store:       store,
		objectStore: objectStore,
	}
	maker, err := token.NewPasetoMaker(server.config.TokenSymmetricKey)
	require.NoError(t, err)
	server.tokenMaker = maker
	return server
}

func TestUpdateLocationBranding_OwnerSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)

	locID := uuid.New()
	current := db.Location{
		ID:            locID,
		OwnerUsername: "host1",
		Slug:          "acme",
		Name:          "Acme",
	}
	updated := current
	updated.PrimaryColor = pgtype.Text{String: "#0F766E", Valid: true}
	updated.FontFamily = pgtype.Text{String: "lora", Valid: true}

	store.EXPECT().GetLocationBySlug(gomock.Any(), "acme").Return(current, nil)
	store.EXPECT().UpdateLocationBranding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.UpdateLocationBrandingParams) (db.Location, error) {
			require.True(t, arg.SetPrimaryColor)
			require.Equal(t, "#0F766E", arg.PrimaryColor.String)
			require.True(t, arg.SetFontFamily)
			require.Equal(t, "lora", arg.FontFamily.String)
			return updated, nil
		})

	server := brandingTestServer(t, store, nil)
	ctx := context.WithValue(context.Background(), authPayloadKey, &token.Payload{
		Username: "host1",
		Roles:    []utils.Role{utils.RoleHost},
	})
	primary := "#0F766E"
	font := "lora"
	res, err := server.UpdateLocationBranding(ctx, &pb.UpdateLocationBrandingRequest{
		LocationSlug: "acme",
		PrimaryColor: &primary,
		FontFamily:   &font,
	})
	require.NoError(t, err)
	require.Equal(t, "#0F766E", res.GetPrimaryColor())
	require.Equal(t, "lora", res.GetFontFamily())
}

func TestUpdateLocationBranding_InvalidColor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	store.EXPECT().GetLocationBySlug(gomock.Any(), "acme").Return(db.Location{
		ID: uuid.New(), OwnerUsername: "host1", Slug: "acme", Name: "Acme",
	}, nil)

	server := brandingTestServer(t, store, nil)
	ctx := context.WithValue(context.Background(), authPayloadKey, &token.Payload{
		Username: "host1",
		Roles:    []utils.Role{utils.RoleHost},
	})
	bad := "red"
	_, err := server.UpdateLocationBranding(ctx, &pb.UpdateLocationBrandingRequest{
		LocationSlug: "acme",
		PrimaryColor: &bad,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestUpdateLocationBranding_PermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	store.EXPECT().GetLocationBySlug(gomock.Any(), "acme").Return(db.Location{
		ID: uuid.New(), OwnerUsername: "host1", Slug: "acme", Name: "Acme",
	}, nil)

	server := brandingTestServer(t, store, nil)
	ctx := context.WithValue(context.Background(), authPayloadKey, &token.Payload{
		Username: "other",
		Roles:    []utils.Role{utils.RoleHost},
	})
	primary := "#0F766E"
	_, err := server.UpdateLocationBranding(ctx, &pb.UpdateLocationBrandingRequest{
		LocationSlug: "acme",
		PrimaryColor: &primary,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	require.Equal(t, codes.PermissionDenied, st.Code())
}

func TestCreateLocationBrandingLogoUpload_OwnerSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)

	locID := uuid.New()
	store.EXPECT().GetLocationBySlug(gomock.Any(), "acme").Return(db.Location{
		ID: locID, OwnerUsername: "host1", Slug: "acme", Name: "Acme",
	}, nil)

	cfg := config.Config{
		TokenSymmetricKey:    utils.RandomString(32),
		StorageDriver:        "local",
		StorageLocalDir:      t.TempDir(),
		StoragePublicBaseURL: "http://localhost:8080/v1/public/media",
	}
	local, err := storage.NewLocal(cfg)
	require.NoError(t, err)

	server := brandingTestServer(t, store, local)
	server.config = cfg
	ctx := context.WithValue(context.Background(), authPayloadKey, &token.Payload{
		Username: "host1",
		Roles:    []utils.Role{utils.RoleHost},
	})
	res, err := server.CreateLocationBrandingLogoUpload(ctx, &pb.CreateLocationBrandingLogoUploadRequest{
		LocationSlug:  "acme",
		ContentType:   "image/png",
		ContentLength: 1024,
	})
	require.NoError(t, err)
	require.Contains(t, res.UploadUrl, "/v1/public/media/upload/")
	require.Contains(t, res.PublicUrl, "/v1/public/media/branding/")
	_, err = time.Parse("2006-01-02T15:04:05Z", res.ExpiresAt)
	require.NoError(t, err)
}

func TestCreateLocationBrandingLogoUpload_InvalidType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	store.EXPECT().GetLocationBySlug(gomock.Any(), "acme").Return(db.Location{
		ID: uuid.New(), OwnerUsername: "host1", Slug: "acme", Name: "Acme",
	}, nil)

	server := brandingTestServer(t, store, nil)
	ctx := context.WithValue(context.Background(), authPayloadKey, &token.Payload{
		Username: "host1",
		Roles:    []utils.Role{utils.RoleHost},
	})
	_, err := server.CreateLocationBrandingLogoUpload(ctx, &pb.CreateLocationBrandingLogoUploadRequest{
		LocationSlug:  "acme",
		ContentType:   "application/pdf",
		ContentLength: 100,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
}
