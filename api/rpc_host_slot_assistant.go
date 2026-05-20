package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/ai"
	"github.com/olshmore/ytter/internal/booking/access"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type hostAIPlanPayload struct {
	ServiceName      string   `json:"service_name"`
	DateFrom         string   `json:"date_from"`
	DateTo           string   `json:"date_to"`
	Weekdays         []string `json:"weekdays"`
	DailyStartLocal  string   `json:"daily_start_local"`
	DailyEndLocal    string   `json:"daily_end_local"`
	SlotMinutes      int32    `json:"slot_minutes"`
	Capacity         int32    `json:"capacity"`
	Status           string   `json:"status"`
	PractitionerName string   `json:"practitioner_name"`
	RoomName         string   `json:"room_name"`
	Confidence       float64  `json:"confidence"`
	Notes            string   `json:"notes"`
}

func (server *Server) HostSlotAssistantPreview(
	ctx context.Context,
	req *pb.HostSlotAssistantPreviewRequest,
) (*pb.HostSlotAssistantPreviewResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	prompt := strings.TrimSpace(req.GetPrompt())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	if prompt == "" {
		return nil, status.Errorf(codes.InvalidArgument, "prompt is required")
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

	services, err := server.store.ListHostServicesByLocation(ctx, db.ListHostServicesByLocationParams{
		LocationID:     loc.ID,
		FilterIsActive: pgtype.Bool{Bool: true, Valid: true},
		Limit:          200,
		Offset:         0,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list services")
	}
	if len(services) == 0 {
		return &pb.HostSlotAssistantPreviewResponse{
			RequiresConfirmation: true,
			ValidationErrors:     []string{"Add at least one active service before using the slot assistant."},
			TraceId:              uuid.NewString(),
		}, nil
	}

	practitioners, _ := server.store.ListPublicFilterPractitionersByLocationSlug(ctx, locationSlug)
	rooms, _ := server.store.ListPublicFilterRoomsByLocationSlug(ctx, locationSlug)

	servicesJSON, _ := json.Marshal(servicesForAIPrompt(services))
	resourcesJSON, _ := json.Marshal(map[string]any{
		"practitioners": practitionersForAIPrompt(practitioners),
		"rooms":         roomsForAIPrompt(rooms),
	})

	traceID := uuid.NewString()
	out := &pb.HostSlotAssistantPreviewResponse{
		RequiresConfirmation: true,
		TraceId:              traceID,
		Confidence:           0,
	}

	var parsed *hostAIPlanPayload
	gw := server.aiGateway
	if gw == nil {
		gw = ai.NewFallbackGateway(server.config.AIEnableLogging)
	}
	if gw.Enabled() {
		genRes, genErr := gw.Generate(ctx, ai.GenerateRequest{
			Feature:      ai.FeatureHostAssistant,
			SystemPrompt: ai.HostSlotAssistantSystemPromptV1,
			UserPrompt: ai.HostSlotAssistantUserPromptV1(
				prompt,
				loc.Name,
				loc.Timezone,
				locationLocalToday(loc.Timezone),
				string(servicesJSON),
				string(resourcesJSON),
			),
			Schema:    ai.HostSlotAssistantPlanSchema,
			MaxTokens: 1024,
		})
		if genErr == nil && genRes != nil && genRes.Mode == ai.ResponseModeSuccess && len(genRes.JSON) > 0 {
			var p hostAIPlanPayload
			if json.Unmarshal(genRes.JSON, &p) == nil {
				parsed = &p
				out.Model = genRes.Model
				out.Confidence = p.Confidence
				if genRes.TraceID != "" {
					out.TraceId = genRes.TraceID
				}
			}
		}
	}

	if parsed == nil {
		out.ValidationErrors = []string{
			"Could not generate a slot plan automatically. Use the batch create form below or check AI configuration.",
		}
		return out, nil
	}

	out.PlanDateFrom = strings.TrimSpace(parsed.DateFrom)
	out.PlanDateTo = strings.TrimSpace(parsed.DateTo)

	resolved, validation, blocking := server.resolveHostAIPlan(services, practitioners, rooms, loc.Timezone, parsed)
	out.ValidationErrors = append(out.ValidationErrors, validation...)
	if blocking {
		out.PlanOperations = nil
		return out, nil
	}

	existing, err := server.store.ListHostSlotsByLocation(ctx, db.ListHostSlotsByLocationParams{
		LocationID: loc.ID,
		Limit:      5000,
		Offset:     0,
		FromDate:   optionalDatePGText(resolved.DateFrom.Format("2006-01-02")),
		ToDate:     optionalDatePGText(resolved.DateTo.Format("2006-01-02")),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load existing slots")
	}

	ops, expandValidation, totalCount, expandBlocking := expandBatchSlotPlan(
		resolved,
		loadExistingSlotIntervals(existing),
	)
	out.ValidationErrors = append(out.ValidationErrors, expandValidation...)
	out.PlanOperations = ops
	out.TotalSlotCount = int32(totalCount)
	blocking = blocking || expandBlocking

	if parsed.Notes != "" && parsed.Confidence < 0.5 {
		out.ValidationErrors = append(out.ValidationErrors, parsed.Notes)
	}

	batchReq := hostBatchRequestFromResolved(locationSlug, resolved)
	stored := newStoredHostSlotPlan(locationSlug, payload.Username, batchReq, blocking)
	server.hostSlotPlans.put(stored)
	out.PlanId = stored.planID

	if blocking {
		out.RequiresConfirmation = true
	} else if len(ops) > 0 {
		out.RequiresConfirmation = true
	}

	return out, nil
}

func (server *Server) HostSlotAssistantPublish(
	ctx context.Context,
	req *pb.HostSlotAssistantPublishRequest,
) (*pb.HostSlotAssistantPublishResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}

	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	planID := strings.TrimSpace(req.GetPlanId())
	idempotencyKey := strings.TrimSpace(req.GetIdempotencyKey())

	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	if planID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "plan_id is required")
	}
	if !req.GetConfirmed() {
		return nil, status.Errorf(codes.InvalidArgument, "confirmed must be true")
	}

	if cached, ok := server.hostSlotPlans.getPublishCache(idempotencyKey); ok {
		return cached, nil
	}

	plan, ok := server.hostSlotPlans.get(planID, locationSlug, payload.Username)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "plan not found or expired; generate a new preview")
	}
	if plan.hasBlocking {
		return nil, status.Errorf(codes.FailedPrecondition, "plan has blocking validation issues; fix and preview again")
	}

	batchRes, err := server.CreateHostLocationSlotsBatch(ctx, plan.batch)
	if err != nil {
		return nil, err
	}

	out := &pb.HostSlotAssistantPublishResponse{
		CreatedCount: batchRes.GetCreatedCount(),
		SkippedCount: batchRes.GetSkippedCount(),
		Errors:       batchRes.GetErrors(),
		TraceId:      uuid.NewString(),
	}
	server.hostSlotPlans.putPublishCache(idempotencyKey, out)
	server.hostSlotPlans.delete(planID)
	return out, nil
}

