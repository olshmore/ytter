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

func TestListMyBookings_Unauthenticated(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	_, err := server.ListMyBookings(context.Background(), &pb.ListMyBookingsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestListMyBookings_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "client_a", []utils.Role{utils.RoleClient}, time.Minute)

	slotID := uuid.New()
	bookingID := uuid.New()
	now := time.Now().UTC()

	store.EXPECT().
		GetUserByUsername(gomock.Any(), "client_a").
		Return(db.User{Email: "client@example.com"}, nil)
	store.EXPECT().
		CountMyBookings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.CountMyBookingsParams) (int32, error) {
			require.True(t, arg.ClientUsername.Valid)
			require.Equal(t, "client_a", arg.ClientUsername.String)
			require.True(t, arg.FilterStatus.Valid)
			require.Equal(t, "confirmed", arg.FilterStatus.String)
			require.True(t, arg.FilterGuestEmail.Valid)
			require.Equal(t, "client@example.com", arg.FilterGuestEmail.String)
			return 1, nil
		})
	store.EXPECT().
		ListMyBookings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.ListMyBookingsParams) ([]db.ListMyBookingsRow, error) {
			require.True(t, arg.ClientUsername.Valid)
			require.Equal(t, "client_a", arg.ClientUsername.String)
			require.True(t, arg.FilterGuestEmail.Valid)
			require.Equal(t, "client@example.com", arg.FilterGuestEmail.String)
			require.Equal(t, int32(20), arg.Limit)
			require.Equal(t, int32(0), arg.Offset)
			return []db.ListMyBookingsRow{
				{
					BookingID:        bookingID,
					Status:           "confirmed",
					BookedAt:         now,
					GuestName:        "Client A",
					GuestEmail:       "client@example.com",
					GuestPhone:       pgtype.Text{String: "+123", Valid: true},
					CancelReason:     pgtype.Text{},
					LocationID:       uuid.New(),
					LocationSlug:     "royal-wellness",
					LocationName:     "Royal Wellness",
					SlotID:           slotID,
					ServiceName:      "Massage",
					PractitionerName: "Pat",
					RoomName:         "Room 1",
					StartAt:          now.Add(24 * time.Hour),
					EndAt:            now.Add(25 * time.Hour),
				},
			}, nil
		})

	res, err := server.ListMyBookings(ctx, &pb.ListMyBookingsRequest{
		Status: "confirmed",
		Limit:  20,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, bookingID.String(), res.Items[0].BookingId)
	require.Equal(t, "confirmed", res.Items[0].Status)
	require.NotNil(t, res.Items[0].Location)
	require.Equal(t, "royal-wellness", res.Items[0].Location.Slug)
	require.Equal(t, "Royal Wellness", res.Items[0].Location.Name)
	require.EqualValues(t, 1, res.TotalCount)
}

func TestCancelMyBooking_InvalidID(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "client_a", []utils.Role{utils.RoleClient}, time.Minute)

	_, err := server.CancelMyBooking(ctx, &pb.CancelMyBookingRequest{BookingId: "bad-id"})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCancelMyBooking_AccessDenied(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "client_a", []utils.Role{utils.RoleClient}, time.Minute)

	store.EXPECT().
		GetUserByUsername(gomock.Any(), "client_a").
		Return(db.User{Email: "client@example.com"}, nil)
	store.EXPECT().
		CancelMyBookingTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.CancelMyBookingTxParams) (db.CancelMyBookingTxResult, error) {
			require.Equal(t, "client@example.com", arg.ActorEmail)
			return db.CancelMyBookingTxResult{}, db.ErrClientBookingAccessDenied
		})

	_, err := server.CancelMyBooking(ctx, &pb.CancelMyBookingRequest{
		BookingId: uuid.NewString(),
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetMyBookingRebookContext_AllowByGuestEmailFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "client_a", []utils.Role{utils.RoleClient}, time.Minute)

	bookingID := uuid.New()
	locationID := uuid.New()
	serviceID := uuid.New()

	store.EXPECT().
		GetRebookContextByBookingID(gomock.Any(), bookingID).
		Return(db.GetRebookContextByBookingIDRow{
			BookingID:        bookingID,
			ClientUsername:   pgtype.Text{},
			GuestEmail:       "client@example.com",
			LocationID:       locationID,
			LocationSlug:     "royal-wellness",
			LocationName:     "Royal Wellness",
			ServiceID:        serviceID,
			ServiceName:      "Massage",
			LocationIsActive: true,
			ServiceIsActive:  true,
		}, nil)
	store.EXPECT().
		GetUserByUsername(gomock.Any(), "client_a").
		Return(db.User{Email: "client@example.com"}, nil)

	res, err := server.GetMyBookingRebookContext(ctx, &pb.GetMyBookingRebookContextRequest{
		BookingId: bookingID.String(),
	})
	require.NoError(t, err)
	require.Equal(t, bookingID.String(), res.SourceBookingId)
	require.Equal(t, "royal-wellness", res.Location.Slug)
	require.Equal(t, "Massage", res.Service.Name)
}

