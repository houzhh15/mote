package tools

import (
	"reflect"
	"strings"
)

// BuildSchema generates a JSON Schema from a Go struct type using reflection.
// It supports the following struct tags:
//   - json: field name (uses json tag name if present)
//   - jsonschema: additional schema attributes (description, required, enum, default)
//
// Example usage:
//
//	type Args struct {
//	    Path    string `json:"path" jsonschema:"description=File path,required"`
//	    Content string `json:"content" jsonschema:"description=File content"`
//	}
//	schema := BuildSchema(Args{})
func BuildSchema(v any) map[string]any {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	return buildObjectSchema(t)
}

// buildObjectSchema builds a JSON Schema for a struct type.
func buildObjectSchema(t reflect.Type) map[string]any {
	properties := make(map[string]any)
	required := []string{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get field name from json tag
		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" && parts[0] != "-" {
				fieldName = parts[0]
			}
			if parts[0] == "-" {
				continue
			}
		}

		// Build property schema
		propSchema := buildPropertySchema(field)

		// Parse jsonschema tag for additional attributes
		if jsTag := field.Tag.Get("jsonschema"); jsTag != "" {
			parseJSONSchemaTag(jsTag, propSchema, fieldName, &required)
		}

		properties[fieldName] = propSchema
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// buildPropertySchema builds the schema for a single field.
func buildPropertySchema(field reflect.StructField) map[string]any {
	t := field.Type

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := make(map[string]any)

	switch t.Kind() {
	case reflect.String:
		schema["type"] = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		schema["type"] = "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		// Build items schema for element type
		elemSchema := buildTypeSchema(t.Elem())
		schema["items"] = elemSchema
	case reflect.Map:
		schema["type"] = "object"
		// If value type is known, add additionalProperties schema
		if t.Elem().Kind() != reflect.Interface {
			schema["additionalProperties"] = buildTypeSchema(t.Elem())
		}
	case reflect.Struct:
		// Recursive schema for nested structs
		return buildObjectSchema(t)
	default:
		// Default to object for unknown types
		schema["type"] = "object"
	}

	return schema
}

// buildTypeSchema builds a schema for a basic type.
func buildTypeSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Struct:
		return buildObjectSchema(t)
	default:
		return map[string]any{"type": "object"}
	}
}

// parseJSONSchemaTag parses the jsonschema tag and updates the schema.
// Supported attributes:
//   - description=<text>: adds description field
//   - required: marks field as required
//   - enum=<v1|v2|v3>: adds enum values (pipe-separated)
//   - default=<value>: adds default value
func parseJSONSchemaTag(tag string, schema map[string]any, fieldName string, required *[]string) {
	attrs := strings.Split(tag, ",")
	for _, attr := range attrs {
		attr = strings.TrimSpace(attr)

		if attr == "required" {
			*required = append(*required, fieldName)
			continue
		}

		if strings.HasPrefix(attr, "description=") {
			schema["description"] = strings.TrimPrefix(attr, "description=")
			continue
		}

		if strings.HasPrefix(attr, "enum=") {
			enumStr := strings.TrimPrefix(attr, "enum=")
			enumVals := strings.Split(enumStr, "|")
			anyVals := make([]any, len(enumVals))
			for i, v := range enumVals {
				anyVals[i] = v
			}
			schema["enum"] = anyVals
			continue
		}

		if strings.HasPrefix(attr, "default=") {
			schema["default"] = strings.TrimPrefix(attr, "default=")
			continue
		}
	}
}

// MergeSchemas merges multiple JSON Schemas into one.
// Properties from later schemas override earlier ones.
func MergeSchemas(schemas ...map[string]any) map[string]any {
	result := map[string]any{
		"type":       "object",
		"properties": make(map[string]any),
	}

	var allRequired []string

	for _, schema := range schemas {
		if props, ok := schema["properties"].(map[string]any); ok {
			resultProps := result["properties"].(map[string]any)
			for k, v := range props {
				resultProps[k] = v
			}
		}

		if req, ok := schema["required"].([]string); ok {
			allRequired = append(allRequired, req...)
		}
	}

	if len(allRequired) > 0 {
		// Deduplicate required fields
		seen := make(map[string]bool)
		unique := []string{}
		for _, r := range allRequired {
			if !seen[r] {
				seen[r] = true
				unique = append(unique, r)
			}
		}
		result["required"] = unique
	}

	return result
}
