package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/olshmore/ytter/internal/ai"
	"github.com/olshmore/ytter/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (server *Server) PublicBookingAssistantSuggest(
	ctx context.Context,
	req *pb.PublicBookingAssistantSuggestRequest,
) (*pb.PublicBookingAssistantSuggestResponse, error) {
	prompt := strings.TrimSpace(req.GetPrompt())
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	if prompt == "" {
		return nil, status.Errorf(codes.InvalidArgument, "prompt is required")
	}

	slotsReq := &pb.ListPublicSlotsRequest{
		LocationSlug:  locationSlug,
		OnlyAvailable: true,
		Limit:         48,
		Offset:        0,
		TimeSlot:      "any",
	}
	if sid := strings.TrimSpace(req.GetServiceId()); sid != "" {
		slotsReq.ServiceId = sid
	}
	if pid := strings.TrimSpace(req.GetPractitionerId()); pid != "" {
		slotsReq.PractitionerId = pid
	}
	if rid := strings.TrimSpace(req.GetRoomId()); rid != "" {
		slotsReq.RoomId = rid
	}
	if d := strings.TrimSpace(req.GetDate()); d != "" {
		slotsReq.Date = d
	}

	slotsResp, err := server.ListPublicSlots(ctx, slotsReq)
	if err != nil {
		return nil, err
	}

	bookable := make([]*pb.PublicSlotsItem, 0)
	for _, item := range slotsResp.Items {
		if item != nil && item.IsBookable && item.AvailabilityStatus == "available" {
			bookable = append(bookable, item)
		}
	}

	minimal := minimalSlotsForPrompt(bookable, 24)
	inventoryJSON, err := json.Marshal(minimal)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal inventory")
	}

	allowedIDs := allowedSlotIDs(minimal)
	traceID := uuid.NewString()

	out := &pb.PublicBookingAssistantSuggestResponse{
		TraceId: traceID,
		Entities: map[string]string{
			"location_slug": locationSlug,
		},
	}

	gw := server.aiGateway
	if gw == nil {
		gw = ai.NewFallbackGateway(server.config.AIEnableLogging)
	}

	if gw.Enabled() {
		genRes, genErr := gw.Generate(ctx, ai.GenerateRequest{
			Feature:      ai.FeatureGuestAssistant,
			SystemPrompt: ai.GuestBookingAssistantSystemPromptV1,
			UserPrompt:   ai.GuestBookingAssistantUserPromptV1(prompt, string(inventoryJSON)),
			Schema:       ai.GuestBookingAssistantResponseSchema,
			MaxTokens:    768,
		})
		if genErr == nil && genRes != nil && genRes.Mode == ai.ResponseModeSuccess && len(genRes.JSON) > 0 {
			if parsed := parseAssistantPayload(genRes.JSON); parsed != nil {
				out = enrichSuggestResponsePB(*parsed, allowedIDs)
				if genRes.TraceID != "" {
					out.TraceId = genRes.TraceID
				}
				out.Model = genRes.Model
				return out, nil
			}
		}
	}

	out.Intent = heuristicIntent(bookable, prompt)
	out.Confidence = 0.85
	if len(bookable) == 0 {
		out.ClarifyingQuestion = "We could not find open times that match yet. Pick a wider date window or browse the picker below."
	}
	out.SlotSuggestions = deterministicSuggestionsPB(bookable, 8, prompt)
	return out, nil
}

type minimalSlotRow struct {
	SlotID      string `json:"slot_id"`
	StartAt     string `json:"start_at"`
	EndAt       string `json:"end_at"`
	ServiceName string `json:"service_name"`
	IsBookable  bool   `json:"is_bookable"`
}

func minimalSlotsForPrompt(items []*pb.PublicSlotsItem, max int) []minimalSlotRow {
	out := make([]minimalSlotRow, 0, min(len(items), max))
	for _, it := range items {
		svc := ""
		if it.Service != nil {
			svc = it.Service.Name
		}
		out = append(out, minimalSlotRow{
			SlotID:      it.SlotId,
			StartAt:     it.StartAt,
			EndAt:       it.EndAt,
			ServiceName: svc,
			IsBookable:  it.IsBookable,
		})
		if len(out) >= max {
			break
		}
	}
	return out
}

