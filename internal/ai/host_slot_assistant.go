package ai

var HostSlotAssistantPlanSchema = &Schema{
	Name:        "host_slot_assistant_plan",
	Description: "Structured batch slot plan from host natural language.",
	Type:        "object",
	Required: []string{
		"service_name",
		"date_from",
		"date_to",
		"weekdays",
		"daily_start_local",
		"daily_end_local",
		"slot_minutes",
		"capacity",
		"status",
		"practitioner_name",
		"room_name",
		"confidence",
		"notes",
	},
	Properties: map[string]*Schema{
		"service_name":       {Type: "string"},
		"date_from":          {Type: "string"},
		"date_to":            {Type: "string"},
		"daily_start_local":  {Type: "string"},
		"daily_end_local":    {Type: "string"},
		"slot_minutes":       {Type: "integer"},
		"capacity":           {Type: "integer"},
		"status":             {Type: "string", Enum: []string{"available", "unavailable"}},
		"practitioner_name":  {Type: "string"},
		"room_name":          {Type: "string"},
		"confidence":         {Type: "number"},
		"notes":              {Type: "string"},
		"weekdays": {
			Type: "array",
			Items: &Schema{
				Type: "string",
				Enum: []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			},
		},
	},
}

const HostSlotAssistantSystemPromptV1 = `You are a scheduling assistant for Appointa hosts. Convert the host's natural-language request into a recurring batch slot plan.

Rules:
- service_name MUST match one of the SERVICES listed below (copy the exact name).
- date_from and date_to are inclusive calendar dates in YYYY-MM-DD (location timezone context).
- When the host does not specify dates, start from TODAY (provided below) and cover a sensible forward window (e.g. 2–4 weeks).
- Never choose a date range that ends before TODAY; guests only see future appointment times.
- date_from and date_to must both be on or after TODAY (same YYYY-MM-DD); never use a past calendar year.
- weekdays: use only mon,tue,wed,thu,fri,sat,sun values in the array.
- daily_start_local and daily_end_local use 24h HH:MM in location-local wall time.
- slot_minutes is the length of each appointment slot (e.g. 30).
- capacity is seats per slot (usually 1).
- status is available or unavailable.
- practitioner_name and room_name: copy from RESOURCES when specified; otherwise empty string.
- notes: brief host-facing summary; empty string if none.
- confidence: 0.0–1.0 for how well the prompt matched available services/resources.
- If the request is impossible or ambiguous, still return best-effort fields and low confidence with notes explaining what is missing.

Respond ONLY with JSON matching the response_format schema.`

func HostSlotAssistantUserPromptV1(hostPrompt, locationName, timezone, todayLocal, servicesJSON, resourcesJSON string) string {
	return "Location: " + locationName + " (timezone: " + timezone + ")\n" +
		"TODAY (location-local calendar date): " + todayLocal + "\n\n" +
		"Host request:\n" + hostPrompt + "\n\n" +
		"SERVICES (JSON):\n" + servicesJSON + "\n\n" +
		"RESOURCES (JSON — practitioners and rooms; may be empty):\n" + resourcesJSON
}
