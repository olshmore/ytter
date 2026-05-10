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
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCreatePublicBooking_StatusPassThrough(t *testing.T) {
	testCases := []struct {
		name   string
		status string
	}{
		{name: "Confirmed", status: "confirmed"},
		{name: "Pending", status: "pending"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			storeCtrl := gomock.NewController(t)
			defer storeCtrl.Finish()
			store := mockdb.NewMockStore(storeCtrl)

			locationID := uuid.New()
			slotID := uuid.New()
			bookingID := uuid.New()

			store.EXPECT().
				CreatePublicBookingTx(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, arg db.CreatePublicBookingTxParams) (db.CreatePublicBookingTxResult, error) {
					require.Equal(t, locationID, arg.LocationID)
					require.Equal(t, slotID, arg.SlotID)
					require.Equal(t, "Guest User", arg.GuestName)
					require.Equal(t, "guest@example.com", arg.GuestEmail)
					require.NotEmpty(t, arg.CancelTokenHash)

					return db.CreatePublicBookingTxResult{
						Booking: db.Booking{
							ID:         bookingID,
							LocationID: locationID,
							SlotID:     slotID,
							Status:     tc.status,
							GuestName:  "Guest User",
							GuestEmail: "guest@example.com",
							BookedAt:   time.Now().UTC(),
						},
					}, nil
				})

			server := newTestServer(t, store, nil)
			res, err := server.CreatePublicBooking(context.Background(), &pb.CreatePublicBookingRequest{
				LocationId: locationID.String(),
				SlotId:     slotID.String(),
				GuestName:  "Guest User",
				GuestEmail: "guest@example.com",
			})

			require.NoError(t, err)
			require.Equal(t, tc.status, res.Booking.Status)
			require.Equal(t, bookingID.String(), res.Booking.Id)
			require.NotEmpty(t, res.CancelToken)
		})
	}
}

func TestListPublicLocations_FutureInventoryOnly(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	activeID := uuid.New()
	store.EXPECT().
		ListPublicLocations(gomock.Any()).
		Return([]db.ListPublicLocationsRow{
			{
				ID:                          activeID,
				Slug:                        "active-clinic",
				Name:                        "Active Clinic",
				Timezone:                    "Europe/London",
				BookingRequiresHostApproval: false,
			},
		}, nil)

	res, err := server.ListPublicLocations(context.Background(), &pb.ListPublicLocationsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, activeID.String(), res.Items[0].Id)
	require.Equal(t, "active-clinic", res.Items[0].Slug)
}

