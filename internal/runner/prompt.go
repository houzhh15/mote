package runner

import (
	"encoding/json"
	"fmt"
	"strings"

	"mote/internal/mcp/client"
	"mote/internal/tools"
)

// MCPInjectionMode controls how MCP tools are injected into the system prompt.
type MCPInjectionMode int

const (
	// MCPInjectionFull injects all MCP tool details (description + parameters).
	// Use for first message or when user explicitly asks about MCP tools.
	MCPInjectionFull MCPInjectionMode = iota

	// MCPInjectionSummary only injects server names and tool counts.
	// Use for subsequent messages to reduce token usage.
	MCPInjectionSummary

	// MCPInjectionNone disables MCP tool injection entirely.
	MCPInjectionNone
)

// PromptBuilder builds system prompts with tool information.
type PromptBuilder struct {
	basePrompt       string
	registry         *tools.Registry
	mcpManager       *client.Manager
	mcpInjectionMode MCPInjectionMode
}

// NewPromptBuilder creates a new PromptBuilder.
func NewPromptBuilder(registry *tools.Registry) *PromptBuilder {
	return &PromptBuilder{
		registry:         registry,
		basePrompt:       defaultBasePrompt,
		mcpInjectionMode: MCPInjectionFull, // Default to full for first message
	}
}

// SetMCPManager sets the MCP client manager for dynamic tool injection.
func (pb *PromptBuilder) SetMCPManager(m *client.Manager) {
	pb.mcpManager = m
}

// SetMCPInjectionMode sets the MCP injection mode.
func (pb *PromptBuilder) SetMCPInjectionMode(mode MCPInjectionMode) {
	pb.mcpInjectionMode = mode
}

// GetMCPInjectionMode returns the current MCP injection mode.
func (pb *PromptBuilder) GetMCPInjectionMode() MCPInjectionMode {
	return pb.mcpInjectionMode
}

const defaultBasePrompt = `You are a helpful AI assistant with access to various tools. 
You can use these tools to help accomplish tasks requested by the user.

When using tools:
1. Analyze the user's request and determine if tools are needed.
2. **IMPORTANT: Prefer using built-in tools over MCP tools when both can accomplish the task.**
3. **For memory/context retrieval, ALWAYS try memory_search or mote_memory_search first before other tools.**
4. Call the appropriate tool with the correct arguments.
5. Wait for the tool result before proceeding.
6. **IMPORTANT: After receiving tool results, you MUST summarize the results in natural language for the user. Never return raw JSON or tool output directly - always explain what you found in a human-readable way.**
7. If a tool call fails, explain the error and try an alternative approach if possible.

Tool Selection Priority:
- For searching memories/context: Use memory_search or mote_memory_search (built-in)
- For file operations: Use built-in file tools
- For external services/APIs: Use MCP tools only when built-in tools cannot accomplish the task

Always be helpful, accurate, and concise in your responses. When tools return data, extract the key information and present it clearly to the user.`

// SetBasePrompt sets a custom base prompt.
func (pb *PromptBuilder) SetBasePrompt(prompt string) {
	pb.basePrompt = prompt
}

