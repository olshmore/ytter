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
