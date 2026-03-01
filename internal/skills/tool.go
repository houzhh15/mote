package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mote/internal/jsvm"
	"mote/internal/tools"

	"github.com/google/uuid"
)

// SkillTool implements tools.Tool interface for skill-defined tools.
type SkillTool struct {
	skillID  string
	skillDir string
	def      *ToolDef
	runtime  *jsvm.Runtime
	config   map[string]any // Skill config from manifest.json
}

// NewSkillTool creates a new skill tool.
func NewSkillTool(skillID, skillDir string, def *ToolDef, runtime *jsvm.Runtime, config map[string]any) *SkillTool {
	return &SkillTool{
		skillID:  skillID,
		skillDir: skillDir,
		def:      def,
		runtime:  runtime,
		config:   config,
	}
}

// Name returns the tool name.
func (t *SkillTool) Name() string {
	return t.def.Name
}

// Description returns the tool description.
func (t *SkillTool) Description() string {
	return t.def.Description
}

// Parameters returns the JSON Schema for the tool's input parameters.
func (t *SkillTool) Parameters() map[string]any {
	if t.def.Parameters == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	// Ensure required is never null (use empty array instead)
	required := t.def.Parameters.Required
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       t.def.Parameters.Type,
		"properties": t.def.Parameters.Properties,
		"required":   required,
	}
}

// Execute runs the tool handler via JSVM.
func (t *SkillTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	// Parse handler to get filename and function name
	filename, funcName, err := ParseHandler(t.def.Handler)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("invalid handler: %v", err)), nil
	}

	// Read handler script
	scriptPath := filepath.Join(t.skillDir, filename)
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		return tools.NewErrorResult(fmt.Sprintf("failed to read handler: %v", err)), nil
	}

	// Build script with function call (inject skill config)
	script := buildToolScript(string(scriptContent), funcName, args, t.config)

	// Execute with timeout
	timeout := t.def.Timeout.Duration
	if timeout == 0 {
		timeout = DefaultToolTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	executionID := uuid.New().String()
	result, err := t.runtime.Execute(execCtx, script, filename, executionID)
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return tools.NewErrorResult("tool execution timed out"), nil
		}
		return tools.NewErrorResult(fmt.Sprintf("execution error: %v", err)), nil
	}

	// Convert result to JSON string for proper serialization
	content, err := serializeResult(result.Value)
	if err != nil {
		content = fmt.Sprintf("%v", result.Value)
	}
	return tools.NewSuccessResult(content), nil
}

// serializeResult converts the JS execution result to a JSON string.
func serializeResult(value interface{}) (string, error) {
	if value == nil {
		return "null", nil
	}
	// For simple types, just format them directly
	switch v := value.(type) {
	case string:
		return v, nil
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", v), nil
	default:
		// For complex types (maps, slices), use JSON encoding
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// SkillID returns the ID of the skill that defines this tool.
func (t *SkillTool) SkillID() string {
	return t.skillID
}

// HandlerPath returns the path to the handler script.
func (t *SkillTool) HandlerPath() string {
	filename, _, _ := ParseHandler(t.def.Handler)
	return filepath.Join(t.skillDir, filename)
}

// buildToolScript builds a script that calls the handler function.
// It injects the skill's config from manifest.json as a SKILL_CONFIG global,
// making it accessible to JS handlers.
func buildToolScript(script, funcName string, args map[string]any, config map[string]any) string {
	// JSON encode args
	argsJSON := "{}"
	if args != nil {
		argsJSON = encodeArgs(args)
	}

	// JSON encode skill config
	configJSON := "{}"
	if config != nil {
		if data, err := json.Marshal(config); err == nil {
			configJSON = string(data)
		}
	}

	return fmt.Sprintf(`
var SKILL_CONFIG = %s;

%s

(function() {
    var args = %s;
    var result = %s(args);
    return result;
})();
`, configJSON, script, argsJSON, funcName)
}

// encodeArgs converts args map to JSON string.
func encodeArgs(args map[string]any) string {
	// Simple encoding, handles basic types
	result := "{"
	first := true
	for k, v := range args {
		if !first {
			result += ","
		}
		first = false
		result += fmt.Sprintf(`"%s":`, k)
		switch val := v.(type) {
		case string:
			result += fmt.Sprintf(`"%s"`, escapeString(val))
		case int, int64, float64:
			result += fmt.Sprintf(`%v`, val)
		case bool:
			result += fmt.Sprintf(`%v`, val)
		case nil:
			result += "null"
		default:
			result += fmt.Sprintf(`"%v"`, val)
		}
	}
	result += "}"
	return result
}

// escapeString escapes special characters in a string for JSON.
func escapeString(s string) string {
	escaped := ""
	for _, c := range s {
		switch c {
		case '"':
			escaped += `\"`
		case '\\':
			escaped += `\\`
		case '\n':
			escaped += `\n`
		case '\r':
			escaped += `\r`
		case '\t':
			escaped += `\t`
		default:
			escaped += string(c)
		}
	}
	return escaped
}

// Ensure SkillTool implements tools.Tool.
var _ tools.Tool = (*SkillTool)(nil)

// SkillToolFactory creates skill tools.
type SkillToolFactory struct {
	runtime *jsvm.Runtime
}

// NewSkillToolFactory creates a new skill tool factory.
func NewSkillToolFactory(runtime *jsvm.Runtime) *SkillToolFactory {
	return &SkillToolFactory{runtime: runtime}
}

// Create creates a skill tool from a definition.
func (f *SkillToolFactory) Create(skillID, skillDir string, def *ToolDef, config map[string]any) *SkillTool {
	return NewSkillTool(skillID, skillDir, def, f.runtime, config)
}

// SkillToolInfo contains information about a registered skill tool.
type SkillToolInfo struct {
	SkillID      string    `json:"skill_id"`
	ToolName     string    `json:"tool_name"`
	Description  string    `json:"description,omitempty"`
	Handler      string    `json:"handler"`
	RegisteredAt time.Time `json:"registered_at"`
}
