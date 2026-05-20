package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	mockdb "github.com/olshmore/ytter/db/mock"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/ai"
	"github.com/olshmore/ytter/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubAIGateway implements ai.Gateway for handler tests without calling OpenAI.
type stubAIGateway struct {
	enabled  bool
	generate func(context.Context, ai.GenerateRequest) (*ai.GenerateResponse, error)
}

func (s *stubAIGateway) Enabled() bool { return s.enabled }

func (s *stubAIGateway) Generate(ctx context.Context, req ai.GenerateRequest) (*ai.GenerateResponse, error) {
	if s.generate != nil {
		return s.generate(ctx, req)
	}
	return ai.NewFallbackGateway(false).Generate(ctx, req)
}

func newSuggestTestServer(t *testing.T, store db.Store, gw ai.Gateway) *Server {
	t.Helper()
	server := newTestServer(t, store, nil)
	if gw != nil {
		server.aiGateway = gw
	}
	return server
}

func testBookableSlotRow(locationSlug, serviceName string, slotID uuid.UUID) db.ListPublicSlotsByLocationSlugRow {
	locationID := uuid.New()
	serviceID := uuid.New()
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	return db.ListPublicSlotsByLocationSlugRow{
		SlotID:                      slotID,
		StartAt:                     start,
		EndAt:                       start.Add(30 * time.Minute),
		SlotStatus:                  "available",
		Capacity:                    1,
		BookedCount:                 0,
		ServiceID:                   serviceID,
		ServiceName:                 serviceName,
		DurationMinutes:             30,
		PriceMinorUnits:             5000,
		Currency:                    "GBP",
		LocationID:                  locationID,
		LocationSlug:                locationSlug,
		LocationName:                "QA Clinic",
		LocationTimezone:            "Europe/London",
		BookingRequiresHostApproval: false,
	}
}

func TestPublicBookingAssistantSuggest_MissingLocationSlug(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	server := newSuggestTestServer(t, mockdb.NewMockStore(storeCtrl), nil)

	_, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		Prompt: "haircut tomorrow",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPublicBookingAssistantSuggest_MissingPrompt(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	server := newSuggestTestServer(t, mockdb.NewMockStore(storeCtrl), nil)

	_, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "qa-clinic",
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPublicBookingAssistantSuggest_LocationNotFound(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newSuggestTestServer(t, store, nil)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "missing-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "missing-clinic").
		Return(db.Location{}, pgx.ErrNoRows)

	_, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "missing-clinic",
		Prompt:       "any time this week",
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestPublicBookingAssistantSuggest_NoBookableSlots(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newSuggestTestServer(t, store, nil)

	locationID := uuid.New()
	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{}, nil)
	store.EXPECT().
		GetLocationBySlug(gomock.Any(), "qa-clinic").
		Return(db.Location{
			ID:       locationID,
			Slug:     "qa-clinic",
			Name:     "QA Clinic",
			IsActive: true,
		}, nil)

	res, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "book something soon",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.TraceId)
	require.Equal(t, "no_availability_match", res.Intent)
	require.Contains(t, res.ClarifyingQuestion, "browse the picker")
	require.Empty(t, res.SlotSuggestions)
	require.Equal(t, "qa-clinic", res.Entities["location_slug"])
}

func TestPublicBookingAssistantSuggest_DeterministicSuggestions(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newSuggestTestServer(t, store, nil)

	slotID := uuid.New()
	row := testBookableSlotRow("qa-clinic", "Haircut", slotID)

	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{row}, nil)

	res, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "haircut appointment",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.TraceId)
	require.Equal(t, "book_slot", res.Intent)
	require.InDelta(t, 0.85, res.Confidence, 0.001)
	require.Len(t, res.SlotSuggestions, 1)
	require.Equal(t, slotID.String(), res.SlotSuggestions[0].SlotId)
	require.Equal(t, "Haircut", res.SlotSuggestions[0].ServiceName)
}

func TestPublicBookingAssistantSuggest_NarrowTimeIntent(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)
	server := newSuggestTestServer(t, store, nil)

	slotID := uuid.New()
	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			testBookableSlotRow("qa-clinic", "Consult", slotID),
		}, nil)

	res, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "something tomorrow morning",
	})
	require.NoError(t, err)
	require.Equal(t, "narrow_time_preferences", res.Intent)
}

func TestPublicBookingAssistantSuggest_AIGatewaySuccess_FiltersUnknownSlots(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)

	realSlotID := uuid.New()
	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			testBookableSlotRow("qa-clinic", "Massage", realSlotID),
		}, nil)

	fakeSlotID := uuid.NewString()
	gw := &stubAIGateway{
		enabled: true,
		generate: func(_ context.Context, req ai.GenerateRequest) (*ai.GenerateResponse, error) {
			require.Equal(t, ai.FeatureGuestAssistant, req.Feature)
			require.Contains(t, req.UserPrompt, realSlotID.String())

			payload, _ := json.Marshal(map[string]any{
				"intent": "book_slot",
				"entities": map[string]string{
					"service": "massage",
				},
				"confidence": 0.92,
				"slot_suggestions": []map[string]string{
					{
						"slot_id":       realSlotID.String(),
						"start_at":      time.Now().UTC().Format(time.RFC3339),
						"end_at":        time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
						"service_name":  "Massage",
					},
					{
						"slot_id":      fakeSlotID,
						"start_at":     time.Now().UTC().Format(time.RFC3339),
						"end_at":       time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
						"service_name": "Fake",
					},
				},
			})
			return &ai.GenerateResponse{
				Feature:    ai.FeatureGuestAssistant,
				Mode:       ai.ResponseModeSuccess,
				Model:      "gpt-4.1",
				TraceID:    "trace-from-gateway",
				JSON:       payload,
				Confidence: 1,
			}, nil
		},
	}

	server := newSuggestTestServer(t, store, gw)
	res, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "massage please",
	})
	require.NoError(t, err)
	require.Equal(t, "trace-from-gateway", res.TraceId)
	require.Equal(t, "gpt-4.1", res.Model)
	require.Equal(t, "book_slot", res.Intent)
	require.InDelta(t, 0.92, res.Confidence, 0.001)
	require.Len(t, res.SlotSuggestions, 1)
	require.Equal(t, realSlotID.String(), res.SlotSuggestions[0].SlotId)
}

