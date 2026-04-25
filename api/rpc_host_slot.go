package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/booking/access"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (server *Server) ListHostLocationSlots(ctx context.Context, req *pb.ListHostLocationSlotsRequest) (*pb.ListHostLocationSlotsResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}

	statusFilter := strings.TrimSpace(req.GetStatus())
	if statusFilter != "" && !validHostSlotStatus(statusFilter) {
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

	serviceFilter, err := optionalUUIDFromString(req.GetServiceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service_id")
	}
	practitionerFilter, err := optionalUUIDFromString(req.GetPractitionerId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid practitioner_id")
	}
	roomFilter, err := optionalUUIDFromString(req.GetRoomId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid room_id")
	}

	limit := normalizeLimit(req.GetLimit())
	offset := normalizeOffset(req.GetOffset())

	loc, err := server.store.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, loc.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	filter := db.CountHostSlotsByLocationParams{
		LocationID:           loc.ID,
		FilterStatus:         optionalStatusPGText(statusFilter),
		FromDate:             optionalDatePGText(fromDate),
		ToDate:               optionalDatePGText(toDate),
		FilterServiceID:      serviceFilter,
		FilterPractitionerID: practitionerFilter,
		FilterRoomID:         roomFilter,
	}

	total, err := server.store.CountHostSlotsByLocation(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count slots")
	}

	rows, err := server.store.ListHostSlotsByLocation(ctx, db.ListHostSlotsByLocationParams{
		LocationID:           loc.ID,
		Limit:                limit,
		Offset:               offset,
		FilterStatus:         filter.FilterStatus,
		FromDate:             filter.FromDate,
		ToDate:               filter.ToDate,
		FilterServiceID:      filter.FilterServiceID,
		FilterPractitionerID: filter.FilterPractitionerID,
		FilterRoomID:         filter.FilterRoomID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list slots")
	}

	items := make([]*pb.HostSlotItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, &pb.HostSlotItem{
			Id:               row.ID.String(),
			LocationId:       row.LocationID.String(),
			ServiceId:        row.ServiceID.String(),
			ServiceName:      row.ServiceName,
			PractitionerId:   valueOrEmptyUUID(row.PractitionerID),
			PractitionerName: row.PractitionerName,
			RoomId:           valueOrEmptyUUID(row.RoomID),
			RoomName:         row.RoomName,
			StartAt:          row.StartAt.Format(time.RFC3339),
			EndAt:            row.EndAt.Format(time.RFC3339),
			Capacity:         row.Capacity,
			BookedCount:      row.BookedCount,
			Status:           row.Status,
		})
	}

	return &pb.ListHostLocationSlotsResponse{
		Items:      items,
		TotalCount: total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (server *Server) CreateHostLocationSlot(ctx context.Context, req *pb.CreateHostLocationSlotRequest) (*pb.CreateHostLocationSlotResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}

	loc, err := server.store.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, loc.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	serviceID, err := uuid.Parse(strings.TrimSpace(req.GetServiceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service_id")
	}

	serviceName, err := server.assertServiceInLocation(ctx, serviceID, loc.ID)
	if err != nil {
		return nil, err
	}

	practitionerID, practitionerName, err := server.optionalPractitionerForLocation(ctx, req.PractitionerId, loc.ID)
	if err != nil {
		return nil, err
	}
	roomID, roomName, err := server.optionalRoomForLocation(ctx, req.RoomId, loc.ID)
	if err != nil {
		return nil, err
	}

	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.GetStartAt()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_at")
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.GetEndAt()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_at")
	}
	if !endAt.After(startAt) {
		return nil, status.Errorf(codes.InvalidArgument, "end_at must be after start_at")
	}
	if req.GetCapacity() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "capacity must be greater than zero")
	}

	slotStatus := "available"
	if req.Status != nil {
		slotStatus = strings.TrimSpace(req.GetStatus())
		if !validHostSlotStatus(slotStatus) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid status")
		}
	}

	slot, err := server.store.CreateHostLocationSlot(ctx, db.CreateHostLocationSlotParams{
		LocationID:     loc.ID,
		ServiceID:      serviceID,
		PractitionerID: practitionerID,
		RoomID:         roomID,
		StartAt:        startAt,
		EndAt:          endAt,
		Capacity:       req.GetCapacity(),
		Status:         slotStatus,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == db.CheckViolation {
			return nil, status.Errorf(codes.FailedPrecondition, "slot violates data constraints")
		}
		return nil, status.Errorf(codes.Internal, "failed to create slot")
	}

	return &pb.CreateHostLocationSlotResponse{
		Slot: hostSlotItemFromModel(slot, serviceName, practitionerName, roomName),
	}, nil
}

