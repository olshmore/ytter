package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/booking/canceltoken"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/olshmore/ytter/pkg/validator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (server *Server) ListPublicLocations(ctx context.Context, _ *pb.ListPublicLocationsRequest) (*pb.ListPublicLocationsResponse, error) {
	rows, err := server.store.ListPublicLocations(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list public locations")
	}

	items := make([]*pb.PublicLocationItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, &pb.PublicLocationItem{
			Id:                          row.ID.String(),
			Slug:                        row.Slug,
			Name:                        row.Name,
			Timezone:                    row.Timezone,
			BookingRequiresHostApproval: row.BookingRequiresHostApproval,
		})
	}

	return &pb.ListPublicLocationsResponse{Items: items}, nil
}

func (server *Server) ListPublicSlots(ctx context.Context, req *pb.ListPublicSlotsRequest) (*pb.ListPublicSlotsResponse, error) {
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}

	rows, err := server.store.ListPublicSlotsByLocationSlug(ctx, locationSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list public slots")
	}
	if len(rows) == 0 {
		loc, err := server.store.GetLocationBySlug(ctx, locationSlug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, status.Errorf(codes.NotFound, "location not found")
			}
			return nil, status.Errorf(codes.Internal, "failed to load location")
		}
		if !loc.IsActive {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return &pb.ListPublicSlotsResponse{
			Location: &pb.PublicSlotsLocation{
				Id:                          loc.ID.String(),
				Slug:                        loc.Slug,
				Name:                        loc.Name,
				Timezone:                    loc.Timezone,
				BookingRequiresHostApproval: loc.BookingRequiresHostApproval,
			},
			Items:          []*pb.PublicSlotsItem{},
			TotalCount:     0,
			AvailableCount: 0,
			Limit:          normalizeLimit(req.GetLimit()),
			Offset:         normalizeOffset(req.GetOffset()),
		}, nil
	}

	search := strings.TrimSpace(strings.ToLower(req.GetSearch()))
	serviceID := strings.TrimSpace(req.GetServiceId())
	practitionerID := strings.TrimSpace(req.GetPractitionerId())
	roomID := strings.TrimSpace(req.GetRoomId())
	date := strings.TrimSpace(req.GetDate())
	timeSlot := strings.TrimSpace(req.GetTimeSlot())
	if timeSlot == "" {
		timeSlot = "any"
	}
	onlyAvailable := req.GetOnlyAvailable()
	limit := normalizeLimit(req.GetLimit())
	offset := normalizeOffset(req.GetOffset())
	now := time.Now()

	filtered := make([]db.ListPublicSlotsByLocationSlugRow, 0, len(rows))
	for _, row := range rows {
		if !row.StartAt.After(now) {
			continue
		}
		if search != "" {
			h := strings.ToLower(row.ServiceName + " " + row.PractitionerDisplayName.String + " " + row.RoomName.String)
			if !strings.Contains(h, search) {
				continue
			}
		}
		if serviceID != "" && row.ServiceID.String() != serviceID {
			continue
		}
		if practitionerID != "" && valueOrEmptyUUID(row.PractitionerID) != practitionerID {
			continue
		}
		if roomID != "" && valueOrEmptyUUID(row.RoomID) != roomID {
			continue
		}
		if date != "" && row.StartAt.Format("2006-01-02") != date {
			continue
		}
		if timeSlot != "any" && !matchesTimeBucket(row.StartAt, timeSlot) {
			continue
		}
		isBookable := row.SlotStatus == "available" && row.BookedCount < row.Capacity && row.StartAt.After(now)
		if onlyAvailable && !isBookable {
			continue
		}
		filtered = append(filtered, row)
	}

	totalCount := len(filtered)
	availableCount := 0
	for _, row := range filtered {
		if row.SlotStatus == "available" && row.BookedCount < row.Capacity && row.StartAt.After(now) {
			availableCount++
		}
	}

	start := minInt(int(offset), totalCount)
	end := minInt(int(offset+limit), totalCount)
	items := make([]*pb.PublicSlotsItem, 0, end-start)
	for _, row := range filtered[start:end] {
		isBookable := row.SlotStatus == "available" && row.BookedCount < row.Capacity && row.StartAt.After(now)
		spacesLeft := row.Capacity - row.BookedCount
		if spacesLeft < 0 {
			spacesLeft = 0
		}
		items = append(items, &pb.PublicSlotsItem{
			SlotId: row.SlotID.String(),
			Service: &pb.PublicSlotsService{
				Id:                              row.ServiceID.String(),
				Name:                            row.ServiceName,
				DurationMinutes:                 row.DurationMinutes,
				PriceMinorUnits:                 row.PriceMinorUnits,
				Currency:                        row.Currency,
				CancellationMinHoursBeforeStart: row.EffectiveCancellationMinHoursBeforeStart,
			},
			Practitioner: &pb.PublicSlotsPractitioner{
				Id:          valueOrEmptyUUID(row.PractitionerID),
				Name:        row.PractitionerDisplayName.String,
				DisplayName: row.PractitionerDisplayName.String,
			},
			Room: &pb.PublicSlotsRoom{
				Id:   valueOrEmptyUUID(row.RoomID),
				Name: row.RoomName.String,
			},
			StartAt:            row.StartAt.Format(time.RFC3339),
			EndAt:              row.EndAt.Format(time.RFC3339),
			AvailabilityStatus: row.SlotStatus,
			IsBookable:         isBookable,
			Capacity:           row.Capacity,
			BookedCount:        row.BookedCount,
			SpacesLeft:         spacesLeft,
		})
	}

	first := rows[0]
	return &pb.ListPublicSlotsResponse{
		Location: &pb.PublicSlotsLocation{
			Id:                          first.LocationID.String(),
			Slug:                        first.LocationSlug,
			Name:                        first.LocationName,
			Timezone:                    first.LocationTimezone,
			BookingRequiresHostApproval: first.BookingRequiresHostApproval,
		},
		Items:          items,
		TotalCount:     int32(totalCount),
		AvailableCount: int32(availableCount),
		Limit:          limit,
		Offset:         offset,
	}, nil
}

