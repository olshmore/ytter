package api

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/booking/access"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/olshmore/ytter/pkg/validator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var isValidCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func (server *Server) ListHostLocationServices(ctx context.Context, req *pb.ListHostLocationServicesRequest) (*pb.ListHostLocationServicesResponse, error) {
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

	limit := normalizeLimit(req.GetLimit())
	offset := normalizeOffset(req.GetOffset())
	filterActive := pgtype.Bool{}
	if req.IsActive != nil {
		filterActive = pgtype.Bool{Bool: req.GetIsActive(), Valid: true}
	}
	total, err := server.store.CountHostServicesByLocation(ctx, db.CountHostServicesByLocationParams{
		LocationID:     loc.ID,
		FilterIsActive: filterActive,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count services")
	}
	rows, err := server.store.ListHostServicesByLocation(ctx, db.ListHostServicesByLocationParams{
		LocationID:     loc.ID,
		FilterIsActive: filterActive,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list services")
	}
	items := make([]*pb.HostServiceItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, hostServiceItemFromDB(row))
	}
	return &pb.ListHostLocationServicesResponse{
		Items:      items,
		TotalCount: total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (server *Server) CreateHostLocationService(ctx context.Context, req *pb.CreateHostLocationServiceRequest) (*pb.CreateHostLocationServiceResponse, error) {
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

	name := strings.TrimSpace(req.GetName())
	if err := validator.ValidateString(name, 1, 160); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid name")
	}
	if req.GetDurationMinutes() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "duration_minutes must be greater than zero")
	}
	if req.GetPriceMinorUnits() < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "price_minor_units must be non-negative")
	}
	description := ""
	if req.Description != nil {
		description = strings.TrimSpace(req.GetDescription())
	}
	currency := "GBP"
	if req.Currency != nil {
		currency = strings.ToUpper(strings.TrimSpace(req.GetCurrency()))
	}
	if !isValidCurrency.MatchString(currency) {
		return nil, status.Errorf(codes.InvalidArgument, "currency must be a 3-letter uppercase code")
	}
	isActive := true
	if req.IsActive != nil {
		isActive = req.GetIsActive()
	}
	cancellationHours := pgtype.Int4{}
	if req.CancellationMinHoursBeforeStart != nil {
		if req.GetCancellationMinHoursBeforeStart() < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "cancellation_min_hours_before_start must be non-negative")
		}
		cancellationHours = pgtype.Int4{Int32: req.GetCancellationMinHoursBeforeStart(), Valid: true}
	}

	service, err := server.store.CreateHostLocationService(ctx, db.CreateHostLocationServiceParams{
		LocationID:                      loc.ID,
		Name:                            name,
		Description:                     description,
		DurationMinutes:                 req.GetDurationMinutes(),
		PriceMinorUnits:                 req.GetPriceMinorUnits(),
		Currency:                        currency,
		IsActive:                        isActive,
		CancellationMinHoursBeforeStart: cancellationHours,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create service")
	}
	return &pb.CreateHostLocationServiceResponse{
		Service: hostServiceItemFromService(service),
	}, nil
}

func (server *Server) UpdateHostLocationService(ctx context.Context, req *pb.UpdateHostLocationServiceRequest) (*pb.UpdateHostLocationServiceResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	serviceID, err := uuid.Parse(strings.TrimSpace(req.GetServiceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service_id")
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
	_, err = server.store.GetHostServiceByID(ctx, db.GetHostServiceByIDParams{
		ID:         serviceID,
		LocationID: loc.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "service not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load service")
	}

	arg := db.UpdateHostLocationServiceParams{
		ID:         serviceID,
		LocationID: loc.ID,
	}
	if req.Name != nil {
		name := strings.TrimSpace(req.GetName())
		if err := validator.ValidateString(name, 1, 160); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid name")
		}
		arg.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Description != nil {
		arg.Description = pgtype.Text{String: strings.TrimSpace(req.GetDescription()), Valid: true}
	}
	if req.DurationMinutes != nil {
		if req.GetDurationMinutes() <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "duration_minutes must be greater than zero")
		}
		arg.DurationMinutes = pgtype.Int4{Int32: req.GetDurationMinutes(), Valid: true}
	}
	if req.PriceMinorUnits != nil {
		if req.GetPriceMinorUnits() < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "price_minor_units must be non-negative")
		}
		arg.PriceMinorUnits = pgtype.Int8{Int64: req.GetPriceMinorUnits(), Valid: true}
	}
	if req.Currency != nil {
		currency := strings.ToUpper(strings.TrimSpace(req.GetCurrency()))
		if !isValidCurrency.MatchString(currency) {
			return nil, status.Errorf(codes.InvalidArgument, "currency must be a 3-letter uppercase code")
		}
		arg.Currency = pgtype.Text{String: currency, Valid: true}
	}
	if req.IsActive != nil {
		arg.IsActive = pgtype.Bool{Bool: req.GetIsActive(), Valid: true}
	}
	if req.CancellationMinHoursBeforeStart != nil {
		if req.GetCancellationMinHoursBeforeStart() < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "cancellation_min_hours_before_start must be non-negative")
		}
		arg.CancellationMinHoursBeforeStart = pgtype.Int4{
			Int32: req.GetCancellationMinHoursBeforeStart(),
			Valid: true,
		}
	}

	service, err := server.store.UpdateHostLocationService(ctx, arg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update service")
	}
	return &pb.UpdateHostLocationServiceResponse{
		Service: hostServiceItemFromService(service),
	}, nil
}

func hostServiceItemFromDB(s db.ListHostServicesByLocationRow) *pb.HostServiceItem {
	item := &pb.HostServiceItem{
		Id:              s.ID.String(),
		LocationId:      s.LocationID.String(),
		Name:            s.Name,
		Description:     s.Description,
		DurationMinutes: s.DurationMinutes,
		PriceMinorUnits: s.PriceMinorUnits,
		Currency:        s.Currency,
		IsActive:        s.IsActive,
	}
	if s.CancellationMinHoursBeforeStart.Valid {
		v := s.CancellationMinHoursBeforeStart.Int32
		item.CancellationMinHoursBeforeStart = &v
	}
	return item
}

func hostServiceItemFromService(s db.Service) *pb.HostServiceItem {
	item := &pb.HostServiceItem{
		Id:              s.ID.String(),
		LocationId:      s.LocationID.String(),
		Name:            s.Name,
		Description:     s.Description,
		DurationMinutes: s.DurationMinutes,
		PriceMinorUnits: s.PriceMinorUnits,
		Currency:        s.Currency,
		IsActive:        s.IsActive,
	}
	if s.CancellationMinHoursBeforeStart.Valid {
		v := s.CancellationMinHoursBeforeStart.Int32
		item.CancellationMinHoursBeforeStart = &v
	}
	return item
}
