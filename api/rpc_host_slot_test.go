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

func TestListHostLocationSlots_MissingLocationSlug(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.ListHostLocationSlots(ctx, &pb.ListHostLocationSlotsRequest{LocationSlug: ""})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListHostLocationSlots_InvalidStatus(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.ListHostLocationSlots(ctx, &pb.ListHostLocationSlotsRequest{
		LocationSlug: "qa-clinic",
		Status:     "unknown",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateHostLocationSlot_OK(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	locationID := uuid.New()
	serviceID := uuid.New()
	practitionerID := uuid.New()
	roomID := uuid.New()
	startAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	endAt := startAt.Add(time.Hour)
	slotID := uuid.New()

	store.EXPECT().GetLocationBySlug(gomock.Any(), "qa-clinic").Return(db.Location{
		ID:            locationID,
		OwnerUsername: "hostuser",
		Slug:          "qa-clinic",
		Timezone:      "Europe/London",
		IsActive:      true,
	}, nil)
	store.EXPECT().GetServiceByID(gomock.Any(), serviceID).Return(db.Service{
		ID:         serviceID,
		LocationID: locationID,
		Name:       "Massage",
	}, nil)
	store.EXPECT().GetPractitionerByID(gomock.Any(), practitionerID).Return(db.Practitioner{
		ID:          practitionerID,
		LocationID:  locationID,
		DisplayName: "Alice",
	}, nil)
	store.EXPECT().GetRoomByID(gomock.Any(), roomID).Return(db.Room{
		ID:         roomID,
		LocationID: locationID,
		Name:       "Room A",
	}, nil)
	store.EXPECT().
		CreateHostLocationSlot(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg db.CreateHostLocationSlotParams) (db.AppointmentSlot, error) {
			require.Equal(t, locationID, arg.LocationID)
			require.Equal(t, serviceID, arg.ServiceID)
			require.True(t, arg.PractitionerID.Valid)
			require.True(t, arg.RoomID.Valid)
			require.Equal(t, int32(2), arg.Capacity)
			return db.AppointmentSlot{
				ID:             slotID,
				LocationID:     arg.LocationID,
				ServiceID:      arg.ServiceID,
				PractitionerID: arg.PractitionerID,
				RoomID:         arg.RoomID,
				StartAt:        arg.StartAt,
				EndAt:          arg.EndAt,
				Capacity:       arg.Capacity,
				BookedCount:    0,
				Status:         arg.Status,
			}, nil
		})

	pr := practitionerID.String()
	rm := roomID.String()
	res, err := server.CreateHostLocationSlot(ctx, &pb.CreateHostLocationSlotRequest{
		LocationSlug:   "qa-clinic",
		ServiceId:      serviceID.String(),
		PractitionerId: &pr,
		RoomId:         &rm,
		StartAt:        startAt.Format(time.RFC3339),
		EndAt:          endAt.Format(time.RFC3339),
		Capacity:       2,
	})
	require.NoError(t, err)
	require.Equal(t, slotID.String(), res.Slot.Id)
	require.Equal(t, "Massage", res.Slot.ServiceName)
	require.Equal(t, "Alice", res.Slot.PractitionerName)
	require.Equal(t, "Room A", res.Slot.RoomName)
}
