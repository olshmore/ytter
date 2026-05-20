package ai

var GuestBookingAssistantResponseSchema = &Schema{
	Name:        "guest_booking_assistant_response",
	Description: "Structured guest booking assistant output.",
	Type:        "object",
	Required:    []string{"intent", "entities", "slot_suggestions"},
	Properties: map[string]*Schema{
		"intent": {
			Type:        "string",
			Description: "Short intent label, e.g. book_slot.",
		},
		"clarifying_question": {
			Type:        "string",
			Description: "Optional question when the request is ambiguous; empty string if suggestions are sufficient.",
		},
		"entities": {
			Type:        "object",
			Description: "Loose key/value constraints inferred from the user (service, weekday, rough_time, …). Must be JSON object.",
			Properties:  map[string]*Schema{},
		},
		"confidence": {
			Type: "number",
		},
		"slot_suggestions": {
			Type: "array",
			Items: &Schema{
				Type: "object",
				Required: []string{"slot_id", "start_at", "end_at", "service_name"},
				Properties: map[string]*Schema{
					"slot_id":       {Type: "string"},
					"start_at":      {Type: "string"},
					"end_at":        {Type: "string"},
					"service_name":  {Type: "string"},
				},
			},
		},
	},
}

const GuestBookingAssistantSystemPromptV1 = `You are a booking concierge for Appointa. Your job:
- Read the customer's natural-language request.
- Set clarifying_question to a short question when critical details are missing; otherwise use an empty string.
- Set confidence between 0 and 1 reflecting how well the request matches the suggestions.
- Propose ONLY slots from the AVAILABLE SLOTS JSON array below — never invent slot_id or times.
- For each slot_suggestions item, copy slot_id, start_at, end_at, and service_name from inventory.
- If nothing matches reasonably, use an empty slot_suggestions array and optionally a clarifying_question.
- NEVER confirm a booking — only propose times.
Respond ONLY with JSON matching the response_format schema.`

func GuestBookingAssistantUserPromptV1(customerPrompt string, inventoryJSON string) string {
	return "Customer prompt:\n" + customerPrompt + "\n\nAVAILABLE SLOTS (JSON — only these slot_ids are valid):\n" + inventoryJSON
}
