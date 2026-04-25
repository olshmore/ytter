package api

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestListHostLocations_Unauthenticated(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	_, err := server.ListHostLocations(context.Background(), &pb.ListHostLocationsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestListHostLocations_HostOK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	locID := uuid.New()
	store.EXPECT().
		ListHostLocationsByOwner(gomock.Any(), "hostuser").
		Return([]db.ListHostLocationsByOwnerRow{
			{
				ID:                                       locID,
				Slug:                                     "wellness",
				Name:                                     "Wellness",
				BookingRequiresHostApproval:              true,
				EffectiveCancellationMinHoursBeforeStart: 24,
			},
		}, nil)

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	res, err := server.ListHostLocations(ctx, &pb.ListHostLocationsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, locID.String(), res.Items[0].Id)
	require.Equal(t, "wellness", res.Items[0].Slug)
	require.True(t, res.Items[0].BookingRequiresHostApproval)
	require.EqualValues(t, 24, res.Items[0].CancellationMinHoursBeforeStart)
}

func TestListHostLocations_AdminListsAll(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	locID := uuid.New()
	store.EXPECT().
		ListAllHostLocations(gomock.Any()).
		Return([]db.ListAllHostLocationsRow{
			{
				ID:                                       locID,
				Slug:                                     "a",
				Name:                                     "A",
				BookingRequiresHostApproval:              false,
				EffectiveCancellationMinHoursBeforeStart: 48,
			},
		}, nil)

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "admin", []utils.Role{utils.RoleAdmin}, time.Minute)

	res, err := server.ListHostLocations(ctx, &pb.ListHostLocationsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.EqualValues(t, 48, res.Items[0].CancellationMinHoursBeforeStart)
}

func TestListHostLocationBookings_MissingLocationSlug(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.ListHostLocationBookings(ctx, &pb.ListHostLocationBookingsRequest{LocationSlug: ""})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListHostLocationBookings_InvalidStatus(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.ListHostLocationBookings(ctx, &pb.ListHostLocationBookingsRequest{
		LocationSlug: "qa-clinic",
		Status:     "nope",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHostApproveBooking_InvalidUUID(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.HostApproveBooking(ctx, &pb.HostApproveBookingRequest{BookingId: "x"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateHostLocation_InvalidSlug(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.CreateHostLocation(ctx, &pb.CreateHostLocationRequest{
		Name:     "Clinic",
		Slug:     "Invalid Slug!",
		Timezone: "Europe/London",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateHostLocation_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	locationID := uuid.New()
	store.EXPECT().
		CreateLocation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.CreateLocationParams) (db.Location, error) {
			require.Equal(t, "hostuser", arg.OwnerUsername)
			require.Equal(t, "qa-clinic", arg.Slug)
			require.True(t, arg.IsActive)
			return db.Location{
				ID:            locationID,
				OwnerUsername: arg.OwnerUsername,
				Name:          arg.Name,
				Slug:          arg.Slug,
				Timezone:      arg.Timezone,
				IsActive:      arg.IsActive,
			}, nil
		})

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	res, err := server.CreateHostLocation(ctx, &pb.CreateHostLocationRequest{
		Name:     "QA Clinic",
		Slug:     "qa-clinic",
		Timezone: "Europe/London",
	})
	require.NoError(t, err)
	require.Equal(t, locationID.String(), res.Location.Id)
	require.Equal(t, "hostuser", res.Location.OwnerUsername)
}

func TestGetHostLocation_ForbiddenForNonOwner(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	locationID := uuid.New()
	store.EXPECT().
		GetLocationByID(gomock.Any(), locationID).
		Return(db.Location{
			ID:            locationID,
			OwnerUsername: "otherhost",
			Name:          "Clinic",
			Slug:          "clinic",
			Timezone:      "Europe/London",
			IsActive:      true,
		}, nil)

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.GetHostLocation(ctx, &pb.GetHostLocationRequest{LocationId: locationID.String()})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetHostLocationBySlug_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	locationID := uuid.New()
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "qa-clinic").
		Return(db.Location{
			ID:            locationID,
			OwnerUsername: "hostuser",
			Name:          "QA Clinic",
			Slug:          "qa-clinic",
			Timezone:      "Europe/London",
			IsActive:      true,
		}, nil)

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	res, err := server.GetHostLocationBySlug(ctx, &pb.GetHostLocationBySlugRequest{LocationSlug: "qa-clinic"})
	require.NoError(t, err)
	require.Equal(t, locationID.String(), res.Location.Id)
	require.Equal(t, "qa-clinic", res.Location.Slug)
}

func TestGetHostLocationBySlug_MissingSlug(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.GetHostLocationBySlug(ctx, &pb.GetHostLocationBySlugRequest{LocationSlug: "   "})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUpdateHostLocation_BookingApprovalSetting(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	locationID := uuid.New()

	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "qa-clinic").
		Return(db.Location{
			ID:                              locationID,
			OwnerUsername:                   "hostuser",
			Name:                            "QA Clinic",
			Slug:                            "qa-clinic",
			Timezone:                        "Europe/London",
			IsActive:                        true,
			BookingRequiresHostApproval:     false,
			CancellationMinHoursBeforeStart: pgtype.Int4{Int32: 24, Valid: true},
		}, nil)

	store.EXPECT().
		UpdateLocation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.UpdateLocationParams) (db.Location, error) {
			require.True(t, arg.BookingRequiresHostApproval.Valid)
			require.True(t, arg.BookingRequiresHostApproval.Bool)
			return db.Location{
				ID:                              locationID,
				OwnerUsername:                   "hostuser",
				Name:                            "QA Clinic",
				Slug:                            "qa-clinic",
				Timezone:                        "Europe/London",
				IsActive:                        true,
				BookingRequiresHostApproval:     true,
				CancellationMinHoursBeforeStart: pgtype.Int4{Int32: 24, Valid: true},
			}, nil
		})

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)
	requiresApproval := true

	res, err := server.UpdateHostLocation(ctx, &pb.UpdateHostLocationRequest{
		LocationSlug:                "qa-clinic",
		BookingRequiresHostApproval: &requiresApproval,
	})
	require.NoError(t, err)
	require.True(t, res.Location.BookingRequiresHostApproval)
}
