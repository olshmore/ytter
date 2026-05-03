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
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/olshmore/ytter/pkg/validator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (server *Server) ListMyBookings(ctx context.Context, req *pb.ListMyBookingsRequest) (*pb.ListMyBookingsResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleClient, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	statusFilter := strings.TrimSpace(req.GetStatus())
	if statusFilter != "" && !validHostBookingListStatus(statusFilter) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid status filter")
	}
	fromDate := strings.TrimSpace(req.GetFromDate())
	if fromDate != "" {
		if _, err := time.Parse("2006-01-02", fromDate); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid from_date")
		}
	}
	toDate := strings.TrimSpace(req.GetToDate())
	if toDate != "" {
		if _, err := time.Parse("2006-01-02", toDate); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid to_date")
		}
	}

	limit := normalizeLimit(req.GetLimit())
	offset := normalizeOffset(req.GetOffset())
	filter := db.CountMyBookingsParams{
		ClientUsername:   pgtype.Text{String: payload.Username, Valid: true},
		FilterGuestEmail: pgtype.Text{},
		FilterStatus:     optionalStatusPGText(statusFilter),
		FromDate:         optionalDatePGText(fromDate),
		ToDate:           optionalDatePGText(toDate),
	}
	user, userErr := server.store.GetUserByUsername(ctx, payload.Username)
	if userErr == nil {
		filter.FilterGuestEmail = pgtype.Text{String: user.Email, Valid: true}
	}

	total, err := server.store.CountMyBookings(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count bookings")
	}
	rows, err := server.store.ListMyBookings(ctx, db.ListMyBookingsParams{
		ClientUsername:   pgtype.Text{String: payload.Username, Valid: true},
		Limit:            limit,
		Offset:           offset,
		FilterGuestEmail: filter.FilterGuestEmail,
		FilterStatus:     filter.FilterStatus,
		FromDate:         filter.FromDate,
		ToDate:           filter.ToDate,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list bookings")
	}

	items := make([]*pb.HostBookingListItem, 0, len(rows))
	for _, row := range rows {
		phone := ""
		if row.GuestPhone.Valid {
			phone = row.GuestPhone.String
		}
		cancelReason := ""
		if row.CancelReason.Valid {
			cancelReason = row.CancelReason.String
		}
		items = append(items, &pb.HostBookingListItem{
			BookingId:    row.BookingID.String(),
			Status:       row.Status,
			BookedAt:     row.BookedAt.Format(time.RFC3339),
			GuestName:    row.GuestName,
			GuestEmail:   row.GuestEmail,
			GuestPhone:   phone,
			CancelReason: cancelReason,
			IsWaitlist: row.IsWaitlist,
			Location: &pb.BookingLocationSummary{
				Id:   row.LocationID.String(),
				Slug: row.LocationSlug,
				Name: row.LocationName,
			},
			Slot: &pb.HostBookingSlotSummary{
				SlotId:           row.SlotID.String(),
				ServiceName:      row.ServiceName,
				PractitionerName: row.PractitionerName,
				RoomName:         row.RoomName,
				StartAt:          row.StartAt.Format(time.RFC3339),
				EndAt:            row.EndAt.Format(time.RFC3339),
			},
		})
	}

	return &pb.ListMyBookingsResponse{
		Items:      items,
		TotalCount: total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (server *Server) CancelMyBooking(ctx context.Context, req *pb.CancelMyBookingRequest) (*pb.CancelMyBookingResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleClient, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	bookingID, err := uuid.Parse(strings.TrimSpace(req.GetBookingId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}
	_, err = server.store.CancelMyBookingTx(ctx, db.CancelMyBookingTxParams{
		BookingID:    bookingID,
		Actor:        payload,
		ActorEmail:   userEmailByUsername(ctx, server.store, payload.Username),
		CancelReason: strings.TrimSpace(req.GetCancelReason()),
		Now:          time.Now(),
	})
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, status.Errorf(codes.NotFound, "booking not found")
		case errors.Is(err, db.ErrClientBookingAccessDenied):
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		case errors.Is(err, db.ErrClientBookingCancelState), strings.Contains(err.Error(), "outside cancellation window"):
			return nil, status.Errorf(codes.FailedPrecondition, "booking cannot be cancelled")
		default:
			return nil, status.Errorf(codes.Internal, "failed to cancel booking")
		}
	}

	return &pb.CancelMyBookingResponse{}, nil
}

func userEmailByUsername(ctx context.Context, store db.Store, username string) string {
	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return ""
	}
	return user.Email
}

