package ai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaValidate(t *testing.T) {
	schema := &Schema{
		Name:     "smoke",
		Type:     "object",
		Required: []string{"status", "value"},
		Properties: map[string]*Schema{
			"status": {Type: "string", Enum: []string{"ok"}},
			"value":  {Type: "integer"},
			"tags": {
				Type:  "array",
				Items: &Schema{Type: "string"},
			},
		},
	}

	t.Run("valid payload", func(t *testing.T) {
		err := schema.Validate(json.RawMessage(`{"status":"ok","value":42,"tags":["a","b"]}`))
		require.NoError(t, err)
	})

	t.Run("missing required", func(t *testing.T) {
		err := schema.Validate(json.RawMessage(`{"status":"ok"}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "value")
	})

	t.Run("enum mismatch", func(t *testing.T) {
		err := schema.Validate(json.RawMessage(`{"status":"nope","value":1}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "enum")
	})

	t.Run("wrong type", func(t *testing.T) {
		err := schema.Validate(json.RawMessage(`{"status":"ok","value":"forty-two"}`))
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		err := schema.Validate(json.RawMessage(`not-json`))
		require.Error(t, err)
	})
}

func TestHostSlotAssistantSchema_OpenAIStrictRequiredIncludesAllProperties(t *testing.T) {
	keys := HostSlotAssistantPlanSchema.openAIStrictRequired()
	require.Contains(t, keys, "service_name")
	require.Contains(t, keys, "weekdays")
	require.Contains(t, keys, "notes")
}

func TestGuestBookingAssistantSchema_OpenAIStrictRequiredIncludesAllProperties(t *testing.T) {
	items := GuestBookingAssistantResponseSchema.Properties["slot_suggestions"].Items
	encoded := items.toOpenAIJSONSchema()
	root, ok := encoded["schema"].(map[string]any)
	require.True(t, ok)
	required, ok := root["required"].([]string)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"slot_id", "start_at", "end_at", "service_name"}, required)
}

func TestOpenAIStrictRequired_AllPropertyKeys(t *testing.T) {
	s := &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"a": {Type: "string"},
			"b": {Type: "string"},
		},
		Required: []string{"a"},
	}
	require.ElementsMatch(t, []string{"a", "b"}, s.openAIStrictRequired())

	encoded := s.toOpenAIJSONSchema()
	inner := encoded["schema"].(map[string]any)
	require.ElementsMatch(t, []string{"a", "b"}, inner["required"])
}

func TestSchemaToOpenAIJSONSchema(t *testing.T) {
	schema := &Schema{
		Name:     "smoke",
		Type:     "object",
		Required: []string{"status"},
		Properties: map[string]*Schema{
			"status": {Type: "string", Enum: []string{"ok"}},
		},
	}

	encoded := schema.toOpenAIJSONSchema()
	require.Equal(t, "smoke", encoded["name"])
	require.Equal(t, true, encoded["strict"])

	root, ok := encoded["schema"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", root["type"])
	require.Equal(t, false, root["additionalProperties"])
}