func (server *Server) UpdateHostLocationSlot(ctx context.Context, req *pb.UpdateHostLocationSlotRequest) (*pb.UpdateHostLocationSlotResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	slotID, err := uuid.Parse(strings.TrimSpace(req.GetSlotId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid slot_id")
	}

	loc, err := server.store.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, loc.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	current, err := server.store.GetHostSlotByID(ctx, db.GetHostSlotByIDParams{
		ID:         slotID,
		LocationID: loc.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "slot not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load slot")
	}

	arg := db.UpdateHostLocationSlotParams{
		ID:         slotID,
		LocationID: loc.ID,
	}

	serviceName := ""
	serviceID := current.ServiceID
	if req.ServiceId != nil {
		serviceID, err = uuid.Parse(strings.TrimSpace(req.GetServiceId()))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid service_id")
		}
		serviceName, err = server.assertServiceInLocation(ctx, serviceID, loc.ID)
		if err != nil {
			return nil, err
		}
		arg.ServiceID = pgtype.UUID{Bytes: serviceID, Valid: true}
	} else {
		serviceName, err = server.serviceNameByID(ctx, serviceID)
		if err != nil {
			return nil, err
		}
	}

	practitionerID := current.PractitionerID
	practitionerName := ""
	if req.PractitionerId != nil {
		practitionerID, practitionerName, err = server.optionalPractitionerForLocation(ctx, req.PractitionerId, loc.ID)
		if err != nil {
			return nil, err
		}
		arg.PractitionerID = practitionerID
	} else {
		practitionerName, err = server.practitionerDisplayNameByID(ctx, practitionerID)
		if err != nil {
			return nil, err
		}
	}

	roomID := current.RoomID
	roomName := ""
	if req.RoomId != nil {
		roomID, roomName, err = server.optionalRoomForLocation(ctx, req.RoomId, loc.ID)
		if err != nil {
			return nil, err
		}
		arg.RoomID = roomID
	} else {
		roomName, err = server.roomNameByID(ctx, roomID)
		if err != nil {
			return nil, err
		}
	}

	effectiveStart := current.StartAt
	if req.StartAt != nil {
		effectiveStart, err = time.Parse(time.RFC3339, strings.TrimSpace(req.GetStartAt()))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid start_at")
		}
		arg.StartAt = pgtype.Timestamptz{Time: effectiveStart, Valid: true}
	}
	effectiveEnd := current.EndAt
	if req.EndAt != nil {
		effectiveEnd, err = time.Parse(time.RFC3339, strings.TrimSpace(req.GetEndAt()))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid end_at")
		}
		arg.EndAt = pgtype.Timestamptz{Time: effectiveEnd, Valid: true}
	}
	if !effectiveEnd.After(effectiveStart) {
		return nil, status.Errorf(codes.InvalidArgument, "end_at must be after start_at")
	}

	if req.Capacity != nil {
		if req.GetCapacity() <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "capacity must be greater than zero")
		}
		if req.GetCapacity() < current.BookedCount {
			return nil, status.Errorf(codes.FailedPrecondition, "capacity cannot be lower than booked_count")
		}
		arg.Capacity = pgtype.Int4{Int32: req.GetCapacity(), Valid: true}
	}

	if req.Status != nil {
		nextStatus := strings.TrimSpace(req.GetStatus())
		if !validHostSlotStatus(nextStatus) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid status")
		}
		arg.Status = pgtype.Text{String: nextStatus, Valid: true}
	}

	slot, err := server.store.UpdateHostLocationSlot(ctx, arg)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == db.CheckViolation {
			return nil, status.Errorf(codes.FailedPrecondition, "slot violates data constraints")
		}
		return nil, status.Errorf(codes.Internal, "failed to update slot")
	}

	return &pb.UpdateHostLocationSlotResponse{
		Slot: hostSlotItemFromModel(slot, serviceName, practitionerName, roomName),
	}, nil
}