func TestGetMyBookingRebookContext_PermissionDeniedForOtherClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	store := mockdb.NewMockStore(ctrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "client_a", []utils.Role{utils.RoleClient}, time.Minute)

	bookingID := uuid.New()
	locationID := uuid.New()
	serviceID := uuid.New()

	store.EXPECT().
		GetRebookContextByBookingID(gomock.Any(), bookingID).
		Return(db.GetRebookContextByBookingIDRow{
			BookingID:        bookingID,
			ClientUsername:   pgtype.Text{String: "client_b", Valid: true},
			GuestEmail:       "client_b@example.com",
			LocationID:       locationID,
			LocationSlug:     "royal-wellness",
			LocationName:     "Royal Wellness",
			ServiceID:        serviceID,
			ServiceName:      "Massage",
			LocationIsActive: true,
			ServiceIsActive:  true,
		}, nil)
	store.EXPECT().
		GetUserByUsername(gomock.Any(), "client_a").
		Return(db.User{Email: "client_a@example.com"}, nil)

	_, err := server.GetMyBookingRebookContext(ctx, &pb.GetMyBookingRebookContextRequest{
		BookingId: bookingID.String(),
	})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestJoinPublicWaitlist_AlreadyExists(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	locationID := uuid.New()
	serviceID := uuid.New()
	slotID := uuid.New()

	store.EXPECT().
		GetLocationByID(gomock.Any(), locationID).
		Return(db.Location{ID: locationID, Slug: "qa-clinic", Name: "QA Clinic", IsActive: true}, nil)
	store.EXPECT().
		GetServiceByID(gomock.Any(), serviceID).
		Return(db.Service{ID: serviceID, LocationID: locationID, Name: "Massage", IsActive: true}, nil)
	store.EXPECT().
		GetHostSlotByID(gomock.Any(), db.GetHostSlotByIDParams{ID: slotID, LocationID: locationID}).
		Return(db.AppointmentSlot{ID: slotID, LocationID: locationID, ServiceID: serviceID, Capacity: 2}, nil)
	store.EXPECT().
		GetActiveWaitlistEntryByIdentity(gomock.Any(), gomock.Any()).
		Return(db.WaitlistEntry{ID: uuid.New()}, nil)

	_, err := server.JoinPublicWaitlist(context.Background(), &pb.JoinPublicWaitlistRequest{
		LocationId: locationID.String(),
		ServiceId:  serviceID.String(),
		SlotId:     slotID.String(),
		GuestName:  "Jane Doe",
		GuestEmail: "jane@example.com",
	})
	require.Error(t, err)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestJoinPublicWaitlist_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	locationID := uuid.New()
	serviceID := uuid.New()
	slotID := uuid.New()
	entryID := uuid.New()

	store.EXPECT().
		GetLocationByID(gomock.Any(), locationID).
		Return(db.Location{ID: locationID, Slug: "qa-clinic", Name: "QA Clinic", IsActive: true}, nil)
	store.EXPECT().
		GetServiceByID(gomock.Any(), serviceID).
		Return(db.Service{ID: serviceID, LocationID: locationID, Name: "Massage", IsActive: true}, nil)
	store.EXPECT().
		GetHostSlotByID(gomock.Any(), db.GetHostSlotByIDParams{ID: slotID, LocationID: locationID}).
		Return(db.AppointmentSlot{ID: slotID, LocationID: locationID, ServiceID: serviceID, Capacity: 2}, nil)
	store.EXPECT().
		GetActiveWaitlistEntryByIdentity(gomock.Any(), gomock.Any()).
		Return(db.WaitlistEntry{}, pgx.ErrNoRows)
	store.EXPECT().
		CreateWaitlistEntry(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.CreateWaitlistEntryParams) (db.WaitlistEntry, error) {
			require.Equal(t, locationID, arg.LocationID)
			require.Equal(t, serviceID, arg.ServiceID)
			require.Equal(t, slotID, arg.SlotID)
			require.Equal(t, "Jane Doe", arg.GuestName)
			require.Equal(t, "jane@example.com", arg.GuestEmail)
			return db.WaitlistEntry{ID: entryID, Status: "active"}, nil
		})
	store.EXPECT().
		CountActiveWaitlistEntriesForSlot(gomock.Any(), slotID).
		Return(int32(3), nil)

	res, err := server.JoinPublicWaitlist(context.Background(), &pb.JoinPublicWaitlistRequest{
		LocationId: locationID.String(),
		ServiceId:  serviceID.String(),
		SlotId:     slotID.String(),
		GuestName:  "Jane Doe",
		GuestEmail: "jane@example.com",
	})
	require.NoError(t, err)
	require.Equal(t, entryID.String(), res.WaitlistEntryId)
	require.Equal(t, "active", res.Status)
	require.EqualValues(t, 3, res.PositionHint)
}
