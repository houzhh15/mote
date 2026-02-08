package tools

import (
	"reflect"
	"testing"
)

func TestBuildSchema(t *testing.T) {
	t.Run("simple struct", func(t *testing.T) {
		type Simple struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		schema := BuildSchema(Simple{})

		if schema["type"] != "object" {
			t.Errorf("expected type 'object', got %v", schema["type"])
		}

		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatal("expected properties to be a map")
		}

		nameSchema := props["name"].(map[string]any)
		if nameSchema["type"] != "string" {
			t.Errorf("expected name type 'string', got %v", nameSchema["type"])
		}

		countSchema := props["count"].(map[string]any)
		if countSchema["type"] != "integer" {
			t.Errorf("expected count type 'integer', got %v", countSchema["type"])
		}
	})

	t.Run("with jsonschema tags", func(t *testing.T) {
		type WithTags struct {
			Path    string `json:"path" jsonschema:"description=File path,required"`
			Content string `json:"content" jsonschema:"description=File content"`
			Mode    string `json:"mode" jsonschema:"enum=read|write|append,default=read"`
		}

		schema := BuildSchema(WithTags{})
		props := schema["properties"].(map[string]any)

		pathSchema := props["path"].(map[string]any)
		if pathSchema["description"] != "File path" {
			t.Errorf("expected description 'File path', got %v", pathSchema["description"])
		}

		required, ok := schema["required"].([]string)
		if !ok {
			t.Fatal("expected required to be a string slice")
		}
		if len(required) != 1 || required[0] != "path" {
			t.Errorf("expected required=[path], got %v", required)
		}

		modeSchema := props["mode"].(map[string]any)
		enum := modeSchema["enum"].([]any)
		if len(enum) != 3 {
			t.Errorf("expected 3 enum values, got %d", len(enum))
		}
		if modeSchema["default"] != "read" {
			t.Errorf("expected default 'read', got %v", modeSchema["default"])
		}
	})

	t.Run("with all basic types", func(t *testing.T) {
		type AllTypes struct {
			Str     string  `json:"str"`
			Int     int     `json:"int"`
			Int64   int64   `json:"int64"`
			Uint    uint    `json:"uint"`
			Float32 float32 `json:"float32"`
			Float64 float64 `json:"float64"`
			Bool    bool    `json:"bool"`
		}

		schema := BuildSchema(AllTypes{})
		props := schema["properties"].(map[string]any)

		testCases := []struct {
			field    string
			expected string
		}{
			{"str", "string"},
			{"int", "integer"},
			{"int64", "integer"},
			{"uint", "integer"},
			{"float32", "number"},
			{"float64", "number"},
			{"bool", "boolean"},
		}

		for _, tc := range testCases {
			fieldSchema := props[tc.field].(map[string]any)
			if fieldSchema["type"] != tc.expected {
				t.Errorf("expected %s type %q, got %v", tc.field, tc.expected, fieldSchema["type"])
			}
		}
	})

	t.Run("with slice", func(t *testing.T) {
		type WithSlice struct {
			Tags []string `json:"tags"`
		}

		schema := BuildSchema(WithSlice{})
		props := schema["properties"].(map[string]any)

		tagsSchema := props["tags"].(map[string]any)
		if tagsSchema["type"] != "array" {
			t.Errorf("expected tags type 'array', got %v", tagsSchema["type"])
		}
		tagsItems := tagsSchema["items"].(map[string]any)
		if tagsItems["type"] != "string" {
			t.Errorf("expected tags items type 'string', got %v", tagsItems["type"])
		}
	})

	t.Run("with nested struct", func(t *testing.T) {
		type Inner struct {
			Value string `json:"value"`
		}
		type Outer struct {
			Name  string `json:"name"`
			Inner Inner  `json:"inner"`
		}

		schema := BuildSchema(Outer{})
		props := schema["properties"].(map[string]any)

		innerSchema := props["inner"].(map[string]any)
		if innerSchema["type"] != "object" {
			t.Errorf("expected inner type 'object', got %v", innerSchema["type"])
		}

		innerProps := innerSchema["properties"].(map[string]any)
		valueSchema := innerProps["value"].(map[string]any)
		if valueSchema["type"] != "string" {
			t.Errorf("expected value type 'string', got %v", valueSchema["type"])
		}
	})

	t.Run("with pointer", func(t *testing.T) {
		type WithPtr struct {
			Name *string `json:"name"`
		}

		schema := BuildSchema(&WithPtr{})
		props := schema["properties"].(map[string]any)

		nameSchema := props["name"].(map[string]any)
		if nameSchema["type"] != "string" {
			t.Errorf("expected name type 'string', got %v", nameSchema["type"])
		}
	})

	t.Run("non-struct type", func(t *testing.T) {
		schema := BuildSchema("not a struct")
		if schema["type"] != "object" {
			t.Errorf("expected type 'object' for non-struct, got %v", schema["type"])
		}
	})
}

func TestMergeSchemas(t *testing.T) {
	schema1 := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "string"},
		},
		"required": []string{"a"},
	}

	schema2 := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"b": map[string]any{"type": "integer"},
		},
		"required": []string{"b"},
	}

	merged := MergeSchemas(schema1, schema2)

	props := merged["properties"].(map[string]any)
	if _, ok := props["a"]; !ok {
		t.Error("expected property 'a' in merged schema")
	}
	if _, ok := props["b"]; !ok {
		t.Error("expected property 'b' in merged schema")
	}

	required := merged["required"].([]string)
	if !reflect.DeepEqual(required, []string{"a", "b"}) {
		t.Errorf("expected required=[a,b], got %v", required)
	}
}

func TestMergeSchemasDedup(t *testing.T) {
	schema1 := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{"a", "b"},
	}

	schema2 := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{"b", "c"},
	}

	merged := MergeSchemas(schema1, schema2)
	required := merged["required"].([]string)

	if len(required) != 3 {
		t.Errorf("expected 3 required fields, got %d: %v", len(required), required)
	}

	reqSet := make(map[string]bool)
	for _, r := range required {
		reqSet[r] = true
	}
	for _, expected := range []string{"a", "b", "c"} {
		if !reqSet[expected] {
			t.Errorf("expected %q in required", expected)
		}
	}
}
