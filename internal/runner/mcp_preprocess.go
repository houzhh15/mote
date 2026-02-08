package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"mote/internal/mcp/client"
	"mote/internal/mcp/transport"
)

// MCPConfig represents an MCP server configuration that can be parsed from user input.
type MCPConfig struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
}

// MCPPreprocessResult contains the result of preprocessing user input for MCP commands.
type MCPPreprocessResult struct {
	Handled  bool
	Response string
	Error    error
}

// jsonPattern matches JSON-like content in user input
var jsonPattern = regexp.MustCompile(`\{[\s\S]*"name"\s*:\s*"[^"]+"\s*[\s\S]*"type"\s*:\s*"(http|stdio)"[\s\S]*\}`)

// PreprocessMCPInput checks if user input contains MCP configuration and handles it directly.
// This bypasses LLM parameter extraction issues for structured JSON configs.
func (r *Runner) PreprocessMCPInput(ctx context.Context, input string) *MCPPreprocessResult {
	// Quick check for MCP-related keywords
	lower := strings.ToLower(input)
	if !strings.Contains(lower, "mcp") && !strings.Contains(lower, "server") {
		return nil
	}

	// Try to find JSON configuration in input
	config := extractMCPConfig(input)
	if config == nil {
		return nil
	}

	// Validate required fields
	if config.Name == "" || config.Type == "" {
		return nil
	}

	// Execute MCP add operation directly
	result := r.executeMCPAdd(ctx, config)
	return result
}

// extractMCPConfig extracts MCP configuration from user input.
func extractMCPConfig(input string) *MCPConfig {
	// First try to find JSON object in input
	matches := jsonPattern.FindAllString(input, -1)
	for _, match := range matches {
		var config MCPConfig
		if err := json.Unmarshal([]byte(match), &config); err == nil {
			if config.Name != "" && config.Type != "" {
				return &config
			}
		}
	}

	// Try to find JSON in code blocks
	codeBlockPattern := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	codeMatches := codeBlockPattern.FindAllStringSubmatch(input, -1)
	for _, m := range codeMatches {
		if len(m) > 1 {
			var config MCPConfig
			if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &config); err == nil {
				if config.Name != "" && config.Type != "" {
					return &config
				}
			}
		}
	}

	// Try parsing the entire input as JSON (after trimming)
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var config MCPConfig
		if err := json.Unmarshal([]byte(trimmed), &config); err == nil {
			if config.Name != "" && config.Type != "" {
				return &config
			}
		}
	}

	return nil
}

// executeMCPAdd executes the MCP add operation directly.
func (r *Runner) executeMCPAdd(ctx context.Context, config *MCPConfig) *MCPPreprocessResult {
	if r.mcpManager == nil {
		return &MCPPreprocessResult{
			Handled:  true,
			Response: "❌ MCP manager is not initialized",
		}
	}

	// Build client config
	clientConfig := client.ClientConfig{
		Command: config.Name, // Use name as identifier
	}

	switch config.Type {
	case "http":
		if config.URL == "" {
			return &MCPPreprocessResult{
				Handled:  true,
				Response: "❌ URL is required for HTTP type MCP server",
			}
		}
		clientConfig.TransportType = transport.TransportHTTP
		clientConfig.URL = config.URL
		clientConfig.Headers = config.Headers

	case "stdio":
		if config.Command == "" {
			return &MCPPreprocessResult{
				Handled:  true,
				Response: "❌ Command is required for stdio type MCP server",
			}
		}
		clientConfig.TransportType = transport.TransportStdio
		clientConfig.Command = config.Command
		clientConfig.Args = config.Args

	default:
		return &MCPPreprocessResult{
			Handled:  true,
			Response: fmt.Sprintf("❌ Unsupported server type: %s (use 'http' or 'stdio')", config.Type),
		}
	}

	// Connect to the server
	if err := r.mcpManager.Connect(ctx, clientConfig); err != nil {
		return &MCPPreprocessResult{
			Handled:  true,
			Response: fmt.Sprintf("❌ Failed to connect to MCP server '%s': %v", config.Name, err),
			Error:    err,
		}
	}

	// Build success message
	var details []string
	details = append(details, fmt.Sprintf("- Name: %s", config.Name))
	details = append(details, fmt.Sprintf("- Type: %s", config.Type))

	if config.Type == "http" {
		details = append(details, fmt.Sprintf("- URL: %s", config.URL))
		if len(config.Headers) > 0 {
			details = append(details, fmt.Sprintf("- Headers: %d configured", len(config.Headers)))
		}
	} else {
		details = append(details, fmt.Sprintf("- Command: %s", config.Command))
		if len(config.Args) > 0 {
			details = append(details, fmt.Sprintf("- Args: %v", config.Args))
		}
	}

	// Get tool count
	servers := r.mcpManager.ListServers()
	for _, s := range servers {
		if s.Name == config.Name {
			details = append(details, fmt.Sprintf("- Tools available: %d", s.ToolCount))
			break
		}
	}

	response := fmt.Sprintf("✅ Successfully connected to MCP server!\n\n%s", strings.Join(details, "\n"))
	return &MCPPreprocessResult{
		Handled:  true,
		Response: response,
	}
}