func allowedSlotIDs(rows []minimalSlotRow) map[string]struct{} {
	m := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if r.SlotID != "" {
			m[r.SlotID] = struct{}{}
		}
	}
	return m
}

type parsedAssistant struct {
	Intent             string                                   `json:"intent"`
	ClarifyingQuestion string                                   `json:"clarifying_question"`
	Entities           map[string]interface{}                   `json:"entities"`
	Confidence         float64                                  `json:"confidence"`
	SlotSuggestions    []parsedAssistantSlotSuggestion          `json:"slot_suggestions"`
}

type parsedAssistantSlotSuggestion struct {
	SlotID         string  `json:"slot_id"`
	StartAt        string  `json:"start_at"`
	EndAt          string  `json:"end_at"`
	ServiceName    string  `json:"service_name"`
	ConfidenceHint float64 `json:"confidence_hint"`
}

func parseAssistantPayload(raw json.RawMessage) *parsedAssistant {
	var p parsedAssistant
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	return &p
}

func enrichSuggestResponsePB(p parsedAssistant, allowed map[string]struct{}) *pb.PublicBookingAssistantSuggestResponse {
	if p.Entities == nil {
		p.Entities = map[string]interface{}{}
	}
	entities := map[string]string{}
	for k, v := range p.Entities {
		switch t := v.(type) {
		case string:
			entities[k] = t
		default:
			entities[k] = fmt.Sprint(t)
		}
	}
	suggestions := make([]*pb.PublicBookingAssistantSlotSuggestion, 0, len(p.SlotSuggestions))
	for _, s := range p.SlotSuggestions {
		if _, ok := allowed[s.SlotID]; !ok {
			continue
		}
		suggestions = append(suggestions, &pb.PublicBookingAssistantSlotSuggestion{
			SlotId:         s.SlotID,
			StartAt:        s.StartAt,
			EndAt:          s.EndAt,
			ServiceName:    s.ServiceName,
			ConfidenceHint: s.ConfidenceHint,
		})
	}
	intent := strings.TrimSpace(p.Intent)
	if intent == "" {
		intent = "book_slot"
	}
	return &pb.PublicBookingAssistantSuggestResponse{
		Intent:             intent,
		Entities:           entities,
		ClarifyingQuestion: strings.TrimSpace(p.ClarifyingQuestion),
		SlotSuggestions:    suggestions,
		Confidence:         p.Confidence,
	}
}

func heuristicIntent(bookable []*pb.PublicSlotsItem, prompt string) string {
	pl := strings.ToLower(prompt)
	switch {
	case len(bookable) == 0:
		return "no_availability_match"
	case strings.Contains(pl, "tomorrow") || strings.Contains(pl, "next week"):
		return "narrow_time_preferences"
	default:
		return "book_slot"
	}
}

func deterministicSuggestionsPB(bookable []*pb.PublicSlotsItem, max int, prompt string) []*pb.PublicBookingAssistantSlotSuggestion {
	if len(bookable) == 0 {
		return nil
	}

	tokens := tokenize(prompt)
	filtered := make([]*pb.PublicSlotsItem, 0)
	for _, it := range bookable {
		meta := ""
		if it.Service != nil {
			meta += strings.ToLower(it.Service.Name)
		}
		if tokens.isEmptyOrMatch(meta) {
			filtered = append(filtered, it)
		}
	}
	if len(filtered) == 0 {
		filtered = append(filtered[:0:0], bookable...)
	}

	out := make([]*pb.PublicBookingAssistantSlotSuggestion, 0, max)
	for _, it := range filtered {
		svc := ""
		if it.Service != nil {
			svc = it.Service.Name
		}
		out = append(out, &pb.PublicBookingAssistantSlotSuggestion{
			SlotId:      it.SlotId,
			StartAt:     it.StartAt,
			EndAt:       it.EndAt,
			ServiceName: svc,
		})
		if len(out) >= max {
			break
		}
	}
	return out
}

type promptTokens []string

func tokenize(prompt string) promptTokens {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len(f) >= 4 {
			out = append(out, f)
		}
	}
	return out
}

func (t promptTokens) isEmptyOrMatch(meta string) bool {
	if len(t) == 0 || meta == "" {
		return true
	}
	for _, tok := range t {
		if strings.Contains(meta, tok) {
			return true
		}
	}
	return false
}