func hostSlotItemFromModel(slot db.AppointmentSlot, serviceName, practitionerName, roomName string) *pb.HostSlotItem {
	return &pb.HostSlotItem{
		Id:               slot.ID.String(),
		LocationId:       slot.LocationID.String(),
		ServiceId:        slot.ServiceID.String(),
		ServiceName:      serviceName,
		PractitionerId:   valueOrEmptyUUID(slot.PractitionerID),
		PractitionerName: practitionerName,
		RoomId:           valueOrEmptyUUID(slot.RoomID),
		RoomName:         roomName,
		StartAt:          slot.StartAt.Format(time.RFC3339),
		EndAt:            slot.EndAt.Format(time.RFC3339),
		Capacity:         slot.Capacity,
		BookedCount:      slot.BookedCount,
		Status:           slot.Status,
	}
}

func validHostSlotStatus(s string) bool {
	switch s {
	case "available", "booked", "cancelled", "unavailable":
		return true
	default:
		return false
	}
}

func optionalUUIDFromString(v string) (pgtype.UUID, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.UUID{}, nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func (server *Server) assertServiceInLocation(ctx context.Context, serviceID, locationID uuid.UUID) (string, error) {
	service, err := server.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", status.Errorf(codes.InvalidArgument, "service_id not found")
		}
		return "", status.Errorf(codes.Internal, "failed to load service")
	}
	if service.LocationID != locationID {
		return "", status.Errorf(codes.InvalidArgument, "service_id does not belong to location")
	}
	return service.Name, nil
}

func (server *Server) serviceNameByID(ctx context.Context, serviceID uuid.UUID) (string, error) {
	service, err := server.store.GetServiceByID(ctx, serviceID)
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to load service")
	}
	return service.Name, nil
}

func (server *Server) optionalPractitionerForLocation(ctx context.Context, practitionerIDPtr *string, locationID uuid.UUID) (pgtype.UUID, string, error) {
	if practitionerIDPtr == nil {
		return pgtype.UUID{}, "", nil
	}
	practitionerIDText := strings.TrimSpace(*practitionerIDPtr)
	if practitionerIDText == "" {
		return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "invalid practitioner_id")
	}
	practitionerID, err := uuid.Parse(practitionerIDText)
	if err != nil {
		return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "invalid practitioner_id")
	}
	practitioner, err := server.store.GetPractitionerByID(ctx, practitionerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "practitioner_id not found")
		}
		return pgtype.UUID{}, "", status.Errorf(codes.Internal, "failed to load practitioner")
	}
	if practitioner.LocationID != locationID {
		return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "practitioner_id does not belong to location")
	}
	return pgtype.UUID{Bytes: practitionerID, Valid: true}, practitioner.DisplayName, nil
}

func (server *Server) practitionerDisplayNameByID(ctx context.Context, practitionerID pgtype.UUID) (string, error) {
	if !practitionerID.Valid {
		return "", nil
	}
	id, err := uuid.FromBytes(practitionerID.Bytes[:])
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to parse practitioner id")
	}
	practitioner, err := server.store.GetPractitionerByID(ctx, id)
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to load practitioner")
	}
	return practitioner.DisplayName, nil
}

func (server *Server) optionalRoomForLocation(ctx context.Context, roomIDPtr *string, locationID uuid.UUID) (pgtype.UUID, string, error) {
	if roomIDPtr == nil {
		return pgtype.UUID{}, "", nil
	}
	roomIDText := strings.TrimSpace(*roomIDPtr)
	if roomIDText == "" {
		return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "invalid room_id")
	}
	roomID, err := uuid.Parse(roomIDText)
	if err != nil {
		return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "invalid room_id")
	}
	room, err := server.store.GetRoomByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "room_id not found")
		}
		return pgtype.UUID{}, "", status.Errorf(codes.Internal, "failed to load room")
	}
	if room.LocationID != locationID {
		return pgtype.UUID{}, "", status.Errorf(codes.InvalidArgument, "room_id does not belong to location")
	}
	return pgtype.UUID{Bytes: roomID, Valid: true}, room.Name, nil
}

func (server *Server) roomNameByID(ctx context.Context, roomID pgtype.UUID) (string, error) {
	if !roomID.Valid {
		return "", nil
	}
	id, err := uuid.FromBytes(roomID.Bytes[:])
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to parse room id")
	}
	room, err := server.store.GetRoomByID(ctx, id)
	if err != nil {
		return "", status.Errorf(codes.Internal, "failed to load room")
	}
	return room.Name, nil
}
