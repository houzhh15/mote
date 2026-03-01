package prompt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"mote/internal/mcp/client"
	"mote/internal/tools"
)

// MCPInjectionMode controls how MCP tools are injected into the system prompt.
type MCPInjectionMode int

const (
	// MCPInjectionFull injects all MCP tool details (description + parameters).
	MCPInjectionFull MCPInjectionMode = iota
	// MCPInjectionSummary only injects server names and tool counts.
	MCPInjectionSummary
	// MCPInjectionNone disables MCP tool injection entirely.
	MCPInjectionNone
)

// String returns a string representation of MCPInjectionMode.
func (m MCPInjectionMode) String() string {
	switch m {
	case MCPInjectionFull:
		return "full"
	case MCPInjectionSummary:
		return "summary"
	case MCPInjectionNone:
		return "none"
	default:
		return "unknown"
	}
}

// SystemPromptBuilder builds system prompts with tool and memory context.
type SystemPromptBuilder struct {
	config           PromptConfig
	registry         *tools.Registry
	memory           MemorySearcher
	injector         *PromptInjector
	mcpManager       *client.Manager
	mcpInjectionMode MCPInjectionMode
	agents           []AgentInfo
	maxOutputTokens  int // Maximum output tokens for the current model (0 = unknown)
}

// NewSystemPromptBuilder creates a new SystemPromptBuilder.
func NewSystemPromptBuilder(config PromptConfig, registry *tools.Registry) *SystemPromptBuilder {
	if config.AgentName == "" {
		config.AgentName = "Mote"
	}
	if config.Timezone == "" {
		config.Timezone = "UTC"
	}
	return &SystemPromptBuilder{
		config:           config,
		registry:         registry,
		mcpInjectionMode: MCPInjectionFull,
	}
}

// WithMemory sets the memory searcher for context injection.
func (b *SystemPromptBuilder) WithMemory(m MemorySearcher) *SystemPromptBuilder {
	b.memory = m
	return b
}

// GetConfig returns the prompt config (useful for sub-agent builders to inherit settings).
func (b *SystemPromptBuilder) GetConfig() PromptConfig {
	return b.config
}

// SetWorkspaceDir dynamically sets the workspace directory for the current session.
// This is called per-request to inject the session-bound workspace path into the system prompt.
func (b *SystemPromptBuilder) SetWorkspaceDir(dir string) {
	b.config.WorkspaceDir = dir
}

// WithInjector sets the prompt injector for slot-based injections.
func (b *SystemPromptBuilder) WithInjector(injector *PromptInjector) *SystemPromptBuilder {
	b.injector = injector
	return b
}

// WithMCPManager sets the MCP client manager for dynamic tool injection.
func (b *SystemPromptBuilder) WithMCPManager(m *client.Manager) *SystemPromptBuilder {
	b.mcpManager = m
	return b
}

// WithAgents sets the available sub-agents for prompt rendering.
func (b *SystemPromptBuilder) WithAgents(agents []AgentInfo) *SystemPromptBuilder {
	b.agents = agents
	return b
}

// SetMCPInjectionMode sets the MCP injection mode.
func (b *SystemPromptBuilder) SetMCPInjectionMode(mode MCPInjectionMode) {
	b.mcpInjectionMode = mode
}

// SetMaxOutputTokens sets the maximum output token limit for the current model.
// When set (>0), this is injected into the system prompt so the LLM can
// self-regulate output size and avoid truncation.
func (b *SystemPromptBuilder) SetMaxOutputTokens(tokens int) {
	b.maxOutputTokens = tokens
}

// GetMCPInjectionMode returns the current MCP injection mode.
func (b *SystemPromptBuilder) GetMCPInjectionMode() MCPInjectionMode {
	return b.mcpInjectionMode
}

// GetInjector returns the current prompt injector, creating one if not set.
func (b *SystemPromptBuilder) GetInjector() *PromptInjector {
	if b.injector == nil {
		b.injector = NewPromptInjector()
	}
	return b.injector
}

// Build constructs the complete system prompt with memory context.
func (b *SystemPromptBuilder) Build(ctx context.Context, userInput string) (string, error) {
	data := b.prepareData()

	// Search memory if available
	if b.memory != nil && userInput != "" {
		results, err := b.memory.Search(ctx, userInput, 5)
		if err != nil {
			// Log but don't fail - memory search is optional
			// In production, use proper logging
		} else {
			data.Memories = results
		}
	}

	return b.renderTemplates(data)
}

// BuildStatic constructs the system prompt without memory context.
func (b *SystemPromptBuilder) BuildStatic() string {
	data := b.prepareData()
	result, _ := b.renderTemplates(data)
	return result
}

// prepareData creates the PromptData structure.
func (b *SystemPromptBuilder) prepareData() PromptData {
	data := PromptData{
		AgentName:    b.config.AgentName,
		Timezone:     b.config.Timezone,
		CurrentTime:  time.Now().Format("2006-01-02 15:04:05"),
		WorkspaceDir: b.config.WorkspaceDir,
		Constraints:  b.config.Constraints,
		ExtraPrompt:  b.config.ExtraPrompt,
		Agents:       b.agents,
	}

	// Get tools from registry
	if b.registry != nil {
		toolList := b.registry.List()
		for _, t := range toolList {
			data.Tools = append(data.Tools, ToolInfo{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			})
		}
	}

	data.MaxOutputTokens = b.maxOutputTokens

	return data
}

