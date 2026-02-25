package delegate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"mote/internal/config"
	"mote/internal/tools"
)

// ManageAgentsTool implements tools.Tool to let the LLM create, update, delete, and list sub-agents.
type ManageAgentsTool struct{}

// NewManageAgentsTool creates a new manage_agents tool.
func NewManageAgentsTool() *ManageAgentsTool {
	return &ManageAgentsTool{}
}

func (t *ManageAgentsTool) Name() string { return "manage_agents" }

func (t *ManageAgentsTool) Description() string {
	return "Create, update, delete, or list sub-agent configurations. Use this to dynamically manage the pool of available delegate agents."
}

func (t *ManageAgentsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "The action to perform",
				"enum":        []string{"list", "get", "create", "update", "delete"},
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Agent name (required for get/create/update/delete)",
			},
			"config": map[string]any{
				"type":        "object",
				"description": "Agent configuration (required for create/update). Fields: description, provider, model, system_prompt, tools, max_depth, timeout, max_iterations, temperature, enabled",
				"properties": map[string]any{
					"enabled": map[string]any{
						"type":        "boolean",
						"description": "Whether the agent is enabled (default: true)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Description of what the agent specializes in",
					},
					"provider": map[string]any{
						"type":        "string",
						"description": "Provider name (e.g., minimax, glm). Leave empty to inherit default.",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "Model name. Leave empty to inherit default.",
					},
					"system_prompt": map[string]any{
						"type":        "string",
						"description": "System prompt for the agent",
					},
					"tools": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of tool names the agent can use. Empty list inherits all tools.",
					},
					"max_depth": map[string]any{
						"type":        "integer",
						"description": "Maximum delegation depth for this agent (default: 3)",
					},
					"timeout": map[string]any{
						"type":        "string",
						"description": "Timeout duration (e.g., '5m', '10m')",
					},
					"max_iterations": map[string]any{
						"type":        "integer",
						"description": "Maximum iterations for the agent",
					},
					"temperature": map[string]any{
						"type":        "number",
						"description": "Temperature for the agent (0.0-2.0)",
					},
				},
			},
		},
		"required": []string{"action"},
	}
}

func (t *ManageAgentsTool) Execute(ctx context.Context, args map[string]any) (tools.ToolResult, error) {
	action, _ := args["action"].(string)
	name, _ := args["name"].(string)

	switch action {
	case "list":
		return t.listAgents()
	case "get":
		if name == "" {
			return tools.NewErrorResult("parameter 'name' is required for get action"), nil
		}
		return t.getAgent(name)
	case "create":
		if name == "" {
			return tools.NewErrorResult("parameter 'name' is required for create action"), nil
		}
		agentCfg, err := t.parseConfig(args)
		if err != nil {
			return tools.NewErrorResult(err.Error()), nil
		}
		return t.createAgent(name, agentCfg)
	case "update":
		if name == "" {
			return tools.NewErrorResult("parameter 'name' is required for update action"), nil
		}
		agentCfg, err := t.parseConfig(args)
		if err != nil {
			return tools.NewErrorResult(err.Error()), nil
		}
		return t.updateAgent(name, agentCfg)
	case "delete":
		if name == "" {
			return tools.NewErrorResult("parameter 'name' is required for delete action"), nil
		}
		return t.deleteAgent(name)
	default:
		return tools.NewErrorResult(fmt.Sprintf("unknown action: %q (valid: list, get, create, update, delete)", action)), nil
	}
}

func (t *ManageAgentsTool) listAgents() (tools.ToolResult, error) {
	cfg := config.GetConfig()
	if cfg == nil || len(cfg.Agents) == 0 {
		return tools.NewSuccessResult("No agents configured. Use action 'create' to add one."), nil
	}

	names := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Configured agents (%d):\n\n", len(cfg.Agents)))
	for _, name := range names {
		ac := cfg.Agents[name]
		status := "enabled"
		if !ac.IsEnabled() {
			status = "DISABLED"
		}
		desc := ac.Description
		if desc == "" {
			desc = "(no description)"
		}
		model := ac.Model
		if model == "" {
			model = "(default)"
		}
		sb.WriteString(fmt.Sprintf("- **%s** [%s] — %s (model: %s)\n", name, status, desc, model))
	}
	return tools.NewSuccessResult(sb.String()), nil
}

func (t *ManageAgentsTool) getAgent(name string) (tools.ToolResult, error) {
	cfg := config.GetConfig()
	if cfg == nil || cfg.Agents == nil {
		return tools.NewErrorResult("agent not found: " + name), nil
	}
	ac, ok := cfg.Agents[name]
	if !ok {
		return tools.NewErrorResult("agent not found: " + name), nil
	}
	data, err := json.MarshalIndent(map[string]any{"name": name, "config": ac}, "", "  ")
	if err != nil {
		return tools.NewErrorResult("failed to serialize agent: " + err.Error()), nil
	}
	return tools.NewSuccessResult(string(data)), nil
}

func (t *ManageAgentsTool) createAgent(name string, agentCfg config.AgentConfig) (tools.ToolResult, error) {
	if err := config.AddAgent(name, agentCfg); err != nil {
		return tools.NewErrorResult(err.Error()), nil
	}
	return tools.NewSuccessResult(fmt.Sprintf(
		"Agent '%s' created successfully. Note: the agent will be available for delegation after the next session restart or reload.", name)), nil
}

func (t *ManageAgentsTool) updateAgent(name string, agentCfg config.AgentConfig) (tools.ToolResult, error) {
	if err := config.UpdateAgent(name, agentCfg); err != nil {
		return tools.NewErrorResult(err.Error()), nil
	}
	return tools.NewSuccessResult(fmt.Sprintf("Agent '%s' updated successfully.", name)), nil
}

func (t *ManageAgentsTool) deleteAgent(name string) (tools.ToolResult, error) {
	if err := config.RemoveAgent(name); err != nil {
		return tools.NewErrorResult(err.Error()), nil
	}
	return tools.NewSuccessResult(fmt.Sprintf("Agent '%s' deleted successfully.", name)), nil
}

// parseConfig extracts AgentConfig from the 'config' parameter in args.
func (t *ManageAgentsTool) parseConfig(args map[string]any) (config.AgentConfig, error) {
	cfgRaw, ok := args["config"]
	if !ok || cfgRaw == nil {
		return config.AgentConfig{}, fmt.Errorf("parameter 'config' is required for create/update action")
	}

	// Marshal then unmarshal to properly convert the map to AgentConfig
	data, err := json.Marshal(cfgRaw)
	if err != nil {
		return config.AgentConfig{}, fmt.Errorf("invalid config: %w", err)
	}

	var ac config.AgentConfig
	if err := json.Unmarshal(data, &ac); err != nil {
		return config.AgentConfig{}, fmt.Errorf("invalid config: %w", err)
	}
	return ac, nil
}