func TestPublicBookingAssistantSuggest_AIGatewayInvalidJSON_FallsBackToDeterministic(t *testing.T) {
	storeCtrl := gomock.NewController(t)
	defer storeCtrl.Finish()
	store := mockdb.NewMockStore(storeCtrl)

	slotID := uuid.New()
	store.EXPECT().
		ListPublicSlotsByLocationSlug(gomock.Any(), "qa-clinic").
		Return([]db.ListPublicSlotsByLocationSlugRow{
			testBookableSlotRow("qa-clinic", "Facial", slotID),
		}, nil)

	gw := &stubAIGateway{
		enabled: true,
		generate: func(context.Context, ai.GenerateRequest) (*ai.GenerateResponse, error) {
			return &ai.GenerateResponse{
				Feature: ai.FeatureGuestAssistant,
				Mode:    ai.ResponseModeSuccess,
				Model:   "gpt-4.1",
				JSON:    json.RawMessage(`not-valid-json`),
			}, nil
		},
	}

	server := newSuggestTestServer(t, store, gw)
	res, err := server.PublicBookingAssistantSuggest(context.Background(), &pb.PublicBookingAssistantSuggestRequest{
		LocationSlug: "qa-clinic",
		Prompt:       "facial treatment",
	})
	require.NoError(t, err)
	require.Equal(t, "book_slot", res.Intent)
	require.Len(t, res.SlotSuggestions, 1)
	require.Equal(t, slotID.String(), res.SlotSuggestions[0].SlotId)
}

func TestEnrichSuggestResponsePB_DefaultIntentAndEntityCoercion(t *testing.T) {
	allowed := map[string]struct{}{"slot-1": {}}
	res := enrichSuggestResponsePB(parsedAssistant{
		Intent: "",
		Entities: map[string]interface{}{
			"weekday": "tuesday",
			"count":   float64(2),
		},
		SlotSuggestions: []parsedAssistantSlotSuggestion{
			{SlotID: "slot-1", StartAt: "2026-05-20T10:00:00Z", EndAt: "2026-05-20T10:30:00Z", ServiceName: "Cut"},
			{SlotID: "invented", StartAt: "2026-05-20T11:00:00Z", EndAt: "2026-05-20T11:30:00Z"},
		},
		Confidence: 0.8,
	}, allowed)

	require.Equal(t, "book_slot", res.Intent)
	require.Equal(t, "tuesday", res.Entities["weekday"])
	require.Equal(t, "2", res.Entities["count"])
	require.Len(t, res.SlotSuggestions, 1)
	require.Equal(t, "slot-1", res.SlotSuggestions[0].SlotId)
}

func TestParseAssistantPayload(t *testing.T) {
	require.Nil(t, parseAssistantPayload(json.RawMessage(`{broken`)))

	raw := json.RawMessage(`{
		"intent":"book_slot",
		"clarifying_question":"Which service?",
		"entities":{"service":"massage"},
		"confidence":0.7,
		"slot_suggestions":[{"slot_id":"a","start_at":"t0","end_at":"t1","service_name":"Massage"}]
	}`)
	parsed := parseAssistantPayload(raw)
	require.NotNil(t, parsed)
	require.Equal(t, "book_slot", parsed.Intent)
	require.Equal(t, "Which service?", parsed.ClarifyingQuestion)
	require.Len(t, parsed.SlotSuggestions, 1)
}

func TestTokenizeAndMatch(t *testing.T) {
	tokens := tokenize("I need a massage next week")
	require.Contains(t, tokens, "massage")
	require.Contains(t, tokens, "week")

	require.True(t, tokens.isEmptyOrMatch("deep tissue massage"))
	require.False(t, tokens.isEmptyOrMatch("haircut only"))
}

func TestHTTPPathToGRPCMethodMap_PublicBookingAssistantSuggest(t *testing.T) {
	mapping, err := HTTPPathToGRPCMethodMap()
	require.NoError(t, err)
	require.Equal(t, RoutePublicBookingAssistantSuggest, mapping["/v1/public/ai/booking-assistant/suggest"])
}

func TestConfigureRoleBasedAccess_PublicBookingAssistantSuggestIsPublic(t *testing.T) {
	config := ConfigureRoleBasedAccess()
	_, requiresAuth := config[RoutePublicBookingAssistantSuggest]
	require.False(t, requiresAuth)
}