func (server *Server) resolveHostAIPlan(
	services []db.ListHostServicesByLocationRow,
	practitioners []db.ListPublicFilterPractitionersByLocationSlugRow,
	rooms []db.ListPublicFilterRoomsByLocationSlugRow,
	timezone string,
	parsed *hostAIPlanPayload,
) (resolvedBatchSlotPlan, []string, bool) {
	var validation []string
	blocking := false

	svc, ok := resolveServiceByName(services, parsed.ServiceName)
	if !ok {
		validation = append(validation, fmt.Sprintf("service %q not found at this location", strings.TrimSpace(parsed.ServiceName)))
		return resolvedBatchSlotPlan{}, validation, true
	}

	dateFrom, err := time.Parse("2006-01-02", strings.TrimSpace(parsed.DateFrom))
	if err != nil {
		validation = append(validation, "invalid date_from from assistant")
		blocking = true
	}
	dateTo, err := time.Parse("2006-01-02", strings.TrimSpace(parsed.DateTo))
	if err != nil {
		validation = append(validation, "invalid date_to from assistant")
		blocking = true
	}
	if !blocking && dateTo.Before(dateFrom) {
		validation = append(validation, "date_to must be on or after date_from")
		blocking = true
	}
	if !blocking {
		dateMsgs, dateBlocking := validateBatchDateStringsNotBeforeToday(
			parsed.DateFrom,
			parsed.DateTo,
			locationLocalToday(timezone),
		)
		validation = append(validation, dateMsgs...)
		blocking = blocking || dateBlocking
	}

	startDur, err := parseHHMMDuration(parsed.DailyStartLocal)
	if err != nil {
		validation = append(validation, "invalid daily_start_local")
		blocking = true
	}
	endDur, err := parseHHMMDuration(parsed.DailyEndLocal)
	if err != nil {
		validation = append(validation, "invalid daily_end_local")
		blocking = true
	}

	weekdaySet := map[string]struct{}{}
	for _, day := range parsed.Weekdays {
		if key := normalizeWeekdayToken(day); key != "" {
			weekdaySet[key] = struct{}{}
		}
	}

	statusText := strings.TrimSpace(parsed.Status)
	if statusText == "" {
		statusText = "available"
	}
	if !validHostSlotStatus(statusText) {
		validation = append(validation, "invalid status from assistant")
		blocking = true
	}

	slotMinutes := parsed.SlotMinutes
	if slotMinutes <= 0 {
		slotMinutes = 30
	}
	capacity := parsed.Capacity
	if capacity <= 0 {
		capacity = 1
	}

	practitionerID := pgtype.UUID{}
	practitionerName := ""
	if name := strings.TrimSpace(parsed.PractitionerName); name != "" {
		id, display, found, ambig := resolvePractitionerByName(practitioners, name)
		if !found {
			validation = append(validation, fmt.Sprintf("practitioner %q not found", name))
			blocking = true
		} else if ambig {
			validation = append(validation, fmt.Sprintf("practitioner %q is ambiguous; pick a single practitioner", name))
			blocking = true
		} else {
			practitionerID = pgtype.UUID{Bytes: id, Valid: true}
			practitionerName = display
		}
	}

	roomID := pgtype.UUID{}
	roomName := ""
	if name := strings.TrimSpace(parsed.RoomName); name != "" {
		id, display, found, ambig := resolveRoomByName(rooms, name)
		if !found {
			validation = append(validation, fmt.Sprintf("room %q not found", name))
			blocking = true
		} else if ambig {
			validation = append(validation, fmt.Sprintf("room %q is ambiguous; pick a single room", name))
			blocking = true
		} else {
			roomID = pgtype.UUID{Bytes: id, Valid: true}
			roomName = display
		}
	}

	if blocking {
		return resolvedBatchSlotPlan{}, validation, true
	}

	return resolvedBatchSlotPlan{
		ServiceID:        svc.ID,
		ServiceName:      svc.Name,
		PractitionerID:   practitionerID,
		PractitionerName: practitionerName,
		RoomID:           roomID,
		RoomName:         roomName,
		DateFrom:         dateFrom,
		DateTo:           dateTo,
		Weekdays:         weekdaySet,
		DailyStart:       startDur,
		DailyEnd:         endDur,
		SlotMinutes:      slotMinutes,
		Capacity:         capacity,
		Status:           statusText,
	}, validation, false
}