func TestCreatePublicBooking_InvalidLocationID(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	_, err := server.CreatePublicBooking(context.Background(), &pb.CreatePublicBookingRequest{
		LocationId: "bad-id",
		SlotId:     uuid.NewString(),
		GuestName:  "Guest User",
		GuestEmail: "guest@example.com",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListPublicSlots_MissingLocationSlug(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	_, err := server.ListPublicSlots(context.Background(), &pb.ListPublicSlotsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListPublicSlots_EmptyButLocationExists(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	locationID := uuid.New()
	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "qa-clinic").
		Return(db.Location{
			ID:                          locationID,
			Slug:                        "qa-clinic",
			Name:                        "QA Clinic",
			Timezone:                    "Europe/London",
			IsActive:                    true,
			BookingRequiresHostApproval: true,
		}, nil)

	res, err := server.ListPublicSlots(context.Background(), &pb.ListPublicSlotsRequest{
		LocationSlug: "qa-clinic",
	})
	require.NoError(t, err)
	require.Equal(t, locationID.String(), res.Location.Id)
	require.Equal(t, "qa-clinic", res.Location.Slug)
	require.True(t, res.Location.BookingRequiresHostApproval)
	require.Empty(t, res.Items)
	require.EqualValues(t, 0, res.TotalCount)
}

func TestListPublicSlots_FiltersAndAvailability(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	locationID := uuid.New()
	serviceID := uuid.New()
	practitionerID := uuid.New()
	roomID := uuid.New()
	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour).Add(9 * time.Hour)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			{
				SlotID:                                   uuid.New(),
				StartAt:                                  start,
				EndAt:                                    start.Add(30 * time.Minute),
				SlotStatus:                               "available",
				Capacity:                                 2,
				BookedCount:                              1,
				ServiceID:                                serviceID,
				ServiceName:                              "Massage",
				DurationMinutes:                          30,
				PriceMinorUnits:                          5000,
				Currency:                                 "GBP",
				EffectiveCancellationMinHoursBeforeStart: 24,
				PractitionerID:                           pgtype.UUID{Bytes: practitionerID, Valid: true},
				PractitionerDisplayName:                  pgtype.Text{String: "Alice", Valid: true},
				RoomID:                                   pgtype.UUID{Bytes: roomID, Valid: true},
				RoomName:                                 pgtype.Text{String: "Room A", Valid: true},
				LocationID:                               locationID,
				LocationSlug:                             "qa-clinic",
				LocationName:                             "QA Clinic",
				LocationTimezone:                         "Europe/London",
				BookingRequiresHostApproval:              false,
			},
		}, nil)

	res, err := server.ListPublicSlots(context.Background(), &pb.ListPublicSlotsRequest{
		LocationSlug:   "qa-clinic",
		Search:         "massage",
		ServiceId:      serviceID.String(),
		PractitionerId: practitionerID.String(),
		RoomId:         roomID.String(),
		Date:           start.Format("2006-01-02"),
		TimeSlot:       "morning",
		OnlyAvailable:  true,
		Limit:          20,
		Offset:         0,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.EqualValues(t, 1, res.TotalCount)
	require.EqualValues(t, 1, res.AvailableCount)
	require.True(t, res.Items[0].IsBookable)
	require.EqualValues(t, 2, res.Items[0].Capacity)
	require.EqualValues(t, 1, res.Items[0].BookedCount)
	require.EqualValues(t, 1, res.Items[0].SpacesLeft)
}

func TestListPublicSlots_ExcludesPastEvenWhenOnlyAvailableFalse(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	locationID := uuid.New()
	serviceID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			{
				SlotID:                      uuid.New(),
				StartAt:                     now.Add(-2 * time.Hour),
				EndAt:                       now.Add(-90 * time.Minute),
				SlotStatus:                  "booked",
				Capacity:                    1,
				BookedCount:                 1,
				ServiceID:                   serviceID,
				ServiceName:                 "Massage",
				DurationMinutes:             30,
				PriceMinorUnits:             5000,
				Currency:                    "GBP",
				LocationID:                  locationID,
				LocationSlug:                "qa-clinic",
				LocationName:                "QA Clinic",
				LocationTimezone:            "Europe/London",
				BookingRequiresHostApproval: false,
			},
			{
				SlotID:                      uuid.New(),
				StartAt:                     now.Add(2 * time.Hour),
				EndAt:                       now.Add(150 * time.Minute),
				SlotStatus:                  "booked",
				Capacity:                    1,
				BookedCount:                 1,
				ServiceID:                   serviceID,
				ServiceName:                 "Massage",
				DurationMinutes:             30,
				PriceMinorUnits:             5000,
				Currency:                    "GBP",
				LocationID:                  locationID,
				LocationSlug:                "qa-clinic",
				LocationName:                "QA Clinic",
				LocationTimezone:            "Europe/London",
				BookingRequiresHostApproval: false,
			},
		}, nil)

	res, err := server.ListPublicSlots(context.Background(), &pb.ListPublicSlotsRequest{
		LocationSlug:  "qa-clinic",
		OnlyAvailable: false,
		Limit:         20,
		Offset:        0,
		TimeSlot:      "any",
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.EqualValues(t, 1, res.TotalCount)
	require.Equal(t, "booked", res.Items[0].AvailabilityStatus)
}

func TestGetPublicFilterOptions_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	serviceID := uuid.New()
	practitionerID := uuid.New()
	roomID := uuid.New()
	store.EXPECT().
		ListPublicFilterServicesByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicFilterServicesByLocationSlugRow{{ID: serviceID, Name: "Massage"}}, nil)
	store.EXPECT().
		ListPublicFilterPractitionersByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicFilterPractitionersByLocationSlugRow{{ID: practitionerID, DisplayName: "Alice"}}, nil)
	store.EXPECT().
		ListPublicFilterRoomsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicFilterRoomsByLocationSlugRow{{ID: roomID, Name: "Room A"}}, nil)

	res, err := server.GetPublicFilterOptions(context.Background(), &pb.GetPublicFilterOptionsRequest{
		LocationSlug: "qa-clinic",
	})
	require.NoError(t, err)
	require.Len(t, res.Services, 1)
	require.Len(t, res.Practitioners, 1)
	require.Len(t, res.Rooms, 1)
	require.Equal(t, []string{"any", "morning", "afternoon", "evening"}, res.TimeSlots)
}