// renderTemplates renders all templates and combines them.
func (b *SystemPromptBuilder) renderTemplates(data PromptData) (string, error) {
	// Template to slot mapping
	templateSlots := []struct {
		template string
		slot     PromptSlot
	}{
		{baseIdentityTemplate, SlotIdentity},
		{capabilitiesTemplate, SlotCapabilities},
		{agentSectionTemplate, SlotCapabilities},
		{memoryContextTemplate, SlotContext},
		{currentContextTemplate, SlotContext},
		{constraintsTemplate, SlotConstraints},
	}

	var result bytes.Buffer
	for _, ts := range templateSlots {
		tmpl, err := template.New("").Parse(ts.template)
		if err != nil {
			return "", fmt.Errorf("%w: parse: %v", ErrTemplateRender, err)
		}
		if err := tmpl.Execute(&result, data); err != nil {
			return "", fmt.Errorf("%w: execute: %v", ErrTemplateRender, err)
		}

		// Inject slot content after template
		if b.injector != nil {
			content := b.injector.GetContentBySlot(ts.slot)
			if content != "" {
				result.WriteString("\n")
				result.WriteString(content)
				result.WriteString("\n")
			}
		}
	}

	// Append MCP tools section
	if b.mcpManager != nil && b.mcpInjectionMode != MCPInjectionNone {
		mcpSection := b.buildMCPSection()
		if mcpSection != "" {
			result.WriteString("\n")
			result.WriteString(mcpSection)
		}
	}

	// Append tail slot at the end
	if b.injector != nil {
		tailContent := b.injector.GetContentBySlot(SlotTail)
		if tailContent != "" {
			result.WriteString("\n")
			result.WriteString(tailContent)
		}
	}

	// M08B: Append safety rules at the very end of the system prompt
	if !b.config.DisableSafetyPrompt {
		result.WriteString("\n")
		result.WriteString(SafetyRulesPrompt)
	}

	return result.String(), nil
}

// buildMCPSection builds the MCP tools section based on injection mode.
func (b *SystemPromptBuilder) buildMCPSection() string {
	if b.mcpManager == nil {
		return ""
	}

	servers := b.mcpManager.ListServers()
	// Only include connected servers with tools
	connectedServers := make([]client.ServerStatus, 0)
	for _, s := range servers {
		if s.State.String() == "connected" && s.ToolCount > 0 {
			connectedServers = append(connectedServers, s)
		}
	}

	if len(connectedServers) == 0 {
		return ""
	}

	var builder strings.Builder
	switch b.mcpInjectionMode {
	case MCPInjectionFull:
		b.buildMCPFullSection(&builder, connectedServers)
	case MCPInjectionSummary:
		b.buildMCPSummarySection(&builder, connectedServers)
	}
	return builder.String()
}

// buildMCPFullSection builds full MCP tool information.
func (b *SystemPromptBuilder) buildMCPFullSection(builder *strings.Builder, servers []client.ServerStatus) {
	builder.WriteString("\n## External MCP Tools (Use Only When Needed)\n\n")
	builder.WriteString("The following MCP servers provide additional tools. **Use built-in tools first if they can accomplish the task.**\n")
	builder.WriteString("To call MCP tools, use `mcp_call` with server name, tool name, and required arguments:\n\n")

	for _, s := range servers {
		builder.WriteString(fmt.Sprintf("### Server: `%s`\n", s.Name))
		if mcpClient, ok := b.mcpManager.GetClient(s.Name); ok {
			mcpTools := mcpClient.Tools()
			if len(mcpTools) > 0 {
				builder.WriteString("\n**Tools:**\n")
				for _, t := range mcpTools {
					builder.WriteString(fmt.Sprintf("- `%s`", t.Name))
					if t.Description != "" {
						builder.WriteString(fmt.Sprintf(": %s", t.Description))
					}
					builder.WriteString("\n")
					// Add parameter info from InputSchema
					if len(t.InputSchema) > 0 {
						var schema map[string]any
						if err := json.Unmarshal(t.InputSchema, &schema); err == nil {
							if props, ok := schema["properties"].(map[string]any); ok && len(props) > 0 {
								builder.WriteString("  Parameters:\n")
								for pname, pval := range props {
									if pm, ok := pval.(map[string]any); ok {
										builder.WriteString(fmt.Sprintf("    - `%s` (%v): %v\n", pname, pm["type"], pm["description"]))
									}
								}
							}
							if required, ok := schema["required"].([]any); ok && len(required) > 0 {
								reqStrs := make([]string, len(required))
								for i, r := range required {
									reqStrs[i] = fmt.Sprintf("%v", r)
								}
								builder.WriteString(fmt.Sprintf("  Required: %s\n", strings.Join(reqStrs, ", ")))
							}
						}
					}
				}
			}
		}
		builder.WriteString("\n")
	}

	builder.WriteString("**IMPORTANT:** When calling MCP tools via `mcp_call`, always provide:\n")
	builder.WriteString("- `server`: exact server name from above\n")
	builder.WriteString("- `tool`: exact tool name from above\n")
	builder.WriteString("- `arguments`: object with all required parameters\n")
}

// buildMCPSummarySection builds a compact summary of MCP servers.
func (b *SystemPromptBuilder) buildMCPSummarySection(builder *strings.Builder, servers []client.ServerStatus) {
	builder.WriteString("\n## External MCP Tools\n\n")
	builder.WriteString("Available MCP servers (use `mcp_list` for details, prefer built-in tools when possible):\n")

	for _, s := range servers {
		if mcpClient, ok := b.mcpManager.GetClient(s.Name); ok {
			mcpTools := mcpClient.Tools()
			toolNames := make([]string, 0, len(mcpTools))
			for _, t := range mcpTools {
				toolNames = append(toolNames, t.Name)
			}
			builder.WriteString(fmt.Sprintf("- `%s`: %d tools (%s)\n", s.Name, len(mcpTools), strings.Join(toolNames, ", ")))
		}
	}
	builder.WriteString("\nUse `mcp_call` with server, tool, and arguments. Use `mcp_list` if you need parameter details.\n")
}