func hostBatchRequestFromResolved(locationSlug string, plan resolvedBatchSlotPlan) *pb.CreateHostLocationSlotsBatchRequest {
	weekdays := make([]string, 0, len(plan.Weekdays))
	for day := range plan.Weekdays {
		weekdays = append(weekdays, day)
	}

	req := &pb.CreateHostLocationSlotsBatchRequest{
		LocationSlug:    locationSlug,
		ServiceId:       plan.ServiceID.String(),
		DateFrom:        plan.DateFrom.Format("2006-01-02"),
		DateTo:          plan.DateTo.Format("2006-01-02"),
		Weekdays:        weekdays,
		DailyStartLocal: formatDurationHHMM(plan.DailyStart),
		DailyEndLocal:   formatDurationHHMM(plan.DailyEnd),
		SlotMinutes:     plan.SlotMinutes,
		Capacity:        plan.Capacity,
		Status:          plan.Status,
	}
	if plan.PractitionerID.Valid {
		id, err := uuid.FromBytes(plan.PractitionerID.Bytes[:])
		if err == nil {
			s := id.String()
			req.PractitionerId = &s
		}
	}
	if plan.RoomID.Valid {
		id, err := uuid.FromBytes(plan.RoomID.Bytes[:])
		if err == nil {
			s := id.String()
			req.RoomId = &s
		}
	}
	return req
}

func formatDurationHHMM(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func locationLocalToday(timezone string) string {
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || loc == nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01-02")
}

func servicesForAIPrompt(services []db.ListHostServicesByLocationRow) []map[string]any {
	out := make([]map[string]any, 0, len(services))
	for _, s := range services {
		out = append(out, map[string]any{
			"id":   s.ID.String(),
			"name": s.Name,
		})
	}
	return out
}

func practitionersForAIPrompt(rows []db.ListPublicFilterPractitionersByLocationSlugRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"id":   r.ID.String(),
			"name": r.DisplayName,
		})
	}
	return out
}

func roomsForAIPrompt(rows []db.ListPublicFilterRoomsByLocationSlugRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"id":   r.ID.String(),
			"name": r.Name,
		})
	}
	return out
}

func resolvePractitionerByName(
	rows []db.ListPublicFilterPractitionersByLocationSlugRow,
	name string,
) (uuid.UUID, string, bool, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	var matchID uuid.UUID
	var matchName string
	count := 0
	for _, r := range rows {
		n := strings.ToLower(strings.TrimSpace(r.DisplayName))
		if n == want || strings.Contains(n, want) || strings.Contains(want, n) {
			matchID = r.ID
			matchName = r.DisplayName
			count++
		}
	}
	return matchID, matchName, count > 0, count > 1
}

func resolveRoomByName(
	rows []db.ListPublicFilterRoomsByLocationSlugRow,
	name string,
) (uuid.UUID, string, bool, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	var matchID uuid.UUID
	var matchName string
	count := 0
	for _, r := range rows {
		n := strings.ToLower(strings.TrimSpace(r.Name))
		if n == want || strings.Contains(n, want) || strings.Contains(want, n) {
			matchID = r.ID
			matchName = r.Name
			count++
		}
	}
	return matchID, matchName, count > 0, count > 1
}
