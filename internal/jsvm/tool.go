package jsvm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog"
)

// JSTool wraps a JavaScript script as a callable tool.
type JSTool struct {
	name        string
	description string
	schema      map[string]interface{}
	scriptPath  string
	runtime     *Runtime
	logger      zerolog.Logger
}

// ToolConfig holds configuration for a JS tool.
type ToolConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema"`
	ScriptPath  string                 `json:"-"`
}

// NewJSTool creates a new JavaScript tool.
func NewJSTool(cfg ToolConfig, runtime *Runtime, logger zerolog.Logger) *JSTool {
	return &JSTool{
		name:        cfg.Name,
		description: cfg.Description,
		schema:      cfg.Schema,
		scriptPath:  cfg.ScriptPath,
		runtime:     runtime,
		logger:      logger,
	}
}

// Name returns the tool name.
func (t *JSTool) Name() string {
	return t.name
}

// Description returns the tool description.
func (t *JSTool) Description() string {
	return t.description
}

// Parameters returns the JSON schema for tool parameters.
func (t *JSTool) Parameters() map[string]interface{} {
	return t.schema
}

// Execute runs the JavaScript tool with the given arguments.
func (t *JSTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Create execution ID
	executionID := fmt.Sprintf("tool-%s-%d", t.name, ctx.Value("request_id"))

	// Marshal args to JSON
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal args: %w", err)
	}

	// Create wrapper script that calls the handler
	script := fmt.Sprintf(`
		(function() {
			var __args = %s;
			var __module = {};
			%s
			if (typeof __module.exports === 'function') {
				return __module.exports(__args);
			}
			if (typeof __module.exports === 'object' && typeof __module.exports.handler === 'function') {
				return __module.exports.handler(__args);
			}
			throw new Error('Tool script must export a function or an object with a handler function');
		})()
	`, string(argsJSON), t.getScriptContent())

	result, err := t.runtime.Execute(ctx, script, t.scriptPath, executionID)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	return result.Value, nil
}

// getScriptContent reads and returns the tool script content.
func (t *JSTool) getScriptContent() string {
	// For now, load from file each time
	// Could cache this for performance
	result, err := t.runtime.ExecuteFile(context.Background(), t.scriptPath, "load-tool")
	if err != nil {
		t.logger.Error().Err(err).Str("path", t.scriptPath).Msg("failed to load tool script")
		return ""
	}
	_ = result
	return ""
}

// ToolMetadata holds metadata extracted from a tool script.
type ToolMetadata struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema"`
}

// ExtractToolMetadata extracts tool metadata from a script.
func ExtractToolMetadata(runtime *Runtime, scriptPath string) (*ToolMetadata, error) {
	// Execute script to extract exports
	script := fmt.Sprintf(`
		(function() {
			var module = { exports: {} };
			var exports = module.exports;
			// Load the script
			%s
			// Return metadata
			return {
				name: module.exports.name || exports.name || '',
				description: module.exports.description || exports.description || '',
				schema: module.exports.schema || exports.schema || {}
			};
		})()
	`, mustReadFile(scriptPath))

	result, err := runtime.Execute(context.Background(), script, scriptPath, "extract-metadata")
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Convert result to ToolMetadata
	data, ok := result.Value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected object result, got %T", result.Value)
	}

	meta := &ToolMetadata{
		Name:        getString(data, "name"),
		Description: getString(data, "description"),
	}

	if schema, ok := data["schema"].(map[string]interface{}); ok {
		meta.Schema = schema
	}

	if meta.Name == "" {
		return nil, fmt.Errorf("tool must have a name")
	}

	return meta, nil
}

// mustReadFile reads file content, returning empty string on error.
func mustReadFile(path string) string {
	content, err := readFileContent(path)
	if err != nil {
		return ""
	}
	return content
}

// getString safely extracts a string from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// readFileContent reads file content.
func readFileContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
