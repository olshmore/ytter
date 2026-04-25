package api

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestListHostLocationServices_MissingLocationSlug(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.ListHostLocationServices(ctx, &pb.ListHostLocationServicesRequest{LocationSlug: ""})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateHostLocationService_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	locationID := uuid.New()
	serviceID := uuid.New()
	store.EXPECT().GetLocationBySlug(gomock.Any(), "qa-clinic").Return(db.Location{
		ID:            locationID,
		OwnerUsername: "hostuser",
		Slug:          "qa-clinic",
		IsActive:      true,
	}, nil)
	store.EXPECT().CreateHostLocationService(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.CreateHostLocationServiceParams) (db.Service, error) {
			require.Equal(t, locationID, arg.LocationID)
			require.Equal(t, "Massage", arg.Name)
			require.Equal(t, int32(60), arg.DurationMinutes)
			require.Equal(t, int64(5000), arg.PriceMinorUnits)
			return db.Service{
				ID:              serviceID,
				LocationID:      arg.LocationID,
				Name:            arg.Name,
				Description:     arg.Description,
				DurationMinutes: arg.DurationMinutes,
				PriceMinorUnits: arg.PriceMinorUnits,
				Currency:        arg.Currency,
				IsActive:        arg.IsActive,
			}, nil
		})

	res, err := server.CreateHostLocationService(ctx, &pb.CreateHostLocationServiceRequest{
		LocationSlug:    "qa-clinic",
		Name:            "Massage",
		DurationMinutes: 60,
		PriceMinorUnits: 5000,
	})
	require.NoError(t, err)
	require.Equal(t, serviceID.String(), res.Service.Id)
}