// Build generates the complete system prompt including tool descriptions.
func (pb *PromptBuilder) Build() string {
	var builder strings.Builder

	// Add base prompt
	builder.WriteString(pb.basePrompt)
	builder.WriteString("\n\n")

	// Add tool information if registry is available
	if pb.registry != nil {
		toolsList := pb.registry.List()
		if len(toolsList) > 0 {
			builder.WriteString("## Available Tools\n\n")
			builder.WriteString("You have access to the following tools:\n\n")

			for _, tool := range toolsList {
				builder.WriteString(fmt.Sprintf("### %s\n", tool.Name()))
				builder.WriteString(fmt.Sprintf("%s\n", tool.Description()))

				// Add parameter schema if available
				params := tool.Parameters()
				if params != nil {
					if props, ok := params["properties"].(map[string]any); ok && len(props) > 0 {
						builder.WriteString("\n**Parameters:**\n")
						for name, prop := range props {
							if propMap, ok := prop.(map[string]any); ok {
								propType := propMap["type"]
								desc := propMap["description"]
								builder.WriteString(fmt.Sprintf("- `%s` (%v): %v\n", name, propType, desc))
							}
						}
					}
					if required, ok := params["required"].([]string); ok && len(required) > 0 {
						builder.WriteString(fmt.Sprintf("\n**Required:** %s\n", strings.Join(required, ", ")))
					}
				}
				builder.WriteString("\n")
			}

			builder.WriteString("## Tool Usage Guidelines\n\n")
			builder.WriteString("- Call tools by name with the required arguments.\n")
			builder.WriteString("- Wait for tool results before continuing.\n")
			builder.WriteString("- Handle errors gracefully and inform the user if something goes wrong.\n")
			builder.WriteString("- Use tools judiciously - not every request requires a tool.\n")
		}
	}

	// Add MCP server information based on injection mode
	if pb.mcpManager != nil && pb.mcpInjectionMode != MCPInjectionNone {
		servers := pb.mcpManager.ListServers()
		// Only include connected servers with tools
		connectedServers := make([]client.ServerStatus, 0)
		for _, s := range servers {
			if s.State.String() == "connected" && s.ToolCount > 0 {
				connectedServers = append(connectedServers, s)
			}
		}

		if len(connectedServers) > 0 {
			switch pb.mcpInjectionMode {
			case MCPInjectionFull:
				pb.buildMCPFullSection(&builder, connectedServers)
			case MCPInjectionSummary:
				pb.buildMCPSummarySection(&builder, connectedServers)
			}
		}
	}

	return builder.String()
}

// buildMCPFullSection builds the full MCP tool information with descriptions and parameters.
func (pb *PromptBuilder) buildMCPFullSection(builder *strings.Builder, servers []client.ServerStatus) {
	builder.WriteString("\n## External MCP Tools (Use Only When Needed)\n\n")
	builder.WriteString("The following MCP servers provide additional tools. **Use built-in tools first if they can accomplish the task.**\n")
	builder.WriteString("To call MCP tools, use `mcp_call` with server name, tool name, and required arguments:\n\n")

	for _, s := range servers {
		builder.WriteString(fmt.Sprintf("### Server: `%s`\n", s.Name))
		if mcpClient, ok := pb.mcpManager.GetClient(s.Name); ok {
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

// buildMCPSummarySection builds a compact summary of MCP servers (for subsequent messages).
func (pb *PromptBuilder) buildMCPSummarySection(builder *strings.Builder, servers []client.ServerStatus) {
	builder.WriteString("\n## External MCP Tools\n\n")
	builder.WriteString("Available MCP servers (use `mcp_list` for details, prefer built-in tools when possible):\n")

	for _, s := range servers {
		if mcpClient, ok := pb.mcpManager.GetClient(s.Name); ok {
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

// BuildWithContext generates a system prompt with additional context.
func (pb *PromptBuilder) BuildWithContext(context string) string {
	base := pb.Build()
	if context == "" {
		return base
	}
	return base + "\n\n## Additional Context\n\n" + context
}

// BuildMCPSection builds only the MCP section for injection into other prompts.
// This is useful when using SystemPromptBuilder (which doesn't have MCP support)
// but still need MCP tools to be available.
func (pb *PromptBuilder) BuildMCPSection() string {
	if pb.mcpManager == nil || pb.mcpInjectionMode == MCPInjectionNone {
		return ""
	}

	servers := pb.mcpManager.ListServers()
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
	switch pb.mcpInjectionMode {
	case MCPInjectionFull:
		pb.buildMCPFullSection(&builder, connectedServers)
	case MCPInjectionSummary:
		pb.buildMCPSummarySection(&builder, connectedServers)
	}

	return builder.String()
}
