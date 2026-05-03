package api

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/booking/access"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/token"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/olshmore/ytter/pkg/validator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var isValidLocationSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func userHasAdminRole(payload *token.Payload) bool {
	for _, r := range payload.Roles {
		if r == utils.RoleAdmin {
			return true
		}
	}
	return false
}

func (server *Server) ListHostLocations(ctx context.Context, _ *pb.ListHostLocationsRequest) (*pb.ListHostLocationsResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	if userHasAdminRole(payload) {
		all, err := server.store.ListAllHostLocations(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list locations")
		}
		items := make([]*pb.HostLocationItem, 0, len(all))
		for _, loc := range all {
			items = append(items, &pb.HostLocationItem{
				Id:                          loc.ID.String(),
				OwnerUsername:               loc.OwnerUsername,
				Slug:                        loc.Slug,
				Name:                        loc.Name,
				Timezone:                    loc.Timezone,
				IsActive:                    loc.IsActive,
				BookingRequiresHostApproval: loc.BookingRequiresHostApproval,
				CancellationMinHoursBeforeStart: loc.EffectiveCancellationMinHoursBeforeStart,
			})
		}
		return &pb.ListHostLocationsResponse{Items: items}, nil
	}

	rows, err := server.store.ListHostLocationsByOwner(ctx, payload.Username)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list locations")
	}
	items := make([]*pb.HostLocationItem, 0, len(rows))
	for _, loc := range rows {
		items = append(items, &pb.HostLocationItem{
			Id:                          loc.ID.String(),
			OwnerUsername:               loc.OwnerUsername,
			Slug:                        loc.Slug,
			Name:                        loc.Name,
			Timezone:                    loc.Timezone,
			IsActive:                    loc.IsActive,
			BookingRequiresHostApproval: loc.BookingRequiresHostApproval,
			CancellationMinHoursBeforeStart: loc.EffectiveCancellationMinHoursBeforeStart,
		})
	}
	return &pb.ListHostLocationsResponse{Items: items}, nil
}

func validateLocationSlug(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return status.Errorf(codes.InvalidArgument, "slug is required")
	}
	if slug != strings.ToLower(slug) || !isValidLocationSlug.MatchString(slug) {
		return status.Errorf(codes.InvalidArgument, "slug must be lowercase letters, digits, and hyphens")
	}
	if len(slug) < 3 || len(slug) > 120 {
		return status.Errorf(codes.InvalidArgument, "slug length must be between 3 and 120")
	}
	return nil
}

func validateTimezone(tz string) error {
	if strings.TrimSpace(tz) == "" {
		return status.Errorf(codes.InvalidArgument, "timezone is required")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid timezone")
	}
	return nil
}

func hostLocationItemFromDB(loc db.Location) *pb.HostLocationItem {
	cancellationHours := int32(24)
	if loc.CancellationMinHoursBeforeStart.Valid {
		cancellationHours = loc.CancellationMinHoursBeforeStart.Int32
	}
	return &pb.HostLocationItem{
		Id:                              loc.ID.String(),
		OwnerUsername:                   loc.OwnerUsername,
		Slug:                            loc.Slug,
		Name:                            loc.Name,
		Timezone:                        loc.Timezone,
		IsActive:                        loc.IsActive,
		BookingRequiresHostApproval:     loc.BookingRequiresHostApproval,
		CancellationMinHoursBeforeStart: cancellationHours,
	}
}

func (server *Server) CreateHostLocation(ctx context.Context, req *pb.CreateHostLocationRequest) (*pb.CreateHostLocationResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	name := strings.TrimSpace(req.GetName())
	if err := validator.ValidateString(name, 1, 160); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid name")
	}
	slug := strings.TrimSpace(req.GetSlug())
	if err := validateLocationSlug(slug); err != nil {
		return nil, err
	}
	timezone := strings.TrimSpace(req.GetTimezone())
	if err := validateTimezone(timezone); err != nil {
		return nil, err
	}
	isActive := true
	if req.IsActive != nil {
		isActive = req.GetIsActive()
	}

	loc, err := server.store.CreateLocation(ctx, db.CreateLocationParams{
		OwnerUsername: payload.Username,
		Name:          name,
		Slug:          slug,
		Timezone:      timezone,
		IsActive:      isActive,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Errorf(codes.AlreadyExists, "location slug already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create location")
	}
	return &pb.CreateHostLocationResponse{Location: hostLocationItemFromDB(loc)}, nil
}