func (server *Server) GetPublicFilterOptions(ctx context.Context, req *pb.GetPublicFilterOptionsRequest) (*pb.GetPublicFilterOptionsResponse, error) {
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}

	services, err := server.store.ListPublicFilterServicesByLocationSlug(ctx, locationSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list filter services")
	}
	practitioners, err := server.store.ListPublicFilterPractitionersByLocationSlug(ctx, locationSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list practitioners")
	}
	rooms, err := server.store.ListPublicFilterRoomsByLocationSlug(ctx, locationSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rooms")
	}

	sRows := make([]*pb.PublicFilterService, 0, len(services))
	for _, s := range services {
		sRows = append(sRows, &pb.PublicFilterService{Id: s.ID.String(), Name: s.Name})
	}
	pRows := make([]*pb.PublicFilterPractitioner, 0, len(practitioners))
	for _, p := range practitioners {
		pRows = append(pRows, &pb.PublicFilterPractitioner{
			Id: p.ID.String(), Name: p.DisplayName, DisplayName: p.DisplayName,
		})
	}
	rRows := make([]*pb.PublicFilterRoom, 0, len(rooms))
	for _, rm := range rooms {
		rRows = append(rRows, &pb.PublicFilterRoom{Id: rm.ID.String(), Name: rm.Name})
	}

	return &pb.GetPublicFilterOptionsResponse{
		Services:      sRows,
		Practitioners: pRows,
		Rooms:         rRows,
		TimeSlots:     []string{"any", "morning", "afternoon", "evening"},
	}, nil
}

func (server *Server) CreatePublicBooking(ctx context.Context, req *pb.CreatePublicBookingRequest) (*pb.CreatePublicBookingResponse, error) {
	if err := validator.ValidateName(req.GetGuestName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid guest_name")
	}
	if err := validator.ValidateEmail(req.GetGuestEmail()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid guest_email")
	}
	locationID, err := uuid.Parse(req.GetLocationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid location_id")
	}
	slotID, err := uuid.Parse(req.GetSlotId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid slot_id")
	}

	clientUsername := ""
	if payload, authErr := server.extractAuthPayload(ctx); authErr == nil {
		if hasPermission(payload.Roles, []utils.Role{utils.RoleClient, utils.RoleAdmin}) {
			clientUsername = payload.Username
		}
	}

	rawToken := uuid.NewString()
	result, err := server.store.CreatePublicBookingTx(ctx, db.CreatePublicBookingTxParams{
		LocationID:      locationID,
		SlotID:          slotID,
		GuestName:       strings.TrimSpace(req.GetGuestName()),
		GuestEmail:      strings.TrimSpace(req.GetGuestEmail()),
		GuestPhone:      strings.TrimSpace(req.GetGuestPhone()),
		GuestNotes:      strings.TrimSpace(req.GetGuestNotes()),
		CancelTokenHash: canceltoken.Hash(rawToken),
		ClientUsername:  clientUsername,
	})
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "slot unavailable or invalid booking state")
	}

	clientUsername = ""
	if result.Booking.ClientUsername.Valid {
		clientUsername = result.Booking.ClientUsername.String
	}
	return &pb.CreatePublicBookingResponse{
		Booking: &pb.CreatePublicBookingBooking{
			Id:             result.Booking.ID.String(),
			LocationId:     result.Booking.LocationID.String(),
			SlotId:         result.Booking.SlotID.String(),
			Status:         result.Booking.Status,
			GuestName:      result.Booking.GuestName,
			GuestEmail:     result.Booking.GuestEmail,
			ClientUsername: clientUsername,
			BookedAt:       result.Booking.BookedAt.Format(time.RFC3339),
		},
		CancelToken: rawToken,
	}, nil
}

func (server *Server) CancelPublicBooking(ctx context.Context, req *pb.CancelPublicBookingRequest) (*pb.CancelPublicBookingResponse, error) {
	if strings.TrimSpace(req.GetCancelToken()) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "cancel_token is required")
	}
	bookingID, err := uuid.Parse(req.GetBookingId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}

	_, err = server.store.CancelPublicBookingTx(ctx, db.CancelPublicBookingTxParams{
		BookingID:    bookingID,
		CancelToken:  canceltoken.Hash(strings.TrimSpace(req.GetCancelToken())),
		CancelReason: strings.TrimSpace(req.GetCancelReason()),
		Now:          time.Now(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "invalid cancel token") {
			return nil, status.Errorf(codes.NotFound, "booking not found")
		}
		return nil, status.Errorf(codes.FailedPrecondition, "booking cannot be cancelled")
	}
	return &pb.CancelPublicBookingResponse{}, nil
}

func normalizeLimit(v int32) int32 {
	if v <= 0 {
		return 20
	}
	if v > 100 {
		return 100
	}
	return v
}

func normalizeOffset(v int32) int32 {
	if v < 0 {
		return 0
	}
	return v
}

func matchesTimeBucket(t time.Time, bucket string) bool {
	h := t.Hour()
	switch bucket {
	case "morning":
		return h >= 6 && h < 12
	case "afternoon":
		return h >= 12 && h < 17
	case "evening":
		return h >= 17 && h < 22
	default:
		return true
	}
}

func valueOrEmptyUUID(v pgtype.UUID) string {
	if !v.Valid {
		return ""
	}
	u, err := uuid.FromBytes(v.Bytes[:])
	if err != nil {
		return ""
	}
	return u.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
