package ai

import (
	"encoding/json"
	"fmt"
	"sort"
)

type Schema struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Type        string             `json:"type"`
	Required    []string           `json:"required,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
}

func (s *Schema) toOpenAIJSONSchema() map[string]any {
	if s == nil {
		return nil
	}
	return map[string]any{
		"name":   s.Name,
		"strict": true,
		"schema": s.toJSONSchemaNode(true),
	}
}

func (s *Schema) toJSONSchemaNode(rootStrict bool) map[string]any {
	node := map[string]any{
		"type": s.Type,
	}
	if s.Description != "" {
		node["description"] = s.Description
	}
	if len(s.Enum) > 0 {
		enumVals := make([]any, len(s.Enum))
		for i, v := range s.Enum {
			enumVals[i] = v
		}
		node["enum"] = enumVals
	}
	if s.Type == "object" {
		props := map[string]any{}
		for k, v := range s.Properties {
			props[k] = v.toJSONSchemaNode(false)
		}
		node["properties"] = props
		node["required"] = s.openAIStrictRequired()
		node["additionalProperties"] = false
	}
	if s.Type == "array" && s.Items != nil {
		node["items"] = s.Items.toJSONSchemaNode(false)
	}
	_ = rootStrict
	return node
}

func (s *Schema) openAIStrictRequired() []string {
	if s == nil || s.Type != "object" {
		return nil
	}
	if len(s.Properties) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Schema) Validate(payload json.RawMessage) error {
	if s == nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return fmt.Errorf("payload is not valid JSON: %w", err)
	}
	return s.validateValue(decoded, "$")
}

func (s *Schema) validateValue(v any, path string) error {
	switch s.Type {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object", path)
		}
		for _, key := range s.Required {
			if _, present := obj[key]; !present {
				return fmt.Errorf("%s.%s: required field missing", path, key)
			}
		}
		for key, child := range s.Properties {
			val, present := obj[key]
			if !present {
				continue
			}
			if err := child.validateValue(val, path+"."+key); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array", path)
		}
		if s.Items != nil {
			for i, item := range arr {
				if err := s.Items.validateValue(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("%s: expected string", path)
		}
		if len(s.Enum) > 0 {
			match := false
			for _, allowed := range s.Enum {
				if str == allowed {
					match = true
					break
				}
			}
			if !match {
				return fmt.Errorf("%s: %q not in enum", path, str)
			}
		}
	case "number":
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("%s: expected number", path)
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return fmt.Errorf("%s: expected integer", path)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s: expected boolean", path)
		}
	case "":
	default:
		return fmt.Errorf("%s: unsupported schema type %q", path, s.Type)
	}
	return nil
}