func (server *Server) GetMyBookingRebookContext(ctx context.Context, req *pb.GetMyBookingRebookContextRequest) (*pb.GetMyBookingRebookContextResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleClient, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	bookingID, err := uuid.Parse(strings.TrimSpace(req.GetBookingId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}

	row, err := server.store.GetRebookContextByBookingID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "booking not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load booking")
	}

	if !userHasAdminRole(payload) {
		usernameMatch := row.ClientUsername.Valid && row.ClientUsername.String == payload.Username
		emailMatch := false
		if user, userErr := server.store.GetUserByUsername(ctx, payload.Username); userErr == nil {
			emailMatch = strings.EqualFold(strings.TrimSpace(row.GuestEmail), strings.TrimSpace(user.Email))
		}
		if !usernameMatch && !emailMatch {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
	}

	resp := &pb.GetMyBookingRebookContextResponse{
		SourceBookingId: row.BookingID.String(),
		Location: &pb.RebookContextLocation{
			Id:   row.LocationID.String(),
			Slug: row.LocationSlug,
			Name: row.LocationName,
		},
		Service: &pb.RebookContextService{
			Id:   row.ServiceID.String(),
			Name: row.ServiceName,
		},
		PreferredPractitionerId: valueOrEmptyUUID(row.PractitionerID),
		IsRebookable:            row.LocationIsActive && row.ServiceIsActive,
	}
	if !resp.IsRebookable {
		resp.ReasonCode = "ENTITY_INACTIVE"
	}
	return resp, nil
}

func (server *Server) JoinPublicWaitlist(ctx context.Context, req *pb.JoinPublicWaitlistRequest) (*pb.JoinPublicWaitlistResponse, error) {
	if err := validator.ValidateName(req.GetGuestName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid guest_name")
	}
	if err := validator.ValidateEmail(req.GetGuestEmail()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid guest_email")
	}

	locationID, err := uuid.Parse(strings.TrimSpace(req.GetLocationId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid location_id")
	}
	serviceID, err := uuid.Parse(strings.TrimSpace(req.GetServiceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service_id")
	}
	slotID, err := uuid.Parse(strings.TrimSpace(req.GetSlotId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid slot_id")
	}

	loc, err := server.store.GetLocationByID(ctx, locationID)
	if err != nil || !loc.IsActive {
		return nil, status.Errorf(codes.NotFound, "location not found")
	}
	service, err := server.store.GetServiceByID(ctx, serviceID)
	if err != nil || service.LocationID != locationID || !service.IsActive {
		return nil, status.Errorf(codes.InvalidArgument, "service_id does not belong to location")
	}
	slot, err := server.store.GetHostSlotByID(ctx, db.GetHostSlotByIDParams{
		ID:         slotID,
		LocationID: locationID,
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "slot_id does not belong to location")
	}
	if slot.ServiceID != serviceID {
		return nil, status.Errorf(codes.InvalidArgument, "slot/service mismatch")
	}

	_, err = server.store.GetActiveWaitlistEntryByIdentity(ctx, db.GetActiveWaitlistEntryByIdentityParams{
		LocationID: locationID,
		ServiceID:  serviceID,
		SlotID:     slotID,
		GuestEmail: strings.TrimSpace(req.GetGuestEmail()),
	})
	if err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "already on waitlist")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Errorf(codes.Internal, "failed to check waitlist")
	}

	var practitionerID pgtype.UUID
	if strings.TrimSpace(req.GetPractitionerId()) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(req.GetPractitionerId()))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid practitioner_id")
		}
		practitionerID = pgtype.UUID{Bytes: parsed, Valid: true}
	}
	var preferredDate pgtype.Date
	if strings.TrimSpace(req.GetPreferredDate()) != "" {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(req.GetPreferredDate()))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid preferred_date")
		}
		preferredDate = pgtype.Date{Time: d, Valid: true}
	}

	entry, err := server.store.CreateWaitlistEntry(ctx, db.CreateWaitlistEntryParams{
		LocationID:     locationID,
		ServiceID:      serviceID,
		SlotID:         slotID,
		GuestName:      strings.TrimSpace(req.GetGuestName()),
		GuestEmail:     strings.TrimSpace(req.GetGuestEmail()),
		GuestPhone:     nullableText(strings.TrimSpace(req.GetGuestPhone())),
		PractitionerID: practitionerID,
		PreferredDate:  preferredDate,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to join waitlist")
	}

	position, err := server.store.CountActiveWaitlistEntriesForSlot(ctx, slotID)
	if err != nil {
		position = 0
	}

	return &pb.JoinPublicWaitlistResponse{
		WaitlistEntryId: entry.ID.String(),
		Status:          entry.Status,
		PositionHint:    position,
	}, nil
}

func nullableText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}