func (server *Server) GetHostLocation(ctx context.Context, req *pb.GetHostLocationRequest) (*pb.GetHostLocationResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	locationID, err := uuid.Parse(strings.TrimSpace(req.GetLocationId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid location_id")
	}
	loc, err := server.store.GetLocationByID(ctx, locationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, loc.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	return &pb.GetHostLocationResponse{Location: hostLocationItemFromDB(loc)}, nil
}

func (server *Server) GetHostLocationBySlug(ctx context.Context, req *pb.GetHostLocationBySlugRequest) (*pb.GetHostLocationBySlugResponse, error) {
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
	return &pb.GetHostLocationBySlugResponse{Location: hostLocationItemFromDB(loc)}, nil
}

func (server *Server) UpdateHostLocation(ctx context.Context, req *pb.UpdateHostLocationRequest) (*pb.UpdateHostLocationResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	current, err := server.store.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, current.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	arg := db.UpdateLocationParams{ID: current.ID}
	if req.Name != nil {
		name := strings.TrimSpace(req.GetName())
		if err := validator.ValidateString(name, 1, 160); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid name")
		}
		arg.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Slug != nil {
		slug := strings.TrimSpace(req.GetSlug())
		if err := validateLocationSlug(slug); err != nil {
			return nil, err
		}
		arg.Slug = pgtype.Text{String: slug, Valid: true}
	}
	if req.Timezone != nil {
		timezone := strings.TrimSpace(req.GetTimezone())
		if err := validateTimezone(timezone); err != nil {
			return nil, err
		}
		arg.Timezone = pgtype.Text{String: timezone, Valid: true}
	}
	if req.IsActive != nil {
		arg.IsActive = pgtype.Bool{Bool: req.GetIsActive(), Valid: true}
	}
	if req.BookingRequiresHostApproval != nil {
		arg.BookingRequiresHostApproval = pgtype.Bool{
			Bool:  req.GetBookingRequiresHostApproval(),
			Valid: true,
		}
	}

	loc, err := server.store.UpdateLocation(ctx, arg)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Errorf(codes.AlreadyExists, "location slug already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to update location")
	}
	return &pb.UpdateHostLocationResponse{Location: hostLocationItemFromDB(loc)}, nil
}

func (server *Server) ListHostLocationBookings(ctx context.Context, req *pb.ListHostLocationBookingsRequest) (*pb.ListHostLocationBookingsResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
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

	filter := db.CountHostBookingsByLocationParams{
		LocationID:   loc.ID,
		FilterStatus: optionalStatusPGText(statusFilter),
		FromDate:     optionalDatePGText(fromDate),
		ToDate:       optionalDatePGText(toDate),
	}

	total, err := server.store.CountHostBookingsByLocation(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count bookings")
	}

	listRows, err := server.store.ListHostBookingsByLocation(ctx, db.ListHostBookingsByLocationParams{
		LocationID:   loc.ID,
		Limit:        limit,
		Offset:       offset,
		FilterStatus: filter.FilterStatus,
		FromDate:     filter.FromDate,
		ToDate:       filter.ToDate,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list bookings")
	}

	items := make([]*pb.HostBookingListItem, 0, len(listRows))
	for _, row := range listRows {
		phone := ""
		if row.GuestPhone.Valid {
			phone = row.GuestPhone.String
		}
		cancelReason := ""
		if row.CancelReason.Valid {
			cancelReason = row.CancelReason.String
		}
		items = append(items, &pb.HostBookingListItem{
			BookingId:  row.BookingID.String(),
			Status:     row.Status,
			BookedAt:   row.BookedAt.Format(time.RFC3339),
			GuestName:  row.GuestName,
			GuestEmail: row.GuestEmail,
			GuestPhone: phone,
			CancelReason: cancelReason,
			IsWaitlist: row.IsWaitlist,
			Slot: &pb.HostBookingSlotSummary{
				SlotId:            row.SlotID.String(),
				ServiceName:       row.ServiceName,
				PractitionerName:  row.PractitionerName,
				RoomName:          row.RoomName,
				StartAt:           row.StartAt.Format(time.RFC3339),
				EndAt:             row.EndAt.Format(time.RFC3339),
			},
		})
	}

	return &pb.ListHostLocationBookingsResponse{
		Items:      items,
		TotalCount: total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (server *Server) HostApproveBooking(ctx context.Context, req *pb.HostApproveBookingRequest) (*pb.HostApproveBookingResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	bookingID, err := uuid.Parse(strings.TrimSpace(req.GetBookingId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}
	res, err := server.store.HostApproveBookingTx(ctx, db.HostApproveBookingTxParams{
		BookingID: bookingID,
		Actor:     payload,
	})
	if err != nil {
		return nil, mapHostBookingTxError(err)
	}
	return &pb.HostApproveBookingResponse{Booking: hostBookingDetailFromDB(res.Booking)}, nil
}

func (server *Server) HostRejectBooking(ctx context.Context, req *pb.HostRejectBookingRequest) (*pb.HostRejectBookingResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	bookingID, err := uuid.Parse(strings.TrimSpace(req.GetBookingId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}
	_, err = server.store.HostRejectBookingTx(ctx, db.HostRejectBookingTxParams{
		BookingID:    bookingID,
		Actor:        payload,
		CancelReason: strings.TrimSpace(req.GetCancelReason()),
	})
	if err != nil {
		return nil, mapHostBookingTxError(err)
	}
	return &pb.HostRejectBookingResponse{}, nil
}

func (server *Server) HostCancelBooking(ctx context.Context, req *pb.HostCancelBookingRequest) (*pb.HostCancelBookingResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	bookingID, err := uuid.Parse(strings.TrimSpace(req.GetBookingId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}
	_, err = server.store.HostCancelBookingTx(ctx, db.HostCancelBookingTxParams{
		BookingID:    bookingID,
		Actor:        payload,
		CancelReason: strings.TrimSpace(req.GetCancelReason()),
	})
	if err != nil {
		return nil, mapHostBookingTxError(err)
	}
	return &pb.HostCancelBookingResponse{}, nil
}

func (server *Server) HostSetBookingNoShow(ctx context.Context, req *pb.HostSetBookingNoShowRequest) (*pb.HostSetBookingNoShowResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	bookingID, err := uuid.Parse(strings.TrimSpace(req.GetBookingId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid booking_id")
	}
	res, err := server.store.HostSetBookingNoShowTx(ctx, db.HostSetBookingNoShowTxParams{
		BookingID: bookingID,
		NoShow:    req.GetNoShow(),
		Actor:     payload,
	})
	if err != nil {
		return nil, mapHostBookingTxError(err)
	}
	return &pb.HostSetBookingNoShowResponse{Booking: hostBookingDetailFromDB(res.Booking)}, nil
}

func mapHostBookingTxError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return status.Errorf(codes.NotFound, "booking not found")
	case errors.Is(err, db.ErrHostBookingAccessDenied):
		return status.Errorf(codes.PermissionDenied, "permission denied")
	case errors.Is(err, db.ErrHostBookingApproveState),
		errors.Is(err, db.ErrHostBookingRejectState),
		errors.Is(err, db.ErrHostBookingCancelState),
		errors.Is(err, db.ErrHostBookingNoShowState):
		return status.Errorf(codes.FailedPrecondition, "booking state does not allow this operation")
	default:
		return status.Errorf(codes.Internal, "operation failed")
	}
}

func hostBookingDetailFromDB(b db.Booking) *pb.HostBookingDetail {
	cu := ""
	if b.ClientUsername.Valid {
		cu = b.ClientUsername.String
	}
	return &pb.HostBookingDetail{
		Id:             b.ID.String(),
		LocationId:     b.LocationID.String(),
		SlotId:         b.SlotID.String(),
		Status:         b.Status,
		GuestName:      b.GuestName,
		GuestEmail:     b.GuestEmail,
		ClientUsername: cu,
		BookedAt:       b.BookedAt.Format(time.RFC3339),
	}
}

func validHostBookingListStatus(s string) bool {
	switch s {
	case "pending", "confirmed", "cancelled", "completed", "no_show":
		return true
	default:
		return false
	}
}

func optionalStatusPGText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optionalDatePGText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func (server *Server) GetHostSetupChecklist(ctx context.Context, _ *pb.GetHostSetupChecklistRequest) (*pb.GetHostSetupChecklistResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locations, err := server.hostAccessibleLocations(ctx, payload)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load locations")
	}
	if len(locations) == 0 {
		return &pb.GetHostSetupChecklistResponse{}, nil
	}

	sample := locations[0]
	hasLocation := true
	hasService := false
	hasFutureSlot := false
	today := time.Now().Format("2006-01-02")

	for _, loc := range locations {
		serviceCount, err := server.store.CountHostServicesByLocation(ctx, db.CountHostServicesByLocationParams{
			LocationID:     loc.ID,
			FilterIsActive: pgtype.Bool{Bool: true, Valid: true},
		})
		if err == nil && serviceCount > 0 {
			hasService = true
		}

		slots, err := server.store.ListHostSlotsByLocation(ctx, db.ListHostSlotsByLocationParams{
			LocationID:   loc.ID,
			Limit:        50,
			Offset:       0,
			FilterStatus: pgtype.Text{String: "available", Valid: true},
			FromDate:     pgtype.Text{String: today, Valid: true},
		})
		if err == nil {
			now := time.Now()
			for _, slot := range slots {
				if slot.Capacity > slot.BookedCount && slot.StartAt.After(now) {
					hasFutureSlot = true
					break
				}
			}
		}

		if hasService && hasFutureSlot {
			sample = loc
			break
		}
	}

	progress := int32(0)
	for _, done := range []bool{hasLocation, hasService, hasFutureSlot} {
		if done {
			progress++
		}
	}

	return &pb.GetHostSetupChecklistResponse{
		HasLocation:        hasLocation,
		HasService:         hasService,
		HasFutureSlot:      hasFutureSlot,
		ProgressDoneCount:  progress,
		ReadyForBooking:    progress == 3,
		SampleLocationSlug: sample.Slug,
	}, nil
}

func (server *Server) GetHostBookingAnalyticsSummary(ctx context.Context, req *pb.GetHostBookingAnalyticsSummaryRequest) (*pb.GetHostBookingAnalyticsSummaryResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	fromDate := strings.TrimSpace(req.GetFromDate())
	toDate := strings.TrimSpace(req.GetToDate())
	if fromDate == "" || toDate == "" {
		return nil, status.Errorf(codes.InvalidArgument, "from_date and to_date are required")
	}
	if _, err := time.Parse("2006-01-02", fromDate); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid from_date")
	}
	if _, err := time.Parse("2006-01-02", toDate); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid to_date")
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

	summary, err := server.store.GetHostBookingAnalyticsSummary(ctx, db.GetHostBookingAnalyticsSummaryParams{
		LocationID: loc.ID,
		FromDate:   pgtype.Text{String: fromDate, Valid: true},
		ToDate:     pgtype.Text{String: toDate, Valid: true},
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to build analytics summary")
	}

	total := float64(summary.TotalCount)
	fillRate := 0.0
	cancellationRate := 0.0
	noShowProxy := 0.0
	if total > 0 {
		fillRate = float64(summary.FilledCount) / total
		cancellationRate = float64(summary.CancelledCount) / total
		noShowProxy = float64(summary.NoShowCount) / total
	}

	return &pb.GetHostBookingAnalyticsSummaryResponse{
		LocationSlug:              locationSlug,
		FromDate:                  fromDate,
		ToDate:                    toDate,
		FillRate:                  fillRate,
		CancellationRate:          cancellationRate,
		PendingApprovalAvgMinutes: numericToFloat64(summary.PendingApprovalAvgMinutes),
		NoShowProxyRate:           noShowProxy,
		GeneratedAt:               time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func numericToFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int64:
		return float64(t)
	case int32:
		return float64(t)
	case []byte:
		parsed, err := strconv.ParseFloat(string(t), 64)
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func (server *Server) hostAccessibleLocations(ctx context.Context, payload *token.Payload) ([]db.Location, error) {
	if userHasAdminRole(payload) {
		rows, err := server.store.ListAllHostLocations(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]db.Location, 0, len(rows))
		for _, row := range rows {
			loc, err := server.store.GetLocationByID(ctx, row.ID)
			if err == nil {
				out = append(out, loc)
			}
		}
		return out, nil
	}
	rows, err := server.store.ListHostLocationsByOwner(ctx, payload.Username)
	if err != nil {
		return nil, err
	}
	out := make([]db.Location, 0, len(rows))
	for _, row := range rows {
		loc, err := server.store.GetLocationByID(ctx, row.ID)
		if err == nil {
			out = append(out, loc)
		}
	}
	return out, nil
}
