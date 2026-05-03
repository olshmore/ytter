package api

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func TestHostSetBookingNoShow_InvalidUUID(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.HostSetBookingNoShow(ctx, &pb.HostSetBookingNoShowRequest{BookingId: "x", NoShow: true})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHostSetBookingNoShow_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	bookingID := uuid.New()
	locID := uuid.New()
	slotID := uuid.New()
	now := time.Now().UTC()
	store.EXPECT().
		HostSetBookingNoShowTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.HostSetBookingNoShowTxParams) (db.HostSetBookingNoShowTxResult, error) {
			require.Equal(t, bookingID, arg.BookingID)
			require.True(t, arg.NoShow)
			return db.HostSetBookingNoShowTxResult{
				Booking: db.Booking{
					ID:         bookingID,
					LocationID: locID,
					SlotID:     slotID,
					Status:     "no_show",
					GuestName:  "Jane",
					GuestEmail: "jane@example.com",
					BookedAt:   now,
				},
			}, nil
		})

	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	res, err := server.HostSetBookingNoShow(ctx, &pb.HostSetBookingNoShowRequest{
		BookingId: bookingID.String(),
		NoShow:    true,
	})
	require.NoError(t, err)
	require.Equal(t, "no_show", res.Booking.Status)
	require.Equal(t, bookingID.String(), res.Booking.Id)
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

func TestGetHostSetupChecklist_ReadyForBooking(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	locationID := uuid.New()
	now := time.Now().UTC()

	store.EXPECT().
		ListHostLocationsByOwner(gomock.Any(), "hostuser").
		Return([]db.ListHostLocationsByOwnerRow{
			{
				ID:            locationID,
				OwnerUsername: "hostuser",
				Slug:          "qa-clinic",
				Name:          "QA Clinic",
				Timezone:      "Europe/London",
				IsActive:      true,
			},
		}, nil)
	store.EXPECT().
		GetLocationByID(gomock.Any(), locationID).
		Return(db.Location{
			ID:            locationID,
			OwnerUsername: "hostuser",
			Slug:          "qa-clinic",
			Name:          "QA Clinic",
			Timezone:      "Europe/London",
			IsActive:      true,
		}, nil)
	store.EXPECT().
		CountHostServicesByLocation(gomock.Any(), gomock.Any()).
		Return(int32(2), nil)
	store.EXPECT().
		ListHostSlotsByLocation(gomock.Any(), gomock.Any()).
		Return([]db.ListHostSlotsByLocationRow{
			{
				ID:          uuid.New(),
				LocationID:  locationID,
				ServiceID:   uuid.New(),
				StartAt:     now.Add(2 * time.Hour),
				EndAt:       now.Add(3 * time.Hour),
				Capacity:    2,
				BookedCount: 1,
				Status:      "available",
			},
		}, nil)

	res, err := server.GetHostSetupChecklist(ctx, &pb.GetHostSetupChecklistRequest{})
	require.NoError(t, err)
	require.True(t, res.HasLocation)
	require.True(t, res.HasService)
	require.True(t, res.HasFutureSlot)
	require.True(t, res.ReadyForBooking)
	require.EqualValues(t, 3, res.ProgressDoneCount)
	require.Equal(t, "qa-clinic", res.SampleLocationSlug)
}

func TestGetHostBookingAnalyticsSummary_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	locationID := uuid.New()
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "qa-clinic").
		Return(db.Location{ID: locationID, OwnerUsername: "hostuser", Slug: "qa-clinic", Name: "QA Clinic", IsActive: true}, nil)
	store.EXPECT().
		GetHostBookingAnalyticsSummary(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.GetHostBookingAnalyticsSummaryParams) (db.GetHostBookingAnalyticsSummaryRow, error) {
			require.Equal(t, locationID, arg.LocationID)
			require.Equal(t, "2026-05-01", arg.FromDate.String)
			require.Equal(t, "2026-05-31", arg.ToDate.String)
			return db.GetHostBookingAnalyticsSummaryRow{
				TotalCount:                10,
				FilledCount:               7,
				CancelledCount:            2,
				PendingCount:              1,
				NoShowCount:               1,
				PendingApprovalAvgMinutes: []byte("15.5"),
			}, nil
		})

	res, err := server.GetHostBookingAnalyticsSummary(ctx, &pb.GetHostBookingAnalyticsSummaryRequest{
		LocationSlug: "qa-clinic",
		FromDate:     "2026-05-01",
		ToDate:       "2026-05-31",
	})
	require.NoError(t, err)
	require.Equal(t, "qa-clinic", res.LocationSlug)
	require.InDelta(t, 0.7, res.FillRate, 0.0001)
	require.InDelta(t, 0.2, res.CancellationRate, 0.0001)
	// no_show_proxy_rate is no_show_count / total_count (pending excluded).
	require.InDelta(t, 0.1, res.NoShowProxyRate, 0.0001)
	require.InDelta(t, 15.5, res.PendingApprovalAvgMinutes, 0.0001)
}

func TestGetHostBookingAnalyticsSummary_InvalidFromDate(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.GetHostBookingAnalyticsSummary(ctx, &pb.GetHostBookingAnalyticsSummaryRequest{
		LocationSlug: "qa-clinic",
		FromDate:     "2026/05/01",
		ToDate:       "2026-05-31",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetHostSetupChecklist_LocationLoadFailure(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	store.EXPECT().
		ListHostLocationsByOwner(gomock.Any(), "hostuser").
		Return(nil, pgx.ErrNoRows)

	_, err := server.GetHostSetupChecklist(ctx, &pb.GetHostSetupChecklistRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}