func TestCancelPublicBooking_InvalidToken(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	_, err := server.CancelPublicBooking(context.Background(), &pb.CancelPublicBookingRequest{
		BookingId:   uuid.NewString(),
		CancelToken: " ",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListPublicSlots_NotFoundWhenLocationMissing(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "missing-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "missing-clinic").
		Return(db.Location{}, pgx.ErrNoRows)

	_, err := server.ListPublicSlots(context.Background(), &pb.ListPublicSlotsRequest{
		LocationSlug: "missing-clinic",
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetPublicCalendarAvailability_MonthSummary(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	month := monthStart.Format("2006-01")

	serviceID := uuid.New()
	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			{
				StartAt:          monthStart.AddDate(0, 0, 2).Add(9 * time.Hour),
				EndAt:            monthStart.AddDate(0, 0, 2).Add(10 * time.Hour),
				SlotStatus:       "available",
				Capacity:         2,
				BookedCount:      1,
				ServiceID:        serviceID,
				LocationTimezone: "UTC",
			},
			{
				StartAt:          monthStart.AddDate(0, 0, 2).Add(12 * time.Hour),
				EndAt:            monthStart.AddDate(0, 0, 2).Add(13 * time.Hour),
				SlotStatus:       "available",
				Capacity:         1,
				BookedCount:      0,
				ServiceID:        serviceID,
				LocationTimezone: "UTC",
			},
		}, nil)

	res, err := server.GetPublicCalendarAvailability(context.Background(), &pb.GetPublicCalendarAvailabilityRequest{
		LocationSlug: "qa-clinic",
		Month:        month,
		ServiceId:    serviceID.String(),
	})
	require.NoError(t, err)
	require.Equal(t, "qa-clinic", res.LocationSlug)
	require.Equal(t, month, res.Month)
	require.NotEmpty(t, res.Days)
	targetDay := monthStart.AddDate(0, 0, 2).Format("2006-01-02")
	dayMap := make(map[string]*pb.PublicCalendarAvailabilityDay, len(res.Days))
	for _, day := range res.Days {
		dayMap[day.Date] = day
	}
	require.Contains(t, dayMap, targetDay)
	require.Equal(t, int32(2), dayMap[targetDay].AvailableCount)
	require.Equal(t, "available", dayMap[targetDay].State)
}

func TestGetPublicCalendarAvailability_InvalidMonth(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	_, err := server.GetPublicCalendarAvailability(context.Background(), &pb.GetPublicCalendarAvailabilityRequest{
		LocationSlug: "qa-clinic",
		Month:        "2026/05",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetPublicCalendarAvailability_ServiceFilter(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	month := monthStart.Format("2006-01")

	selectedServiceID := uuid.New()
	otherServiceID := uuid.New()
	targetDay := monthStart.AddDate(0, 0, 3)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			{
				StartAt:          targetDay.Add(9 * time.Hour),
				EndAt:            targetDay.Add(10 * time.Hour),
				SlotStatus:       "available",
				Capacity:         1,
				BookedCount:      0,
				ServiceID:        selectedServiceID,
				LocationTimezone: "UTC",
			},
			{
				StartAt:          targetDay.Add(11 * time.Hour),
				EndAt:            targetDay.Add(12 * time.Hour),
				SlotStatus:       "available",
				Capacity:         1,
				BookedCount:      0,
				ServiceID:        otherServiceID,
				LocationTimezone: "UTC",
			},
		}, nil)

	res, err := server.GetPublicCalendarAvailability(context.Background(), &pb.GetPublicCalendarAvailabilityRequest{
		LocationSlug: "qa-clinic",
		Month:        month,
		ServiceId:    selectedServiceID.String(),
	})
	require.NoError(t, err)

	dayMap := make(map[string]*pb.PublicCalendarAvailabilityDay, len(res.Days))
	for _, day := range res.Days {
		dayMap[day.Date] = day
	}
	require.Equal(t, int32(1), dayMap[targetDay.Format("2006-01-02")].AvailableCount)
}

func TestGetPublicCalendarAvailability_InvalidTimezone(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)

	_, err := server.GetPublicCalendarAvailability(context.Background(), &pb.GetPublicCalendarAvailabilityRequest{
		LocationSlug: "qa-clinic",
		Month:        "2026-05",
		Timezone:     "Mars/Olympus",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetPublicCalendarAvailability_NotFoundWhenLocationMissing(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "missing-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "missing-clinic").
		Return(db.Location{}, pgx.ErrNoRows)

	_, err := server.GetPublicCalendarAvailability(context.Background(), &pb.GetPublicCalendarAvailabilityRequest{
		LocationSlug: "missing-clinic",
		Month:        "2026-05",
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetPublicCalendarAvailability_PastStateForPastMonth(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)

	now := time.Now().UTC()
	pastMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0).Format("2006-01")

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)

	res, err := server.GetPublicCalendarAvailability(context.Background(), &pb.GetPublicCalendarAvailabilityRequest{
		LocationSlug: "qa-clinic",
		Month:        pastMonth,
		Timezone:     "UTC",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Days)
	require.Equal(t, "past", res.Days[0].State)
}
