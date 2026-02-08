package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// NewMCPCmd creates the mcp command.
func NewMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers",
		Long: `List, connect, and manage Model Context Protocol (MCP) servers.

MCP (Model Context Protocol) enables AI models to access external tools
and data sources. Use these commands to manage MCP server connections.`,
		Example: `  # List all connected MCP servers
  mote mcp list

  # Add a stdio-based MCP server
  mote mcp add filesystem npx -y @anthropic/mcp-server-filesystem /home

  # Add an HTTP-based MCP server
  mote mcp add-http myserver http://localhost:8001/mcp

  # Add an HTTP server with headers (JSON format)
  mote mcp add-http myserver http://localhost:8001/mcp --headers '{"Authorization": "Bearer token"}'

  # Import servers from JSON config
  echo '{"local": {"type": "http", "url": "http://127.0.0.1:8001/mcp"}}' | mote mcp import -

  # List all available MCP tools
  mote mcp tools`,
	}

	cmd.AddCommand(newMCPListCmd())
	cmd.AddCommand(newMCPAddCmd())
	cmd.AddCommand(newMCPAddHTTPCmd())
	cmd.AddCommand(newMCPImportCmd())
	cmd.AddCommand(newMCPRemoveCmd())
	cmd.AddCommand(newMCPToolsCmd())

	return cmd
}

func newMCPListCmd() *cobra.Command {
	var (
		jsonOutput bool
		serverURL  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MCP servers",
		Long:  `List all connected MCP servers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPList(serverURL, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMCPAddCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "add <name> <command> [args...]",
		Short: "Add a stdio-based MCP server",
		Long: `Add and connect to a new stdio-based MCP server.

The server will be started as a subprocess, communicating via stdin/stdout.
For HTTP-based servers, use 'mote mcp add-http' instead.`,
		Example: `  # Add a filesystem MCP server
  mote mcp add filesystem npx -y @anthropic/mcp-server-filesystem /home

  # Add a Python MCP server
  mote mcp add myserver python -m my_mcp_server

  # Add with custom server URL
  mote mcp add myserver ./server --url http://localhost:8080`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAdd(serverURL, args[0], args[1], args[2:])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMCPAddHTTPCmd() *cobra.Command {
	var (
		serverURL string
		headers   string
	)

	cmd := &cobra.Command{
		Use:   "add-http <name> <mcp-url>",
		Short: "Add an HTTP-based MCP server",
		Long: `Add and connect to an HTTP-based MCP server.

For stdio-based servers (subprocess), use 'mote mcp add' instead.`,
		Example: `  # Add a simple HTTP MCP server
  mote mcp add-http myserver http://localhost:8001/mcp

  # Add with authentication header
  mote mcp add-http myserver http://localhost:8001/mcp --headers '{"Authorization": "Bearer token"}'

  # Add with multiple headers
  mote mcp add-http myserver http://localhost:8001/mcp --headers '{"Authorization": "Bearer token", "X-Custom": "value"}'`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAddHTTP(serverURL, args[0], args[1], headers)
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")
	cmd.Flags().StringVar(&headers, "headers", "", "HTTP headers as JSON object")

	return cmd
}

func newMCPImportCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "import <file|-|json>",
		Short: "Import MCP servers from JSON config",
		Long: `Import one or more MCP servers from a JSON configuration.

The JSON format supports both single server and multiple servers:

Single server format (name as wrapper):
  {"server_name": {"type": "http", "url": "...", "headers": {...}}}

Multiple servers:
  {
    "server1": {"type": "http", "url": "http://localhost:8001/mcp"},
    "server2": {"type": "stdio", "command": "npx", "args": ["-y", "@anthropic/mcp-server-filesystem"]}
  }

Use '-' to read from stdin.`,
		Example: `  # Import from file
  mote mcp import servers.json

  # Import from stdin
  echo '{"local": {"type": "http", "url": "http://127.0.0.1:8001/mcp"}}' | mote mcp import -

  # Import with headers
  mote mcp import '{"myserver": {"type": "http", "url": "http://localhost:8001/mcp", "headers": {"Authorization": "Bearer token"}}}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPImport(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMCPRemoveCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"delete", "rm"},
		Short:   "Remove an MCP server",
		Long:    `Disconnect and remove an MCP server.`,
		Example: `  # Remove a server
  mote mcp remove myserver

  # Using alias
  mote mcp rm myserver`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPRemove(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMCPToolsCmd() *cobra.Command {
	var (
		serverFilter string
		jsonOutput   bool
		serverURL    string
	)

	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List MCP tools",
		Long:  `List all tools provided by connected MCP servers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPTools(serverURL, serverFilter, jsonOutput)
		},
	}

	cmd.Flags().StringVarP(&serverFilter, "server", "s", "", "filter by server name")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

type mcpServerInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Transport string `json:"transport"`
	Error     string `json:"error,omitempty"`
}

type mcpServersResponse struct {
	Servers []mcpServerInfo `json:"servers"`
}

type mcpToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Server      string         `json:"server"`
	Schema      map[string]any `json:"schema,omitempty"`
}

type mcpToolsResponse struct {
	Tools []mcpToolInfo `json:"tools"`
}

func runMCPList(serverURL string, jsonOutput bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/mcp/servers", serverURL)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var response mcpServersResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response.Servers)
	}

	if len(response.Servers) == 0 {
		fmt.Println("No MCP servers connected.")
		fmt.Println("\nTo add an MCP server, use:")
		fmt.Println("  mote mcp add <name> <command> [args...]")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tTRANSPORT\tERROR")
	fmt.Fprintln(w, "----\t------\t---------\t-----")

	for _, s := range response.Servers {
		errStr := "-"
		if s.Error != "" {
			if len(s.Error) > 30 {
				errStr = s.Error[:30] + "..."
			} else {
				errStr = s.Error
			}
		}

		statusIcon := "✓"
		if s.Status != "connected" && s.Status != "Connected" {
			statusIcon = "✗"
		}

		fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\n",
			s.Name,
			statusIcon,
			s.Status,
			s.Transport,
			errStr,
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d servers\n", len(response.Servers))

	return nil
}

func runMCPAdd(serverURL, name, command string, args []string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	reqBody := map[string]any{
		"name":    name,
		"type":    "stdio",
		"command": command,
	}
	if len(args) > 0 {
		reqBody["args"] = args
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/mcp/servers", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Added MCP server '%s' (stdio)\n", name)
	fmt.Printf("  Command: %s %s\n", command, strings.Join(args, " "))
	return nil
}

func runMCPAddHTTP(serverURL, name, mcpURL, headersJSON string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	reqBody := map[string]any{
		"name": name,
		"type": "http",
		"url":  mcpURL,
	}

	if headersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
			return fmt.Errorf("invalid headers JSON: %w", err)
		}
		reqBody["headers"] = headers
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/mcp/servers", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Added MCP server '%s' (http)\n", name)
	fmt.Printf("  URL: %s\n", mcpURL)
	return nil
}

func runMCPImport(serverURL, input string) error {
	client := &http.Client{Timeout: 60 * time.Second}

	var jsonData []byte
	var err error

	if input == "-" {
		// Read from stdin
		jsonData, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
	} else if strings.HasPrefix(input, "{") {
		// Direct JSON input
		jsonData = []byte(input)
	} else {
		// Read from file
		jsonData, err = os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
	}

	// Validate JSON
	var config map[string]any
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/mcp/servers/import", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Imported []struct {
			Name      string `json:"name"`
			Status    string `json:"status"`
			Transport string `json:"transport"`
		} `json:"imported"`
		Count  int      `json:"count"`
		Errors []string `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Count > 0 {
		fmt.Printf("✓ Imported %d MCP server(s):\n", result.Count)
		for _, s := range result.Imported {
			fmt.Printf("  - %s (%s)\n", s.Name, s.Transport)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\n⚠️  Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}

func runMCPRemove(serverURL, name string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/mcp/servers/%s", serverURL, name)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Removed MCP server '%s'\n", name)
	return nil
}

func runMCPTools(serverURL, serverFilter string, jsonOutput bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/mcp/tools", serverURL)
	if serverFilter != "" {
		url = fmt.Sprintf("%s?server=%s", url, serverFilter)
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var response mcpToolsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response.Tools)
	}

	if len(response.Tools) == 0 {
		fmt.Println("No MCP tools available.")
		if serverFilter != "" {
			fmt.Printf("(Filtered by server: %s)\n", serverFilter)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSERVER\tDESCRIPTION")
	fmt.Fprintln(w, "----\t------\t-----------")

	for _, t := range response.Tools {
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n",
			t.Name,
			t.Server,
			desc,
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d tools\n", len(response.Tools))

	return nil
}
