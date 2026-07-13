package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/ai"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newHostAssistantTestServer(t *testing.T, store db.Store, gw ai.Gateway) *Server {
	t.Helper()
	server := newTestServer(t, store, nil)
	if gw != nil {
		server.aiGateway = gw
	}
	return server
}

func TestHostSlotAssistantPreview_MissingPrompt(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	server := newHostAssistantTestServer(t, mockdb.NewMockStore(storeCtrl), nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.HostSlotAssistantPreview(ctx, &pb.HostSlotAssistantPreviewRequest{
		LocationSlug: "qa-clinic",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHostSlotAssistantPreview_AIGeneratesPlan(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)

	locationID := uuid.New()
	serviceID := uuid.New()

	// Next Monday at least one day ahead, then a Mon–Sun window with mon/wed slots.
	from := time.Now().UTC().AddDate(0, 0, 1)
	for from.Weekday() != time.Monday {
		from = from.AddDate(0, 0, 1)
	}
	to := from.AddDate(0, 0, 6)

	planJSON, err := json.Marshal(hostAIPlanPayload{
		ServiceName:     "Massage",
		DateFrom:        from.Format("2006-01-02"),
		DateTo:          to.Format("2006-01-02"),
		Weekdays:        []string{"mon", "wed"},
		DailyStartLocal: "09:00",
		DailyEndLocal:   "10:00",
		SlotMinutes:     30,
		Capacity:        1,
		Status:          "available",
		Confidence:      0.9,
	})
	require.NoError(t, err)

	gw := &stubAIGateway{
		enabled: true,
		generate: func(_ context.Context, req ai.GenerateRequest) (*ai.GenerateResponse, error) {
			require.Equal(t, ai.FeatureHostAssistant, req.Feature)
			return &ai.GenerateResponse{
				Feature: ai.FeatureHostAssistant,
				Mode:    ai.ResponseModeSuccess,
				Model:   "test-model",
				TraceID: "trace-host-1",
				JSON:    planJSON,
			}, nil
		},
	}

	server := newHostAssistantTestServer(t, store, gw)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	store.EXPECT().GetLocationBySlug(gomock.Any(), "qa-clinic").Return(db.Location{
		ID:            locationID,
		OwnerUsername: "hostuser",
		Slug:          "qa-clinic",
		Name:          "QA Clinic",
		Timezone:      "Europe/London",
	}, nil)
	store.EXPECT().ListHostServicesByLocation(gomock.Any(), gomock.Any()).Return([]db.ListHostServicesByLocationRow{
		{ID: serviceID, Name: "Massage", LocationID: locationID, IsActive: true},
	}, nil)
	store.EXPECT().ListPublicFilterPractitionersByLocationSlug(gomock.Any(), "qa-clinic").Return(nil, nil)
	store.EXPECT().ListPublicFilterRoomsByLocationSlug(gomock.Any(), "qa-clinic").Return(nil, nil)
	store.EXPECT().ListHostSlotsByLocation(gomock.Any(), gomock.Any()).Return(nil, nil)

	res, err := server.HostSlotAssistantPreview(ctx, &pb.HostSlotAssistantPreviewRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "Massage slots Monday and Wednesday mornings in June first week",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.GetPlanId())
	require.NotEmpty(t, res.GetPlanOperations())
	require.Empty(t, res.GetValidationErrors())
	require.Equal(t, "test-model", res.GetModel())
}

func TestHostSlotAssistantPublish_RequiresConfirmation(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	server := newHostAssistantTestServer(t, mockdb.NewMockStore(storeCtrl), nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	_, err := server.HostSlotAssistantPublish(ctx, &pb.HostSlotAssistantPublishRequest{
		LocationSlug: "qa-clinic",
		PlanId:       uuid.NewString(),
		Confirmed:    false,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHostSlotAssistantPublish_ExecutesBatch(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newHostAssistantTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	locationID := uuid.New()
	serviceID := uuid.New()
	planID := uuid.NewString()

	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	slotDate := tomorrow.Format("2006-01-02")
	weekday := strings.ToLower(tomorrow.Weekday().String()[:3])

	server.hostSlotPlans.put(&storedHostSlotPlan{
		planID:        planID,
		locationSlug:  "qa-clinic",
		ownerUsername: "hostuser",
		hasBlocking:   false,
		createdAt:     time.Now(),
		expiresAt:     time.Now().Add(hostSlotPlanTTL),
		batch: &pb.CreateHostLocationSlotsBatchRequest{
			LocationSlug:    "qa-clinic",
			ServiceId:       serviceID.String(),
			DateFrom:        slotDate,
			DateTo:          slotDate,
			Weekdays:        []string{weekday},
			DailyStartLocal: "09:00",
			DailyEndLocal:   "09:30",
			SlotMinutes:     30,
			Capacity:        1,
			Status:          "available",
		},
	})

	store.EXPECT().GetLocationBySlug(gomock.Any(), "qa-clinic").Return(db.Location{
		ID:            locationID,
		OwnerUsername: "hostuser",
		Slug:          "qa-clinic",
	}, nil)
	store.EXPECT().GetServiceByID(gomock.Any(), serviceID).Return(db.Service{
		ID:         serviceID,
		LocationID: locationID,
		Name:       "Massage",
	}, nil)
	store.EXPECT().
		CreateHostLocationSlot(gomock.Any(), gomock.Any()).
		Return(db.AppointmentSlot{ID: uuid.New()}, nil)

	res, err := server.HostSlotAssistantPublish(ctx, &pb.HostSlotAssistantPublishRequest{
		LocationSlug:    "qa-clinic",
		PlanId:          planID,
		Confirmed:       true,
		IdempotencyKey:  "idem-1",
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), res.GetCreatedCount())

	// idempotent replay
	res2, err := server.HostSlotAssistantPublish(ctx, &pb.HostSlotAssistantPublishRequest{
		LocationSlug:   "qa-clinic",
		PlanId:         uuid.NewString(),
		Confirmed:      true,
		IdempotencyKey: "idem-1",
	})
	require.NoError(t, err)
	require.Equal(t, res.GetCreatedCount(), res2.GetCreatedCount())
}

func TestHostSlotAssistantPreview_NoServices(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newHostAssistantTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	locationID := uuid.New()
	store.EXPECT().GetLocationBySlug(gomock.Any(), "qa-clinic").Return(db.Location{
		ID:            locationID,
		OwnerUsername: "hostuser",
	}, nil)
	store.EXPECT().ListHostServicesByLocation(gomock.Any(), gomock.Any()).Return(nil, nil)

	res, err := server.HostSlotAssistantPreview(ctx, &pb.HostSlotAssistantPreviewRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "add slots",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.GetValidationErrors())
}

func TestHostSlotAssistantPreview_LocationNotFound(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newHostAssistantTestServer(t, store, nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	store.EXPECT().GetLocationBySlug(gomock.Any(), "missing").Return(db.Location{}, pgx.ErrNoRows)

	_, err := server.HostSlotAssistantPreview(ctx, &pb.HostSlotAssistantPreviewRequest{
		LocationSlug: "missing",
		Prompt:       "slots",
	})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestValidateBatchDateStringsNotBeforeToday_RejectsPastYear(t *testing.T) {
	msgs, blocking := validateBatchDateStringsNotBeforeToday(
		"2024-05-17",
		"2024-05-31",
		"2026-05-17",
	)
	require.True(t, blocking)
	require.Contains(t, strings.Join(msgs, " "), "date_from")
	require.Contains(t, strings.Join(msgs, " "), "17 May 2026")
	require.Contains(t, strings.Join(msgs, " "), "date_to")
}

func TestValidateBatchDateStringsNotBeforeToday_AllowsToday(t *testing.T) {
	msgs, blocking := validateBatchDateStringsNotBeforeToday(
		"2026-05-17",
		"2026-06-01",
		"2026-05-17",
	)
	require.False(t, blocking)
	require.Empty(t, msgs)
}

func TestExpandBatchSlotPlan_AllPastIsBlocking(t *testing.T) {
	past := time.Now().UTC().AddDate(0, 0, -14)
	plan := resolvedBatchSlotPlan{
		ServiceName: "Massage",
		DateFrom:    past,
		DateTo:      past.AddDate(0, 0, 2),
		Weekdays: map[string]struct{}{
			weekdayKeyFromTime(past):             {},
			weekdayKeyFromTime(past.AddDate(0, 0, 1)): {},
			weekdayKeyFromTime(past.AddDate(0, 0, 2)): {},
		},
		DailyStart:  9 * time.Hour,
		DailyEnd:    10 * time.Hour,
		SlotMinutes: 30,
		Capacity:    1,
		Status:      "available",
	}
	_, validation, _, blocking := expandBatchSlotPlan(plan, nil)
	require.True(t, blocking)
	require.Contains(t, strings.Join(validation, " "), "past")
}

func TestExpandBatchSlotPlan_OverlapMarksError(t *testing.T) {
	start := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	existing := []slotTimeInterval{
		{Start: start, End: start.Add(30 * time.Minute)},
	}
	plan := resolvedBatchSlotPlan{
		ServiceName: "Massage",
		DateFrom:    time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		DateTo:      time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		Weekdays:    map[string]struct{}{"tue": {}},
		DailyStart:  9 * time.Hour,
		DailyEnd:    10 * time.Hour,
		SlotMinutes: 30,
		Capacity:    1,
		Status:      "available",
	}
	ops, _, _, blocking := expandBatchSlotPlan(plan, existing)
	require.True(t, blocking)
	require.Len(t, ops, 2)
	require.Equal(t, "error", ops[0].GetStatus())
}

func TestHTTPPathToGRPCMethodMap_HostSlotAssistant(t *testing.T) {
	mapping, err := HTTPPathToGRPCMethodMap()
	require.NoError(t, err)
	require.Equal(t, RouteHostSlotAssistantPreview, mapping["/v1/host/ai/slot-assistant/preview"])
	require.Equal(t, RouteHostSlotAssistantPublish, mapping["/v1/host/ai/slot-assistant/publish"])
}

func TestConfigureRoleBasedAccess_HostSlotAssistantRequiresHostAuth(t *testing.T) {
	config := ConfigureRoleBasedAccess()
	roles, exists := config[RouteHostSlotAssistantPreview]
	require.True(t, exists)
	require.ElementsMatch(t, []utils.Role{utils.RoleHost, utils.RoleAdmin}, roles)
	roles, exists = config[RouteHostSlotAssistantPublish]
	require.True(t, exists)
	require.ElementsMatch(t, []utils.Role{utils.RoleHost, utils.RoleAdmin}, roles)
}

func TestHostSlotAssistantPublish_BlocksWhenPlanHasBlocking(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	server := newHostAssistantTestServer(t, mockdb.NewMockStore(storeCtrl), nil)
	ctx := newContextWithBearerToken(t, server.tokenMaker, "hostuser", []utils.Role{utils.RoleHost}, time.Minute)

	planID := uuid.NewString()
	server.hostSlotPlans.put(&storedHostSlotPlan{
		planID:        planID,
		locationSlug:  "qa-clinic",
		ownerUsername: "hostuser",
		hasBlocking:   true,
		createdAt:     time.Now(),
		expiresAt:     time.Now().Add(hostSlotPlanTTL),
		batch:         &pb.CreateHostLocationSlotsBatchRequest{LocationSlug: "qa-clinic"},
	})

	_, err := server.HostSlotAssistantPublish(ctx, &pb.HostSlotAssistantPublishRequest{
		LocationSlug:   "qa-clinic",
		PlanId:         planID,
		Confirmed:      true,
		IdempotencyKey: "idem-blocked",
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestResolveHostAIPlan_RejectsPastDateRange(t *testing.T) {
	server := &Server{}
	_, validation, blocking := server.resolveHostAIPlan(
		[]db.ListHostServicesByLocationRow{
			{ID: uuid.New(), Name: "Massage"},
		},
		nil,
		nil,
		"Europe/London",
		&hostAIPlanPayload{
			ServiceName:     "Massage",
			DateFrom:        "2024-05-17",
			DateTo:          "2024-05-31",
			Weekdays:        []string{"mon"},
			DailyStartLocal: "09:00",
			DailyEndLocal:   "17:00",
			SlotMinutes:     30,
			Capacity:        1,
			Status:          "available",
		},
	)
	require.True(t, blocking)
	require.Contains(t, strings.Join(validation, " "), "date_from must be on or after today")
}

func TestResolveHostAIPlan_InvalidService(t *testing.T) {
	server := &Server{}
	_, validation, blocking := server.resolveHostAIPlan(
		[]db.ListHostServicesByLocationRow{
			{ID: uuid.New(), Name: "Haircut"},
		},
		nil,
		nil,
		"UTC",
		&hostAIPlanPayload{
			ServiceName:     "Massage",
			DateFrom:        "2026-06-01",
			DateTo:          "2026-06-02",
			Weekdays:        []string{"mon"},
			DailyStartLocal: "09:00",
			DailyEndLocal:   "17:00",
			SlotMinutes:     30,
			Capacity:        1,
			Status:          "available",
		},
	)
	require.True(t, blocking)
	require.NotEmpty(t, validation)
}
